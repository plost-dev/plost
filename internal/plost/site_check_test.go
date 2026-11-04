package plost

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// probeFakes replaces the network probes so the scheduler, claims and storage can be tested without a network.
type probeFakes struct {
	mu                     sync.Mutex
	httpCalls, expiryCalls int
	urls                   []string
	result                 func(u *url.URL) HTTPCheck
	started                chan struct{} // receives one value each time an HTTP probe begins
	release                chan struct{} // when set, an HTTP probe waits for it to close
}

func okCheck(*url.URL) HTTPCheck {
	status, ms := 200, 42
	return HTTPCheck{CheckedAt: isoTime(time.Now()), OK: true, Status: &status, DurationMs: &ms}
}

func fakeExpiry() ExpirySnapshot {
	return ExpirySnapshot{CheckedAt: isoTime(time.Now()),
		Domain:      DomainExpiry{Domain: "example.com", ExpiresAt: "2027-01-01T00:00:00.000Z", RegisteredAt: "2010-01-01T00:00:00.000Z", Registrar: "Example Registrar", Retryable: boolPtr(false)},
		Certificate: CertificateResult{ExpiresAt: "2026-12-01T00:00:00.000Z", Valid: true, Validation: "DV"}}
}

func (x *testApp) useProbes(f *probeFakes) {
	if f.result == nil {
		f.result = okCheck
	}
	x.app.siteProbe = siteChecker{
		http: func(ctx context.Context, u *url.URL) HTTPCheck {
			f.mu.Lock()
			f.httpCalls++
			f.urls = append(f.urls, u.String())
			f.mu.Unlock()
			if f.started != nil {
				f.started <- struct{}{}
			}
			if f.release != nil {
				<-f.release
			}
			return f.result(u)
		},
		expiry: func(ctx context.Context, host string) ExpirySnapshot {
			f.mu.Lock()
			f.expiryCalls++
			f.mu.Unlock()
			return fakeExpiry()
		},
	}
}

func (f *probeFakes) calls() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.httpCalls, f.expiryCalls
}

type checkResponse struct {
	Report    SiteCheckReport `json:"report"`
	CanManage bool            `json:"canManage"`
}

func (x *testApp) checks(t *testing.T, domain string) SiteCheckReport {
	t.Helper()
	var out checkResponse
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/"+domain+"/checks", nil, 200), &out); err != nil {
		t.Fatal(err)
	}
	if !out.CanManage {
		t.Fatal("the only user manages every site")
	}
	return out.Report
}

func (x *testApp) waitForChecks(t *testing.T, domain string, n int) SiteCheckReport {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if r := x.checks(t, domain); len(r.Checks) >= n {
			return r
		}
	}
	t.Fatalf("%s did not reach %d recorded checks", domain, n)
	return SiteCheckReport{}
}

func (x *testApp) siteID(t *testing.T, domain string) int64 {
	t.Helper()
	var id int64
	if err := x.app.db.QueryRow("SELECT id FROM sites WHERE domain=?", domain).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSiteCheckNeedsALogin(t *testing.T) {
	x := newTestApp(t)
	x.useProbes(&probeFakes{})
	for _, tc := range [][2]string{{"GET", "/api/sites/example.com/checks"}, {"PUT", "/api/sites/example.com/checks"}, {"POST", "/api/sites/example.com/checks/run"}} {
		x.call(t, tc[0], tc[1], map[string]any{"enabled": true}, 401)
	}
	x.setup(t)
	x.addSite(t, "example.com")
	// A shared link only ever shows the statistics report, never what the server learns about the site.
	var share struct{ Token string }
	json.Unmarshal(x.call(t, "POST", "/api/sites/example.com/shares", map[string]string{"name": "view"}, 200), &share)
	body := string(x.call(t, "GET", "/api/share/"+share.Token+"?period=24h", nil, 200))
	for _, leak := range []string{"certificate", "expiresAt", "registrar", "checks"} {
		if strings.Contains(body, leak) {
			t.Fatalf("the shared report mentions %q", leak)
		}
	}
}

func TestSiteCheckSettingsAndReport(t *testing.T) {
	x := newTestApp(t)
	x.useProbes(&probeFakes{})
	x.setup(t)
	x.addSite(t, "example.com")

	raw := string(x.call(t, "GET", "/api/sites/example.com/checks", nil, 200))
	if !strings.Contains(raw, `"checks":[]`) || !strings.Contains(raw, `"latest":null`) || !strings.Contains(raw, `"expiry":null`) {
		t.Fatalf("an unconfigured site has empty lists, not nulls where the page expects arrays: %s", raw)
	}
	// A new site starts with its checks on.
	if r := x.checks(t, "example.com"); !r.Enabled || r.Path != "/" || r.URL != "https://example.com/" {
		t.Fatalf("defaults: %+v", r)
	}
	x.call(t, "GET", "/api/sites/unknown.example.org/checks", nil, 404)
	x.call(t, "PUT", "/api/sites/unknown.example.org/checks", map[string]any{"enabled": true, "path": "/"}, 404)
	x.call(t, "POST", "/api/sites/unknown.example.org/checks/run", map[string]any{}, 404)

	for _, path := range []string{"health", "//evil.example/", "https://evil.example/", "/a#frag", "/" + strings.Repeat("a", 2048)} {
		x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": path}, 400)
	}
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/", "extra": 1}, 400)
	if r := x.checks(t, "example.com"); r.Path != "/" {
		t.Fatal("a rejected save must not change anything")
	}

	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": false, "path": "/health"}, 200)
	if r := x.checks(t, "example.com"); r.Enabled || r.Path != "/health" || r.URL != "https://example.com/health" {
		t.Fatalf("saved paused settings: %+v", r)
	}
	// Running a paused check is refused with the reason.
	x.call(t, "POST", "/api/sites/example.com/checks/run", map[string]any{}, 409)
}

func TestSiteCheckRunsAndRecords(t *testing.T) {
	fakes := &probeFakes{}
	x := newTestApp(t)
	x.useProbes(fakes)
	x.setup(t)
	x.addSite(t, "example.com")

	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/health"}, 200)
	r := x.waitForChecks(t, "example.com", 1)
	if r.Latest == nil || !r.Latest.OK || r.Latest.Status == nil || *r.Latest.Status != 200 || r.Latest.DurationMs == nil || !r.Enabled {
		t.Fatalf("saving an enabled check records a first result within seconds: %+v", r)
	}
	if r.Expiry == nil || r.Expiry.Certificate.ExpiresAt != "2026-12-01T00:00:00.000Z" || r.Expiry.Domain.Registrar != "Example Registrar" {
		t.Fatalf("the first run also reads the certificate and domain expiry: %+v", r.Expiry)
	}
	if http, expiry := fakes.calls(); http != 1 || expiry != 1 || fakes.urls[0] != "https://example.com/health" {
		t.Fatalf("probes: %d http, %d expiry, %v", http, expiry, fakes.urls)
	}

	// The site list shows the certificate expiry for the toolbar and cards.
	var sites []map[string]any
	json.Unmarshal(x.call(t, "GET", "/api/sites", nil, 200), &sites)
	if len(sites) != 1 || sites[0]["certificateExpiresAt"] != "2026-12-01T00:00:00.000Z" {
		t.Fatalf("site list: %v", sites)
	}

	// Manual checks are limited to one a minute; a new one is allowed once that minute has passed.
	x.call(t, "POST", "/api/sites/example.com/checks/run", map[string]any{}, 409)
	id := x.siteID(t, "example.com")
	reset := func(dueIn time.Duration) {
		if _, err := x.app.db.Exec("UPDATE site_check_settings SET last_started_at=0,next_check_at=? WHERE site_id=?", time.Now().Add(dueIn).Unix(), id); err != nil {
			t.Fatal(err)
		}
	}
	reset(time.Hour)
	x.call(t, "POST", "/api/sites/example.com/checks/run", map[string]any{}, 200)
	x.waitForChecks(t, "example.com", 2)
	if http, expiry := fakes.calls(); http != 2 || expiry != 2 {
		t.Fatalf("a manual check refreshes everything: %d http, %d expiry", http, expiry)
	}

	// The scheduler runs what is due and refreshes the expiry only when it is stale.
	reset(-time.Second)
	x.app.runDueSiteChecks()
	x.waitForChecks(t, "example.com", 3)
	if http, expiry := fakes.calls(); http != 3 || expiry != 2 {
		t.Fatalf("a scheduled check does not repeat a fresh expiry lookup: %d http, %d expiry", http, expiry)
	}
	reset(time.Hour)
	x.app.runDueSiteChecks()
	if http, _ := fakes.calls(); http != 3 {
		t.Fatal("nothing is due yet, so nothing runs")
	}
	// Pausing stops scheduled runs even if one was due.
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": false, "path": "/health"}, 200)
	reset(-time.Second)
	x.app.runDueSiteChecks()
	if http, _ := fakes.calls(); http != 3 {
		t.Fatal("a paused site must not be checked")
	}
	if r := x.checks(t, "example.com"); len(r.Checks) != 3 || r.Enabled {
		t.Fatalf("history stays after pausing: %+v", r)
	}
}

func TestSiteCheckReportsFailures(t *testing.T) {
	status := 503
	failing := &probeFakes{result: func(*url.URL) HTTPCheck {
		return HTTPCheck{CheckedAt: isoTime(time.Now()), Status: &status, DurationMs: func() *int { v := 80; return &v }(), Error: "HTTP 503"}
	}}
	x := newTestApp(t)
	x.useProbes(failing)
	x.setup(t)
	x.addSite(t, "example.com")
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/"}, 200)
	r := x.waitForChecks(t, "example.com", 1)
	if r.Latest.OK || r.Latest.Error != "HTTP 503" || *r.Latest.Status != 503 {
		t.Fatalf("an unhealthy answer: %+v", r.Latest)
	}

	// A check that never got an answer has no status or duration, so the chart leaves a gap there.
	failing.result = func(*url.URL) HTTPCheck { return HTTPCheck{CheckedAt: isoTime(time.Now()), Error: "检查超时"} }
	id := x.siteID(t, "example.com")
	x.app.db.Exec("UPDATE site_check_settings SET last_started_at=0 WHERE site_id=?", id)
	x.call(t, "POST", "/api/sites/example.com/checks/run", map[string]any{}, 200)
	raw := string(x.call(t, "GET", "/api/sites/example.com/checks", nil, 200))
	if !strings.Contains(raw, `"status":null,"durationMs":null,"error":"检查超时"`) {
		t.Fatalf("a timeout is reported without invented numbers: %s", raw)
	}
}

func TestSiteCheckResultIsDroppedWhenTheSiteChangesMidRun(t *testing.T) {
	fakes := &probeFakes{started: make(chan struct{}, 4), release: make(chan struct{})}
	x := newTestApp(t)
	x.useProbes(fakes)
	x.setup(t)
	x.addSite(t, "example.com")
	released := false
	release := func() {
		if !released {
			released = true
			close(fakes.release)
		}
	}
	defer release()

	waitStarted := func() {
		t.Helper()
		select {
		case <-fakes.started:
		case <-time.After(5 * time.Second):
			t.Fatal("the probe never started")
		}
	}
	settle := func(domain string) {
		t.Helper()
		time.Sleep(200 * time.Millisecond) // the discarded run has no result to wait for, so give it time to finish
		if r := x.checks(t, domain); len(r.Checks) != 0 || r.Latest != nil {
			t.Fatalf("a result for outdated settings was kept: %+v", r)
		}
	}

	// Pausing while the request is running discards its answer.
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/"}, 200)
	waitStarted()
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": false, "path": "/"}, 200)
	release()
	settle("example.com")

	// So does renaming the site: the old address's answer must not be shown for the new one.
	// Starts are limited to one a minute per site, so pretend that minute has passed.
	x.app.db.Exec("UPDATE site_check_settings SET last_started_at=0 WHERE site_id=?", x.siteID(t, "example.com"))
	fakes.release, released = make(chan struct{}), false
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/"}, 200)
	waitStarted()
	x.call(t, "PUT", "/api/sites/example.com/settings", map[string]any{"domain": "renamed.example.com", "name": "Renamed", "timezone": "Asia/Tashkent"}, 200)
	release()
	settle("renamed.example.com")
	if r := x.checks(t, "renamed.example.com"); r.Expiry != nil || r.URL != "https://renamed.example.com/" || !r.Enabled {
		t.Fatalf("after a rename the settings stay but nothing about the old address does: %+v", r)
	}
	var due, version int64
	x.app.db.QueryRow("SELECT next_check_at,version FROM site_check_settings WHERE site_id=?", x.siteID(t, "renamed.example.com")).Scan(&due, &version)
	if due > time.Now().Unix() || version < 3 {
		t.Fatalf("a rename schedules a fresh look straight away (due %d, version %d)", due, version)
	}
}

func TestSiteCheckHistoryWindowAndRetention(t *testing.T) {
	x := newTestApp(t)
	x.useProbes(&probeFakes{})
	x.setup(t)
	x.addSite(t, "example.com")
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": false, "path": "/"}, 200)
	id := x.siteID(t, "example.com")
	now := time.Now()
	seed := func(age time.Duration, target string) {
		if _, err := x.app.db.Exec("INSERT INTO site_check_results(site_id,target_url,checked_at,ok,status,duration_ms,error) VALUES(?,?,?,1,200,50,'')", id, target, now.Add(-age).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	seed(92*24*time.Hour, "https://example.com/")
	seed(30*time.Hour, "https://example.com/")
	seed(2*time.Hour, "https://example.com/")
	seed(time.Hour, "https://example.com/")
	seed(30*time.Minute, "https://example.com/old-path")

	r := x.checks(t, "example.com")
	if len(r.Checks) != 2 {
		t.Fatalf("only the last 24 hours of the current address are shown: %d checks", len(r.Checks))
	}
	first, _ := time.Parse(time.RFC3339Nano, r.Checks[0].CheckedAt)
	last, _ := time.Parse(time.RFC3339Nano, r.Checks[1].CheckedAt)
	if !first.Before(last) || r.Latest == nil || r.Latest.CheckedAt != r.Checks[1].CheckedAt {
		t.Fatal("history runs oldest to newest and the newest is the latest")
	}

	counted := 0
	for _, day := range r.Uptime.Days {
		counted += day.Total
	}
	if len(r.Uptime.Days) != 90 || counted != 3 || r.Uptime.Days[0].Percent != nil {
		t.Fatalf("uptime has 90 days with only the current address in the window counted: %d days, %d checks", len(r.Uptime.Days), counted)
	}

	// Recording a new result prunes what is older than the 90-day window, and nothing else.
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/"}, 200)
	x.waitForChecks(t, "example.com", 3)
	var old, kept int
	x.app.db.QueryRow("SELECT count(*) FROM site_check_results WHERE checked_at<?", now.Add(-91*24*time.Hour).Unix()).Scan(&old)
	x.app.db.QueryRow("SELECT count(*) FROM site_check_results").Scan(&kept)
	if old != 0 || kept != 5 {
		t.Fatalf("retention: %d older than 91 days, %d rows left (want 0 and 5)", old, kept)
	}
}

func TestSiteCheckDataGoesWithTheSite(t *testing.T) {
	x := newTestApp(t)
	x.useProbes(&probeFakes{})
	x.setup(t)
	x.addSite(t, "example.com")
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": true, "path": "/"}, 200)
	x.waitForChecks(t, "example.com", 1)
	x.call(t, "DELETE", "/api/sites/example.com", map[string]string{"confirmation": "example.com"}, 200)
	var settings, results int
	x.app.db.QueryRow("SELECT count(*) FROM site_check_settings").Scan(&settings)
	x.app.db.QueryRow("SELECT count(*) FROM site_check_results").Scan(&results)
	if settings != 0 || results != 0 {
		t.Fatalf("deleting a site left %d settings and %d results behind", settings, results)
	}
}

// The default probes refuse local targets before any connection, so a site cannot be pointed at the inside of a network.
func TestDefaultSiteProbesRefuseLocalTargets(t *testing.T) {
	probe := newSiteChecker(nil)
	for _, raw := range []string{"https://127.0.0.1/", "https://localhost/", "https://10.0.0.1/", "https://[::1]/", "https://db.internal/"} {
		u, _ := url.Parse(raw)
		if got := probe.http(context.Background(), u); got.OK || got.Error != "不允许检查内网或保留地址" || got.Status != nil {
			t.Errorf("%s: %+v", raw, got)
		}
	}
}
