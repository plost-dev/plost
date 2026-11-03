package plost

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

// API keys are read-only credentials for /api/v1/stats, one site each. Only a hash is kept, so the key is shown once
// when it is created; deleting the key (or its site) revokes it.
const apiKeyPrefix = "plst_"

func (a *App) apiKeys(w http.ResponseWriter, r *http.Request) error {
	rows, err := a.db.QueryContext(r.Context(), "SELECT k.id,k.name,s.domain,k.hint,k.created_at,k.last_used_at FROM api_keys k JOIN sites s ON s.id=k.site_id ORDER BY k.id DESC")
	if err != nil {
		return err
	}
	defer rows.Close()
	keys := []map[string]any{}
	for rows.Next() {
		var id, created, used int64
		var name, domain, hint string
		if err := rows.Scan(&id, &name, &domain, &hint, &created, &used); err != nil {
			return err
		}
		keys = append(keys, map[string]any{"id": id, "name": name, "domain": domain, "hint": hint, "createdAt": created, "lastUsedAt": used})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return jsonResponse(w, map[string]any{"keys": keys})
}

func (a *App) createAPIKey(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Domain string `json:"domain"`
		Name   string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		return failure(400, "请输入 API Key 名称")
	}
	s, err := readSite(r.Context(), a.db, in.Domain)
	if err != nil {
		return failure(404, "网站不存在")
	}
	key := apiKeyPrefix + randomHex(24)
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO api_keys(site_id,name,token_hash,hint,created_at) VALUES(?,?,?,?,?)", s.ID, in.Name, digest(key), key[len(key)-4:], unixNow()); err != nil {
		return err
	}
	return jsonResponse(w, map[string]string{"key": key})
}

func (a *App) deleteAPIKey(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return failure(400, "API Key 无效")
	}
	res, err := a.db.ExecContext(r.Context(), "DELETE FROM api_keys WHERE id=?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return failure(404, "API Key 不存在")
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

// apiKeySite is the site an API key reads, noting when the key was last used (at most once a minute).
func (a *App) apiKeySite(ctx context.Context, key string) (Site, error) {
	if len(key) != len(apiKeyPrefix)+48 || !strings.HasPrefix(key, apiKeyPrefix) {
		return Site{}, sql.ErrNoRows
	}
	var id int64
	var domain string
	if err := a.db.QueryRowContext(ctx, "SELECT k.id,s.domain FROM api_keys k JOIN sites s ON s.id=k.site_id WHERE k.token_hash=?", digest(key)).Scan(&id, &domain); err != nil {
		return Site{}, err
	}
	now := unixNow()
	_, _ = a.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at=? WHERE id=? AND last_used_at<?", now, id, now-60)
	return readSite(ctx, a.db, domain)
}
