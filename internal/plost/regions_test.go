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

func TestReportCountsVisitorsByRegion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "city.mmdb")
	place := func(country, region string) mmdbtype.Map {
		return mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String(country)}, "subdivisions": mmdbtype.Slice{mmdbtype.Map{"iso_code": mmdbtype.String(region)}}}
	}
	writeGeoDB(t, path, map[string]any{"203.0.113.0/26": place("CN", "GD"), "203.0.113.64/26": place("US", "NE"), "203.0.113.128/26": "JP", "203.0.113.192/26": place("DE", "BY"), "198.51.100.0/24": mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String("FR")}, "subdivisions": mmdbtype.Slice{mmdbtype.Map{"iso_code": mmdbtype.String("IDF")}, mmdbtype.Map{"iso_code": mmdbtype.String("75")}}}}, time.Now())
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
	var report Report
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today", nil, 200), &report); err != nil {
		t.Fatal(err)
	}
	want := []RegionRow{{"CN", "CN-GD", 2}, {"FR", "FR-75", 1}, {"US", "US-NE", 1}}
	if len(report.Regions) != len(want) || report.Regions[0] != want[0] || report.Regions[1] != want[1] || report.Regions[2] != want[2] {
		t.Fatalf("regions: %+v", report.Regions)
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
