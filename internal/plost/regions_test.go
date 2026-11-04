package plost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func TestReportCountsVisitorsByRegion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "city.mmdb")
	place := func(country, region string) mmdbtype.Map {
		return mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String(country)}, "subdivisions": mmdbtype.Slice{mmdbtype.Map{"iso_code": mmdbtype.String(region)}}}
	}
	writeGeoDB(t, path, map[string]any{"203.0.113.0/26": place("CN", "GD"), "203.0.113.64/26": mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String("US")}, "subdivisions": mmdbtype.Slice{mmdbtype.Map{"iso_code": mmdbtype.String("NE")}}, "city": mmdbtype.Map{"names": mmdbtype.Map{"en": mmdbtype.String("Omaha"), "zh-CN": mmdbtype.String("奥马哈")}}}, "203.0.113.128/26": "JP", "203.0.113.192/26": place("DE", "BY"), "198.51.100.0/25": mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String("FR")}, "subdivisions": mmdbtype.Slice{mmdbtype.Map{"iso_code": mmdbtype.String("IDF")}, mmdbtype.Map{"iso_code": mmdbtype.String("75")}}}, "198.51.100.128/25": mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String("CZ")}, "subdivisions": mmdbtype.Slice{mmdbtype.Map{"iso_code": mmdbtype.String("42")}, mmdbtype.Map{"iso_code": mmdbtype.String("423")}}}}, time.Now())
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
	send := func(addr, agent string, header map[string]string) {
		b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "n": "pageview"})
		r, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/150 Safari/537.36 "+agent)
		r.Header.Set("X-Forwarded-For", addr)
		for k, v := range header {
			r.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	send("203.0.113.5", "a", nil)
	send("203.0.113.6", "b", nil)
	send("203.0.113.70", "c", nil)
	send("203.0.113.130", "d", nil)                                     // the database places this one only by country
	send("203.0.113.200", "e", map[string]string{"CF-IPCountry": "AT"}) // the CDN disagrees: no region rather than a wrong one
	send("198.51.100.9", "f", nil)                                      // a region and its department: the department, as on the map
	send("198.51.100.200", "g", nil)                                    // Czechia's region and district: the region, as on the map
	var report Report
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today", nil, 200), &report); err != nil {
		t.Fatal(err)
	}
	want := []RegionRow{{"CN", "CN-GD", 2}, {"CZ", "CZ-42", 1}, {"FR", "FR-75", 1}, {"US", "US-NE", 1}}
	if fmt.Sprint(report.Regions) != fmt.Sprint(want) {
		t.Fatalf("regions: %+v", report.Regions)
	}
	// Cities come from the database's names: English as the key, Chinese as the label.
	if fmt.Sprint(report.Cities) != fmt.Sprint([]CityRow{{"US", "Omaha", "奥马哈", 1}}) {
		t.Fatalf("cities: %+v", report.Cities)
	}
	// The visitors online now carry their province too; one placed only by country has an empty one.
	if live := fmt.Sprint(report.LiveRegions); !strings.Contains(live, "{CN CN-GD 2}") || !strings.Contains(live, "{JP  1}") {
		t.Fatalf("live regions: %v", live)
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today&country=US", nil, 200), &report)
	if len(report.Regions) != 1 || report.Regions[0].Name != "US-NE" {
		t.Fatalf("country filter: %+v", report.Regions)
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today&region=CN-GD", nil, 200), &report)
	if report.Overview.Visitors != 2 {
		t.Fatalf("region filter: %+v", report.Overview)
	}
}

// The map colours every country from the report's list, so it is not cut to the 50 busiest like other lists.
func TestReportListsEveryCountry(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var site int64
	x.app.db.QueryRow("SELECT id FROM sites").Scan(&site)
	now := time.Now().Unix()
	for i := 0; i < 120; i++ {
		code := string(rune('A'+i/26)) + string(rune('A'+i%26))
		if _, err := x.app.db.Exec("INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,user_id,session_id,session_visitor_id,country_code) VALUES(?,?,?,?,?,?,?,?,?)",
			site, now-int64(i), "pageview", "example.com", "/", fmt.Sprint("u", i), fmt.Sprint("s", i), fmt.Sprint("h", i), code); err != nil {
			t.Fatal(err)
		}
	}
	var report Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today", nil, 200), &report)
	if len(report.Countries) != 120 {
		t.Fatalf("countries = %d, want 120", len(report.Countries))
	}
}
