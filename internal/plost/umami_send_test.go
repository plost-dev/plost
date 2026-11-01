package plost

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// sendUmami posts a body the way Umami's tracker does: JSON to /api/send from the site's page.
func sendUmami(t *testing.T, x *testApp, body any, agent string) int {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", x.server.URL+"/api/send", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("User-Agent", agent)
	resp := mustDo(t, x.client, req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func storedEvents(t *testing.T, a *App) []map[string]string {
	t.Helper()
	rows, err := a.db.Query("SELECT name,pathname,referrer_source,title,language,screen,props,via,user_id FROM events_v2 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var v [9]string
		if err := rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &v[7], &v[8]); err != nil {
			t.Fatal(err)
		}
		out = append(out, map[string]string{"name": v[0], "path": v[1], "source": v[2], "title": v[3], "language": v[4], "screen": v[5], "props": v[6], "via": v[7], "user": v[8]})
	}
	return out
}

func TestUmamiShapedRequestsAreStored(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	// What Plost's tracker now sends: the site's domain as "website", the whole address, plus its own fields.
	ours := map[string]any{"type": "event", "payload": map[string]any{"website": "example.com", "hostname": "example.com", "url": "https://example.com/pricing?utm_source=news",
		"referrer": "https://search.example.org/", "title": "Pricing", "language": "zh-CN", "screen": "1920x1080", "v": "browser-aaaaaaaa", "i": "evt-umami-0001", "w": 1440}}
	if status := sendUmami(t, x, ours, testChrome); status != 202 {
		t.Fatalf("got %d", status)
	}
	// What Umami's own tracker sends: a website id that is not a domain, and a path instead of an address.
	theirs := map[string]any{"type": "event", "payload": map[string]any{"website": "b1d117e6-f63d-4dec-be28-255ec6dc50c1", "hostname": "www.example.com", "url": "/docs",
		"title": "Docs", "language": "en-US", "screen": "390x844", "name": "signup", "data": map[string]any{"plan": "pro"}}}
	if status := sendUmami(t, x, theirs, testChrome); status != 202 {
		t.Fatalf("got %d", status)
	}
	// Identify calls are answered and ignored; a repeated event id is stored once.
	sendUmami(t, x, map[string]any{"type": "identify", "payload": map[string]any{"website": "example.com", "data": map[string]any{"user": "1"}}}, testChrome)
	sendUmami(t, x, ours, testChrome)
	got := storedEvents(t, x.app)
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %v", got)
	}
	if e := got[0]; e["name"] != "pageview" || e["path"] != "/pricing" || e["source"] != "news" || e["title"] != "Pricing" || e["language"] != "zh-CN" || e["screen"] != "1920x1080" || e["via"] != "send" || e["user"] == "" {
		t.Errorf("first event: %v", e)
	}
	if e := got[1]; e["name"] != "signup" || e["path"] != "/docs" || e["props"] != `{"plan":"pro"}` || e["screen"] != "390x844" || e["via"] != "send" {
		t.Errorf("second event: %v", e)
	}
}

func TestUmamiShapedRequestsGetTheirPreflightAndNeedNoAdminHeader(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	req, _ := http.NewRequest("OPTIONS", x.server.URL+"/api/send", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	resp := mustDo(t, x.client, req)
	resp.Body.Close()
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "https://example.com" || resp.Header.Get("Access-Control-Allow-Headers") != "Content-Type" || resp.Header.Get("Access-Control-Max-Age") == "" {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	for name, body := range map[string]string{"not JSON": "nope", "no payload": `{"type":"event"}`} {
		req, _ := http.NewRequest("POST", x.server.URL+"/api/send", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://example.com")
		resp := mustDo(t, x.client, req)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: got %d", name, resp.StatusCode)
		}
	}
	bot := map[string]any{"type": "event", "payload": map[string]any{"website": "example.com", "hostname": "example.com", "url": "/"}}
	if status := sendUmami(t, x, bot, "Googlebot/2.1"); status != 202 || len(storedEvents(t, x.app)) != 0 {
		t.Fatalf("a bot is answered and not stored: %d", status)
	}
}
