package plost

import (
	"encoding/json"
	"errors"
	"strings"
)

// The tracker sends its normal requests as a JSON POST to /api/send with
// {"type":"event","payload":{...}}. Some content blockers refuse requests shaped like other analytics tools (a
// text POST to /api/event) while they let this one through, so this shape on the wire keeps those visitors.
// /api/event stays for older copies of the script and for proxies set up with data-api.

// errIgnoredRequest marks a well-formed request that records nothing, such as an "identify".
var errIgnoredRequest = errors.New("not an event")

// fromTracker turns a /api/send body into the fields /api/event takes. Trackers from other products work
// too: "website" may be a site's domain, otherwise the page's hostname names the site, and "url" may be a
// path or a whole address.
func fromTracker(b []byte) ([]byte, error) {
	var in struct {
		Type    string                     `json:"type"`
		Payload map[string]json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(b, &in); err != nil || in.Payload == nil {
		return nil, errors.New("invalid body")
	}
	if in.Type != "event" {
		return nil, errIgnoredRequest
	}
	p := in.Payload
	out := map[string]json.RawMessage{}
	// Fields only Plost's tracker adds (browser id, event id, window width, retry transport) pass through as they are.
	for _, k := range []string{"v", "i", "w", "x"} {
		if v, ok := p[k]; ok {
			out[k] = v
		}
	}
	str := func(v string) json.RawMessage { b, _ := json.Marshal(v); return b }
	host := first(p, "hostname")
	if site := first(p, "website"); site != "" {
		if _, err := normalizeDomain(site); err == nil {
			out["d"] = str(site)
		}
	}
	if _, ok := out["d"]; !ok && host != "" {
		out["d"] = str(host)
	}
	if u := first(p, "url"); strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		out["u"] = str(u)
	} else if host != "" {
		if !strings.HasPrefix(u, "/") {
			u = "/" + u
		}
		out["u"] = str("https://" + host + u)
	}
	for from, to := range map[string]string{"referrer": "r", "title": "t", "language": "l", "screen": "s", "name": "n"} {
		if v := first(p, from); v != "" {
			out[to] = str(v)
		}
	}
	if data, ok := p["data"]; ok && len(data) > 0 && string(data) != "null" {
		out["p"] = data
	}
	return json.Marshal(out)
}
