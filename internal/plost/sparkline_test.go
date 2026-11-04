package plost

import (
	"encoding/json"
	"testing"
)

func TestSparklineSettings(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var prefs map[string]string
	if err := json.Unmarshal(x.call(t, "GET", "/api/preferences", nil, 200), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["sparklineStyle"] != "area-line" || prefs["sparklineRange"] != "24h" {
		t.Fatalf("defaults: %v", prefs)
	}
	x.call(t, "PUT", "/api/sparkline", map[string]any{"style": "rainbow", "range": "30d"}, 400)
	x.call(t, "PUT", "/api/sparkline", map[string]any{"style": "bars", "range": "7d"}, 400)
	x.call(t, "PUT", "/api/sparkline", map[string]any{"style": "bars-today", "range": "24h"}, 400)
	x.call(t, "PUT", "/api/sparkline", map[string]any{"style": "mirror", "range": "30d"}, 400) // retired 2026-11-04
	x.call(t, "PUT", "/api/sparkline", map[string]any{"style": "peak", "range": "30d"}, 200)
	if err := json.Unmarshal(x.call(t, "GET", "/api/preferences", nil, 200), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["sparklineStyle"] != "peak" || prefs["sparklineRange"] != "30d" {
		t.Fatalf("saved: %v", prefs)
	}
	var sites []struct {
		Series []Point `json:"series"`
	}
	if err := json.Unmarshal(x.call(t, "GET", "/api/sites?period=24h&ui=1", nil, 200), &sites); err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || len(sites[0].Series) != 30 {
		t.Fatalf("30-day trend: %d sites, %d points", len(sites), len(sites[0].Series))
	}
	// A style saved before the 2026-11-03 cut shows as its nearest remaining one.
	if _, err := x.app.db.Exec("UPDATE settings SET value='barcode' WHERE key='sparkline_style'"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(x.call(t, "GET", "/api/preferences", nil, 200), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["sparklineStyle"] != "bars" {
		t.Fatalf("retired style: %v", prefs)
	}
	// And so do the six dropped on 2026-11-04.
	for old, want := range map[string]string{"mirror": "bars", "blocks": "bars", "step": "area-line", "dots": "bars", "bars-average": "bars", "lollipop": "bars"} {
		if _, err := x.app.db.Exec("UPDATE settings SET value=? WHERE key='sparkline_style'", old); err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(x.call(t, "GET", "/api/preferences", nil, 200), &prefs)
		if prefs["sparklineStyle"] != want {
			t.Fatalf("%s shows as %q, want %q", old, prefs["sparklineStyle"], want)
		}
	}
}

func TestGoogleMapStyleSetting(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	var prefs map[string]string
	if err := json.Unmarshal(x.call(t, "GET", "/api/preferences", nil, 200), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["googleMapStyle"] != "roadmap" {
		t.Fatalf("default: %v", prefs)
	}
	x.call(t, "PUT", "/api/google-map-style", map[string]any{"styleId": "streets-v12"}, 400)
	x.call(t, "PUT", "/api/google-map-style", map[string]any{"styleId": "dark"}, 200)
	if err := json.Unmarshal(x.call(t, "GET", "/api/preferences", nil, 200), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["googleMapStyle"] != "dark" || prefs["mapboxStyle"] != "standard" {
		t.Fatalf("saved: %v", prefs)
	}
}
