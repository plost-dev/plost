package plost

import (
	"encoding/json"
	"strings"
	"testing"
)

// The chosen map service goes to the statistics pages, the overview, both or neither, as the map usage says.
func TestMapUsage(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.app.cfg.MapboxToken = "pk.test" // As if read from the env file.
	x.call(t, "PUT", "/api/mapbox", map[string]any{"enabled": true}, 200)

	detail := func() string {
		var report struct {
			Details map[string]any `json:"details"`
		}
		json.Unmarshal(x.call(t, "GET", "/api/sites/example.com/report?period=today&ui=1", nil, 200), &report)
		token, _ := report.Details["mapboxToken"].(string)
		return token
	}
	overview := func() map[string]string {
		var got map[string]string
		json.Unmarshal(x.call(t, "GET", "/api/overview/map", nil, 200), &got)
		return got
	}
	var usage map[string]bool
	json.Unmarshal(x.call(t, "GET", "/api/map-usage", nil, 200), &usage)
	if !usage["detail"] || usage["overview"] {
		t.Fatalf("default usage = %v", usage)
	}
	if detail() != "pk.test" || overview()["service"] != "" {
		t.Fatalf("by default only the statistics pages use Mapbox: %q %v", detail(), overview())
	}
	x.call(t, "PUT", "/api/map-usage", map[string]any{"detail": false, "overview": true}, 200)
	if got := overview(); detail() != "" || got["service"] != "mapbox" || got["key"] != "pk.test" {
		t.Fatalf("overview only: %q %v", detail(), got)
	}
	x.call(t, "PUT", "/api/mapbox", map[string]any{"enabled": false}, 200)
	if overview()["service"] != "" {
		t.Fatal("with no service on, the overview keeps its own globe")
	}
	if !strings.Contains(string(x.call(t, "GET", "/api/map-usage", nil, 200)), `"overview":true`) {
		t.Fatal("the usage is kept while the service is off")
	}
}
