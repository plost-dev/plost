package plost

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

// visitorMode decides what the statistics call a visitor and a visit. The whole instance uses one mode, so the
// site list, the report, shared pages and the API always show the same numbers.
//
// persistent counts browsers: one anonymous id kept in the visited site's localStorage, so a browser that changes
// its IP address is still one visitor, and a visit ends after 30 minutes without activity.
//
// sessions follows Umami: a visitor is a hash of IP address, user agent and calendar month, and a visit is one
// visitor in one UTC hour. Someone who switches IP address or proxy counts again, which is what makes the two
// products agree.
type visitorMode string

const (
	modePersistent visitorMode = "persistent"
	modeSessions   visitorMode = "sessions"
)

func (a *App) visitorMode(ctx context.Context) visitorMode {
	if a.setting(ctx, "visitor_mode", string(modePersistent)) == string(modeSessions) {
		return modeSessions
	}
	return modePersistent
}

// visitor is the SQL expression that identifies one visitor. Events stored before the session hash existed carry no
// hash and keep counting by browser id.
func (m visitorMode) visitor() string {
	if m == modeSessions {
		return "COALESCE(NULLIF(session_visitor_id,''),user_id)"
	}
	return "user_id"
}

// visit is the SQL expression that identifies one visit. A session-mode visit is one visitor in one UTC hour.
func (m visitorMode) visit() string {
	if m == modeSessions {
		return "(COALESCE(NULLIF(session_visitor_id,''),user_id)||':'||(timestamp/3600))"
	}
	return "session_id"
}

// sessionVisitorID hashes the address and user agent with the instance secret and the UTC month, so the raw address
// is never stored and the same person cannot be followed from one month to the next.
func (a *App) sessionVisitorID(site int64, ip, ua string, at time.Time) string {
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte("session-visitor|" + strconvID(site) + "|" + ip + "|" + ua + "|" + at.UTC().Format("2006-01")))
	return hex.EncodeToString(h.Sum(nil))
}

func (a *App) visitorModeSettings(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet {
		return jsonResponse(w, map[string]string{"mode": string(a.visitorMode(r.Context()))})
	}
	var in struct {
		Mode string `json:"mode"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Mode != string(modePersistent) && in.Mode != string(modeSessions) {
		return failure(400, "访客统计方式无效")
	}
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('visitor_mode',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", in.Mode); err != nil {
		return err
	}
	return jsonResponse(w, map[string]string{"mode": in.Mode})
}
