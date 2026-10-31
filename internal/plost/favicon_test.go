package plost

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// get calls the handler the way the router does: with the path parameter already decoded.
func faviconGet(name string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/favicon/sources/x", nil)
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	(&App{}).faviconSource(rec, req)
	return rec
}

func TestFaviconRoutesPublicNamesToFaviconLa(t *testing.T) {
	for _, c := range [][2]string{
		{"github.com", "github.com"}, {"cnnic.cn", "cnnic.cn"}, {"php.net", "php.net"},
		{"Google", "google.com"}, {"Bing", "bing.com"}, {"X (Twitter)", "x.com"},
		{"Orange Webmail", "orange.fr"}, {"Bing Images", "bing.com"},
		{" https://WWW.CNNIC.CN./about?x=1#section ", "cnnic.cn"},
		{"http://github.com:80/docs", "github.com"}, {"github.com:443/docs", "github.com"},
		{"www.github.com/path", "github.com"}, {"https://例子.中国/", "xn--fsqu00a.xn--fiqs8s"},
		{"news.ycombinator.com", "news.ycombinator.com"},
	} {
		rec := faviconGet(c[0])
		if rec.Code != 302 || rec.Header().Get("Location") != "https://favicon.la/"+c[1] {
			t.Errorf("%q: got %d %q, want 302 to %s", c[0], rec.Code, rec.Header().Get("Location"), c[1])
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%q: referrer is not suppressed", c[0])
		}
	}
}

func TestFaviconKeepsPlaceholdersAndPrivateNamesLocal(t *testing.T) {
	link, site := faviconGet("placeholder").Body.String(), faviconGet("site-placeholder").Body.String()
	if link == site || !strings.Contains(site, "<circle") {
		t.Fatal("the link and site placeholders must be different pictures")
	}
	check := func(names []string, want string) {
		t.Helper()
		for _, name := range names {
			rec := faviconGet(name)
			if rec.Code != 200 || rec.Header().Get("Location") != "" || rec.Header().Get("Content-Type") != "image/svg+xml" ||
				rec.Header().Get("Cache-Control") != "public, max-age=86400" || rec.Body.String() != want {
				t.Errorf("%q must be answered locally with the expected placeholder, got %d %q", name, rec.Code, rec.Header().Get("Location"))
			}
		}
	}
	check([]string{"", "  ", "placeholder", "Direct", "DIRECT / NONE", "(none)"}, link)
	check([]string{
		"localhost", "https://localhost/", "office", "office.lan", "office.local", "a.localhost",
		"audit.local.test", "a.invalid", "a.example", "a.internal", "a.onion", "router.home.arpa",
		"example.com", "sub.example.net", "example.org",
		"127.0.0.1", "8.8.8.8", "http://2130706433/", "http://0x7f000001/", "http://127.1/",
		"[::1]", "https://[2606:4700:4700::1111]/",
		"%", "%25", "%ZZ", "__proto__", "constructor", "toString", "hasOwnProperty", "newsletter",
		"javascript:alert(1)", "file:///tmp/icon.svg", "ftp://github.com", "https://name:secret@github.com/",
		"https://github.com:bad/", "https://", "a..com", "-a.com", "a-.com", "a b.com",
		"github.com\\evil.com", "a\n.com", strings.Repeat("a", 64) + ".com", strings.Repeat("a", 2049),
	}, site)
}

func TestFaviconTableCoversTheBundledSources(t *testing.T) {
	if len(sourceDomains()) < 300 {
		t.Fatalf("bundled source table did not load: %d entries", len(sourceDomains()))
	}
}

func TestFaviconRouteIsPublicAndDecodesNames(t *testing.T) {
	x := newTestApp(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for path, want := range map[string]string{
		"/favicon/sources/github.com":                            "https://favicon.la/github.com",
		"/favicon/sources/X%20%28Twitter%29":                     "https://favicon.la/x.com",
		"/favicon/sources/https%3A%2F%2Fwww.cnnic.cn%2Fabout":    "https://favicon.la/cnnic.cn",
		"/favicon/sources/%E4%BE%8B%E5%AD%90.%E4%B8%AD%E5%9B%BD": "https://favicon.la/xn--fsqu00a.xn--fiqs8s",
		"/favicon/sources/audit.local.test":                      "",
		"/favicon/sources/":                                      "",
	} {
		resp, err := client.Get(x.server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if want != "" && (resp.StatusCode != 302 || resp.Header.Get("Location") != want) {
			t.Errorf("%s: got %d %q, want 302 to %s", path, resp.StatusCode, resp.Header.Get("Location"), want)
		}
		if want == "" && (resp.StatusCode != 200 || !strings.HasPrefix(string(body), "<svg")) {
			t.Errorf("%s: got %d, want a local placeholder", path, resp.StatusCode)
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "https://favicon.la") {
			t.Errorf("%s: the page policy must allow the icon service", path)
		}
	}
}
