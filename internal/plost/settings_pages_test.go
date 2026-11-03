package plost

import (
	"net/http"
	"testing"
)

// The settings pages have static addresses; signed-out visitors are sent to the login page and unknown pages, including the old /account and /backup, are 404.
func TestSettingsPages(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for path, want := range map[string]int{"/settings": 303, "/settings/security": 303, "/settings/backup": 303, "/settings/nope": 404, "/account": 404, "/backup": 404} {
		resp, err := client.Get(x.server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want || want == 303 && resp.Header.Get("Location") != "/login" {
			t.Fatalf("%s: %d %q", path, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
}
