package plost

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// writeGeoDB writes a small MaxMind-format database to path by renaming a finished file over it, the way a
// provider's updater does. Each network maps to a record; a string stands for {"country": {"iso_code": ...}}.
func writeGeoDB(t *testing.T, path string, records map[string]any, mod time.Time) {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "GeoLite2-Country", IPVersion: 6, RecordSize: 24, IncludeReservedNetworks: true})
	if err != nil {
		t.Fatal(err)
	}
	for cidr, rec := range records {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		var value mmdbtype.DataType
		switch v := rec.(type) {
		case string:
			value = mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String(v)}}
		default:
			value = mmdbtype.Map{"continent": mmdbtype.Map{"code": mmdbtype.String("EU")}}
		}
		if err := w.Insert(network, value); err != nil {
			t.Fatal(err)
		}
	}
	tmp := path + ".new"
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteTo(f); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, mod, mod); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

var geoRecords = map[string]any{
	"203.0.113.0/24": "US",
	"2001:db8::/32":  "JP",
	"192.0.2.0/24":   42,   // a record that carries no country
	"10.0.0.0/8":     "ZZ", // private space is never looked up
	"198.18.0.0/16":  "us", // codes are upper-cased
	"198.19.0.0/16":  "USA",
}

func TestGeoIPFindsTheCountryOfPublicAddresses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "country.mmdb")
	writeGeoDB(t, path, geoRecords, time.Now())
	g, err := openGeoIP(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for address, want := range map[string]string{
		"203.0.113.9":        "US",
		"::ffff:203.0.113.9": "US", // an IPv4 visitor seen on a dual-stack socket
		"2001:db8::1":        "JP",
		"198.51.100.1":       "", // not in the database
		"192.0.2.7":          "", // in the database without a country
		"10.1.2.3":           "",
		"127.0.0.1":          "",
		"::1":                "",
		"198.18.0.1":         "US",
		"198.19.0.1":         "", // not a two-letter code
		"2001:db8:ffff::1":   "JP",
		"0.0.0.0":            "",
		"255.255.255.255":    "",
		"169.254.1.1":        "",
		"fe80::1":            "",
	} {
		if got := g.country(netip.MustParseAddr(address), now); got != want {
			t.Errorf("country(%s) = %q, want %q", address, got, want)
		}
	}
	if got := g.country(netip.Addr{}, now); got != "" {
		t.Errorf("an invalid address must give no country, got %q", got)
	}
}

func TestGeoIPPicksUpANewerFileAndKeepsTheOldOneWhenItCannotBeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "country.mmdb")
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeGeoDB(t, path, map[string]any{"203.0.113.0/24": "US"}, t0)
	g, err := openGeoIP(path)
	if err != nil {
		t.Fatal(err)
	}
	address := netip.MustParseAddr("203.0.113.9")
	now := time.Now()
	if got := g.country(address, now); got != "US" {
		t.Fatalf("got %q", got)
	}

	waitIdle := func() {
		t.Helper()
		for i := 0; g.reloading.Load(); i++ {
			if i > 500 {
				t.Fatal("the reload never finished")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Inside the check interval a changed file is not even looked at.
	writeGeoDB(t, path, map[string]any{"203.0.113.0/24": "JP"}, t0.Add(time.Minute))
	if got := g.country(address, now.Add(10*time.Second)); got != "US" {
		t.Errorf("the file must not be checked more than once a minute, got %q", got)
	}

	// After the interval the new file replaces the old one.
	later := now.Add(2 * geoCheckEvery)
	g.country(address, later)
	waitIdle()
	if got := g.country(address, later); got != "JP" {
		t.Fatalf("the newer file must be used, got %q", got)
	}

	// A file that is not a database (still being written, say) leaves the working one in place.
	bad := path + ".bad"
	if err := os.WriteFile(bad, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(bad, t0.Add(2*time.Minute), t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(bad, path); err != nil {
		t.Fatal(err)
	}
	later = later.Add(2 * geoCheckEvery)
	g.country(address, later)
	waitIdle()
	if got := g.country(address, later); got != "JP" {
		t.Errorf("a broken file must not replace the working database, got %q", got)
	}

	// The broken file is not tried again until it changes, so a bad file does not fill the log.
	if g.failed.Load() == nil {
		t.Fatal("the failed file must be remembered")
	}
	failed := *g.failed.Load()
	later = later.Add(2 * geoCheckEvery)
	g.country(address, later)
	waitIdle()
	if g.failed.Load() == nil || *g.failed.Load() != failed {
		t.Error("an unchanged broken file must not be loaded again")
	}

	// Once a good file is there, it is used and the failure is forgotten.
	writeGeoDB(t, path, map[string]any{"203.0.113.0/24": "DE"}, t0.Add(3*time.Minute))
	later = later.Add(2 * geoCheckEvery)
	g.country(address, later)
	waitIdle()
	if got := g.country(address, later); got != "DE" || g.failed.Load() != nil {
		t.Errorf("a repaired file must be used, got %q", got)
	}

	// Without a change nothing is loaded again.
	before := g.db.Load()
	later = later.Add(2 * geoCheckEvery)
	g.country(address, later)
	waitIdle()
	if g.db.Load() != before {
		t.Error("an unchanged file must keep the same reader")
	}
}

func TestOpenRefusesAGeoIPDatabaseThatCannotBeUsed(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.mmdb")
	if err := os.WriteFile(broken, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"missing": filepath.Join(dir, "missing.mmdb"), "broken": broken, "directory": dir} {
		if a, err := Open(Config{DataDir: t.TempDir(), Version: "test", GeoIPPath: path}); err == nil {
			a.Close()
			t.Errorf("%s: starting with an unusable database must fail", name)
		}
	}
}

func TestRequestCountryTrustsOnlyAddressesItKnows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "country.mmdb")
	writeGeoDB(t, path, geoRecords, time.Now())
	open := func(trust bool) *App {
		a, err := Open(Config{DataDir: t.TempDir(), Version: "test", TrustProxy: trust, GeoIPPath: path})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { a.Close() })
		return a
	}
	request := func(peer string, headers map[string]string) *http.Request {
		r := httptest.NewRequest("POST", "/api/event", nil)
		r.RemoteAddr = peer
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}
	now := time.Now()
	direct, behind := open(false), open(true)
	for name, c := range map[string]struct {
		app  *App
		r    *http.Request
		want string
	}{
		"a direct visitor":                         {direct, request("203.0.113.7:4000", nil), "US"},
		"a direct visitor on IPv6":                 {direct, request("[2001:db8::7]:4000", nil), "JP"},
		"a proxy that was not trusted":             {direct, request("203.0.113.7:4000", map[string]string{"X-Forwarded-For": "2001:db8::1"}), ""},
		"a local proxy that was not trusted":       {direct, request("127.0.0.1:4000", map[string]string{"X-Forwarded-For": "203.0.113.7"}), ""},
		"a local proxy that was trusted":           {behind, request("127.0.0.1:4000", map[string]string{"X-Forwarded-For": "203.0.113.7"}), "US"},
		"the last forwarded address":               {behind, request("127.0.0.1:4000", map[string]string{"X-Forwarded-For": "2001:db8::1, 203.0.113.7"}), "US"},
		"Cloudflare's header":                      {behind, request("127.0.0.1:4000", map[string]string{"CF-IPCountry": "de", "X-Forwarded-For": "203.0.113.7"}), "DE"},
		"Cloudflare's header from a remote peer":   {behind, request("198.51.100.9:4000", map[string]string{"CF-IPCountry": "DE"}), ""},
		"a broken Cloudflare header":               {behind, request("127.0.0.1:4000", map[string]string{"CF-IPCountry": "T1", "X-Forwarded-For": "203.0.113.7"}), "US"},
		"a trusted proxy that sent no address":     {behind, request("127.0.0.1:4000", nil), ""},
		"a trusted proxy with a nonsense address":  {behind, request("127.0.0.1:4000", map[string]string{"X-Forwarded-For": "not-an-address"}), ""},
		"an address missing from the database":     {direct, request("198.51.100.7:4000", nil), ""},
		"a private address":                        {direct, request("10.0.0.7:4000", nil), ""},
		"Cloudflare's header without a trust flag": {direct, request("127.0.0.1:4000", map[string]string{"CF-IPCountry": "DE"}), ""},
	} {
		if got := c.app.requestCountry(c.r, now); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
	noDB, err := Open(Config{DataDir: t.TempDir(), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { noDB.Close() })
	if got := noDB.requestCountry(request("203.0.113.7:4000", nil), now); got != "" {
		t.Errorf("without a database there is no country, got %q", got)
	}
}

func TestCollectionStoresTheCountryFromTheGeoIPDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "country.mmdb")
	writeGeoDB(t, path, geoRecords, time.Now())
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test", TrustProxy: true, GeoIPPath: path})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	t.Cleanup(func() { server.Close(); a.Close() })
	jar, _ := cookiejar.New(nil)
	x := &testApp{a, server, &http.Client{Jar: jar, Timeout: 15 * time.Second}}
	x.setup(t)
	x.addSite(t, "example.com")
	send := func(visitor string, headers map[string]string) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"d": "example.com", "u": "https://example.com/", "v": visitor, "n": "pageview"})
		req, _ := http.NewRequest("POST", server.URL+"/api/event", bytes.NewReader(b))
		req.Header.Set("User-Agent", testChrome)
		req.Header.Set("Origin", "https://example.com")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp := mustDo(t, x.client, req)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 202 {
			t.Fatalf("collection returned %d", resp.StatusCode)
		}
	}
	send("browser-aaaaaaaa", map[string]string{"X-Forwarded-For": "203.0.113.5"})
	send("browser-bbbbbbbb", map[string]string{"X-Forwarded-For": "2001:db8::5"})
	send("browser-cccccccc", map[string]string{"X-Forwarded-For": "198.51.100.5"})
	send("browser-dddddddd", map[string]string{"X-Forwarded-For": "203.0.113.5", "CF-IPCountry": "FR"})
	// Events are written by a background worker; wait for all four.
	var got []string
	for i := 0; i < 500; i++ {
		got = got[:0]
		rows, err := a.db.Query("SELECT country_code FROM events_v2 ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			got = append(got, c)
		}
		rows.Close()
		if len(got) == 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	want := []string{"US", "JP", "", "FR"}
	if len(got) != len(want) {
		t.Fatalf("got %d events: %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: country %q, want %q (all: %q)", i, got[i], want[i], got)
		}
	}
}
