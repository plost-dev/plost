package plost

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"testing"
)

// A pasted address becomes its host when a site is added.
func TestNormalizeDomainDropsTheRestOfAnAddress(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.Example.com/blog?x=1#top": "example.com",
		"http://shop.example.com:8080/":        "shop.example.com",
		"www.example.com":                      "example.com",
		"example.com/":                         "example.com",
	} {
		if got, err := normalizeDomain(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	if _, err := normalizeDomain("https://user@example.com"); err == nil {
		t.Error("an address with a user name is refused")
	}
}

// A site's own icon replaces favicon.la at the same address for those who may see the site, and can be removed.
func TestSiteIcon(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	png.Encode(&buf, img)

	upload := func(body []byte, want int) []byte {
		r, _ := http.NewRequest("PUT", x.server.URL+"/api/sites/example.com/icon", bytes.NewReader(body))
		r.Header.Set("X-Plost-Request", "1")
		r.Header.Set("Origin", x.server.URL)
		resp, err := x.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("upload: %d %s", resp.StatusCode, b)
		}
		return b
	}
	upload([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 400)
	upload(bytes.Repeat([]byte{0}, siteIconMax+10), 413)
	var got struct{ IconVersion int64 }
	json.Unmarshal(upload(buf.Bytes(), 200), &got)
	if got.IconVersion == 0 {
		t.Fatal("no icon version")
	}

	noRedirect := func(c *http.Client) *http.Client {
		return &http.Client{Jar: c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	fetch := func(c *http.Client) *http.Response {
		resp, err := noRedirect(c).Get(x.server.URL + "/favicon/sources/example.com")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if resp := fetch(x.client); resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("signed in: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// A stranger cannot learn from the icon that a private site is tracked here.
	if resp := fetch(&http.Client{}); resp.Header.Get("Content-Type") == "image/png" {
		t.Fatalf("signed out: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	x.call(t, "DELETE", "/api/sites/example.com/icon", nil, 200)
	if resp := fetch(x.client); resp.Header.Get("Content-Type") == "image/png" {
		t.Fatalf("after removal: %d", resp.StatusCode)
	}
}

// A new site has its checks on until its settings turn them off.
func TestNewSiteChecksOn(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	if !x.checks(t, "example.com").Enabled {
		t.Fatal("checks are off for a new site")
	}
	x.call(t, "PUT", "/api/sites/example.com/checks", map[string]any{"enabled": false, "path": "/"}, 200)
	if x.checks(t, "example.com").Enabled {
		t.Fatal("checks stay on after turning them off")
	}
}
