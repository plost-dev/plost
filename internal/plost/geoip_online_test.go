package plost

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGeoService answers like geo.plost.io and counts the lookups it gets.
func fakeGeoService(t *testing.T, answers map[string]string) (*httptest.Server, *atomic.Int32) {
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		body, ok := answers[strings.TrimPrefix(r.URL.Path, "/v1/")]
		if !ok {
			http.Error(w, `{"error":"rate_limited"}`, 429)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func TestOnlineGeoCachesAnswersAndFailures(t *testing.T) {
	srv, asked := fakeGeoService(t, map[string]string{
		"203.0.113.5": `{"ip":"203.0.113.5","country_code":"cn","region":"浙江","city":"杭州","latitude":30.29,"longitude":120.153,"source":"ip2region"}`,
		"203.0.113.6": `{"ip":"203.0.113.6","country_code":"???"}`,
	})
	o := newOnlineGeo(srv.URL+"/v1/", srv.Client())
	now := time.Now()
	p := o.place(netip.MustParseAddr("203.0.113.5"), now)
	if p.country != "CN" || p.region != "浙江" || !p.placed || p.lat != 30.29 || p.lon != 120.15 {
		t.Fatalf("place = %+v", p)
	}
	o.place(netip.MustParseAddr("203.0.113.5"), now.Add(time.Hour))
	if asked.Load() != 1 {
		t.Fatalf("a cached answer was asked for again (%d lookups)", asked.Load())
	}
	if o.place(netip.MustParseAddr("203.0.113.5"), now.Add(25*time.Hour)); asked.Load() != 2 {
		t.Fatal("an answer older than a day should be asked for again")
	}
	if p := o.place(netip.MustParseAddr("203.0.113.6"), now); p.country != "" || p.placed {
		t.Fatalf("an invalid country should be dropped: %+v", p)
	}
	// A refused lookup gives nothing and is not repeated within the minute.
	o.place(netip.MustParseAddr("203.0.113.7"), now)
	o.place(netip.MustParseAddr("203.0.113.7"), now.Add(30*time.Second))
	if asked.Load() != 4 {
		t.Fatalf("lookups = %d, want 4", asked.Load())
	}
	// Private addresses never leave the server.
	o.place(netip.MustParseAddr("10.1.2.3"), now)
	o.place(netip.MustParseAddr("127.0.0.1"), now)
	if asked.Load() != 4 {
		t.Fatal("a private address was sent to the service")
	}
}

func TestCollectionUsesTheOnlineServiceWhenChosen(t *testing.T) {
	srv, asked := fakeGeoService(t, map[string]string{
		"203.0.113.5": `{"ip":"203.0.113.5","country_code":"TW","region":"台湾","city":"台北","latitude":25.04,"longitude":121.57,"source":"ip2region"}`,
	})
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test", TrustProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	a.online = newOnlineGeo(srv.URL+"/v1/", srv.Client())
	server := httptest.NewServer(a.Handler())
	t.Cleanup(func() { server.Close(); a.Close() })
	jar, _ := cookiejar.New(nil)
	x := &testApp{a, server, &http.Client{Jar: jar, Timeout: 15 * time.Second}}
	x.setup(t)
	x.addSite(t, "example.com")
	x.call(t, "PUT", "/api/geoip", map[string]any{"source": "online"}, 200)
	for deadline := time.Now().Add(5 * time.Second); a.geoUpdating.Load(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the update did not finish")
		}
	}
	if !a.geoOnline.Load() || a.geo.ready() {
		t.Fatal("online lookups should be on and no database loaded")
	}
	for _, visitor := range []string{"browser-aaaaaaaa", "browser-bbbbbbbb"} {
		b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "v": visitor, "n": "pageview"})
		req, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		req.Header.Set("User-Agent", testChrome)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-Forwarded-For", "203.0.113.5")
		resp := mustDo(t, x.client, req)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	var n int
	for i := 0; i < 500 && n < 2; i++ {
		a.db.QueryRow("SELECT count(*) FROM events_v2 WHERE country_code='TW'").Scan(&n)
		time.Sleep(10 * time.Millisecond)
	}
	if n != 2 {
		t.Fatalf("events with the online country = %d, want 2", n)
	}
	if asked.Load() != 1 {
		t.Fatalf("lookups = %d; the second event should use the cached answer", asked.Load())
	}
	site, _ := readSite(t.Context(), a.db, "example.com")
	if pts := a.live.points(site.ID, time.Now(), ""); len(pts) == 0 || pts[0].Country != "TW" {
		t.Fatalf("the visitor should be placed on the live map: %+v", pts)
	}

	// Choosing a database again stops the online lookups.
	x.call(t, "PUT", "/api/geoip", map[string]any{"source": "off"}, 200)
	if a.geoOnline.Load() {
		t.Fatal("online lookups should stop")
	}
}

// With the cnip.io option on, a visitor in mainland China gets the province and city api.cnip.io gives (its
// coordinates come as strings), the province stored as the ISO code a City database would give; the report counts
// them by city. Other countries are not sent to cnip.io.
func TestChinaVisitorsArePlacedByCnip(t *testing.T) {
	var asked atomic.Int32
	cnip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Write([]byte(`{"ip":"203.0.113.5","isp":"电信","country_code":"CN","country":"中国","region":"江苏","province":"江苏","city":"南京","latitude":"32.041544","longitude":"118.767413"}`))
	}))
	t.Cleanup(cnip.Close)
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test", TrustProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	a.cnip = newOnlineGeo(cnip.URL+"/geoip/", cnip.Client())
	a.cnip.stats = a.cnipStats
	server := httptest.NewServer(a.Handler())
	t.Cleanup(func() { server.Close(); a.Close() })
	jar, _ := cookiejar.New(nil)
	x := &testApp{a, server, &http.Client{Jar: jar, Timeout: 15 * time.Second}}
	x.setup(t)
	x.addSite(t, "example.com")
	send := func(visitor, country string) {
		b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "v": visitor, "n": "pageview"})
		req, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		req.Header.Set("User-Agent", testChrome)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-Forwarded-For", "203.0.113.5")
		req.Header.Set("CF-IPCountry", country)
		resp := mustDo(t, x.client, req)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	wait := func(want int) {
		var n int
		for i := 0; i < 500 && n < want; i++ {
			a.db.QueryRow("SELECT count(*) FROM events_v2").Scan(&n)
			time.Sleep(10 * time.Millisecond)
		}
		if n != want {
			t.Fatalf("events = %d, want %d", n, want)
		}
	}

	// Always on: there is no switch any more.
	x.call(t, "PUT", "/api/geoip", map[string]any{"cnCity": false}, 400)
	send("browser-bbbbbbbb", "CN")
	send("browser-cccccccc", "US") // another country keeps the database (none here): no lookup
	wait(2)
	if asked.Load() != 1 {
		t.Fatalf("cnip.io lookups = %d, want 1", asked.Load())
	}
	// The page counts the lookups and the cities found, and the counts survive a restart.
	var view geoView
	json.Unmarshal(x.call(t, "GET", "/api/geoip", nil, 200), &view)
	if view.CNStats.Calls != 1 || view.CNStats.Located != 1 || view.CNStats.Cities != 1 || view.CNStats.Since == 0 {
		t.Fatalf("cnStats = %+v", view.CNStats)
	}
	a.mu.RLock()
	a.flushCNIPStats(t.Context())
	a.mu.RUnlock()
	if again := a.loadCNIPStats(); again.Calls != 1 || len(again.Cities) != 1 || again.Since != view.CNStats.Since {
		t.Fatalf("reloaded stats = %+v", again)
	}
	var stored string
	a.db.QueryRow("SELECT value FROM settings WHERE key='cnip_stats'").Scan(&stored)
	if strings.Contains(stored, "203.0.113") {
		t.Fatalf("an address was stored: %s", stored)
	}
	var region, city, cityZh string
	a.db.QueryRow("SELECT region,city,city_zh FROM events_v2 WHERE user_id LIKE '%bbbbbbbb%' OR country_code='CN' ORDER BY id DESC LIMIT 1").Scan(&region, &city, &cityZh)
	if region != "CN-JS" || city != "南京" || cityZh != "南京" {
		t.Fatalf("stored place = %q %q %q", region, city, cityZh)
	}
	var report Report
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today", nil, 200), &report)
	if len(report.Cities) != 1 || report.Cities[0] != (CityRow{Country: "CN", Name: "南京", Label: "南京", Value: 1}) {
		t.Fatalf("cities = %+v", report.Cities)
	}
}

func TestChinaRegionCode(t *testing.T) {
	for name, want := range map[string]string{"江苏": "CN-JS", "江苏省": "CN-JS", "广西壮族自治区": "CN-GX", "北京市": "CN-BJ", "新疆维吾尔自治区": "CN-XJ", "河南": "CN-HA", "未知": "未知"} {
		if got := chinaRegionCode(name); got != want {
			t.Errorf("chinaRegionCode(%q) = %q, want %q", name, got, want)
		}
	}
}
