package plost

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReportLiveCountries(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	now := time.Now().Unix()
	for _, e := range []struct {
		at      int64
		visitor string
		country string
	}{
		{now - 30, "a", "JP"}, {now - 20, "a", "JP"}, {now - 60, "b", "JP"}, {now - 90, "c", "DE"},
		{now - 600, "d", "US"}, // Online ten minutes ago, not now.
		{now - 40, "e", ""},    // Country unknown: not on the map.
	} {
		if _, err := x.app.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id,country_code) SELECT id,?,'pageview','example.com','/',?,?,?,? FROM sites", e.at, e.visitor, e.visitor, e.visitor, e.country); err != nil {
			t.Fatal(err)
		}
	}
	var report Report
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today", nil, 200), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Live) != 2 || report.Live[0] != (Row{Name: "JP", Value: 2}) || report.Live[1] != (Row{Name: "DE", Value: 1}) {
		t.Fatalf("live: %+v", report.Live)
	}
}
