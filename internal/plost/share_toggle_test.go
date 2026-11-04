package plost

import (
	"encoding/json"
	"testing"
)

func TestShareLinkIsMadeOffAndSwitchedWithoutChangingAddress(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	type shares struct {
		Shares []struct {
			Slug    string `json:"slug"`
			Enabled bool   `json:"enabled"`
		} `json:"shares"`
	}
	read := func() shares {
		var s shares
		json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/settings", nil, 200), &s)
		return s
	}
	first := read()
	if len(first.Shares) != 1 || first.Shares[0].Enabled || len(first.Shares[0].Slug) != 48 {
		t.Fatalf("a site gets one share link, switched off: %+v", first.Shares)
	}
	slug := first.Shares[0].Slug
	if again := read(); len(again.Shares) != 1 || again.Shares[0].Slug != slug {
		t.Fatal("reading the settings again must not add another link")
	}
	x.call(t, "GET", "/api/share/"+slug, nil, 404)
	x.call(t, "PUT", "/api/sites/example.com/share", map[string]bool{"enabled": true}, 200)
	x.call(t, "GET", "/api/share/"+slug, nil, 200)
	x.call(t, "PUT", "/api/sites/example.com/share", map[string]bool{"enabled": false}, 200)
	x.call(t, "GET", "/api/share/"+slug, nil, 404)
	if s := read(); s.Shares[0].Slug != slug || s.Shares[0].Enabled {
		t.Fatal("switching off keeps the same address")
	}
}
