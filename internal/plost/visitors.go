package plost

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

// visitorMode decides how visitors, visits and bounces are counted. The whole instance uses one mode, so the
// site list, the report, shared pages and the API always show the same numbers. All three work from the same stored
// events, so any period can be read under any mode.
//
// plost is this program's own way and the default: a visitor is one browser, identified by the anonymous id kept
// in the visited site's localStorage, so a browser that changes its IP address is still one visitor. A visit ends
// after 30 minutes without activity and a bounce is a visit with a single pageview.
//
// umami follows Umami: a visitor is a hash of IP address, user agent and calendar month, and a visit is one
// visitor in one UTC hour. Someone who switches IP address or proxy counts again, which is what makes the two
// products agree.
//
// google follows Google Analytics 4 as far as the stored events allow: users are browsers identified by a
// first-party id (as GA's client id cookie does), and a session ends after 30 minutes without activity. Its bounce
// rate is the share of sessions that were not engaged, where an engaged session has two or more pageviews or lasts
// at least ten seconds. GA measures time in the foreground tab, which this program cannot, so the last event of the
// visit stands in for it; modelled data and cross-device user ids are not reproduced.
type visitorMode string

const (
	modePlost  visitorMode = "plost"
	modeUmami  visitorMode = "umami"
	modeGoogle visitorMode = "google"
)

// parseVisitorMode accepts the current names. The stored setting also accepts the two names an earlier build
// wrote, so an instance that already chose one keeps its choice.
func parseVisitorMode(value string, stored bool) (visitorMode, bool) {
	switch value {
	case string(modePlost), string(modeUmami), string(modeGoogle):
		return visitorMode(value), true
	case "persistent":
		return modePlost, stored
	case "sessions":
		return modeUmami, stored
	}
	return modePlost, false
}

func (a *App) visitorMode(ctx context.Context) visitorMode {
	mode, _ := parseVisitorMode(a.setting(ctx, "visitor_mode", string(modePlost)), true)
	return mode
}

// visitor is the SQL expression that identifies one visitor. Events stored before the session hash existed carry no
// hash and keep counting by browser id.
func (m visitorMode) visitor() string {
	if m == modeUmami {
		return "COALESCE(NULLIF(session_visitor_id,''),user_id)"
	}
	return "user_id"
}

// visit is the SQL expression that identifies one visit. An Umami visit is one visitor in one UTC hour.
func (m visitorMode) visit() string {
	if m == modeUmami {
		return "(COALESCE(NULLIF(session_visitor_id,''),user_id)||':'||(timestamp/3600))"
	}
	return "session_id"
}

// visitScope narrows the events that make up a visit. Google also looks at events other than pageviews, because a
// link click ten seconds after landing is what makes a single-pageview session engaged.
func (m visitorMode) visitScope() string {
	if m == modeGoogle {
		return ""
	}
	return " AND name='pageview'"
}

// bounced is the SQL condition, over the pv and span columns below, that makes a visit a bounce.
func (m visitorMode) bounced() string {
	if m == modeGoogle {
		return "(pv<2 AND span<10)"
	}
	return "pv=1"
}

// visitColumns are what a per-visit subquery computes: pageviews, the span between the first and last pageview
// (the duration) and the span between the first and last event of any kind.
const visitColumns = "sum(name='pageview') AS pv,max(CASE WHEN name='pageview' THEN timestamp END)-min(CASE WHEN name='pageview' THEN timestamp END) AS dur,max(timestamp)-min(timestamp) AS span"

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
	mode, ok := parseVisitorMode(in.Mode, false)
	if !ok {
		return failure(400, "访客统计方式无效")
	}
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('visitor_mode',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(mode)); err != nil {
		return err
	}
	return jsonResponse(w, map[string]string{"mode": string(mode)})
}
