package plost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSessionVisitorIDIsMonthlySiteScopedAndHidesTheAddress(t *testing.T) {
	x := newTestApp(t)
	june := time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC)
	same := x.app.sessionVisitorID(1, "203.0.113.7", "agent", june)
	if len(same) != 64 || strings.Contains(same, "203.0.113.7") {
		t.Fatalf("the hash must be a plain digest, got %q", same)
	}
	for name, other := range map[string]string{
		"address":    x.app.sessionVisitorID(1, "203.0.113.8", "agent", june),
		"user agent": x.app.sessionVisitorID(1, "203.0.113.7", "other", june),
		"site":       x.app.sessionVisitorID(2, "203.0.113.7", "agent", june),
		"month":      x.app.sessionVisitorID(1, "203.0.113.7", "agent", june.Add(2*time.Minute)),
	} {
		if other == same {
			t.Errorf("a different %s must give a different visitor", name)
		}
	}
	if x.app.sessionVisitorID(1, "203.0.113.7", "agent", june.Add(-48*time.Hour)) != same {
		t.Error("the same address and agent within a month must be one visitor")
	}
	// Seen from another timezone the month still ends at UTC midnight.
	if x.app.sessionVisitorID(1, "203.0.113.7", "agent", june.In(time.FixedZone("UTC+8", 8*3600))) != same {
		t.Error("the month must be taken in UTC")
	}
}

func TestCollectionStoresTheSessionHashNextToTheBrowserId(t *testing.T) {
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test", TrustProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	t.Cleanup(func() { server.Close(); a.Close() })
	jar, _ := cookiejar.New(nil)
	x := &testApp{a, server, &http.Client{Jar: jar, Timeout: 15 * time.Second}}
	x.setup(t)
	x.addSite(t, "example.com")
	send := func(visitor, ip, agent string) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "v": visitor, "n": "pageview"})
		req, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		req.Header.Set("User-Agent", agent)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-Forwarded-For", ip)
		resp := mustDo(t, x.client, req)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 202 {
			t.Fatalf("collection returned %d", resp.StatusCode)
		}
	}
	const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36"
	send("browser-aaaaaaaa", "203.0.113.1", chrome)
	send("browser-aaaaaaaa", "203.0.113.2", chrome) // the same browser on another network
	send("browser-bbbbbbbb", "203.0.113.1", chrome) // another browser behind the first address
	rows, err := a.db.Query("SELECT user_id,session_visitor_id FROM events_v2 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var user, hash []string
	for rows.Next() {
		var u, h string
		if err := rows.Scan(&u, &h); err != nil {
			t.Fatal(err)
		}
		user, hash = append(user, u), append(hash, h)
	}
	if len(user) != 3 || hash[0] == "" || hash[1] == "" || hash[2] == "" {
		t.Fatalf("every event needs both ids, got %d rows %q", len(user), hash)
	}
	if user[0] != user[1] || hash[0] == hash[1] {
		t.Error("one browser on two addresses is one browser id but two session hashes")
	}
	if user[0] == user[2] || hash[0] != hash[2] {
		t.Error("two browsers behind one address and agent are two browser ids but one session hash")
	}
}

// seed writes events with chosen identities and times, so the expectations do not depend on the clock.
func seedVisitors(t *testing.T, x *testApp, site int64) (from, to time.Time) {
	t.Helper()
	ten := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	for _, e := range []struct {
		at                          time.Duration
		user, session, hash, region string
	}{
		{5 * time.Minute, "u1", "S1", "s1", "US"},
		{20 * time.Minute, "u1", "S1", "s1", "US"},
		{40 * time.Minute, "u1", "S1", "s2", "US"}, // the same browser after changing address
		{10 * time.Minute, "u2", "S2", "s3", "US"},
		{90 * time.Minute, "u2", "S3", "s3", "US"}, // returns in the next hour
		{15 * time.Minute, "u3", "S4", "", "DE"},   // stored before the session hash existed
	} {
		_, err := x.app.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id,country_code) VALUES(?,?,?,?,?,?,?,?,?)",
			site, ten.Add(e.at).Unix(), "pageview", "example.com", "/", e.user, e.session, e.hash, e.region)
		if err != nil {
			t.Fatal(err)
		}
	}
	return ten, ten.Add(2 * time.Hour)
}

func TestVisitorModesCountTheSameEventsTwoWays(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var site int64
	_ = x.app.db.QueryRow("SELECT id FROM sites").Scan(&site)
	from, to := seedVisitors(t, x, site)
	ctx := context.Background()
	rng := Range{From: from, To: to}

	persistent, err := x.app.overview(ctx, site, rng, nil, modePersistent)
	if err != nil {
		t.Fatal(err)
	}
	// Browsers u1, u2, u3; visits S1..S4; one of three S-visits with a single view is a bounce.
	if persistent.Visitors != 3 || persistent.Visits != 4 || persistent.Pageviews != 6 || persistent.BounceRate != 75 {
		t.Errorf("persistent overview: %+v", persistent)
	}
	sessions, err := x.app.overview(ctx, site, rng, nil, modeSessions)
	if err != nil {
		t.Fatal(err)
	}
	// Hashes s1, s2, s3 plus u3 (no hash, falls back to its browser id); visits are (s1,10h) (s2,10h) (s3,10h) (s3,11h) (u3,10h).
	if sessions.Visitors != 4 || sessions.Visits != 5 || sessions.Pageviews != 6 || sessions.BounceRate != 80 || sessions.Duration != 180 {
		t.Errorf("session overview: %+v", sessions)
	}
	if sessions.Visitors < persistent.Visitors {
		t.Error("a session visitor can never be fewer than the browsers behind it here")
	}

	for mode, want := range map[visitorMode][2][2]int64{
		modePersistent: {{3, 3}, {1, 1}}, // [hour 10: visitors, visits] [hour 11: visitors, visits]
		modeSessions:   {{4, 4}, {1, 1}},
	} {
		points, err := x.app.series(ctx, site, Range{From: from, To: to, Period: "custom"}, url.Values{"interval": {"hour"}}, mode)
		if err != nil || len(points) != 2 {
			t.Fatalf("%s series: %v %d", mode, err, len(points))
		}
		for i := range points {
			if points[i].Visitors != want[i][0] || points[i].Visits != want[i][1] {
				t.Errorf("%s hour %d: visitors %d visits %d, want %v", mode, 10+i, points[i].Visitors, points[i].Visits, want[i])
			}
		}
	}

	for mode, want := range map[visitorMode]map[string]int64{
		modePersistent: {"US": 2, "DE": 1},
		modeSessions:   {"US": 3, "DE": 1},
	} {
		rows, err := x.app.rows(ctx, site, rng, nil, "country_code", false, false, mode)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int64{}
		for _, r := range rows {
			got[r.Name] = r.Value
		}
		if got["US"] != want["US"] || got["DE"] != want["DE"] {
			t.Errorf("%s countries: %v, want %v", mode, got, want)
		}
	}

	// Every event is on "/", so the entry page row is the number of visitors whose visit started there.
	for mode, want := range map[visitorMode]int64{modePersistent: 3, modeSessions: 4} {
		details, err := x.app.reportDetails(ctx, Site{ID: site, Timezone: "UTC"}, rng, nil, mode)
		if err != nil {
			t.Fatal(err)
		}
		entries, _ := details["entryPages"].([]Row)
		if len(entries) != 1 || entries[0].Value != want {
			t.Errorf("%s entry pages: %v, want %d visitors", mode, entries, want)
		}
		pages, _ := details["pageVisitors"].([]Row)
		if len(pages) != 1 || pages[0].Value != want {
			t.Errorf("%s page visitors: %v, want %d", mode, pages, want)
		}
	}
}

func TestVisitorModeSettingDrivesEveryReader(t *testing.T) {
	x := newTestApp(t)
	x.call(t, "GET", "/api/visitor-mode", nil, 401)
	x.setup(t)
	x.addSite(t, "example.com")
	var site int64
	_ = x.app.db.QueryRow("SELECT id FROM sites").Scan(&site)
	seedVisitors(t, x, site)
	// The seed lies in the past, so read it back with an explicit range.
	query := "?period=custom&from=2026-11-01&to=2026-11-01"
	// The test site is in Tashkent (UTC+5), so its 2026-11-01 holds the whole seed (10:00-12:00 UTC).
	var mode struct {
		Mode string `json:"mode"`
	}
	json.Unmarshal(x.call(t, "GET", "/api/visitor-mode", nil, 200), &mode)
	if mode.Mode != "persistent" {
		t.Fatalf("a new instance keeps the browser-based count, got %q", mode.Mode)
	}
	read := func() Report {
		var report Report
		json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report"+query, nil, 200), &report)
		return report
	}
	before := read()
	if before.VisitorMode != "persistent" || before.Overview.Visitors != 3 {
		t.Fatalf("persistent report: %q %+v", before.VisitorMode, before.Overview)
	}
	x.call(t, "PUT", "/api/visitor-mode", map[string]string{"mode": "weekly"}, 400)
	x.call(t, "PUT", "/api/visitor-mode", map[string]string{"mode": "sessions"}, 200)
	after := read()
	if after.VisitorMode != "sessions" || after.Overview.Visitors != 4 || after.Overview.Visits != 5 {
		t.Fatalf("session report: %q %+v", after.VisitorMode, after.Overview)
	}
	var list []struct {
		Overview Overview `json:"overview"`
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites"+query, nil, 200), &list)
	if len(list) != 1 || list[0].Overview.Visitors != 4 {
		t.Errorf("the site list must follow the mode too: %+v", list)
	}

	var share struct {
		Token string `json:"token"`
	}
	json.Unmarshal(x.call(t, "POST", "/api/sites/example.com/shares", map[string]string{"name": "Read only"}, 200), &share)
	var shared Report
	json.Unmarshal(x.call(t, "GET", "/api/share/"+share.Token+query, nil, 200), &shared)
	if shared.VisitorMode != "sessions" || shared.Overview.Visitors != 4 {
		t.Errorf("a shared page must show the same count: %q %+v", shared.VisitorMode, shared.Overview)
	}
	req, _ := http.NewRequest("GET", x.server.URL+"/api/v1/stats?period=yesterday", nil)
	req.Header.Set("Authorization", "Bearer "+share.Token)
	resp := mustDo(t, x.client, req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"visitorMode":"sessions"`) {
		t.Errorf("the API must say which count it returns: %d %s", resp.StatusCode, body)
	}

	x.call(t, "PUT", "/api/visitor-mode", map[string]string{"mode": "persistent"}, 200)
	if got := read(); got.VisitorMode != "persistent" || got.Overview.Visitors != 3 {
		t.Errorf("switching back: %q %+v", got.VisitorMode, got.Overview)
	}
}

func TestMigrationAddsTheSessionHashToAnOlderDatabase(t *testing.T) {
	x := newTestApp(t)
	if _, err := x.app.db.Exec("ALTER TABLE events_v2 DROP COLUMN session_visitor_id"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(x.app.db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := x.app.db.QueryRow("SELECT count(*) FROM pragma_table_info('events_v2') WHERE name='session_visitor_id'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration did not restore the column: %d %v", n, err)
	}
}
