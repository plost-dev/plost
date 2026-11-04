package plost

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCNIPVersionShownOnTheIPDatabasePage(t *testing.T) {
	var hits atomic.Int32
	var body atomic.Value
	body.Store(`{"name":"ip2region","version":"2026-11-02","releasedAt":1793595600,"updatedAt":1793763275}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(body.Load().(string)))
	}))
	defer srv.Close()
	x := newTestApp(t)
	x.setup(t)
	x.app.cnipVersion = newCNIPVersion(srv.URL, srv.Client())

	var view geoView
	json.Unmarshal(x.call(t, "GET", "/api/geoip", nil, 200), &view)
	if view.CNVersion == nil || view.CNVersion.Version != "2026-11-02" || view.CNVersion.UpdatedAt != 1793763275 {
		t.Fatalf("cnVersion = %+v", view.CNVersion)
	}
	// Reused from memory, not fetched on every page view.
	x.call(t, "GET", "/api/geoip", nil, 200)
	if hits.Load() != 1 {
		t.Fatalf("fetched %d times", hits.Load())
	}

	// A bad answer is not shown; a stale good answer is kept while a refresh fails.
	c := newCNIPVersion(srv.URL, srv.Client())
	body.Store(`{"version":"latest"}`)
	if got := c.get(t.Context()); got != nil {
		t.Fatalf("bad answer accepted: %+v", got)
	}
	body.Store(`{"version":"2026-12-01","updatedAt":1796000000}`)
	c.until = time.Time{}
	if got := c.get(t.Context()); got == nil || got.Version != "2026-12-01" {
		t.Fatalf("good answer: %+v", got)
	}
	srv.Close()
	c.until = time.Time{}
	if got := c.get(t.Context()); got == nil || got.Version != "2026-12-01" {
		t.Fatalf("stale answer dropped: %+v", got)
	}
}
