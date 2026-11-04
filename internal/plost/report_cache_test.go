package plost

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func insertPageview(t *testing.T, a *App, site int64, visitor string, at time.Time) {
	t.Helper()
	_, err := a.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id) VALUES(?,?,'pageview','example.com','/',?,?)", site, at.Unix(), visitor, visitor)
	if err != nil {
		t.Fatal(err)
	}
}

// A long range is counted once and kept, but who is online now is always counted afresh, and a short range is never kept.
func TestLongRangeReportsAreKeptButOnlineNowIsFresh(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	site, err := readSite(context.Background(), x.app.db, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	insertPageview(t, x.app, site.ID, "a", time.Now().Add(-time.Minute))
	report := func(period string) Report {
		var r Report
		if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period="+period+"&ui=1", nil, 200), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if got := report("28d").Overview; got.Pageviews != 1 || got.Online != 1 {
		t.Fatalf("first 28d report: %+v", got)
	}
	insertPageview(t, x.app, site.ID, "b", time.Now().Add(-time.Minute))
	if got := report("28d").Overview; got.Pageviews != 1 || got.Online != 2 {
		t.Errorf("kept 28d report should keep its counts but show 2 online now: %+v", got)
	}
	if got := report("24h").Overview; got.Pageviews != 2 {
		t.Errorf("24h report is never kept: %+v", got)
	}
	// A restored backup is another database: nothing counted before it is shown.
	x.app.epoch++
	if got := report("28d").Overview; got.Pageviews != 2 {
		t.Errorf("report after a new epoch: %+v", got)
	}
}

// A request that stops waiting leaves the count running, so the next request finds the report ready.
func TestAReportOutlivesTheRequestThatAskedForIt(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	a := x.app
	site, err := readSite(context.Background(), a.db, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	insertPageview(t, a, site.ID, "a", time.Now().Add(-time.Hour))
	rng := Range{From: time.Now().Add(-90 * 24 * time.Hour), To: time.Now(), Period: "91d"}
	q := map[string][]string{"period": {"91d"}}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	a.mu.RLock()
	_, err = a.cachedReport(ctx, site, rng, q)
	a.mu.RUnlock()
	if err != nil {
		if e, ok := err.(*apiError); !ok || e.status != 503 {
			t.Fatalf("expired request: %v", err)
		}
	}
	key := reportKey(a.epoch, site, q)
	for deadline := time.Now().Add(10 * time.Second); ; {
		a.reports.mu.Lock()
		entry, ok := a.reports.entries[key]
		a.reports.mu.Unlock()
		if ok {
			if entry.report.Overview.Pageviews != 1 {
				t.Errorf("kept report: %+v", entry.report.Overview)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the report was not kept after its request gave up")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
