package plost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func init() { geoFirstCheck = time.Hour }

// geoArchive builds a MaxMind-style .tar.gz holding one database of the given type that places 81.2.69.0/24.
func geoArchive(t *testing.T, dbType, folder, country string) []byte {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: dbType, Description: map[string]string{"en": "test"}, IPVersion: 6, RecordSize: 24})
	if err != nil {
		t.Fatal(err)
	}
	_, network, _ := net.ParseCIDR("81.2.69.0/24")
	rec := mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String(country)}, "location": mmdbtype.Map{"latitude": mmdbtype.Float64(51.5142), "longitude": mmdbtype.Float64(-0.0931)}}
	if err := w.Insert(network, rec); err != nil {
		t.Fatal(err)
	}
	var db bytes.Buffer
	if _, err := w.WriteTo(&db); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{{folder + "/COPYRIGHT.txt", []byte("test")}, {folder + "/" + folder[:strings.LastIndex(folder, "_")] + ".mmdb", db.Bytes()}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(f.body)
	}
	tw.Close()
	gz.Close()
	return out.Bytes()
}

// geoMirror serves archives the way ipdata.plost.io and MaxMind do, and counts the archive downloads.
type geoMirror struct {
	mu        sync.Mutex
	files     map[string][]byte // edition -> archive
	sums      map[string]string // edition -> published checksum, when it should differ from the archive's
	downloads atomic.Int32
	auth      string // MaxMind: the expected "account:key"
	srv       *httptest.Server
}

func newGeoMirror(t *testing.T) *geoMirror {
	m := &geoMirror{files: map[string][]byte{}, sums: map[string]string{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(name, "/download") { // MaxMind: /<edition>/download?suffix=…
			if user, pass, ok := r.BasicAuth(); !ok || user+":"+pass != m.auth {
				http.Error(w, "unauthorized", 401)
				return
			}
			name = strings.TrimSuffix(name, "/download") + "." + r.URL.Query().Get("suffix")
		}
		edition, isSum := strings.TrimSuffix(strings.TrimSuffix(name, ".sha256"), ".tar.gz"), strings.HasSuffix(name, ".sha256")
		body, ok := m.files[edition]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if isSum {
			sum := m.sums[edition]
			if sum == "" {
				h := sha256.Sum256(body)
				sum = hex.EncodeToString(h[:])
			}
			w.Write([]byte(sum + "  " + edition + "_20261103.tar.gz\n"))
			return
		}
		m.downloads.Add(1)
		w.Write(body)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *geoMirror) set(edition string, archive []byte, sum string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[edition], m.sums[edition] = archive, sum
}

func geoTestApp(t *testing.T, m *geoMirror) *testApp {
	x := newTestApp(t)
	x.app.geoHTTP = m.srv.Client()
	x.app.geoMirrorURL = m.srv.URL + "/"
	x.app.geoMaxMindURL = m.srv.URL + "/"
	return x
}

func geoDirFiles(t *testing.T, a *App) []string {
	t.Helper()
	entries, err := os.ReadDir(a.geoDir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

var geoTestAddr = netip.MustParseAddr("81.2.69.160")

func TestGeoIPDownloadsVerifiesAndReplacesOldVersion(t *testing.T) {
	m := newGeoMirror(t)
	m.set("GeoLite2-City", geoArchive(t, "GeoLite2-City", "GeoLite2-City_20261001", "GB"), "")
	x := geoTestApp(t, m)
	a := x.app
	if a.geo.ready() {
		t.Fatal("a new instance should start without a database")
	}
	// A download cut off earlier leaves a temporary file behind; the next run clears it.
	os.MkdirAll(a.geoDir(), 0o700)
	os.WriteFile(filepath.Join(a.geoDir(), ".download-stale.tmp"), []byte("partial"), 0o600)

	state, err := a.updateGeoIP(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.Edition != "GeoLite2-City" || state.Source != "plost" || state.Error != "" || state.Size == 0 {
		t.Fatalf("state = %+v", state)
	}
	if got := a.geo.country(geoTestAddr, time.Now()); got != "GB" {
		t.Fatalf("country = %q, want GB", got)
	}
	if _, _, ok := a.geo.location(geoTestAddr, time.Now()); !ok {
		t.Fatal("the City database should place the address")
	}
	if files := geoDirFiles(t, a); len(files) != 1 || files[0] != "GeoLite2.mmdb" {
		t.Fatalf("geoip directory holds %v, want only GeoLite2.mmdb", files)
	}

	// The same published version is not downloaded again.
	if _, err := a.updateGeoIP(t.Context()); err != nil || m.downloads.Load() != 1 {
		t.Fatalf("downloads = %d, err = %v; an unchanged version should not be fetched", m.downloads.Load(), err)
	}

	// A new version replaces the old one, which leaves the directory.
	m.set("GeoLite2-City", geoArchive(t, "GeoLite2-City", "GeoLite2-City_20261101", "IE"), "")
	if _, err := a.updateGeoIP(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := a.geo.country(geoTestAddr, time.Now()); got != "IE" {
		t.Fatalf("country after update = %q, want IE", got)
	}
	if files := geoDirFiles(t, a); len(files) != 1 {
		t.Fatalf("geoip directory holds %v after an update, want one database", files)
	}
}

func TestGeoIPRejectsBadDownloadsAndKeepsTheOldDatabase(t *testing.T) {
	m := newGeoMirror(t)
	m.set("GeoLite2-City", geoArchive(t, "GeoLite2-City", "GeoLite2-City_20261001", "GB"), "")
	x := geoTestApp(t, m)
	a := x.app
	if _, err := a.updateGeoIP(t.Context()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		archive []byte
		sum     string
		want    string
	}{
		"checksum":  {geoArchive(t, "GeoLite2-City", "GeoLite2-City_20261101", "IE"), strings.Repeat("0", 64), "SHA-256"},
		"edition":   {geoArchive(t, "GeoLite2-Country", "GeoLite2-City_20261101", "IE"), "", "GeoLite2-Country"},
		"not gzip":  {[]byte("not an archive"), "", "压缩包"},
		"no sum":    {geoArchive(t, "GeoLite2-City", "GeoLite2-City_20261101", "IE"), "nonsense", "SHA-256"},
		"no member": {geoArchive(t, "GeoLite2-City", "Other_20261101", "IE"), "", "GeoLite2-City.mmdb"},
	}
	for name, c := range cases {
		m.set("GeoLite2-City", c.archive, c.sum)
		state, err := a.updateGeoIP(t.Context())
		if err == nil || !strings.Contains(state.Error, c.want) {
			t.Fatalf("%s: err = %v, state error %q, want one naming %q", name, err, state.Error, c.want)
		}
		if got := a.geo.country(geoTestAddr, time.Now()); got != "GB" {
			t.Fatalf("%s: a failed update replaced the database in use (country %q)", name, got)
		}
		if files := geoDirFiles(t, a); len(files) != 1 {
			t.Fatalf("%s: a failed update left %v", name, files)
		}
	}
}

func TestGeoIPSettingsSwitchEditionTurnOffAndUseMaxMind(t *testing.T) {
	m := newGeoMirror(t)
	m.set("GeoLite2-City", geoArchive(t, "GeoLite2-City", "GeoLite2-City_20261001", "GB"), "")
	m.set("GeoLite2-Country", geoArchive(t, "GeoLite2-Country", "GeoLite2-Country_20261001", "FR"), "")
	m.auth = "123456:abcdefghijklmnop_mmk"
	x := geoTestApp(t, m)
	x.setup(t)
	a := x.app
	env := filepath.Join(t.TempDir(), "plost.env")
	a.cfg.EnvFile = env

	wait := func() {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); a.geoUpdating.Load(); {
			if time.Now().After(deadline) {
				t.Fatal("the background update did not finish")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	x.call(t, "PUT", "/api/geoip", map[string]any{"edition": "GeoLite2-Country"}, 200)
	wait()
	if got := a.geo.country(geoTestAddr, time.Now()); got != "FR" {
		t.Fatalf("country after switching to Country = %q", got)
	}
	if files := geoDirFiles(t, a); len(files) != 1 {
		t.Fatalf("switching edition left %v", files)
	}

	// MaxMind needs both keys; a wrong key is reported as such.
	x.call(t, "PUT", "/api/geoip", map[string]any{"source": "maxmind"}, 400)
	x.call(t, "PUT", "/api/geoip", map[string]any{"account": "123456", "key": "short"}, 400)
	x.call(t, "PUT", "/api/geoip", map[string]any{"source": "maxmind", "account": "123456", "key": "wrongwrongwrong_mmk"}, 200)
	wait()
	var view geoView
	json.Unmarshal(x.call(t, "GET", "/api/geoip", nil, 200), &view)
	if !strings.Contains(view.State.Error, "License Key") || view.Key == "" || strings.Contains(view.Key, "wrongwrong") {
		t.Fatalf("view = %+v", view)
	}
	x.call(t, "PUT", "/api/geoip", map[string]any{"account": "123456", "key": "abcdefghijklmnop_mmk", "edition": "GeoLite2-City"}, 200)
	wait()
	view = geoView{}
	json.Unmarshal(x.call(t, "POST", "/api/geoip/update", nil, 200), &view)
	if view.State.Source != "maxmind" || view.State.Error != "" || a.geo.country(geoTestAddr, time.Now()) != "GB" {
		t.Fatalf("after MaxMind download: %+v", view)
	}
	if b, _ := os.ReadFile(env); !strings.Contains(string(b), EnvMaxMindKey+"=abcdefghijklmnop_mmk") {
		t.Fatal("the licence key was not written to the env file")
	}
	var stored int
	a.db.QueryRow("SELECT count(*) FROM settings WHERE value LIKE '%abcdefghijklmnop%'").Scan(&stored)
	if stored != 0 {
		t.Fatal("the licence key must not be kept in the database")
	}

	// Off removes the database.
	x.call(t, "PUT", "/api/geoip", map[string]any{"source": "off"}, 200)
	wait()
	if a.geo.ready() || len(geoDirFiles(t, a)) != 0 {
		t.Fatal("turning GeoIP off should unload and delete the database")
	}
}

func TestGeoIPFlagDatabaseIsLeftAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	writeGeoDB(t, path, map[string]any{"81.2.69.0/24": "GB"}, time.Now())
	a, err := Open(Config{DataDir: t.TempDir(), Version: "test", GeoIPPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.geoManaged() {
		t.Fatal("-geoip-db should turn off the downloads")
	}
	if _, err := os.Stat(a.geoDir()); !os.IsNotExist(err) {
		t.Fatal("no download directory should be made")
	}
}

func TestGeoDue(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ok := geoState{Source: "plost", Edition: "GeoLite2-City", CheckedAt: now.Add(-time.Hour).Unix()}
	failed := ok
	failed.Error = "x"
	cases := []struct {
		name            string
		source, edition string
		state           geoState
		loaded          bool
		want            bool
	}{
		{"fresh install", "plost", "GeoLite2-City", geoState{}, false, true},
		{"checked an hour ago", "plost", "GeoLite2-City", ok, true, false},
		{"a day later", "plost", "GeoLite2-City", geoState{Source: "plost", Edition: "GeoLite2-City", CheckedAt: now.Add(-25 * time.Hour).Unix()}, true, true},
		{"edition changed", "plost", "GeoLite2-Country", ok, true, true},
		{"failed an hour ago", "plost", "GeoLite2-City", failed, true, true},
		{"failed just now", "plost", "GeoLite2-City", geoState{Source: "plost", Edition: "GeoLite2-City", CheckedAt: now.Unix(), Error: "x"}, true, false},
		{"off and empty", "off", "GeoLite2-City", geoState{}, false, false},
		{"off with a database", "off", "GeoLite2-City", ok, true, true},
	}
	for _, c := range cases {
		if got := geoDue(c.source, c.edition, c.state, c.loaded, now); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}
