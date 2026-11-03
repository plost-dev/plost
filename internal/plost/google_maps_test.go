package plost

import (
	"encoding/json"
	"net/http"
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
	x.call(t, "PUT", "/api/google-maps", map[string]any{"enabled": true}, 400) // No key yet.
	x.call(t, "PUT", "/api/google-maps", map[string]any{"key": "sk-not-a-maps-key", "enabled": true}, 400)
	key := "AIza" + strings.Repeat("A", 35)
	x.call(t, "PUT", "/api/mapbox", map[string]any{"token": "pk.test", "enabled": true}, 200)
	var google map[string]any
	if err := json.Unmarshal(x.call(t, "PUT", "/api/google-maps", map[string]any{"key": key, "enabled": false}, 200), &google); err != nil || google["previewKey"] != "" {
		t.Fatalf("key handed out while Google Maps is off: %v %v", err, google)
	}
	json.Unmarshal(x.call(t, "PUT", "/api/google-maps", map[string]any{"enabled": true}, 200), &google)
	if google["previewKey"] != key {
		t.Fatalf("preview key while on: %v", google["previewKey"])
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
	if google["enabled"] != false || google["previewKey"] != "" || strings.Contains(csp(), "googleapis") {
		t.Fatal("Google Maps stayed on after Mapbox was turned on")
	}
}
