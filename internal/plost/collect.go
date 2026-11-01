package plost

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Event struct {
	SiteID                                                                                                                                                                     int64
	At                                                                                                                                                                         int64
	Name, Hostname, Path, Source, Referrer, UserID, SessionID, SessionVisitor, Country, Browser, OS, Device, Language, Screen, Title, UTMSource, UTMMedium, UTMCampaign, Props string
}
type ingest struct {
	event Event
	done  chan error
	epoch uint64
}

var visitorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var screenPattern = regexp.MustCompile(`^[0-9]{1,5}x[0-9]{1,5}$`)
var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)
var botPattern = regexp.MustCompile(`(?i)bot\b|spider|crawler|headless|lighthouse|facebookexternalhit|preview|monitor|uptime`)

func short(s string, max int) string {
	if len(s) > max {
		s = s[:max]
	}
	return strings.ToValidUTF8(s, "")
}
func first(v map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		var s string
		if json.Unmarshal(v[name], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}
func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(host), "www."), ".")
}

func (a *App) collect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	// No credentials or private data are exposed by this ingestion-only endpoint.
	if o := r.Header.Get("Origin"); o != "" && o != "null" {
		w.Header().Set("Access-Control-Allow-Origin", o)
	}
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, failure(413, "请求过大"))
		return
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(b, &data) != nil {
		writeError(w, failure(400, "请求格式无效"))
		return
	}
	ua := r.UserAgent()
	if botPattern.MatchString(ua) {
		w.WriteHeader(202)
		return
	}
	u, err := url.Parse(first(data, "u", "url"))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || len(u.String()) > 8192 {
		writeError(w, failure(400, "页面地址无效"))
		return
	}
	domain := first(data, "d", "domain")
	if domain == "" {
		domain = u.Hostname()
	}
	domain, err = normalizeDomain(domain)
	if err != nil || canonicalHost(u.Hostname()) != domain {
		w.WriteHeader(202)
		return
	}
	if o := r.Header.Get("Origin"); o != "" {
		origin, err := url.Parse(o)
		if err != nil || canonicalHost(origin.Hostname()) != domain || (origin.Scheme != "http" && origin.Scheme != "https") {
			w.WriteHeader(202)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	a.mu.RLock()
	site, err := readSite(ctx, a.db, domain)
	if err != nil {
		a.mu.RUnlock()
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, err)
		} else {
			w.WriteHeader(202)
		}
		return
	}
	loc, _ := time.LoadLocation(site.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now()
	visitor := first(data, "v", "visitor")
	if visitor != "" && !visitorPattern.MatchString(visitor) {
		a.mu.RUnlock()
		writeError(w, failure(400, "访客标识无效"))
		return
	}
	identity := visitor
	if identity == "" {
		identity = a.clientIP(r) + "|" + ua + "|" + now.In(loc).Format("2006-01-02")
	}
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte(strconvID(site.ID) + "|" + identity))
	userID := hex.EncodeToString(h.Sum(nil))
	sessionVisitor := a.sessionVisitor(r, site.ID, ua, now)
	epoch := a.epoch
	a.mu.RUnlock()
	name := first(data, "n", "name")
	if name == "" {
		name = "pageview"
	}
	if len(name) > 255 {
		writeError(w, failure(400, "事件名称过长"))
		return
	}
	ref := first(data, "r", "referrer")
	source := "Direct"
	if parsed, e := url.Parse(ref); e == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		source = canonicalHost(parsed.Hostname())
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		ref = parsed.String()
	} else {
		ref = ""
	}
	if source == domain {
		source = "Direct"
	}
	q := u.Query()
	if q.Get("utm_source") != "" {
		source = short(q.Get("utm_source"), 200)
	}
	props := map[string]any{}
	for _, k := range []string{"p", "props", "m", "meta"} {
		if len(data[k]) > 0 {
			if json.Unmarshal(data[k], &props) != nil {
				writeError(w, failure(400, "事件属性无效"))
				return
			}
			break
		}
	}
	if len(props) > 32 {
		writeError(w, failure(400, "事件属性过多"))
		return
	}
	for k, v := range props {
		if len(k) > 300 {
			writeError(w, failure(400, "事件属性名称过长"))
			return
		}
		switch v := v.(type) {
		case string:
			props[k] = short(v, 2000)
		case nil, bool, float64:
		default:
			writeError(w, failure(400, "事件属性只接受简单值"))
			return
		}
	}
	propsJSON, _ := json.Marshal(props)
	country := ""
	if a.cfg.TrustProxy {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if p := net.ParseIP(ip); p != nil && p.IsLoopback() {
			country = strings.ToUpper(r.Header.Get("CF-IPCountry"))
			if !countryPattern.MatchString(country) {
				country = ""
			}
		}
	}
	browser, osName, device := parseUA(ua)
	var width int
	_ = json.Unmarshal(data["w"], &width)
	if width <= 0 {
		_ = json.Unmarshal(data["width"], &width)
	}
	if device != "Tablet" && device != "Mobile" && width > 0 && width < 768 {
		device = "Mobile"
	}
	screen := strings.ToLower(first(data, "s", "screen"))
	if !screenPattern.MatchString(screen) {
		screen = ""
	}
	pathname := u.EscapedPath()
	if decoded, e := url.PathUnescape(pathname); e == nil {
		pathname = decoded
	}
	if pathname == "" {
		pathname = "/"
	}
	ev := Event{SiteID: site.ID, At: now.Unix(), Name: name, Hostname: canonicalHost(u.Hostname()), Path: short(pathname, 2048), Source: source, Referrer: short(ref, 2048), UserID: userID, SessionVisitor: sessionVisitor, Country: country, Browser: browser, OS: osName, Device: device, Language: short(first(data, "l", "language"), 64), Screen: screen, Title: short(first(data, "t", "title"), 500), UTMSource: short(q.Get("utm_source"), 200), UTMMedium: short(q.Get("utm_medium"), 200), UTMCampaign: short(q.Get("utm_campaign"), 200), Props: string(propsJSON)}
	req := &ingest{event: ev, done: make(chan error, 1), epoch: epoch}
	select {
	case <-a.stop:
		writeError(w, failure(503, "服务正在关闭"))
		return
	default:
	}
	select {
	case a.queue <- req:
	default:
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
		_ = jsonResponse(w, map[string]string{"error": "event_buffer_full"})
		return
	}
	select {
	case err := <-req.done:
		if err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(202)
	case <-ctx.Done():
		writeError(w, failure(503, "采集超时"))
	}
}

func (a *App) ingestLoop() {
	defer a.workers.Done()
	for {
		select {
		case req := <-a.queue:
			batch := []*ingest{req}
			timer := time.NewTimer(4 * time.Millisecond)
		gather:
			for len(batch) < 64 {
				select {
				case next := <-a.queue:
					batch = append(batch, next)
				case <-timer.C:
					break gather
				case <-a.stop:
					break gather
				}
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			err := a.persistBatch(batch)
			for _, r := range batch {
				r.done <- err
			}
		case <-a.stop:
			for {
				select {
				case req := <-a.queue:
					err := a.persistBatch([]*ingest{req})
					req.done <- err
				default:
					return
				}
			}
		}
	}
}

func (a *App) persistBatch(batch []*ingest) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,referrer_source,referrer,user_id,session_id,session_visitor_id,country_code,browser,os,device,language,screen,title,utm_source,utm_medium,utm_campaign,props) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, req := range batch {
		if req.epoch != a.epoch {
			return failure(503, "恢复切换期间请重试采集")
		}
		e := req.event
		var session string
		// A visit continues while the same visitor stays active: the visitor is the address hash when the event has one,
		// otherwise the browser id. A new address starts a new visit, which is what makes it a new visitor.
		if e.SessionVisitor != "" {
			err = tx.QueryRowContext(ctx, "SELECT session_id FROM events_v2 WHERE site_id=? AND session_visitor_id=? AND session_visitor_id<>'' AND timestamp>=? ORDER BY timestamp DESC,id DESC LIMIT 1", e.SiteID, e.SessionVisitor, e.At-1800).Scan(&session)
		} else {
			err = tx.QueryRowContext(ctx, "SELECT session_id FROM events_v2 WHERE site_id=? AND user_id=? AND timestamp>=? ORDER BY timestamp DESC,id DESC LIMIT 1", e.SiteID, e.UserID, e.At-1800).Scan(&session)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if session == "" {
			session = randomHex(16)
		}
		_, err = stmt.ExecContext(ctx, e.SiteID, e.At, e.Name, e.Hostname, e.Path, e.Source, e.Referrer, e.UserID, session, e.SessionVisitor, e.Country, e.Browser, e.OS, e.Device, e.Language, e.Screen, e.Title, e.UTMSource, e.UTMMedium, e.UTMCampaign, e.Props)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func parseUA(ua string) (browser, osName, device string) {
	browser = "Other"
	osName = "Other"
	device = "Desktop"
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "EdgA/") || strings.Contains(ua, "EdgiOS/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	switch {
	case strings.Contains(ua, "Android"):
		osName = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		osName = "iOS"
	case strings.Contains(ua, "Windows"):
		osName = "Windows"
	case strings.Contains(ua, "CrOS"):
		osName = "Chrome OS"
	case strings.Contains(ua, "Macintosh"):
		osName = "macOS"
	case strings.Contains(ua, "Linux"):
		osName = "Linux"
	}
	if strings.Contains(ua, "Mobile") || strings.Contains(ua, "iPhone") {
		device = "Mobile"
	}
	if strings.Contains(ua, "iPad") || (strings.Contains(ua, "Android") && !strings.Contains(ua, "Mobile")) {
		device = "Tablet"
	}
	return
}
