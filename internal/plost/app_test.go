package plost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPassword = "test-only-password-12345"
const testBackupPassword = "test-only-backup-password-12345"

type testApp struct {
	app    *App
	server *httptest.Server
	client *http.Client
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(a.Handler())
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	t.Cleanup(func() { s.Close(); a.Close() })
	return &testApp{a, s, client}
}
func (x *testApp) call(t *testing.T, method, path string, data any, expected int) []byte {
	t.Helper()
	var body io.Reader
	if data != nil {
		b, _ := json.Marshal(data)
		body = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, x.server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Plost-Request", "1")
	r.Header.Set("Origin", x.server.URL)
	resp, err := x.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expected {
		t.Fatalf("%s %s: got status %d, want %d", method, path, resp.StatusCode, expected)
	}
	return b
}
func (x *testApp) setup(t *testing.T) {
	t.Helper()
	x.call(t, "POST", "/api/setup", map[string]any{"email": "owner@example.test", "name": "Owner", "password": testPassword}, 200)
}
func (x *testApp) addSite(t *testing.T, domain string) {
	t.Helper()
	x.call(t, "POST", "/api/sites", map[string]any{"domain": domain, "name": domain, "timezone": "Asia/Tashkent"}, 200)
}
func (x *testApp) event(t *testing.T, domain, visitor, page string) {
	t.Helper()
	payload := map[string]any{"d": domain, "u": "https://" + domain + page, "v": visitor, "n": "pageview", "r": "https://google.com/search?q=private", "w": 1440, "s": "1920x1080", "l": "zh-CN"}
	b, _ := json.Marshal(payload)
	r, _ := http.NewRequest("POST", x.server.URL+"/api/event", bytes.NewReader(b))
	r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36")
	r.Header.Set("Origin", "https://"+domain)
	resp, err := x.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Errorf("collection returned %d", resp.StatusCode)
	}
}

func TestSetupSingletonAndClosedRegistration(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.call(t, "POST", "/api/setup", map[string]any{"email": "other@example.test", "name": "Other", "password": testPassword}, 409)
	x.call(t, "POST", "/api/register", map[string]any{}, 404)
	var email string
	if err := x.app.db.QueryRow("SELECT email FROM users").Scan(&email); err != nil || email != "owner@example.test" {
		t.Fatal("setup replaced the existing owner")
	}
}
func TestConcurrentSetupCreatesOneOwner(t *testing.T) {
	x := newTestApp(t)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			body, _ := json.Marshal(map[string]any{"email": "owner@example.test", "name": "Owner", "password": testPassword})
			req, _ := http.NewRequest("POST", x.server.URL+"/api/setup", bytes.NewReader(body))
			req.Header.Set("X-Plost-Request", "1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				codes <- 0
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			codes <- resp.StatusCode
		})
	}
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		if code == 200 {
			success++
		}
		if code == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent setup did not preserve singleton ownership")
	}
}
func TestCSRFAndManagementAuthorization(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	for _, origin := range []string{"https://evil.example", "null"} {
		req, _ := http.NewRequest("POST", x.server.URL+"/api/sites", strings.NewReader(`{}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Plost-Request", "1")
		resp, err := x.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("cross-origin mutation was accepted")
		}
	}
	req, _ := http.NewRequest("POST", x.server.URL+"/api/sites", strings.NewReader(`{}`))
	resp := mustDo(t, x.client, req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("missing CSRF header was accepted")
	}
	x.call(t, "POST", "/api/logout", nil, 200)
	x.call(t, "GET", "/api/sites", nil, 401)
	x.call(t, "POST", "/api/sites", map[string]any{"domain": "example.com"}, 401)
	x.call(t, "POST", "/api/backup", map[string]any{}, 401)
}

func TestCollectionMetricsDimensionsAndSharing(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.addSite(t, "other.com")
	x.event(t, "example.com", "visitor-first-123", "/one?token=private")
	x.event(t, "example.com", "visitor-first-123", "/two")
	x.event(t, "example.com", "visitor-second-456", "/one")
	var report Report
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report", nil, 200), &report); err != nil {
		t.Fatal(err)
	}
	if report.Overview.Pageviews != 3 || report.Overview.Visitors != 2 || report.Overview.Visits != 2 || report.Overview.Online != 2 {
		t.Fatalf("unexpected metrics: %+v", report.Overview)
	}
	if len(report.Screens) != 1 || report.Screens[0].Name != "1920x1080" || len(report.Browsers) != 1 || report.Browsers[0].Name != "Chrome" || report.OS[0].Name != "macOS" {
		t.Fatal("device dimensions missing")
	}
	if report.Sources[0].Name != "google.com" || len(report.Pages) != 2 {
		t.Fatal("source/page grouping incorrect")
	}
	var ref string
	if err := x.app.db.QueryRow("SELECT referrer FROM events_v2 LIMIT 1").Scan(&ref); err != nil || strings.Contains(ref, "private") {
		t.Fatal("referrer query was retained")
	}
	var filtered Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?page=%2Ftwo", nil, 200), &filtered)
	if filtered.Overview.Pageviews != 1 || filtered.Overview.Visitors != 1 {
		t.Fatal("page filter incorrect")
	}
	var share struct {
		Token string `json:"token"`
	}
	json.Unmarshal(x.call(t, "POST", "/api/sites/example.com/shares", map[string]string{"name": "Read only"}, 200), &share)
	x.call(t, "POST", "/api/logout", nil, 200)
	x.call(t, "GET", "/api/sites/example.com/report", nil, 401)
	var shared Report
	json.Unmarshal(x.call(t, "GET", "/api/share/"+share.Token, nil, 200), &shared)
	if !shared.Readonly || shared.Site.Domain != "example.com" {
		t.Fatal("share scope incorrect")
	}
	x.call(t, "GET", "/api/sites/example.com/settings?token="+share.Token, nil, 401)
	x.call(t, "GET", "/api/sites/example.com/activity?token="+share.Token, nil, 401)
	req, _ := http.NewRequest("GET", x.server.URL+"/api/v1/stats?domain=other.com", nil)
	req.Header.Set("Authorization", "Bearer "+share.Token)
	resp := mustDo(t, x.client, req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("read API did not enforce site scope")
	}
	req, _ = http.NewRequest("GET", x.server.URL+"/api/v1/stats?period=summary", nil)
	req.Header.Set("Authorization", "Bearer "+share.Token)
	resp = mustDo(t, x.client, req)
	defer resp.Body.Close()
	var stats struct {
		APIVersion string `json:"apiVersion"`
		Data       struct {
			Today struct {
				Pageviews int64 `json:"pageviews"`
			} `json:"today"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&stats)
	if resp.StatusCode != 200 || stats.APIVersion != "1" || stats.Data.Today.Pageviews != 3 {
		t.Fatal("legacy summary API contract incorrect")
	}
}

func TestBotAndMismatchedOriginDoNotCount(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	for _, input := range []struct{ ua, origin, page string }{{"Googlebot", "https://example.com", "https://example.com/"}, {"Chrome/130", "https://evil.example", "https://example.com/"}, {"Chrome/130", "https://example.com", "https://other.com/"}} {
		body, _ := json.Marshal(map[string]any{"d": "example.com", "u": input.page})
		req, _ := http.NewRequest("POST", x.server.URL+"/api/event", bytes.NewReader(body))
		req.Header.Set("User-Agent", input.ua)
		req.Header.Set("Origin", input.origin)
		resp := mustDo(t, x.client, req)
		resp.Body.Close()
	}
	var count int
	_ = x.app.db.QueryRow("SELECT count(*) FROM events_v2").Scan(&count)
	if count != 0 {
		t.Fatal("bot or invalid origin counted")
	}
}

func TestConcurrentCollectionUsesOneSession(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Go(func() { x.event(t, "example.com", "same-visitor-123", "/burst") })
	}
	wg.Wait()
	var count, visits int
	if err := x.app.db.QueryRow("SELECT count(*),count(DISTINCT session_id) FROM events_v2").Scan(&count, &visits); err != nil {
		t.Fatal(err)
	}
	if count != 40 || visits != 1 {
		t.Fatalf("burst count=%d visits=%d", count, visits)
	}
}

func TestBackupAuthenticationAndTruncation(t *testing.T) {
	payload := bytes.Repeat([]byte("private-test-data"), 10000)
	var encrypted bytes.Buffer
	if err := encryptBackup(&encrypted, bytes.NewReader(payload), testBackupPassword); err != nil {
		t.Fatal(err)
	}
	original := append([]byte{}, encrypted.Bytes()...)
	var restored bytes.Buffer
	if err := decryptBackup(&restored, bytes.NewReader(original), testBackupPassword); err != nil || !bytes.Equal(restored.Bytes(), payload) {
		t.Fatal("backup roundtrip failed")
	}
	tampered := append([]byte{}, original...)
	tampered[40] ^= 1
	for _, tc := range []struct {
		name     string
		b        []byte
		password string
	}{{"wrong password", original, "incorrect-password"}, {"tamper", tampered, testBackupPassword}, {"truncation", original[:len(original)-20], testBackupPassword}, {"append", append(append([]byte{}, original...), 0), testBackupPassword}, {"legacy", []byte("LITESTATS BACKUP FORMAT OLD"), testBackupPassword}} {
		t.Run(tc.name, func(t *testing.T) {
			if decryptBackup(io.Discard, bytes.NewReader(tc.b), tc.password) == nil {
				t.Fatal("invalid backup accepted")
			}
		})
	}
}

func (x *testApp) upload(t *testing.T, route string, backup []byte, confirm string, expected int) []byte {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	f, _ := mw.CreateFormFile("file", "test.plostbk")
	f.Write(backup)
	mw.WriteField("password", testBackupPassword)
	mw.WriteField("currentPassword", testPassword)
	mw.WriteField("confirmation", confirm)
	mw.Close()
	req, _ := http.NewRequest("POST", x.server.URL+route, &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Plost-Request", "1")
	req.Header.Set("Origin", x.server.URL)
	resp, err := x.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expected {
		t.Fatalf("backup upload: status %d, want %d", resp.StatusCode, expected)
	}
	return out
}

func TestFullBackupRestorePreservesDataAndRevokesSessions(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.event(t, "example.com", "persist-visitor-123", "/saved")
	backup := x.call(t, "POST", "/api/backup", map[string]string{"password": testBackupPassword, "currentPassword": testPassword}, 200)
	if !bytes.HasPrefix(backup, []byte(backupMagic)) {
		t.Fatal("missing backup format identifier")
	}
	x.addSite(t, "after-backup.com")
	x.upload(t, "/api/backup/validate", backup, "", 200)
	x.upload(t, "/api/backup/restore", backup, "", 400)
	x.upload(t, "/api/backup/restore", backup, "RESTORE", 200)
	x.call(t, "GET", "/api/sites", nil, 401)
	x.call(t, "POST", "/api/login", map[string]string{"email": "owner@example.test", "password": testPassword}, 200)
	var sites []map[string]any
	json.Unmarshal(x.call(t, "GET", "/api/sites", nil, 200), &sites)
	if len(sites) != 1 {
		t.Fatal("restore did not restore website snapshot")
	}
	var report Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report", nil, 200), &report)
	if report.Overview.Pageviews != 1 {
		t.Fatal("restore lost events")
	}
	files, _ := filepath.Glob(filepath.Join(x.app.cfg.DataDir, "backups", "before-restore-*.db"))
	if len(files) != 1 {
		t.Fatal("pre-restore snapshot was not retained")
	}
	if err := validateDatabase(context.Background(), files[0]); err != nil {
		t.Fatal("safety backup is not valid")
	}
}

func TestDatabaseRestartAndPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	secret := hexKey(a.secret)
	a.Close()
	b, err := Open(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if secret != hexKey(b.secret) {
		t.Fatal("visitor identity key changed across restart")
	}
	info, _ := os.Stat(filepath.Join(dir, "plost.db"))
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("database permissions expose private data")
	}
}
func hexKey(b []byte) string { return fmt.Sprintf("%x", b) }

func TestSessionHostBinding(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	u, _ := url.Parse(x.server.URL)
	cookies := x.client.Jar.Cookies(u)
	req, _ := http.NewRequest("GET", x.server.URL+"/api/sites", nil)
	req.Host = "different.example"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("session accepted on another host")
	}
}

func TestTOTPStandardVectorAndReplay(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // RFC 6238 test key.
	if totpStep(secret, "287082", time.Unix(59, 0)) != 1 {
		t.Fatal("TOTP standard vector failed")
	}
	x := newTestApp(t)
	x.setup(t)
	if _, err := x.app.db.Exec("UPDATE users SET totp_secret=?,totp_last=0 WHERE id=1", secret); err != nil {
		t.Fatal(err)
	}
	// Recovery codes are atomically consumed, including when a TOTP attempt is invalid.
	_, _ = x.app.db.Exec("INSERT INTO recovery_codes(hash) VALUES(?)", digest("one-use-code"))
	tx, _ := x.app.db.Begin()
	if !consumeCode(tx, "one-use-code") {
		t.Fatal("valid recovery code rejected")
	}
	tx.Commit()
	tx, _ = x.app.db.Begin()
	if consumeCode(tx, "one-use-code") {
		t.Fatal("recovery code replay accepted")
	}
	tx.Rollback()
}

func TestTimezoneAndDSTBoundaries(t *testing.T) {
	rng, err := parseRange(url.Values{"period": {"yesterday"}}, "Asia/Kathmandu")
	if err != nil {
		t.Fatal(err)
	}
	if rng.From.Hour() != 0 || rng.To.Hour() != 0 {
		t.Fatal("calendar range not aligned with website timezone")
	}
	loc, _ := time.LoadLocation("America/New_York")
	from := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	if to.Sub(from) != 23*time.Hour {
		t.Fatal("timezone test fixture incorrect")
	}
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var id int64
	_ = x.app.db.QueryRow("SELECT id FROM sites LIMIT 1").Scan(&id)
	points, err := x.app.series(context.Background(), id, Range{From: from, To: to, Period: "yesterday"}, nil)
	if err != nil || len(points) != 23 {
		t.Fatal("DST day buckets incorrect")
	}
}

func TestBinaryAssetsAndRouteFallback(t *testing.T) {
	x := newTestApp(t)
	for _, p := range []string{"/icons/fontawesome.css", "/fonts/sora-regular.ttf", "/favicon.svg", "/script.js", "/js/script.js", "/tracker.js", "/api/v1/openapi.json"} {
		req, _ := http.NewRequest("GET", x.server.URL+p, nil)
		resp, err := x.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("embedded asset unavailable: %s", p)
		}
	}
	x.call(t, "GET", "/missing-page", nil, 404)
}

func mustDo(t *testing.T, client *http.Client, req *http.Request) *http.Response {
	t.Helper()
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestOnlyOneProcessOwnsDataDirectory(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(Config{DataDir: dir})
	if err == nil {
		b.Close()
		t.Fatal("two processes can open the same data directory")
	}
}

func TestBundledRouteChunksAreEmbedded(t *testing.T) {
	count := 0
	err := filepath.WalkDir("web/assets", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") {
			count++
			if _, err := assets.ReadFile(filepath.ToSlash(path)); err != nil {
				t.Errorf("bundled frontend chunk is missing from executable: %s", filepath.Base(path))
			}
		}
		return nil
	})
	if err != nil || count == 0 {
		t.Fatal("no bundled frontend chunks found")
	}
}

func TestPublicReportAndRevokedShare(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.call(t, "PUT", "/api/sites/example.com/settings", map[string]any{"name": "Example", "timezone": "Etc/UTC", "public": true}, 200)
	var share struct {
		Token string `json:"token"`
	}
	json.Unmarshal(x.call(t, "POST", "/api/sites/example.com/shares", map[string]string{"name": "Temporary"}, 200), &share)
	x.call(t, "DELETE", "/api/sites/example.com/shares/1", nil, 200)
	x.call(t, "GET", "/api/share/"+share.Token, nil, 404)
	x.call(t, "POST", "/api/logout", nil, 200)
	var report Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report", nil, 200), &report)
	if !report.Readonly {
		t.Fatal("public report exposes management capabilities")
	}
	x.call(t, "PUT", "/api/sites/example.com/settings", map[string]any{}, 401)
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	u, _ := url.Parse(x.server.URL)
	oldCookies := x.client.Jar.Cookies(u)
	x.call(t, "PUT", "/api/account", map[string]string{"name": "Owner", "email": "owner@example.test", "locale": "zh-CN", "theme": "system", "currentPassword": testPassword, "password": "a-different-test-password-12345"}, 200)
	x.call(t, "GET", "/api/sites", nil, 200)
	req, _ := http.NewRequest("GET", x.server.URL+"/api/sites", nil)
	for _, c := range oldCookies {
		req.AddCookie(c)
	}
	resp := mustDo(t, http.DefaultClient, req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("password change did not revoke previous session")
	}
	x.call(t, "POST", "/api/logout", nil, 200)
	x.call(t, "POST", "/api/login", map[string]string{"email": "owner@example.test", "password": testPassword}, 401)
}

func TestOriginalUIReportsAndSiteRename(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.addSite(t, "other.com")
	x.event(t, "example.com", "ui-visitor-one", "/one")
	x.event(t, "example.com", "ui-visitor-one", "/two")
	x.event(t, "example.com", "ui-visitor-two", "/one")
	var report Report
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=7d&ui=1", nil, 200), &report); err != nil {
		t.Fatal(err)
	}
	if report.Overview.Visitors != 2 || report.Overview.Visits != 2 || report.Overview.Pageviews != 3 {
		t.Fatal("UI report metrics changed")
	}
	var visits, pv int64
	for _, p := range report.Series {
		visits += p.Visits
		pv += p.Pageviews
	}
	if visits != 2 || pv != 3 {
		t.Fatal("trend lost visits or pageviews")
	}
	b, _ := json.Marshal(report.Details)
	var details struct {
		PageVisitors []Row `json:"pageVisitors"`
		Entry        []Row `json:"entryPages"`
		Exit         []Row `json:"exitPages"`
		Heatmap      []struct {
			Dow, Hour int
			Value     int64
		} `json:"heatmap"`
	}
	if err := json.Unmarshal(b, &details); err != nil {
		t.Fatal(err)
	}
	if len(details.PageVisitors) != 2 || details.PageVisitors[0].Name != "/one" || details.PageVisitors[0].Value != 2 {
		t.Fatal("page visitor counts incorrect")
	}
	if len(details.Entry) != 1 || details.Entry[0].Value != 2 || len(details.Exit) != 2 {
		t.Fatal("entry/exit paths incorrect")
	}
	var ts int64
	x.app.db.QueryRow("SELECT min(timestamp) FROM events_v2").Scan(&ts)
	loc, _ := time.LoadLocation("Asia/Tashkent")
	at := time.Unix(ts, 0).In(loc)
	if len(details.Heatmap) != 1 || details.Heatmap[0].Hour != at.Hour() || details.Heatmap[0].Dow != (int(at.Weekday())+6)%7+1 || details.Heatmap[0].Value != 2 {
		t.Fatal("heatmap timezone/visitor grouping incorrect")
	}
	x.call(t, "GET", "/api/sites?period=invalid&ui=1", nil, 400)
	x.call(t, "PUT", "/api/sites/example.com/settings", map[string]any{"domain": "other.com", "timezone": "Asia/Tashkent"}, 409)
	x.call(t, "PUT", "/api/sites/example.com/settings", map[string]any{"domain": "renamed.com", "timezone": "Asia/Tashkent"}, 200)
	x.call(t, "GET", "/api/sites/example.com/report", nil, 404)
	var renamed Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/renamed.com/report?period=7d&hostname=example.com", nil, 200), &renamed)
	if renamed.Overview.Pageviews != 3 {
		t.Fatal("rename lost historical events or hostname filter failed")
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites/renamed.com/report?period=7d&hostname=other.com", nil, 200), &renamed)
	if renamed.Overview.Pageviews != 0 {
		t.Fatal("hostname filter ignored")
	}
}

func TestUISettingsAuthorizationAndShareVisibility(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.call(t, "PUT", "/api/mapbox", map[string]string{"token": "sk.private-must-be-rejected"}, 400)
	x.call(t, "PUT", "/api/mapbox", map[string]string{"token": "pk.test-public-only"}, 200)
	x.call(t, "PUT", "/api/mapbox", map[string]bool{"enabled": false}, 200)
	var mapSettings struct{ Enabled, Configured bool }
	json.Unmarshal(x.call(t, "GET", "/api/mapbox", nil, 200), &mapSettings)
	if mapSettings.Enabled || !mapSettings.Configured {
		t.Fatal("disabling the map discarded its configuration")
	}
	var mapReport Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=7d&ui=1", nil, 200), &mapReport)
	if mapReport.Details["mapboxToken"] != "" {
		t.Fatal("disabled Mapbox token still exposed to reports")
	}
	x.call(t, "PUT", "/api/mapbox", map[string]bool{"enabled": true}, 200)
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=7d&ui=1", nil, 200), &mapReport)
	if mapReport.Details["mapboxToken"] != "pk.test-public-only" {
		t.Fatal("enabling the map lost its saved public token")
	}
	x.call(t, "PUT", "/api/map-style", map[string]string{"styleId": "dark-v11"}, 200)
	x.call(t, "PUT", "/api/map-style", map[string]string{"styleId": "invalid"}, 400)
	x.call(t, "PUT", "/api/account", map[string]any{"name": "Owner", "email": "owner@example.test", "locale": "zh-CN", "theme": "system", "avatar": "data:image/svg+xml;base64,PHN2Zz4="}, 400)
	var share struct{ Token string }
	json.Unmarshal(x.call(t, "POST", "/api/sites/example.com/shares", map[string]string{"name": "Test"}, 200), &share)
	body := x.call(t, "GET", "/api/sites/example.com/settings", nil, 200)
	if share.Token == "" || !bytes.Contains(body, []byte(share.Token)) {
		t.Fatal("owner cannot reopen share URL")
	}
	x.call(t, "POST", "/api/logout", nil, 200)
	for _, path := range []string{"/api/mapbox", "/api/sites/example.com/visitors", "/api/sites/example.com/settings"} {
		x.call(t, "GET", path, nil, 401)
	}
	x.call(t, "PUT", "/api/map-style", map[string]string{"styleId": "standard"}, 401)
	shared := x.call(t, "GET", "/api/share/"+share.Token+"?period=7d&ui=1", nil, 200)
	if bytes.Contains(shared, []byte(share.Token)) {
		t.Fatal("share report disclosed share credentials")
	}
}
