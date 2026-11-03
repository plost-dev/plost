package plost

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func TestLivePointsPlaceVisitorsByCity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "city.mmdb")
	tokyo := mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String("JP")}, "location": mmdbtype.Map{"latitude": mmdbtype.Float64(35.68954), "longitude": mmdbtype.Float64(139.69171)}}
	writeGeoDB(t, path, map[string]any{"203.0.113.0/25": tokyo, "203.0.113.128/25": "US"}, time.Now())
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test", GeoIPPath: path})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	t.Cleanup(func() { server.Close(); a.Close() })
	jar, _ := cookiejar.New(nil)
	x := &testApp{a, server, &http.Client{Jar: jar, Timeout: 15 * time.Second}}
	x.setup(t)
	x.addSite(t, "example.com")
	send := func(addr, agent string) {
		b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "n": "pageview"})
		r, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/150 Safari/537.36 "+agent)
		r.Header.Set("X-Forwarded-For", addr)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	send("203.0.113.5", "a")
	send("203.0.113.6", "b")
	send("203.0.113.6", "b")   // the same visitor again: still one
	send("203.0.113.200", "c") // the database has only the country for this one
	var report Report
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today", nil, 200), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.LivePoints) != 1 || report.LivePoints[0] != (LivePoint{Lat: 35.69, Lon: 139.69, Country: "JP", Count: 2}) {
		t.Fatalf("live points: %+v", report.LivePoints)
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today&country=US", nil, 200), &report)
	if len(report.LivePoints) != 0 {
		t.Fatalf("country filter: %+v", report.LivePoints)
	}
	// Five minutes later the visitors are no longer online.
	if got := a.live.points(1, time.Now().Add(liveWindow+time.Minute), ""); len(got) != 0 {
		t.Fatalf("after the window: %+v", got)
	}
}
