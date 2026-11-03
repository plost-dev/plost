package plost

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testTrust(t *testing.T, spec, header string, loopback bool) *App {
	t.Helper()
	nets, names, err := ParseTrustedProxies(spec)
	if err != nil {
		t.Fatal(err)
	}
	return &App{cfg: Config{TrustProxy: loopback, TrustedProxies: nets, TrustedProxyNames: names, ClientIPHeader: header}}
}

func TestClientAddr(t *testing.T) {
	cases := []struct {
		name, spec, header string
		loopback           bool
		peer               string
		headers            map[string]string
		want, via          string
	}{
		{name: "direct visitor", peer: "203.0.113.7:4000", want: "203.0.113.7", via: viaDirect},
		{name: "forged header from an untrusted peer", spec: "cloudflare", peer: "203.0.113.7:4000",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1", "CF-Connecting-IP": "1.1.1.1"}, want: "203.0.113.7", via: viaDirect},
		{name: "-trust-proxy, proxy overwrites the chain", loopback: true, peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4"}, want: "198.51.100.4", via: viaForwarded},
		{name: "Nginx appends the Cloudflare edge", spec: "loopback,cloudflare", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4, 162.158.1.2"}, want: "198.51.100.4", via: viaForwarded},
		{name: "untrusted CDN edge is taken for the visitor", spec: "loopback", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4, 162.158.1.2"}, want: "162.158.1.2", via: viaForwarded},
		{name: "visitor cannot prepend a fake address", spec: "loopback,cloudflare", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1, 198.51.100.4, 162.158.1.2"}, want: "198.51.100.4", via: viaForwarded},
		{name: "Docker proxy on a private network", spec: "private", peer: "172.18.0.5:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4"}, want: "198.51.100.4", via: viaForwarded},
		{name: "CDN connects straight to the program", spec: "cloudflare", peer: "[2606:4700::1]:443",
			headers: map[string]string{"X-Forwarded-For": "2001:db8::9"}, want: "2001:db8::9", via: viaForwarded},
		{name: "other CDN listed by network", spec: "loopback,47.246.0.0/16", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4, 47.246.3.3"}, want: "198.51.100.4", via: viaForwarded},
		{name: "named header from a trusted CDN behind the local proxy", spec: "loopback,47.246.0.0/16", header: "Ali-CDN-Real-IP", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "47.246.3.3", "Ali-CDN-Real-IP": "198.51.100.4"}, want: "198.51.100.4", via: viaHeader},
		{name: "named header forged straight to the local proxy", spec: "loopback,cloudflare", header: "CF-Connecting-IP", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.7", "CF-Connecting-IP": "1.1.1.1"}, want: "203.0.113.7", via: viaForwarded},
		{name: "named header from a CDN connecting straight to the program", spec: "cloudflare", header: "CF-Connecting-IP", peer: "162.158.1.2:443",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4", "CF-Connecting-IP": "198.51.100.4"}, want: "198.51.100.4", via: viaHeader},
		{name: "Caddy behind Cloudflare keeps only the edge, so the header is needed", spec: "loopback,cloudflare", header: "CF-Connecting-IP", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "162.158.1.2", "CF-Connecting-IP": "198.51.100.4"}, want: "198.51.100.4", via: viaHeader},
		{name: "named header missing falls back to the chain", spec: "loopback", header: "CF-Connecting-IP", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4"}, want: "198.51.100.4", via: viaForwarded},
		{name: "RFC 7239 Forwarded", spec: "loopback", peer: "127.0.0.1:5000",
			headers: map[string]string{"Forwarded": `for=198.51.100.4;proto=https, for="[2001:db8::1]:443"`}, want: "2001:db8::1", via: viaForwarded},
		{name: "addresses with ports", spec: "loopback", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4:51000"}, want: "198.51.100.4", via: viaForwarded},
		{name: "unreadable entry ends the chain", spec: "loopback,cloudflare", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4, unknown, 162.158.1.2"}, want: "162.158.1.2", via: viaForwarded},
		{name: "LAN visitor behind a LAN proxy", spec: "private", peer: "192.168.1.2:5000",
			headers: map[string]string{"X-Forwarded-For": "192.168.1.50"}, want: "192.168.1.50", via: viaForwarded},
		{name: "X-Real-IP without a chain", spec: "loopback", peer: "127.0.0.1:5000",
			headers: map[string]string{"X-Real-IP": "198.51.100.4"}, want: "198.51.100.4", via: viaRealIP},
		{name: "trusted proxy that sends nothing", spec: "loopback", peer: "127.0.0.1:5000", want: "127.0.0.1", via: viaProxy},
		{name: "IPv4-mapped peer", spec: "loopback", peer: "[::ffff:127.0.0.1]:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4"}, want: "198.51.100.4", via: viaForwarded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := testTrust(t, c.spec, c.header, c.loopback)
			r := httptest.NewRequest("POST", "/api/send", nil)
			r.RemoteAddr = c.peer
			for k, v := range c.headers {
				r.Header.Set(k, v)
			}
			addr, via := a.clientAddr(r)
			if addr.String() != c.want || via != c.via {
				t.Fatalf("got %s via %s, want %s via %s", addr, via, c.want, c.via)
			}
		})
	}
}

func TestAddressKnown(t *testing.T) {
	a := testTrust(t, "", "", false)
	for _, c := range []struct {
		peer, xff string
		want      bool
	}{
		{"203.0.113.7:4000", "", true},
		{"203.0.113.7:4000", "1.1.1.1", false}, // An untrusted proxy in front.
		{"127.0.0.1:4000", "", false},          // A proxy on this machine that was not trusted.
	} {
		r := httptest.NewRequest("POST", "/api/send", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := a.addressKnown(r); got != c.want {
			t.Fatalf("%s %q: got %v", c.peer, c.xff, got)
		}
	}
}

func TestHeaderCountryNeedsTrustedPeer(t *testing.T) {
	a := testTrust(t, "cloudflare", "", false)
	r := httptest.NewRequest("POST", "/api/send", nil)
	r.Header.Set("CF-IPCountry", "jp")
	r.RemoteAddr = "203.0.113.7:4000"
	if got := a.headerCountry(r); got != "" {
		t.Fatalf("untrusted peer: got %q", got)
	}
	r.RemoteAddr = "162.158.1.2:443"
	if got := a.headerCountry(r); got != "JP" {
		t.Fatalf("Cloudflare peer: got %q", got)
	}
	// Through a local proxy: believed only when the proxy got the request from Cloudflare.
	local := testTrust(t, "loopback,cloudflare", "", false)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "198.51.100.4, 162.158.1.2")
	if got := local.headerCountry(r); got != "JP" {
		t.Fatalf("via Cloudflare and a local proxy: got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := local.headerCountry(r); got != "" {
		t.Fatalf("forged straight to the local proxy: got %q", got)
	}
	// The older -trust-proxy believes its local proxy, as before.
	if got := testTrust(t, "", "", true).headerCountry(r); got != "JP" {
		t.Fatalf("-trust-proxy: got %q", got)
	}
	r.Header.Del("X-Forwarded-For")
	r.Header.Del("CF-IPCountry")
	r.Header.Set("CloudFront-Viewer-Country", "DE")
	r.RemoteAddr = "203.0.113.9:443"
	if got := testTrust(t, "203.0.113.0/24", "", false).headerCountry(r); got != "DE" {
		t.Fatalf("CloudFront: got %q", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cdn.txt")
	if err := os.WriteFile(file, []byte("# CDN back-to-origin networks\n47.246.0.0/16\n\n2001:db8::/32 # IPv6\n198.51.100.9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	nets, names, err := ParseTrustedProxies(" Loopback, 10.1.2.0/24 ,@" + file)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 6 || len(names) != 3 || names[0] != "loopback" || names[1] != "10.1.2.0/24" || names[2] != "@"+file {
		t.Fatalf("got %v %v", nets, names)
	}
	if nets[5].String() != "198.51.100.9/32" {
		t.Fatalf("single address: %v", nets[5])
	}
	for _, bad := range []string{"0.0.0.0/0", "::/0", "cdn", "@" + filepath.Join(t.TempDir(), "missing")} {
		if _, _, err := ParseTrustedProxies(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	if h, err := CanonicalClientIPHeader("cf-connecting-ip"); err != nil || h != "CF-Connecting-IP" {
		t.Fatalf("got %q %v", h, err)
	}
	if _, err := CanonicalClientIPHeader("X-Forwarded-For"); err == nil {
		t.Fatal("a chain header was accepted as a single-address header")
	}
}

func TestVisitorAddrWithoutDeclaredProxies(t *testing.T) {
	a := testTrust(t, "", "", false)
	cases := []struct {
		name, peer string
		headers    map[string]string
		want, via  string
		known      bool
	}{
		{"direct visitor", "203.0.113.7:4000", nil, "203.0.113.7", viaDirect, true},
		{"Cloudflare straight to the program", "162.158.1.2:443", map[string]string{"CF-Connecting-IP": "198.51.100.4", "X-Forwarded-For": "198.51.100.4"}, "198.51.100.4", viaHeader, true},
		{"local Caddy that overwrites the chain", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "198.51.100.4"}, "198.51.100.4", viaForwarded, true},
		{"Nginx behind another CDN appends its node", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "198.51.100.4, 47.246.3.3"}, "198.51.100.4", viaForwarded, true},
		{"Aliyun CDN header", "127.0.0.1:5000", map[string]string{"Ali-CDN-Real-IP": "198.51.100.4", "X-Forwarded-For": "198.51.100.4, 47.246.3.3"}, "198.51.100.4", viaHeader, true},
		{"Docker proxy", "172.18.0.5:5000", map[string]string{"X-Forwarded-For": "198.51.100.4"}, "198.51.100.4", viaForwarded, true},
		{"private hops before the visitor are skipped", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "10.0.0.3, 198.51.100.4"}, "198.51.100.4", viaForwarded, true},
		{"LAN visitor", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "192.168.1.50"}, "192.168.1.50", viaForwarded, true},
		{"X-Real-IP only", "127.0.0.1:5000", map[string]string{"X-Real-IP": "198.51.100.4"}, "198.51.100.4", viaRealIP, true},
		{"local proxy that says nothing", "127.0.0.1:5000", nil, "127.0.0.1", viaDirect, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/send", nil)
			r.RemoteAddr = c.peer
			for k, v := range c.headers {
				r.Header.Set(k, v)
			}
			addr, via := a.visitorAddr(r)
			if addr.String() != c.want || via != c.via || a.visitorKnown(r) != c.known {
				t.Fatalf("got %s via %s known %v", addr, via, a.visitorKnown(r))
			}
		})
	}
	// Rate limits keep the strict reading: a forged header does not change the address they count.
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "203.0.113.7:4000"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	r.Header.Set("CF-Connecting-IP", "1.1.1.1")
	if got := a.clientIP(r); got != "203.0.113.7" {
		t.Fatalf("rate-limit address: %s", got)
	}
	r.Header.Set("CF-IPCountry", "jp")
	if got := a.visitorCountryHeader(r); got != "JP" {
		t.Fatalf("country header: %q", got)
	}
}

func TestVisitorAddrStrictWhenProxiesDeclared(t *testing.T) {
	a := testTrust(t, "loopback", "", false)
	r := httptest.NewRequest("POST", "/api/send", nil)
	r.RemoteAddr = "203.0.113.7:4000"
	r.Header.Set("CF-Connecting-IP", "1.1.1.1")
	r.Header.Set("CF-IPCountry", "JP")
	if addr, via := a.visitorAddr(r); addr.String() != "203.0.113.7" || via != viaDirect {
		t.Fatalf("got %s via %s", addr, via)
	}
	if got := a.visitorCountryHeader(r); got != "" {
		t.Fatalf("country header from an untrusted peer: %q", got)
	}
}
