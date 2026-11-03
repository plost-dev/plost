package plost

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

// Visitors, visits and bounces are counted the way Umami counts them, so numbers from the two products can be
// compared. A visitor is a hash of site, IP address, user agent and UTC month, which means someone who switches IP
// address or proxy counts as a new visitor; a visit is one visitor's run of activity, ended by 30 minutes without
// any (the session id stored with each event, see persistBatch); a bounce is a visit with a single pageview.
// Refreshing a page still adds a pageview, but not a visitor or a visit.
//
// Events stored before the hash existed, or that arrived without a usable address (see sessionVisitor), carry no
// hash and are counted by their browser id instead.
const (
	visitorSQL = "COALESCE(NULLIF(session_visitor_id,''),user_id)"
	visitSQL   = "session_id"
)

// sessionVisitorID hashes the address and user agent with the instance secret and the UTC month, so the raw address
// is never stored and the same person cannot be followed from one month to the next.
func (a *App) sessionVisitorID(site int64, ip, ua string, at time.Time) string {
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte("session-visitor|" + strconvID(site) + "|" + ip + "|" + ua + "|" + at.UTC().Format("2006-01")))
	return hex.EncodeToString(h.Sum(nil))
}

// sessionVisitor returns the hash for a collected event, or "" when the visitor's address cannot be known. Behind a
// reverse proxy that the program was not started to trust (-trusted-proxies), every request arrives from the proxy, so
// hashing would merge all visitors into one; those events count by browser id instead of collapsing.
func (a *App) sessionVisitor(r *http.Request, site int64, ua string, at time.Time) string {
	if !a.addressKnown(r) {
		return ""
	}
	return a.sessionVisitorID(site, a.clientIP(r), ua, at)
}
