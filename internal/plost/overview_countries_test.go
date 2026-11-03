package plost

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOverviewCountriesGroupsRecentVisitorsBySite(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "a.example.test")
	x.addSite(t, "b.example.test")
	now := time.Now()
	insert := func(domain string, at time.Time, visitor, country string) {
		t.Helper()
		if _, err := x.app.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id,country_code) SELECT id,?,'pageview',domain,'/',?,?,?,? FROM sites WHERE domain=?", at.Unix(), visitor, visitor, visitor, country, domain); err != nil {
			t.Fatal(err)
		}
	}
	insert("a.example.test", now.Add(-time.Hour), "v1", "DE")
	insert("a.example.test", now.Add(-2*time.Hour), "v1", "DE")
	insert("a.example.test", now.Add(-time.Hour), "v2", "BR")
	insert("b.example.test", now.Add(-time.Hour), "v3", "DE")
	insert("b.example.test", now.Add(-time.Hour), "v4", "DE")
	insert("b.example.test", now.Add(-48*time.Hour), "old", "JP")
	insert("b.example.test", now.Add(-time.Hour), "none", "")

	var got []struct {
		Code     string `json:"code"`
		Visitors int64  `json:"visitors"`
		Sites    []struct {
			Name     string `json:"name"`
			Visitors int64  `json:"visitors"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(x.call(t, "GET", "/api/overview/countries", nil, 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Code != "DE" || got[0].Visitors != 3 || got[1].Code != "BR" || got[1].Visitors != 1 {
		t.Fatalf("countries = %+v", got)
	}
	if len(got[0].Sites) != 2 || got[0].Sites[0].Name != "b.example.test" || got[0].Sites[0].Visitors != 2 {
		t.Fatalf("DE sites = %+v", got[0].Sites)
	}

	x.call(t, "POST", "/api/logout", map[string]any{}, 200)
	x.call(t, "GET", "/api/overview/countries", nil, 401)
}
