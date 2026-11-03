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

func TestOverviewLivePlacesOnlineVisitorsAcrossSites(t *testing.T) {
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
	x.addSite(t, "a.example.test")
	x.addSite(t, "b.example.test")
	send := func(domain, addr, agent string) {
		b, _ := json.Marshal(map[string]any{"d": domain, "u": "https://" + domain + "/", "n": "pageview"})
		r, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/150 Safari/537.36 "+agent)
		r.Header.Set("X-Forwarded-For", addr)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	send("a.example.test", "203.0.113.5", "a")
	send("b.example.test", "203.0.113.6", "b")
	send("b.example.test", "203.0.113.7", "c")
	send("a.example.test", "203.0.113.200", "d") // only the country is known
	// Someone from yesterday is not online.
	if _, err := a.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id,country_code) SELECT id,?,'pageview',domain,'/','old','old','old','DE' FROM sites", time.Now().Add(-24*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}

	var got []OverviewPlace
	if err := json.Unmarshal(x.call(t, "GET", "/api/overview/live", nil, 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("places = %+v", got)
	}
	city, country := got[0], got[1]
	if city.Lat == nil || *city.Lat != 35.69 || *city.Lon != 139.69 || city.Country != "JP" || city.Visitors != 3 {
		t.Fatalf("city place = %+v", city)
	}
	if len(city.Sites) != 2 || city.Sites[0] != (OverviewSite{"b.example.test", 2}) {
		t.Fatalf("city sites = %+v", city.Sites)
	}
	if country.Lat != nil || country.Country != "US" || country.Visitors != 1 || country.Sites[0].Name != "a.example.test" {
		t.Fatalf("country place = %+v", country)
	}

	x.call(t, "POST", "/api/logout", map[string]any{}, 200)
	x.call(t, "GET", "/api/overview/live", nil, 401)
}
