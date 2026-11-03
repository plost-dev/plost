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

const testChrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36"

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

// post sends one pageview as a tracker would, with the given forwarding header (empty for a direct request).
func post(t *testing.T, x *testApp, visitor, forwarded string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "v": visitor, "n": "pageview"})
	req, _ := http.NewRequest("POST", x.server.URL+"/api/event", bytes.NewReader(b))
	req.Header.Set("User-Agent", testChrome)
	req.Header.Set("Origin", "https://example.com")
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	resp := mustDo(t, x.client, req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("collection returned %d", resp.StatusCode)
	}
}

// storedIDs returns the browser id and the session hash of every event, oldest first.
func storedIDs(t *testing.T, a *App) (user, hash []string) {
	t.Helper()
	rows, err := a.db.Query("SELECT user_id,session_visitor_id FROM events_v2 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var u, h string
		if err := rows.Scan(&u, &h); err != nil {
			t.Fatal(err)
		}
		user, hash = append(user, u), append(hash, h)
	}
	return user, hash
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
	post(t, x, "browser-aaaaaaaa", "203.0.113.1")
	post(t, x, "browser-aaaaaaaa", "203.0.113.2") // the same browser on another network
	post(t, x, "browser-bbbbbbbb", "203.0.113.1") // another browser behind the first address
	user, hash := storedIDs(t, a)
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

// With no proxies declared, the forwarded address is the visitor's. A proxy on this machine that says nothing about
// whom it serves still hides every visitor; hashing its address would merge them all, so those events keep no hash
// and count by browser id.
func TestCollectionDoesNotMergeVisitorsWhoseAddressIsUnknown(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	post(t, x, "browser-aaaaaaaa", "203.0.113.1") // forwarded by a proxy nobody declared: found on its own
	post(t, x, "browser-bbbbbbbb", "")            // the test client itself connects from the loopback address
	_, hash := storedIDs(t, x.app)
	if len(hash) != 2 || hash[0] == "" || hash[1] != "" {
		t.Fatalf("the forwarded visitor needs a hash and the silent local one none: %q", hash)
	}
	var site int64
	_ = x.app.db.QueryRow("SELECT id FROM sites").Scan(&site)
	o, err := x.app.overview(context.Background(), site, Range{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)}, nil)
	if err != nil || o.Visitors != 2 {
		t.Errorf("two browsers must stay two visitors: %+v %v", o, err)
	}
}

// seedVisitors writes events with chosen identities and times, so the expectations do not depend on the clock.
func seedVisitors(t *testing.T, x *testApp, site int64) (from, to time.Time) {
	t.Helper()
	ten := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	for _, e := range []struct {
		at                                 time.Duration
		name, user, session, hash, country string
	}{
		{5 * time.Minute, "pageview", "u1", "A1", "s1", "US"},
		{20 * time.Minute, "pageview", "u1", "A1", "s1", "US"},
		{40 * time.Minute, "pageview", "u1", "A2", "s2", "US"}, // the same browser after changing address: another visitor and visit
		{10 * time.Minute, "pageview", "u2", "B1", "s3", "US"},
		{10*time.Minute + 30*time.Second, "Outbound Link: Click", "u2", "B1", "s3", "US"}, // not a pageview
		{90 * time.Minute, "pageview", "u2", "B2", "s3", "US"},                            // back after more than 30 minutes: a new visit
		{15 * time.Minute, "pageview", "u3", "C1", "", "DE"},                              // stored before the session hash existed
	} {
		_, err := x.app.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id,country_code) VALUES(?,?,?,?,?,?,?,?,?)",
			site, ten.Add(e.at).Unix(), e.name, "example.com", "/", e.user, e.session, e.hash, e.country)
		if err != nil {
			t.Fatal(err)
		}
	}
	return ten, ten.Add(2 * time.Hour)
}

func TestVisitorsAndVisitsAreCountedLikeUmami(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var site int64
	_ = x.app.db.QueryRow("SELECT id FROM sites").Scan(&site)
	from, to := seedVisitors(t, x, site)
	ctx := context.Background()
	rng := Range{From: from, To: to}

	// Visitors: hashes s1, s2, s3 and u3 (no hash, so its browser id). Visits A1, A2, B1, B2, C1, of which all but A1
	// have a single pageview. Only A1 has a duration: 15 minutes over five visits.
	o, err := x.app.overview(ctx, site, rng, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.Visitors != 4 || o.Visits != 5 || o.Pageviews != 6 || o.BounceRate != 80 || o.Duration != 180 {
		t.Errorf("overview: %+v", o)
	}

	points, err := x.app.series(ctx, site, Range{From: from, To: to, Period: "custom"}, url.Values{"interval": {"hour"}})
	if err != nil || len(points) != 2 {
		t.Fatalf("series: %v %d", err, len(points))
	}
	for i, want := range [][2]int64{{4, 4}, {1, 1}} { // [visitors, visits] for the hours starting at 10:00 and 11:00
		if points[i].Visitors != want[0] || points[i].Visits != want[1] {
			t.Errorf("hour %d: visitors %d visits %d, want %v", 10+i, points[i].Visitors, points[i].Visits, want)
		}
	}
	if points[0].BounceRate != 75 {
		t.Errorf("hour 10 bounce: %v", points[0].BounceRate)
	}

	rows, err := x.app.rows(ctx, site, rng, nil, "country_code", false, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Name] = r.Value
	}
	if got["US"] != 3 || got["DE"] != 1 {
		t.Errorf("countries: %v", got)
	}

	// Every event is on "/", so these rows are the number of visitors.
	details, err := x.app.reportDetails(ctx, Site{ID: site, Timezone: "UTC"}, rng, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"entryPages", "pageVisitors"} {
		list, _ := details[key].([]Row)
		if len(list) != 1 || list[0].Value != 4 {
			t.Errorf("%s: %v, want 4 visitors", key, list)
		}
	}
}

// A visit continues for as long as the same address hash stays active, and ends after 30 minutes without activity.
func TestAVisitFollowsTheAddressHashAndEndsAfterThirtyMinutes(t *testing.T) {
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
	var site int64
	_ = a.db.QueryRow("SELECT id FROM sites").Scan(&site)
	now := time.Now()
	for _, old := range []struct {
		ip, session string
		age         time.Duration
	}{{"203.0.113.1", "STALE", 31 * time.Minute}, {"203.0.113.2", "ACTIVE", 10 * time.Minute}} {
		_, err := a.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id) VALUES(?,?,?,?,?,?,?,?)",
			site, now.Add(-old.age).Unix(), "pageview", "example.com", "/", "earlier", old.session, a.sessionVisitorID(site, old.ip, testChrome, now))
		if err != nil {
			t.Fatal(err)
		}
	}
	post(t, x, "browser-aaaaaaaa", "203.0.113.1") // its last visit ended 31 minutes ago
	post(t, x, "browser-bbbbbbbb", "203.0.113.2") // another browser, but the same address and agent: still in the visit
	post(t, x, "browser-bbbbbbbb", "203.0.113.9") // the same browser on a new address: a new visit
	rows, err := a.db.Query("SELECT session_id FROM events_v2 WHERE user_id<>'earlier' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sessions []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, id)
	}
	if len(sessions) != 3 || sessions[0] == "STALE" || sessions[1] != "ACTIVE" || sessions[2] == "ACTIVE" || sessions[2] == sessions[0] {
		t.Errorf("visits did not follow the address hash: %q", sessions)
	}
}

// The site list, the report, shared pages and the stats API all go through the same counting.
func TestEveryReaderShowsTheSameVisitors(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var site int64
	_ = x.app.db.QueryRow("SELECT id FROM sites").Scan(&site)
	seedVisitors(t, x, site)
	// The test site is in Tashkent (UTC+5), so its 2026-11-01 holds the whole seed (10:00-12:00 UTC).
	query := "?period=custom&from=2026-11-01&to=2026-11-01"
	var report Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report"+query, nil, 200), &report)
	if report.Overview.Visitors != 4 || report.Overview.Visits != 5 {
		t.Fatalf("report: %+v", report.Overview)
	}
	var list []struct {
		Overview Overview `json:"overview"`
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites"+query, nil, 200), &list)
	if len(list) != 1 || list[0].Overview.Visitors != 4 || list[0].Overview.Visits != 5 {
		t.Errorf("site list: %+v", list)
	}
	var share struct {
		Token string `json:"token"`
	}
	json.Unmarshal(x.call(t, "POST", "/api/sites/example.com/shares", map[string]string{"name": "Read only"}, 200), &share)
	var shared Report
	json.Unmarshal(x.call(t, "GET", "/api/share/"+share.Token+query, nil, 200), &shared)
	if shared.Overview.Visitors != 4 {
		t.Errorf("shared page: %+v", shared.Overview)
	}
}

func TestMigrationAddsTheSessionHashToAnOlderDatabase(t *testing.T) {
	x := newTestApp(t)
	// A database from before the column has neither the column nor the index that uses it.
	for _, statement := range []string{"DROP INDEX events_hash_time", "ALTER TABLE events_v2 DROP COLUMN session_visitor_id"} {
		if _, err := x.app.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(x.app.db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := x.app.db.QueryRow("SELECT count(*) FROM pragma_table_info('events_v2') WHERE name='session_visitor_id'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration did not restore the column: %d %v", n, err)
	}
	if err := x.app.db.QueryRow("SELECT count(*) FROM pragma_index_list('events_v2') WHERE name='events_hash_time'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration did not restore the index: %d %v", n, err)
	}
}
