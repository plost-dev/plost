package plost

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKeysLiveOnlyInEnvFile: the Mapbox token and the Google Maps key saved on the settings pages go into the
// service's env file and nowhere in the database: not in any table, not in the database or WAL file on disk.
// MaxMind keys are not taken at all; GeoLite2 comes only from the Plost mirror.
func TestKeysLiveOnlyInEnvFile(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	env := filepath.Join(t.TempDir(), "plost.env")
	if err := os.WriteFile(env, []byte("# keys\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	x.app.cfg.EnvFile = env

	mapbox := "pk.mapbox-token-that-must-stay-in-env"
	google := "AIza" + strings.Repeat("Q", 35)
	x.call(t, "PUT", "/api/mapbox", map[string]any{"token": mapbox}, 200)
	x.call(t, "PUT", "/api/google-maps", map[string]any{"key": google}, 200)
	// Use them the way the page does afterwards, so any later write would show up too.
	x.call(t, "PUT", "/api/mapbox", map[string]any{"enabled": true}, 200)
	x.call(t, "PUT", "/api/google-maps", map[string]any{"enabled": true}, 200)
	x.call(t, "PUT", "/api/geoip", map[string]any{"account": "123456", "key": "abcdefghijklmnop_mmk"}, 400)

	got, err := os.ReadFile(env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), EnvMapboxToken+"="+mapbox+"\n") || !strings.Contains(string(got), EnvGoogleMapsKey+"="+google+"\n") {
		t.Fatalf("env file: %q", got)
	}
	if strings.Contains(string(got), "MAXMIND") {
		t.Fatalf("env file holds MaxMind keys: %q", got)
	}

	secrets := []string{mapbox, google, "abcdefghijklmnop_mmk"}
	tables, err := x.app.db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		tables.Scan(&name)
		names = append(names, name)
	}
	tables.Close()
	for _, table := range names {
		rows, err := x.app.db.Query(fmt.Sprintf("SELECT * FROM %q", table))
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			rows.Scan(ptrs...)
			for i, v := range values {
				text := fmt.Sprint(v)
				if b, ok := v.([]byte); ok {
					text = string(b)
				}
				for _, secret := range secrets {
					if strings.Contains(text, secret) {
						t.Errorf("table %s column %s holds a key", table, cols[i])
					}
				}
			}
		}
		rows.Close()
	}
	for _, name := range []string{"plost.db", "plost.db-wal"} {
		raw, _ := os.ReadFile(filepath.Join(x.app.cfg.DataDir, name))
		for _, secret := range secrets {
			if bytes.Contains(raw, []byte(secret)) {
				t.Errorf("%s holds a key", name)
			}
		}
	}
}
