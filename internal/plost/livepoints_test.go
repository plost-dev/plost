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
	if len(report.LivePoints) != 1 || report.LivePoints[0] != (LivePoint{Lat: 35.69, Lon: 139.69, Country: "JP", Count: 2, Last: report.LivePoints[0].Last, Arrived: report.LivePoints[0].Arrived}) || report.LivePoints[0].Last == 0 || report.LivePoints[0].Arrived == 0 {
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

// A place's arrival time moves only when a new visitor comes online there, not when someone already there opens
// another page, so the map flashes just the places where someone arrived.
func TestLivePointArrivalTracksNewVisitorsOnly(t *testing.T) {
	l := newLivePlaces()
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC).Unix()
	at := func(sec int64) livePlace { return livePlace{at: t0 + sec, lat: 1, lon: 2, country: "CN"} }
	arrived := func(now int64) int64 {
		p := l.points(1, time.Unix(t0+now, 0), "")
		if len(p) != 1 {
			t.Fatalf("points: %+v", p)
		}
		return p[0].Arrived - t0
	}
	l.record(1, "a", at(0))
	l.record(1, "a", at(60)) // another page by the same visitor
	if got := arrived(60); got != 0 {
		t.Errorf("same visitor moved the arrival to %d", got)
	}
	l.record(1, "b", at(120)) // someone new
	if got := arrived(120); got != 120 {
		t.Errorf("new visitor: arrival %d", got)
	}
	l.record(1, "a", at(60+int64(liveWindow/time.Second)+30)) // a comes back after going offline
	if got := arrived(60 + int64(liveWindow/time.Second) + 30); got != 60+int64(liveWindow/time.Second)+30 {
		t.Errorf("returning visitor: arrival %d", got)
	}
}
