package plost

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	fixA = "aaaaaaaa-0000-4000-8000-000000000001"
	fixB = "aaaaaaaa-0000-4000-8000-000000000002"
	fixC = "aaaaaaaa-0000-4000-8000-000000000003"
)

// fakeUmami stands in for a PostgreSQL snapshot. Its per-day aggregates are computed by separate
// code from the importer's own conversion, so agreement between the two means something.
type fakeUmami struct {
	websites   []UmamiWebsite
	events     map[string][]UmamiEvent
	tamperDays func(website string, days []UmamiDay) []UmamiDay
	hide       func(UmamiEvent) bool // an event counted by the aggregates but lost while streaming
}

func inWindow(e UmamiEvent, w ImportWindow) bool {
	return (w.Since.IsZero() || !e.CreatedAt.Before(w.Since)) && (w.Until.IsZero() || e.CreatedAt.Before(w.Until))
}

func (f *fakeUmami) Websites(context.Context) ([]UmamiWebsite, error) { return f.websites, nil }

func (f *fakeUmami) Events(_ context.Context, id string, w ImportWindow, fn func(UmamiEvent) error) error {
	list := append([]UmamiEvent(nil), f.events[id]...)
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
	for _, e := range list {
		if !inWindow(e, w) || (e.Type != 1 && e.Type != 2) || (f.hide != nil && f.hide(e)) {
			continue
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeUmami) EventTypeCounts(_ context.Context, id string, w ImportWindow) (map[int]int64, error) {
	out := map[int]int64{}
	for _, e := range f.events[id] {
		if inWindow(e, w) {
			out[e.Type]++
		}
	}
	return out, nil
}

func (f *fakeUmami) DayCounts(_ context.Context, id, timezone string, w ImportWindow) ([]UmamiDay, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	type bucket struct {
		pv, custom    int64
		users, visits map[string]bool
	}
	buckets := map[string]*bucket{}
	for _, e := range f.events[id] {
		if !inWindow(e, w) {
			continue
		}
		day := e.CreatedAt.In(loc).Format("2006-01-02")
		b := buckets[day]
		if b == nil {
			b = &bucket{users: map[string]bool{}, visits: map[string]bool{}}
			buckets[day] = b
		}
		switch e.Type {
		case 1:
			b.pv++
			b.users[e.SessionID] = true
			if e.VisitID != "" {
				b.visits[e.VisitID] = true
			} else {
				b.visits[e.SessionID] = true
			}
		case 2:
			b.custom++
		}
	}
	var out []UmamiDay
	for day, b := range buckets {
		out = append(out, UmamiDay{Day: day, Pageviews: b.pv, CustomRaw: b.custom, Visitors: int64(len(b.users)), Visits: int64(len(b.visits))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	if f.tamperDays != nil {
		out = f.tamperDays(id, out)
	}
	return out, nil
}

func umamiFixture() *fakeUmami {
	f := &fakeUmami{
		websites: []UmamiWebsite{{fixA, "Main", "example.com"}, {fixB, "NYC", "nyc.example.com"}, {fixC, "Elsewhere", "other.example.net"}},
		events:   map[string][]UmamiEvent{},
	}
	counter := 0
	add := func(site string, e UmamiEvent) {
		counter++
		e.ID, e.WebsiteID = fmt.Sprintf("ev-%04d", counter), site
		f.events[site] = append(f.events[site], e)
	}
	browsers := []string{"chrome", "firefox", "safari", "edge-chromium", "crios", "opera", "ios", "samsung"}
	systems := []string{"Windows 10", "Mac OS", "Android OS", "iOS", "Linux", "Chrome OS"}
	devices := []string{"desktop", "laptop", "mobile", "tablet", "desktop", "mobile"}
	referrers := []string{"", "www.google.com", "example.com", "news.ycombinator.com", "", "t.co"}
	base := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	for day := 0; day < 3; day++ {
		for i := 0; i < 12; i++ {
			session := fmt.Sprintf("session-%d", i%5)
			add(fixA, UmamiEvent{
				SessionID: session, VisitID: fmt.Sprintf("%s-d%d-v%d", session, day, i/6), Type: 1,
				CreatedAt: base.AddDate(0, 0, day).Add(time.Duration(i) * 41 * time.Minute),
				Hostname:  "www.example.com", Path: fmt.Sprintf("/page/%d", i%4), Title: "Page",
				ReferrerDomain: referrers[i%len(referrers)], ReferrerPath: "/r",
				Browser: browsers[i%len(browsers)], OS: systems[i%len(systems)], Device: devices[i%len(devices)],
				Screen: "1920x1080", Language: "en-US", Country: "us",
			})
		}
	}
	// Either side of local midnight in Asia/Tashkent (UTC+5).
	add(fixA, UmamiEvent{SessionID: "midnight", VisitID: "midnight-v1", Type: 1, CreatedAt: time.Date(2026, 10, 22, 18, 59, 59, 999999000, time.UTC), Path: "/late"})
	add(fixA, UmamiEvent{SessionID: "midnight", VisitID: "midnight-v2", Type: 1, CreatedAt: time.Date(2026, 10, 22, 19, 0, 0, 0, time.UTC), Path: "/early"})
	// Recorded before Umami tracked visits: each session counts as one visit.
	add(fixA, UmamiEvent{SessionID: "legacy-session", Type: 1, CreatedAt: base.Add(30 * time.Hour), Path: "/old"})
	three, trial, when := 3.0, true, time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	plan, flag := "pro", "true"
	add(fixA, UmamiEvent{SessionID: "session-1", VisitID: "session-1-d1-v0", Type: 2, Name: " signup ", CreatedAt: base.Add(27 * time.Hour), Path: "/join", Props: []UmamiProp{
		{Key: "plan", Type: 1, String: &plan}, {Key: "seats", Type: 2, Number: &three}, {Key: "trial", Type: 3, String: &flag}, {Key: "when", Type: 4, Date: &when},
	}})
	_ = trial
	long := strings.Repeat("x", 256)
	add(fixA, UmamiEvent{SessionID: "session-1", VisitID: "session-1-d1-v0", Type: 2, Name: "", CreatedAt: base.Add(28 * time.Hour)})
	add(fixA, UmamiEvent{SessionID: "session-1", VisitID: "session-1-d1-v0", Type: 2, Name: "PageView", CreatedAt: base.Add(28 * time.Hour)})
	add(fixA, UmamiEvent{SessionID: "session-1", VisitID: "session-1-d1-v0", Type: 2, Name: long, CreatedAt: base.Add(28 * time.Hour)})
	add(fixA, UmamiEvent{SessionID: "session-1", VisitID: "session-1-d1-v0", Type: 3, Name: "link", CreatedAt: base.Add(29 * time.Hour)})

	// New York leaves daylight saving time on 2026-11-01 at 06:00 UTC, so that local day has 25 hours.
	for i, at := range []time.Time{
		time.Date(2026, 11, 1, 3, 30, 0, 0, time.UTC), time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC), time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 7, 30, 0, 0, time.UTC), time.Date(2026, 11, 2, 4, 59, 59, 0, time.UTC), time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC),
	} {
		add(fixB, UmamiEvent{SessionID: fmt.Sprintf("ny-%d", i%3), VisitID: fmt.Sprintf("ny-visit-%d", i), Type: 1, CreatedAt: at, Path: "/ny", Browser: "chrome", OS: "Windows 10", Device: "desktop"})
	}
	add(fixB, UmamiEvent{SessionID: "ny-0", VisitID: "ny-visit-0", Type: 2, Name: "click", CreatedAt: time.Date(2026, 11, 1, 5, 45, 0, 0, time.UTC)})
	add(fixC, UmamiEvent{SessionID: "c-1", VisitID: "c-v1", Type: 1, CreatedAt: base, Path: "/"})
	add(fixC, UmamiEvent{SessionID: "c-1", VisitID: "c-v1", Type: 3, CreatedAt: base})
	return f
}

type totals struct{ pageviews, visitors, visits int64 }

// expectedTotals applies Plost's own definitions to the fixture without using the importer.
func expectedTotals(events []UmamiEvent, w ImportWindow) totals {
	users, visits := map[string]bool{}, map[string]bool{}
	var t totals
	for _, e := range events {
		if e.Type != 1 || !inWindow(e, w) {
			continue
		}
		t.pageviews++
		users[e.SessionID] = true
		if e.VisitID != "" {
			visits[e.VisitID] = true
		} else {
			visits[e.SessionID] = true
		}
	}
	t.visitors, t.visits = int64(len(users)), int64(len(visits))
	return t
}

func importDir(t *testing.T, sites map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	a, err := Open(Config{DataDir: dir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for domain, tz := range sites {
		if _, err := a.db.Exec("INSERT INTO sites(domain,name,timezone,public,created_at) VALUES(?,?,?,0,?)", domain, domain, tz, unixNow()); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func openImporter(t *testing.T, dir string, write bool) *Importer {
	t.Helper()
	imp, err := OpenImporter(dir, write)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(imp.Close)
	return imp
}

func scalar(t *testing.T, dir, query string, args ...any) int64 {
	t.Helper()
	imp, err := OpenImporter(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer imp.Close()
	var n int64
	if err := imp.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUmamiConversionRules(t *testing.T) {
	ev := func(mutate func(*UmamiEvent)) UmamiEvent {
		e := UmamiEvent{ID: "e1", WebsiteID: "w", SessionID: "s1", VisitID: "v1", Type: 1, CreatedAt: time.Date(2026, 10, 21, 1, 2, 3, 900000000, time.UTC), Path: "/a"}
		mutate(&e)
		return e
	}
	got, skip, _ := convertUmami(ev(func(e *UmamiEvent) {
		e.Path, e.Hostname, e.ReferrerDomain, e.ReferrerPath = "/a%20b/%C3%BC", "WWW.Example.com", "www.google.com", "/search"
		e.Browser, e.OS, e.Device, e.Screen, e.Country, e.Language, e.Title = "edge-chromium", "Mac OS", "laptop", "1920X1080", "cn", "zh-CN", "Hello"
	}), "example.com")
	want := Event{At: 1792544523, Name: "pageview", Hostname: "example.com", Path: "/a b/ü", Source: "google.com", Referrer: "https://www.google.com/search",
		UserID: "s1", SessionID: "v1", Country: "CN", Browser: "Edge", OS: "macOS", Device: "Desktop", Language: "zh-CN", Screen: "1920x1080", Title: "Hello", Props: "{}"}
	if skip != "" || got != want {
		t.Fatalf("conversion\n got %+v (%q)\nwant %+v", got, skip, want)
	}
	if got.At != time.Date(2026, 10, 21, 1, 2, 3, 0, time.UTC).Unix() {
		t.Fatal("sub-second time must be truncated, not rounded")
	}
	for name, tc := range map[string]struct {
		mutate func(*UmamiEvent)
		check  func(Event) bool
	}{
		"own domain is direct":     {func(e *UmamiEvent) { e.ReferrerDomain = "example.com" }, func(e Event) bool { return e.Source == "Direct" && e.Referrer == "https://example.com" }},
		"referring site wins":      {func(e *UmamiEvent) { e.ReferrerDomain, e.UTMSource = "google.com", "newsletter" }, func(e Event) bool { return e.Source == "google.com" && e.UTMSource == "newsletter" }},
		"utm only is a campaign":   {func(e *UmamiEvent) { e.ReferrerDomain, e.UTMSource = "", "newsletter" }, func(e Event) bool { return e.Source == "utm:newsletter" && e.UTMSource == "newsletter" }},
		"empty path":               {func(e *UmamiEvent) { e.Path = "" }, func(e Event) bool { return e.Path == "/" }},
		"hostname falls back":      {func(e *UmamiEvent) { e.Hostname = "" }, func(e Event) bool { return e.Hostname == "example.com" }},
		"invalid screen and place": {func(e *UmamiEvent) { e.Screen, e.Country = "wide", "XXX" }, func(e Event) bool { return e.Screen == "" && e.Country == "" }},
		"direct by default": {func(e *UmamiEvent) {}, func(e Event) bool {
			return e.Source == "Direct" && e.Referrer == "" && e.Device == "Desktop" && e.Browser == "Other" && e.OS == "Other"
		}},
	} {
		if e, skip, _ := convertUmami(ev(tc.mutate), "example.com"); skip != "" || !tc.check(e) {
			t.Errorf("%s: %+v (%q)", name, e, skip)
		}
	}
	if e, skip, _ := convertUmami(ev(func(e *UmamiEvent) { e.Type, e.Name = 2, " signup " }), "example.com"); skip != "" || e.Name != "signup" {
		t.Errorf("custom event name should be trimmed: %+v %q", e, skip)
	}
	for reason, mutate := range map[string]func(*UmamiEvent){
		"empty_event_name":    func(e *UmamiEvent) { e.Type, e.Name = 2, "  " },
		"reserved_event_name": func(e *UmamiEvent) { e.Type, e.Name = 2, "PageView" },
		"event_name_too_long": func(e *UmamiEvent) { e.Type, e.Name = 2, strings.Repeat("n", 256) },
		"event_type_3":        func(e *UmamiEvent) { e.Type = 3 },
		"event_type_5":        func(e *UmamiEvent) { e.Type = 5 },
	} {
		if _, skip, _ := convertUmami(ev(mutate), "example.com"); skip != reason {
			t.Errorf("want skip %s, got %q", reason, skip)
		}
	}
	for raw, want := range map[string]string{"chrome": "Chrome", "crios": "Chrome", "samsung": "Samsung Internet", "yandexbrowser": "Yandex Browser", "miui": "MIUI Browser", "firefox": "Firefox", "fxios": "Firefox", "safari": "Safari", "ios": "Safari",
		"ios-webview": "iOS WebView", "chromium-webview": "Chromium WebView", "edge-chromium": "Edge", "edge-ios": "Edge", "opera": "Opera", "facebook": "Facebook", "beaker": "Other", "": "Other"} {
		if got := umamiBrowser(raw); got != want {
			t.Errorf("browser %q: %s, want %s", raw, got, want)
		}
	}
	for raw, want := range map[string]string{"Windows 10": "Windows", "Mac OS": "macOS", "Android OS": "Android", "iOS": "iOS", "Linux": "Linux", "Chrome OS": "Chrome OS", "Ubuntu": "Linux", "BlackBerry OS": "Other", "": "Other"} {
		if got := umamiOS(raw); got != want {
			t.Errorf("os %q: %s, want %s", raw, got, want)
		}
	}
	for raw, want := range map[string]string{"mobile": "Mobile", "Tablet": "Tablet", "desktop": "Desktop", "laptop": "Desktop", "": "Desktop"} {
		if got := umamiDevice(raw); got != want {
			t.Errorf("device %q: %s, want %s", raw, got, want)
		}
	}
}

func TestUmamiProps(t *testing.T) {
	s, n, b, d := "text", 4.5, true, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nan, yes, no, array := math.NaN(), "true", "false", `["a","b"]`
	got, dropped := umamiProps([]UmamiProp{
		{Key: "s", Type: 1, String: &s}, {Key: "n", Type: 2, Number: &n}, {Key: "bool", Type: 3, Bool: &b}, {Key: "yes", Type: 3, String: &yes},
		{Key: "no", Type: 3, String: &no}, {Key: "d", Type: 4, Date: &d}, {Key: "arr", Type: 5, String: &array},
		{Key: "nan", Type: 2, Number: &nan}, {Key: "", Type: 1, String: &s}, {Key: strings.Repeat("k", 301), Type: 1, String: &s}, {Key: "empty", Type: 1},
	})
	want := `{"arr":"[\"a\",\"b\"]","bool":true,"d":"2026-01-02T03:04:05Z","n":4.5,"no":false,"s":"text","yes":true}`
	if got != want || dropped != 4 {
		t.Fatalf("props\n got %s (%d dropped)\nwant %s (4 dropped)", got, dropped, want)
	}
	var many []UmamiProp
	for i := 0; i < 40; i++ {
		many = append(many, UmamiProp{Key: fmt.Sprintf("k%02d", i), Type: 1, String: &s})
	}
	if got, dropped = umamiProps(many); dropped != 8 || strings.Count(got, `":"text"`) != 32 {
		t.Fatalf("only 32 properties fit: %d dropped, %s", dropped, got)
	}
	if long := strings.Repeat("é", 3000); true {
		got, _ = umamiProps([]UmamiProp{{Key: "l", Type: 1, String: &long}})
		if len(got) > 2100 {
			t.Fatal("long strings must be bounded")
		}
	}
}

func TestUmamiDryRunWritesNothing(t *testing.T) {
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	imp := openImporter(t, dir, false)
	rep, err := imp.Run(context.Background(), umamiFixture(), ImportOptions{Sites: map[string]string{fixA: "example.com", fixB: "nyc.example.com"}, CreateSites: true, Timezone: "America/New_York"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Apply || rep.Snapshot != "" || len(rep.Sites) != 2 || !rep.Sites[1].CreatedSite || rep.Sites[0].Pageviews == 0 || rep.Sites[0].Inserted != 0 {
		t.Fatalf("unexpected dry run report %+v", rep)
	}
	if n := scalar(t, dir, "SELECT count(*) FROM events_v2"); n != 0 {
		t.Fatalf("dry run wrote %d events", n)
	}
	if n := scalar(t, dir, "SELECT count(*) FROM sites"); n != 1 {
		t.Fatalf("dry run created sites: %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups")); err == nil {
		t.Fatal("dry run created a backup directory")
	}
}

func TestUmamiImportAppliesAndReconciles(t *testing.T) {
	src := umamiFixture()
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	opts := ImportOptions{Sites: map[string]string{fixA: "example.com", fixB: "nyc.example.com"}, CreateSites: true, Timezone: "America/New_York", Apply: true}
	imp := openImporter(t, dir, true)
	rep, err := imp.Run(context.Background(), src, opts)
	imp.Close()
	if err != nil {
		t.Fatalf("%v\n%+v", err, rep)
	}
	if len(rep.Sites) != 2 || len(rep.Unmapped) != 1 || rep.Unmapped[0].WebsiteID != fixC || rep.Unmapped[0].EventTypes["1"] != 1 {
		t.Fatalf("unexpected report %+v", rep)
	}
	if info, err := os.Stat(rep.Snapshot); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("the safety snapshot is missing or not private: %v", err)
	}
	a, b := rep.Sites[0], rep.Sites[1]
	if a.Skipped["empty_event_name"] != 1 || a.Skipped["reserved_event_name"] != 1 || a.Skipped["event_name_too_long"] != 1 || a.OtherTypes["3"] != 1 || a.WithoutVisitID != 1 {
		t.Fatalf("site A skipped %+v other %+v withoutVisit %d", a.Skipped, a.OtherTypes, a.WithoutVisitID)
	}
	if a.Custom != 1 || a.Pageviews != 39 || a.Inserted != 40 || a.AlreadyImported != 0 || b.Domain != "nyc.example.com" || b.Timezone != "America/New_York" || b.Pageviews != 6 || b.Custom != 1 {
		t.Fatalf("unexpected counts A %+v B %+v", a, b)
	}
	if a.FirstEvent != "2026-10-20T03:00:00Z" {
		t.Fatalf("first event %s", a.FirstEvent)
	}
	var days []string
	for _, d := range a.Days {
		days = append(days, d.Day)
	}
	// 18:59:59 UTC is still the 22nd in Tashkent; one second later is the 23rd.
	if !contains(days, "2026-10-22") || !contains(days, "2026-10-23") {
		t.Fatalf("days %v", days)
	}
	for _, d := range b.Days {
		if d.Day == "2026-11-01" && d.Pageviews != 4 {
			t.Fatalf("the 25-hour day should hold 4 pageviews: %+v", d)
		}
	}

	// The numbers the dashboard itself reports must match the source.
	app, err := Open(Config{DataDir: dir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	// The fixture's New York days are in early November, so the range must reach past any date the clock may show.
	all := Range{From: time.Unix(0, 0), To: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)}
	for domain, id := range map[string]string{"example.com": fixA, "nyc.example.com": fixB} {
		site, err := readSite(context.Background(), app.db, domain)
		if err != nil {
			t.Fatal(err)
		}
		o, err := app.overview(context.Background(), site.ID, all, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := expectedTotals(src.events[id], ImportWindow{})
		if o.Pageviews != want.pageviews || o.Visitors != want.visitors || o.Visits != want.visits {
			t.Fatalf("%s dashboard shows %d/%d/%d, Umami has %d/%d/%d (pageviews/visitors/visits)", domain, o.Pageviews, o.Visitors, o.Visits, want.pageviews, want.visitors, want.visits)
		}
	}
	site, _ := readSite(context.Background(), app.db, "example.com")
	browsers := []string{"Chrome", "Chromium WebView", "iOS WebView", "Safari", "Other"}
	for _, rule := range browserRules {
		browsers = append(browsers, rule.name)
	}
	for column, allowed := range map[string][]string{
		"browser": browsers,
		"os":      {"Android", "iOS", "Windows", "Chrome OS", "macOS", "Linux", "Other"},
		"device":  {"Desktop", "Mobile", "Tablet"},
	} {
		rows, err := app.db.Query("SELECT DISTINCT "+column+" FROM events_v2 WHERE site_id=?", site.ID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var v string
			rows.Scan(&v)
			if !contains(allowed, v) {
				t.Errorf("%s %q is outside Plost's vocabulary", column, v)
			}
		}
		rows.Close()
	}
	var props, name, sourceID, session, user string
	if err := app.db.QueryRow("SELECT name,props,source_id,session_id,user_id FROM events_v2 WHERE site_id=? AND name<>'pageview'", site.ID).Scan(&name, &props, &sourceID, &session, &user); err != nil {
		t.Fatal(err)
	}
	if name != "signup" || props != `{"plan":"pro","seats":3,"trial":true,"when":"2026-10-21T08:00:00Z"}` || !strings.HasPrefix(sourceID, "umami:"+fixA+":ev-") || session != "session-1-d1-v0" || user != "session-1" {
		t.Fatalf("custom event stored as %s %s %s %s %s", name, props, sourceID, session, user)
	}
	var legacy string
	app.db.QueryRow("SELECT session_id FROM events_v2 WHERE site_id=? AND user_id='legacy-session'", site.ID).Scan(&legacy)
	if legacy != "legacy-session" {
		t.Fatalf("an event without a visit id should count as one visit per session, got %q", legacy)
	}
	app.Close()

	// Running it again changes nothing.
	before := scalar(t, dir, "SELECT count(*) FROM events_v2")
	imp = openImporter(t, dir, true)
	again, err := imp.Run(context.Background(), src, opts)
	imp.Close()
	if err != nil {
		t.Fatal(err)
	}
	if again.Sites[0].Inserted != 0 || again.Sites[0].AlreadyImported != 40 || again.Sites[1].Inserted != 0 || scalar(t, dir, "SELECT count(*) FROM events_v2") != before {
		t.Fatalf("a repeated import must be a no-op: %+v", again.Sites)
	}
	if again.Snapshot != "" {
		t.Fatal("a run that changed nothing should not keep a snapshot")
	}
	if kept, _ := filepath.Glob(filepath.Join(dir, "backups", "before-import-*.db")); len(kept) != 1 {
		t.Fatalf("only the first run's snapshot should remain, found %d", len(kept))
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func TestUmamiImportHonoursTheWindow(t *testing.T) {
	src := umamiFixture()
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	w := ImportWindow{Since: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)}
	imp := openImporter(t, dir, true)
	rep, err := imp.Run(context.Background(), src, ImportOptions{Sites: map[string]string{fixA: "example.com"}, Window: w, Apply: true})
	if err != nil {
		t.Fatalf("%v %+v", err, rep.Sites)
	}
	want := expectedTotals(src.events[fixA], w)
	if rep.Sites[0].Pageviews != want.pageviews || want.pageviews == 0 || want.pageviews == expectedTotals(src.events[fixA], ImportWindow{}).pageviews {
		t.Fatalf("window imported %d pageviews, want %d", rep.Sites[0].Pageviews, want.pageviews)
	}
	if n := scalar(t, dir, "SELECT count(*) FROM events_v2 WHERE timestamp<? OR timestamp>=?", w.Since.Unix(), w.Until.Unix()); n != 0 {
		t.Fatalf("%d events outside the window were written", n)
	}
	// Widening the window later adds only what is new and still reconciles day by day.
	imp.Close()
	imp = openImporter(t, dir, true)
	wide, err := imp.Run(context.Background(), src, ImportOptions{Sites: map[string]string{fixA: "example.com"}, Apply: true})
	if err != nil {
		t.Fatalf("%v %+v", err, wide.Sites)
	}
	if wide.Sites[0].AlreadyImported != rep.Sites[0].Inserted || wide.Sites[0].Inserted == 0 {
		t.Fatalf("widening the window: %+v after %+v", wide.Sites[0], rep.Sites[0])
	}
}

func TestUmamiImportRollsBackWhenNumbersDisagree(t *testing.T) {
	for name, src := range map[string]*fakeUmami{
		"aggregates disagree": func() *fakeUmami {
			f := umamiFixture()
			f.tamperDays = func(id string, days []UmamiDay) []UmamiDay {
				if id == fixB {
					days[len(days)-1].Visitors++
				}
				return days
			}
			return f
		}(),
		"an event is lost while reading": func() *fakeUmami {
			f := umamiFixture()
			f.hide = func(e UmamiEvent) bool { return e.WebsiteID == fixB && e.SessionID == "ny-1" }
			return f
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
			imp := openImporter(t, dir, true)
			rep, err := imp.Run(context.Background(), src, ImportOptions{Sites: map[string]string{fixA: "example.com", fixB: "nyc.example.com"}, CreateSites: true, Apply: true})
			if !errors.Is(err, ErrReconcile) {
				t.Fatalf("want a reconcile failure, got %v", err)
			}
			var problems int
			for _, s := range rep.Sites {
				problems += len(s.Problems)
			}
			if problems == 0 {
				t.Fatal("the report must say what did not match")
			}
			imp.Close()
			if n := scalar(t, dir, "SELECT count(*) FROM events_v2"); n != 0 {
				t.Fatalf("%d events survived a failed import", n)
			}
			if n := scalar(t, dir, "SELECT count(*) FROM sites"); n != 1 {
				t.Fatalf("a site created by a failed import was kept (%d sites)", n)
			}
		})
	}
}

func TestUmamiImportRefusesToOverlapLiveData(t *testing.T) {
	src := umamiFixture()
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	live := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC).Unix()
	imp := openImporter(t, dir, true)
	if _, err := imp.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id) VALUES(1,?,'pageview','example.com','/','live','live-visit')", live); err != nil {
		t.Fatal(err)
	}
	sites := map[string]string{fixA: "example.com"}
	if _, err := imp.Run(context.Background(), src, ImportOptions{Sites: sites, Apply: true}); err == nil || !strings.Contains(err.Error(), "重叠") {
		t.Fatalf("an import overlapping live data must be refused, got %v", err)
	}
	if _, err := imp.Run(context.Background(), src, ImportOptions{Sites: sites, Window: ImportWindow{Until: time.Unix(live, 0)}, Apply: true}); err != nil {
		t.Fatalf("stopping before the first live event is the intended way: %v", err)
	}
	if _, err := imp.Run(context.Background(), src, ImportOptions{Sites: sites, AllowOverlap: true, Apply: true}); err != nil {
		t.Fatalf("an explicit override should work: %v", err)
	}
}

func TestUmamiImportMappingRules(t *testing.T) {
	src := umamiFixture()
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	imp := openImporter(t, dir, true)
	run := func(opts ImportOptions) (ImportReport, error) {
		opts.Apply = true
		return imp.Run(context.Background(), src, opts)
	}
	for name, tc := range map[string]struct {
		opts ImportOptions
		want string
	}{
		"nothing selected":        {ImportOptions{}, "没有指定"},
		"unknown umami site":      {ImportOptions{Sites: map[string]string{"nope": "example.com"}}, "没有站点"},
		"two sites one target":    {ImportOptions{Sites: map[string]string{fixA: "example.com", fixB: "www.example.com"}}, "同一个域名"},
		"bad target domain":       {ImportOptions{Sites: map[string]string{fixA: "not a domain"}}, "域名无效"},
		"target site is missing":  {ImportOptions{Sites: map[string]string{fixB: "nyc.example.com"}}, "还没有站点"},
		"bad timezone for new":    {ImportOptions{Sites: map[string]string{fixB: "nyc.example.com"}, CreateSites: true, Timezone: "Not/AZone"}, "时区"},
		"window is backwards":     {ImportOptions{Sites: map[string]string{fixA: "example.com"}, Window: ImportWindow{Since: time.Unix(200, 0), Until: time.Unix(100, 0)}}, "早于"},
		"strict refuses skipping": {ImportOptions{Sites: map[string]string{fixA: "example.com"}, Strict: true}, "对账失败"},
	} {
		if _, err := run(tc.opts); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}
	imp.Close()
	if n := scalar(t, dir, "SELECT count(*) FROM events_v2"); n != 0 {
		t.Fatalf("rejected imports wrote %d events", n)
	}
	if n := scalar(t, dir, "SELECT count(*) FROM sites"); n != 1 {
		t.Fatalf("rejected imports created sites: %d", n)
	}
}

func TestUmamiImportNeedsAStoppedInstance(t *testing.T) {
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	app, err := Open(Config{DataDir: dir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if imp, err := OpenImporter(dir, true); err == nil {
		imp.Close()
		t.Fatal("writing while Plost runs must be refused")
	}
	// Looking is harmless, so a preview works against a running instance.
	imp := openImporter(t, dir, false)
	rep, err := imp.Run(context.Background(), umamiFixture(), ImportOptions{Sites: map[string]string{fixA: "example.com"}})
	if err != nil || rep.Sites[0].Pageviews != 39 {
		t.Fatalf("preview against a running instance: %v %+v", err, rep.Sites)
	}
	if _, err := imp.Run(context.Background(), umamiFixture(), ImportOptions{Sites: map[string]string{fixA: "example.com"}, Apply: true}); err == nil {
		t.Fatal("a read-only importer must not apply")
	}
}

func TestUmamiRollbackRemovesOnlyImportedEvents(t *testing.T) {
	src := umamiFixture()
	dir := importDir(t, map[string]string{"example.com": "Asia/Tashkent"})
	imp := openImporter(t, dir, true)
	opts := ImportOptions{Sites: map[string]string{fixA: "example.com", fixB: "nyc.example.com"}, CreateSites: true, Apply: true}
	if _, err := imp.Run(context.Background(), src, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := imp.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id) VALUES(1,?,'pageview','example.com','/','live','live-visit')", time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC).Unix()); err != nil {
		t.Fatal(err)
	}
	preview, err := imp.RollbackUmami(context.Background(), "example.com", false)
	if err != nil || preview.Imported != 40 || preview.Removed != 0 || preview.Snapshot != "" {
		t.Fatalf("a preview must remove nothing: %+v %v", preview, err)
	}
	done, err := imp.RollbackUmami(context.Background(), "example.com", true)
	if err != nil || done.Removed != 40 || done.Snapshot == "" {
		t.Fatalf("rollback: %+v %v", done, err)
	}
	if _, err := os.Stat(done.Snapshot); err != nil {
		t.Fatal("rollback must keep a snapshot first")
	}
	imp.Close()
	if n := scalar(t, dir, "SELECT count(*) FROM events_v2 WHERE site_id=1"); n != 1 {
		t.Fatalf("live events must survive a rollback, %d remain", n)
	}
	if n := scalar(t, dir, "SELECT count(*) FROM events_v2 WHERE site_id=2"); n != 7 {
		t.Fatalf("another site's imported events must be untouched, %d remain", n)
	}
	if _, err := openImporter(t, dir, true).RollbackUmami(context.Background(), "unknown.example.com", true); err == nil {
		t.Fatal("rolling back an unknown site should fail")
	}
}
