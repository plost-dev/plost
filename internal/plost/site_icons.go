package plost

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// A site may have an icon of its own, uploaded on its settings page, in place of the one favicon.la finds for its
// domain. It is kept in the database with the site and served at the same /favicon/sources/{domain} address, so every
// place that shows the site's icon picks it up.

const siteIconMax = 256 << 10

// siteIconTypes are the image types an icon may have. SVG is left out: it can carry script.
var siteIconTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/x-icon": true}

func (a *App) putSiteIcon(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, siteIconMax+1))
	if err != nil || len(data) > siteIconMax {
		return failure(413, "图标不能超过 256 KB")
	}
	kind := http.DetectContentType(data)
	if !siteIconTypes[kind] {
		return failure(400, "图标只支持 PNG、JPG、GIF、WebP 或 ICO")
	}
	at := unixNow()
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO site_icons(site_id,type,data,updated_at) VALUES(?,?,?,?) ON CONFLICT(site_id) DO UPDATE SET type=excluded.type,data=excluded.data,updated_at=excluded.updated_at", s.ID, kind, data, at); err != nil {
		return err
	}
	return jsonResponse(w, map[string]int64{"iconVersion": at})
}

func (a *App) deleteSiteIcon(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	if _, err := a.db.ExecContext(r.Context(), "DELETE FROM site_icons WHERE site_id=?", s.ID); err != nil {
		return err
	}
	return jsonResponse(w, map[string]int64{"iconVersion": 0})
}

// siteIconVersions is when each site's own icon was last changed, for the pages to add to the icon's address so a new
// icon shows at once. A site without one is not listed.
func (a *App) siteIconVersions(r *http.Request) map[int64]int64 {
	out := map[int64]int64{}
	rows, err := a.db.QueryContext(r.Context(), "SELECT site_id,updated_at FROM site_icons")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, at int64
		if rows.Scan(&id, &at) == nil {
			out[id] = at
		}
	}
	return out
}

// serveSiteIcon answers /favicon/sources/{domain} with the site's own icon when it has one and the visitor may know
// about the site: signed in, or the site is public or shared. It reports whether it answered.
func (a *App) serveSiteIcon(w http.ResponseWriter, r *http.Request, name string) bool {
	if a.db == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	var kind string
	var data []byte
	var at int64
	var public bool
	var shares int
	err := a.db.QueryRowContext(r.Context(), `SELECT i.type,i.data,i.updated_at,s.public,(SELECT count(*) FROM shared_links l WHERE l.site_id=s.id)
		FROM site_icons i JOIN sites s ON s.id=i.site_id WHERE s.domain=?`, name).Scan(&kind, &data, &at, &public, &shares)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	if !public && shares == 0 {
		if _, err := a.user(r); err != nil {
			return false
		}
	}
	h := w.Header()
	h.Set("Content-Type", kind)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'")
	// The pages ask with ?v= the icon's version, so a new icon is a new address and the old one may stay cached.
	if r.URL.Query().Get("v") != "" {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, max-age=300")
	}
	_, _ = w.Write(data)
	return true
}
