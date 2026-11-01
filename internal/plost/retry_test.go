package plost

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// track sends one event the way the tracker would: method "POST" to path as fetch or Beacon does, or "GET" with the
// event in the query string as the image retry does. It returns the status and content type.
func track(t *testing.T, x *testApp, method, path string, event map[string]any, agent string) (int, string) {
	t.Helper()
	b, _ := json.Marshal(event)
	var req *http.Request
	if method == "GET" {
		req, _ = http.NewRequest("GET", x.server.URL+path+"?p="+url.QueryEscape(string(b)), nil)
	} else {
		req, _ = http.NewRequest("POST", x.server.URL+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
		req.Header.Set("Origin", "https://example.com")
	}
	req.Header.Set("User-Agent", agent)
	resp := mustDo(t, x.client, req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Content-Type")
}

func pageview(id, via string) map[string]any {
	e := map[string]any{"d": "example.com", "u": "https://example.com/pricing", "v": "browser-aaaaaaaa", "n": "pageview"}
	if id != "" {
		e["i"] = id
	}
	if via != "" {
		e["x"] = via
	}
	return e
}

func storedVia(t *testing.T, a *App) []string {
	t.Helper()
	rows, err := a.db.Query("SELECT pathname||' '||via FROM events_v2 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

func TestRetriesArriveAsImageOrBeaconAndSayHow(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")

	status, kind := track(t, x, "GET", "/e", pageview("evt-image-0001", "img"), testChrome)
	if status != 200 || kind != "image/gif" {
		t.Fatalf("an image retry must get the pixel back, got %d %q", status, kind)
	}
	// A Beacon carries no X-Plost-Request header; the second address must not be treated as an admin call.
	if status, _ := track(t, x, "POST", "/e", pageview("evt-beacon-001", "beacon"), testChrome); status != 202 {
		t.Fatalf("a Beacon retry must be accepted, got %d", status)
	}
	if status, _ := track(t, x, "POST", "/api/event", pageview("evt-normal-001", "carrier-pigeon"), testChrome); status != 202 {
		t.Fatalf("the normal request must still work, got %d", status)
	}
	got := storedVia(t, x.app)
	want := []string{"/pricing img", "/pricing beacon", "/pricing"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stored %q, want %q (an unknown transport counts as the normal way)", got, want)
	}
}

func TestARetriedEventIsStoredOnce(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	// The fetch did arrive although the browser reported a failure, so both retries follow with the same id.
	track(t, x, "POST", "/api/event", pageview("evt-twice-0001", ""), testChrome)
	track(t, x, "GET", "/e", pageview("evt-twice-0001", "img"), testChrome)
	track(t, x, "POST", "/e", pageview("evt-twice-0001", "beacon"), testChrome)
	// Events without an id (older trackers) and with a different id are each stored.
	track(t, x, "POST", "/api/event", pageview("", ""), testChrome)
	track(t, x, "POST", "/api/event", pageview("", ""), testChrome)
	track(t, x, "POST", "/api/event", pageview("evt-other-0001", ""), testChrome)
	if got := storedVia(t, x.app); len(got) != 4 || got[0] != "/pricing" {
		t.Fatalf("want the first copy and three other events, got %q", got)
	}
}

func TestAnImageFromABotIsAnsweredButNotStored(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	status, kind := track(t, x, "GET", "/e", pageview("evt-bot-00001", "img"), "Googlebot/2.1 (+http://www.google.com/bot.html)")
	if status != 200 || kind != "image/gif" {
		t.Fatalf("got %d %q", status, kind)
	}
	other := pageview("evt-elsewhere1", "img")
	other["d"], other["u"] = "elsewhere.test", "https://elsewhere.test/"
	track(t, x, "GET", "/e", other, testChrome)
	if got := storedVia(t, x.app); len(got) != 0 {
		t.Fatalf("nothing should be stored, got %q", got)
	}
	// A refused event leaves its id free, so a later valid copy is still stored.
	if status, _ := track(t, x, "POST", "/e", pageview("evt-bot-00001", "beacon"), testChrome); status != 202 {
		t.Fatalf("got %d", status)
	}
	if got := storedVia(t, x.app); len(got) != 1 {
		t.Fatalf("the valid copy must be stored, got %q", got)
	}
}

func TestRecentEventsForgetAfterAQuarterOfAnHour(t *testing.T) {
	r := newRecentEvents()
	if !r.claim("1:a", 1000) || r.claim("1:a", 1000+recentEventTTL-1) {
		t.Fatal("an id is claimed once within the window")
	}
	if !r.claim("1:a", 1000+recentEventTTL) {
		t.Fatal("after the window the id is free again")
	}
	r.release("1:a")
	if !r.claim("1:a", 1000+recentEventTTL) {
		t.Fatal("a released id is free again")
	}
}

func TestMigrationAddsTheTransportColumn(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(Config{DataDir: dir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("ALTER TABLE events_v2 DROP COLUMN via"); err != nil {
		t.Fatal(err)
	}
	a.Close()
	a, err = Open(Config{DataDir: dir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var n int
	if err := a.db.QueryRow("SELECT count(*) FROM pragma_table_info('events_v2') WHERE name='via'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("the column must be added back: %d %v", n, err)
	}
}
