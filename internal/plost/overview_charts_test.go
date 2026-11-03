package plost

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOverviewChartsSumsSites(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "a.example.test")
	x.addSite(t, "b.example.test")
	now := time.Now()
	insert := func(site string, at time.Time, visitor string, pages int) {
		for i := 0; i < pages; i++ {
			if _, err := x.app.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id) SELECT id,?,'pageview',domain,'/',?,?,? FROM sites WHERE domain=?",
				at.Add(time.Duration(i)*time.Second).Unix(), visitor, visitor, visitor, site); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert("a.example.test", now.Add(-2*time.Hour), "v1", 3)
	insert("a.example.test", now.Add(-3*time.Hour), "v2", 1)
	insert("b.example.test", now.Add(-2*time.Hour), "v3", 2)
	insert("b.example.test", now.Add(-30*time.Hour), "v4", 1) // yesterday's 24 hours
	insert("b.example.test", now.Add(-10*24*time.Hour), "v5", 1)

	var got OverviewCharts
	if err := json.Unmarshal(x.call(t, "GET", "/api/overview/charts", nil, 200), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Visitors != 3 || got.Totals.Pageviews != 6 || got.Previous.Visitors != 1 {
		t.Fatalf("totals = %+v previous = %+v", got.Totals, got.Previous)
	}
	x.call(t, "POST", "/api/logout", map[string]any{}, 200)
	x.call(t, "GET", "/api/overview/charts", nil, 401)
}
