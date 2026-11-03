package plost

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoogleMapsSettings(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	csp := func() string {
		resp, err := http.Get(x.server.URL + "/login")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.Header.Get("Content-Security-Policy")
	}
	if strings.Contains(csp(), "googleapis") {
		t.Fatal("Google hosts allowed before Google Maps is on")
	}
	x.call(t, "PUT", "/api/google-maps", map[string]any{"enabled": true}, 400) // No key in the env file yet.
	key := "AIza" + strings.Repeat("A", 35)
	x.app.cfg.GoogleMapsKey, x.app.cfg.MapboxToken = key, "pk.test" // As if read from the env file.
	x.call(t, "PUT", "/api/mapbox", map[string]any{"enabled": true}, 200)
	var google map[string]any
	if err := json.Unmarshal(x.call(t, "GET", "/api/google-maps", nil, 200), &google); err != nil || google["configured"] != true {
		t.Fatalf("the env key: %v %v", err, google)
	}
	json.Unmarshal(x.call(t, "PUT", "/api/google-maps", map[string]any{"enabled": true}, 200), &google)
	if _, ok := google["previewKey"]; ok || google["enabled"] != true {
		t.Fatalf("the settings answer must not carry the key: %v", google)
	}
	var mapbox map[string]bool
	json.Unmarshal(x.call(t, "GET", "/api/mapbox", nil, 200), &mapbox)
	if mapbox["enabled"] {
		t.Fatal("Mapbox stayed on with Google Maps")
	}
	var report struct {
		Details map[string]any `json:"details"`
	}
	json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?ui=1", nil, 200), &report)
	if report.Details["googleMapsKey"] != key || report.Details["mapboxToken"] != "" {
		t.Fatalf("details: %v %v", report.Details["googleMapsKey"], report.Details["mapboxToken"])
	}
	if !strings.Contains(csp(), "https://maps.googleapis.com") {
		t.Fatal("page policy does not allow Google Maps")
	}
	x.call(t, "PUT", "/api/mapbox", map[string]any{"enabled": true}, 200) // The token saved above.
	google = nil
	json.Unmarshal(x.call(t, "GET", "/api/google-maps", nil, 200), &google)
	if google["enabled"] != false || strings.Contains(csp(), "googleapis") {
		t.Fatal("Google Maps stayed on after Mapbox was turned on")
	}
}

func TestEnvKeys(t *testing.T) {
	for name, value := range map[string]string{EnvMapboxToken: "pk.abc", EnvGoogleMapsKey: "AIza" + strings.Repeat("B", 35),
		EnvBackupAccessKey: "AK", EnvBackupSecretKey: "SK", EnvBackupPassword: "a-long-backup-password"} {
		t.Setenv(name, value)
	}
	var cfg Config
	if err := EnvKeys(&cfg); err != nil || cfg.MapboxToken != "pk.abc" || cfg.BackupPassword != "a-long-backup-password" {
		t.Fatalf("%v %+v", err, cfg)
	}
	for name, value := range map[string]string{EnvMapboxToken: "sk.secret", EnvGoogleMapsKey: "AIza-short", EnvBackupSecretKey: "has space", EnvBackupPassword: "short"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if EnvKeys(&Config{}) == nil {
				t.Fatalf("%s=%q accepted", name, value)
			}
		})
	}
	x := newTestApp(t)
	x.setup(t)
	x.call(t, "PUT", "/api/mapbox", map[string]any{"token": "pk.from-the-page"}, 400) // No env file to save it in.
	// With an env file, a key saved on the page goes into it, in place of its old line, and is used at once.
	env := filepath.Join(t.TempDir(), "plost.env")
	os.WriteFile(env, []byte("# keys\nPLOST_MAPBOX_TOKEN=pk.old\nPLOST_BACKUP_PASSWORD=keep-this-password\n"), 0o600)
	x.app.cfg.EnvFile = env
	x.call(t, "PUT", "/api/mapbox", map[string]any{"token": "sk.secret"}, 400)
	var mapbox map[string]any
	json.Unmarshal(x.call(t, "PUT", "/api/mapbox", map[string]any{"token": " pk.from-the-page "}, 200), &mapbox)
	google := "AIza" + strings.Repeat("C", 35)
	x.call(t, "PUT", "/api/google-maps", map[string]any{"key": google}, 200)
	if mapbox["masked"] != "pk.from-••••••••" || mapbox["editable"] != true || strings.Contains(string(x.call(t, "GET", "/api/google-maps", nil, 200)), google) {
		t.Fatalf("the settings answer must show only the start of a key: %v", mapbox)
	}
	if got, _ := os.ReadFile(env); string(got) != "# keys\nPLOST_MAPBOX_TOKEN=pk.from-the-page\nPLOST_BACKUP_PASSWORD=keep-this-password\nPLOST_GOOGLE_MAPS_KEY="+google+"\n" {
		t.Fatalf("env file: %q", got)
	}
	if token, key := x.app.mapKeys(); token != "pk.from-the-page" || key != google {
		t.Fatal("saved keys are not used at once")
	}
	var n int
	x.app.db.QueryRow("SELECT count(*) FROM settings WHERE key IN ('mapbox_public_token','google_maps_key')").Scan(&n)
	if n != 0 {
		t.Fatal("a key reached the database")
	}
}
