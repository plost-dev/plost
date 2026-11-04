package plost

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Vite's file-route chunks start with underscores; Go's default embed glob omits them.
//
//go:embed all:web
var assets embed.FS

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string        { return e.message }
func failure(status int, s string) error { return &apiError{status, s} }
func jsonResponse(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(v)
}
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	limit := int64(64 << 10)
	if r.URL.Path == "/api/account" {
		limit = 256 << 10
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return failure(400, "请求格式无效或过大")
	}
	if d.Decode(new(any)) != io.EOF {
		return failure(400, "请求格式无效")
	}
	return nil
}

type endpoint func(http.ResponseWriter, *http.Request) error

func (a *App) api(fn endpoint, auth bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		a.mu.RLock()
		defer a.mu.RUnlock()
		if auth {
			if _, err := a.user(r); err != nil {
				writeError(w, failure(401, "请先登录"))
				return
			}
		}
		if err := fn(w, r); err != nil {
			writeError(w, err)
		}
	}
}

// apiOpen is for slow operations (network calls, large transfers). It authenticates under the
// read lock but does not hold it while fn runs, so a waiting restore cannot stall every request.
// fn must lock a.mu itself around any database use.
func (a *App) apiOpen(fn endpoint, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)
		a.mu.RLock()
		_, err := a.user(r)
		a.mu.RUnlock()
		if err != nil {
			writeError(w, failure(401, "请先登录"))
			return
		}
		// The server's default write timeout is too short for a large backup transfer.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 10*time.Second))
		if err := fn(w, r); err != nil {
			writeError(w, err)
		}
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, msg := 500, "暂时无法完成操作，请重试"
	var e *apiError
	if errors.As(err, &e) {
		status, msg = e.status, e.message
	} else if errors.Is(err, context.DeadlineExceeded) {
		status, msg = 503, "查询超时，请缩短统计时间范围"
	} else if !errors.Is(err, context.Canceled) {
		log.Print("Plost request failed")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		if err := a.db.PingContext(r.Context()); err != nil {
			writeError(w, err)
			return
		}
		_ = jsonResponse(w, map[string]any{"ok": true, "version": a.cfg.Version})
	})
	mux.HandleFunc("GET /api/auth", a.api(a.authStatus, false))
	mux.HandleFunc("POST /api/setup", a.api(a.setup, false))
	mux.HandleFunc("POST /api/login", a.api(a.login, false))
	mux.HandleFunc("POST /api/login/totp", a.api(a.verifyTOTP, false))
	mux.HandleFunc("POST /api/login/passkey/begin", a.api(a.passkeyLoginBegin, false))
	mux.HandleFunc("POST /api/login/passkey/finish", a.api(a.passkeyLoginFinish, false))
	mux.HandleFunc("GET /api/passkeys", a.api(a.passkeyList, true))
	mux.HandleFunc("POST /api/passkeys/register/begin", a.api(a.passkeyRegisterBegin, true))
	mux.HandleFunc("POST /api/passkeys/register/finish", a.api(a.passkeyRegisterFinish, true))
	mux.HandleFunc("DELETE /api/passkeys/{id}", a.api(a.passkeyDelete, true))
	mux.HandleFunc("POST /api/logout", a.api(a.logout, false))
	mux.HandleFunc("GET /api/account", a.api(a.profile, true))
	mux.HandleFunc("PUT /api/account", a.api(a.profile, true))
	mux.HandleFunc("POST /api/account/totp", a.api(a.manageTOTP, true))
	mux.HandleFunc("GET /api/preferences", a.api(a.preferences, false))
	mux.HandleFunc("PUT /api/mapbox", a.api(a.mapboxSettings, true))
	mux.HandleFunc("GET /api/mapbox", a.api(a.mapboxSettings, true))
	mux.HandleFunc("GET /api/geoip", a.api(a.geoipGet, true))
	mux.HandleFunc("PUT /api/geoip", a.api(a.geoipPut, true))
	mux.HandleFunc("POST /api/geoip/update", a.apiOpen(a.geoipUpdate, geoDownloadTime))
	mux.HandleFunc("PUT /api/google-maps", a.api(a.googleMapsSettings, true))
	mux.HandleFunc("GET /api/google-maps", a.api(a.googleMapsSettings, true))
	mux.HandleFunc("PUT /api/map-style", a.api(a.mapStyle, true))
	mux.HandleFunc("PUT /api/google-map-style", a.api(a.googleMapStyle, true))
	mux.HandleFunc("GET /api/map-usage", a.api(a.mapUsage, true))
	mux.HandleFunc("PUT /api/map-usage", a.api(a.mapUsage, true))
	mux.HandleFunc("GET /api/overview/map", a.api(a.overviewMap, true))
	mux.HandleFunc("PUT /api/sparkline", a.api(a.sparklineSettingsPut, true))
	mux.HandleFunc("GET /api/sites", a.api(a.listSites, true))
	mux.HandleFunc("GET /api/overview/live", a.api(a.overviewLive, true))
	mux.HandleFunc("GET /api/changes", a.changeStream)
	mux.HandleFunc("GET /api/overview/charts", a.api(a.overviewCharts, true))
	mux.HandleFunc("POST /api/sites", a.api(a.createSite, true))
	mux.HandleFunc("GET /api/sites/{domain}/settings", a.api(a.siteSettings, true))
	mux.HandleFunc("PUT /api/sites/{domain}/settings", a.api(a.updateSite, true))
	mux.HandleFunc("DELETE /api/sites/{domain}", a.api(a.deleteSite, true))
	mux.HandleFunc("PUT /api/sites/{domain}/icon", a.api(a.putSiteIcon, true))
	mux.HandleFunc("DELETE /api/sites/{domain}/icon", a.api(a.deleteSiteIcon, true))
	mux.HandleFunc("GET /api/sites/{domain}/report", a.api(a.siteReport, false))
	mux.HandleFunc("GET /api/sites/{domain}/checks", a.api(a.siteCheckGet, true))
	mux.HandleFunc("PUT /api/sites/{domain}/checks", a.api(a.siteCheckPut, true))
	mux.HandleFunc("POST /api/sites/{domain}/checks/run", a.apiOpen(a.siteCheckRun, 30*time.Second))
	mux.HandleFunc("GET /api/sites/{domain}/activity", a.api(a.activity, true))
	mux.HandleFunc("GET /api/sites/{domain}/visitors", a.api(a.visitors, true))
	mux.HandleFunc("POST /api/sites/{domain}/shares", a.api(a.createShare, true))
	mux.HandleFunc("DELETE /api/sites/{domain}/shares/{id}", a.api(a.deleteShare, true))
	mux.HandleFunc("GET /api/share/{token}", a.api(a.shareReport, false))
	mux.HandleFunc("GET /api/api-keys", a.api(a.apiKeys, true))
	mux.HandleFunc("POST /api/api-keys", a.api(a.createAPIKey, true))
	mux.HandleFunc("DELETE /api/api-keys/{id}", a.api(a.deleteAPIKey, true))
	mux.HandleFunc("GET /api/v1/stats", a.api(a.statsAPI, false))
	mux.HandleFunc("GET /api/v1/openapi.json", a.openapi)
	mux.HandleFunc("GET /api/export/{domain}", a.api(a.exportCSV, true))
	mux.HandleFunc("POST /api/event", a.collect)
	mux.HandleFunc("OPTIONS /api/event", a.collect)
	// The tracker's normal address, shaped like Umami's (see umami_send.go).
	mux.HandleFunc("POST /api/send", a.collect)
	mux.HandleFunc("OPTIONS /api/send", a.collect)
	// The tracker's second address, used only when a request to /api/event fails: an image (GET) or a Beacon (POST).
	mux.HandleFunc("GET /e", a.collect)
	mux.HandleFunc("POST /e", a.collect)
	mux.HandleFunc("OPTIONS /e", a.collect)
	mux.HandleFunc("GET /api/backup/settings", a.api(a.backupSettingsGet, true))
	mux.HandleFunc("PUT /api/backup/settings", a.api(a.backupSettingsPut, true))
	mux.HandleFunc("POST /api/backup/cloud/test", a.apiOpen(a.cloudTest, 30*time.Second))
	mux.HandleFunc("POST /api/backup/cloud/scan", a.apiOpen(a.cloudScan, 60*time.Second))
	mux.HandleFunc("POST /api/backup/run", a.apiOpen(a.runNow, 10*time.Second))
	mux.HandleFunc("GET /api/backup/local", a.api(a.localList, true))
	mux.HandleFunc("GET /api/backup/local/{name}", a.apiOpen(a.localDownload, 30*time.Minute))
	mux.HandleFunc("POST /api/backup", a.api(a.downloadBackup, true))
	mux.HandleFunc("POST /api/backup/validate", a.validateBackup)
	mux.HandleFunc("POST /api/backup/restore", a.restoreBackup)
	for _, p := range []string{"/script.js", "/js/script.js", "/tracker.js"} {
		mux.HandleFunc("GET "+p, func(w http.ResponseWriter, r *http.Request) { a.serveAsset(w, r, "script.js") })
	}
	mux.HandleFunc("GET /assets/{file...}", func(w http.ResponseWriter, r *http.Request) { a.serveAsset(w, r, "assets/"+r.PathValue("file")) })
	mux.HandleFunc("GET /favicon/sources/{name...}", a.faviconSource)
	for _, prefix := range []string{"brands", "images", "flags", "fonts", "map-previews", "icons", "attribution"} {
		mux.HandleFunc("GET /"+prefix+"/{file...}", func(w http.ResponseWriter, r *http.Request) { a.serveAsset(w, r, strings.TrimPrefix(r.URL.Path, "/")) })
	}
	for _, name := range []string{"theme.js", "favicon.svg", "favicon-blue.svg", "favicon-green.svg", "favicon-black.svg"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) { a.serveAsset(w, r, strings.TrimPrefix(r.URL.Path, "/")) })
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, failure(404, "接口不存在")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(405)
			return
		}
		a.page(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		if a.cfg.Origin != "" {
			u, _ := url.Parse(a.cfg.Origin)
			if !strings.EqualFold(r.Host, u.Host) {
				writeError(w, failure(421, "服务域名与配置不匹配"))
				return
			}
		}
		if r.URL.Path != "/api/event" && r.URL.Path != "/api/send" && r.URL.Path != "/e" {
			w.Header().Set("Content-Security-Policy", a.contentSecurityPolicy())
			if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				origin := a.cfg.Origin
				if origin == "" {
					scheme := "http"
					if r.TLS != nil {
						scheme = "https"
					}
					origin = scheme + "://" + r.Host
				}
				if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) || r.Header.Get("X-Plost-Request") != "1" {
					writeError(w, failure(403, "请求来源无效"))
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	b, err := assets.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Built files carry a hash of their content in the name, so a browser may keep them for good and not ask again on
	// a reload; the other files may change under the same name.
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
	w.Header().Set("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(b) > 512 {
		b = gzipped(name, b)
		w.Header().Set("Content-Encoding", "gzip")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

func (a *App) page(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var count int
	if err := a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		writeError(w, err)
		return
	}
	u, _ := a.user(r)
	redirect := ""
	switch r.URL.Path {
	case "/", "/register", "/sites", "/sites/":
		if count == 0 {
			redirect = "/setup"
		} else if u == nil {
			redirect = "/login"
		} else {
			redirect = "/dashboard"
		}
	case "/setup":
		if count > 0 {
			if u != nil {
				redirect = "/dashboard"
			} else {
				redirect = "/login"
			}
		}
	case "/login":
		if count == 0 {
			redirect = "/setup"
		} else if u != nil {
			redirect = "/dashboard"
		}
	case "/dashboard", "/sites/new", "/settings", "/settings/security", "/settings/api", "/settings/appearance",
		"/settings/map", "/settings/geoip", "/settings/backup":
		if count == 0 {
			redirect = "/setup"
		} else if u == nil {
			redirect = "/login"
		}
	default:
		valid := strings.HasPrefix(r.URL.Path, "/sites/") || strings.HasPrefix(r.URL.Path, "/share/") || r.URL.Path == "/docs/api"
		if !valid {
			w.WriteHeader(404)
		}
	}
	if redirect != "" {
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	b, _ := assets.ReadFile("web/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (a *App) listSites(w http.ResponseWriter, r *http.Request) error {
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,domain,name,timezone,public,created_at FROM sites ORDER BY created_at,id")
	if err != nil {
		return err
	}
	sites := []Site{}
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.ID, &s.Domain, &s.Name, &s.Timezone, &s.Public, &s.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		sites = append(sites, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	expiries := a.certificateExpiries(r.Context())
	icons := a.siteIconVersions(r)
	_, trendSpan := a.sparklineSettings(r.Context())
	result := []map[string]any{}
	for _, s := range sites {
		rng, err := parseRange(r.URL.Query(), s.Timezone)
		if err != nil {
			return err
		}
		over, err := a.overview(r.Context(), s.ID, rng, nil)
		if err != nil {
			return err
		}
		var expiry any
		if at, ok := expiries[s.ID]; ok {
			expiry = at
		}
		item := map[string]any{"site": s, "overview": over, "certificateExpiresAt": expiry, "iconVersion": icons[s.ID]}
		if r.URL.Query().Get("ui") == "1" {
			series, err := a.series(r.Context(), s.ID, rng, nil)
			if err != nil {
				return err
			}
			prev := Range{From: rng.From.Add(-rng.To.Sub(rng.From)), To: rng.From}
			old, err := a.overview(r.Context(), s.ID, prev, nil)
			if err != nil {
				return err
			}
			if trendSpan == "30d" {
				if series, err = a.dailyTrend(r.Context(), s); err != nil {
					return err
				}
			}
			item["series"] = series
			item["previous"] = old
		}
		result = append(result, item)
	}
	return jsonResponse(w, result)
}

type siteInput struct {
	Domain   string `json:"domain"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Public   bool   `json:"public"`
}

func validateSite(in siteInput) (siteInput, error) {
	d, err := normalizeDomain(in.Domain)
	if err != nil {
		return in, failure(400, err.Error())
	}
	in.Domain = d
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = d
	}
	if len(in.Name) > 200 {
		return in, failure(400, "网站名称过长")
	}
	if in.Timezone == "" {
		in.Timezone = "Etc/UTC"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return in, failure(400, "时区无效")
	}
	return in, nil
}

func (a *App) createSite(w http.ResponseWriter, r *http.Request) error {
	var in siteInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in, err := validateSite(in)
	if err != nil {
		return err
	}
	res, err := a.db.ExecContext(r.Context(), "INSERT INTO sites(domain,name,timezone,public,created_at) VALUES(?,?,?,?,?)", in.Domain, in.Name, in.Timezone, in.Public, unixNow())
	if err != nil {
		if _, e := readSite(r.Context(), a.db, in.Domain); e == nil {
			return failure(409, "这个网站已经添加")
		}
		return err
	}
	// A new site has its checks on from the start; its settings page can turn them off.
	if id, err := res.LastInsertId(); err == nil {
		_ = a.saveSiteCheck(r.Context(), Site{ID: id, Domain: in.Domain}, true, "/")
	}
	return jsonResponse(w, map[string]string{"domain": in.Domain})
}

func (a *App) siteSettings(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,name,created_at,slug FROM shared_links WHERE site_id=? ORDER BY id DESC", s.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	shares := []map[string]any{}
	for rows.Next() {
		var id, at int64
		var name, slug string
		if err := rows.Scan(&id, &name, &at, &slug); err != nil {
			return err
		}
		shares = append(shares, map[string]any{"id": id, "name": name, "createdAt": at, "slug": slug})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return jsonResponse(w, map[string]any{"site": s, "shares": shares, "iconVersion": a.siteIconVersions(r)[s.ID]})
}

func (a *App) updateSite(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	var in siteInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Domain == "" {
		in.Domain = s.Domain
	}
	in, err = validateSite(in)
	if err != nil {
		return err
	}
	_, err = a.db.ExecContext(r.Context(), "UPDATE sites SET domain=?,name=?,timezone=?,public=? WHERE id=?", in.Domain, in.Name, in.Timezone, in.Public, s.ID)
	if err != nil {
		if other, e := readSite(r.Context(), a.db, in.Domain); e == nil && other.ID != s.ID {
			return failure(409, "这个网站已经添加")
		}
		return err
	}
	if in.Domain != s.Domain { // the checks now concern another address, so look again soon
		_, _ = a.db.ExecContext(r.Context(), "UPDATE site_check_settings SET version=version+1,next_check_at=? WHERE site_id=?", unixNow(), s.ID)
	}
	return jsonResponse(w, map[string]any{"ok": true, "domain": in.Domain})
}
func (a *App) deleteSite(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	var in struct {
		Confirmation string `json:"confirmation"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Confirmation != s.Domain {
		return failure(400, "请输入完整域名以确认删除")
	}
	_, err = a.db.ExecContext(r.Context(), "DELETE FROM sites WHERE id=?", s.ID)
	if err != nil {
		return err
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}
func (a *App) createShare(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		return failure(400, "请输入分享名称")
	}
	token := randomHex(24)
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO shared_links(site_id,name,token_hash,created_at,slug) VALUES(?,?,?,?,?)", s.ID, in.Name, digest(token), unixNow(), token)
	if err != nil {
		return err
	}
	return jsonResponse(w, map[string]string{"token": token})
}
func (a *App) deleteShare(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return failure(400, "分享无效")
	}
	res, err := a.db.ExecContext(r.Context(), "DELETE FROM shared_links WHERE id=? AND site_id=?", id, s.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return failure(404, "分享不存在")
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) sharedSite(r *http.Request, token string) (Site, error) {
	if len(token) != 48 {
		return Site{}, sql.ErrNoRows
	}
	var d string
	err := a.db.QueryRowContext(r.Context(), "SELECT s.domain FROM shared_links l JOIN sites s ON s.id=l.site_id WHERE l.token_hash=?", digest(token)).Scan(&d)
	if err != nil {
		return Site{}, err
	}
	return readSite(r.Context(), a.db, d)
}

// contentSecurityPolicy allows Google's map hosts only while Google Maps is the visitor map, so instances that do not
// use it keep the stricter policy.
func (a *App) contentSecurityPolicy() string {
	script, style, img, font, connect := "'self'", "'self' 'unsafe-inline'", "'self' data: blob: https://favicon.la https://*.favicon.la", "'self'", "'self' https://api.mapbox.com https://*.tiles.mapbox.com https://events.mapbox.com"
	if a.googleMaps.Load() {
		script += " https://maps.googleapis.com https://maps.gstatic.com"
		style += " https://fonts.googleapis.com"
		img += " https://*.googleapis.com https://*.gstatic.com https://*.google.com https://*.googleusercontent.com"
		font += " https://fonts.gstatic.com"
		connect += " https://*.googleapis.com https://*.google.com https://*.gstatic.com"
	}
	return "default-src 'self'; script-src " + script + "; style-src " + style + "; img-src " + img + "; font-src " + font + "; connect-src " + connect + "; worker-src 'self' blob:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
}

// gzippedAssets keeps each embedded file compressed once, at the best level, instead of again on every request.
var gzippedAssets sync.Map

func gzipped(name string, b []byte) []byte {
	if z, ok := gzippedAssets.Load(name); ok {
		return z.([]byte)
	}
	var buf bytes.Buffer
	z, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = z.Write(b)
	_ = z.Close()
	gzippedAssets.Store(name, buf.Bytes())
	return buf.Bytes()
}
