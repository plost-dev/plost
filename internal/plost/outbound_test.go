package plost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

// Some local proxy tools answer every name, even a nonexistent one, with an address from 198.18.0.0/15 and
// forward the traffic themselves. Outbound checks refuse that range unless the operator opts in.
func TestOutboundGuardHonoursTheAllowList(t *testing.T) {
	fakeIP := []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}
	for _, tc := range []struct {
		address     string
		allow       []netip.Prefix
		wantAllowed bool
		explanation string
	}{
		{"198.18.25.43:443", nil, false, "reserved range is refused by default"},
		{"198.18.25.43:443", fakeIP, true, "the opted-in range is allowed"},
		{"198.19.255.1:443", fakeIP, true, "the whole /15 is covered"},
		{"198.20.0.1:443", fakeIP, true, "next to the range is ordinary public space"},
		{"198.51.100.7:443", fakeIP, false, "another reserved range stays refused"},
		{"100.64.0.1:443", fakeIP, false, "shared address space stays refused"},
		{"10.0.0.1:443", fakeIP, false, "other private networks stay refused"},
		{"127.0.0.1:443", fakeIP, false, "loopback stays refused"},
		{"[::ffff:198.18.1.1]:443", fakeIP, true, "an IPv4 address in IPv6 form follows the IPv4 rule"},
		{"[::ffff:10.0.0.1]:443", fakeIP, false, "and so does a refused one"},
		{"8.8.8.8:443", nil, true, "public addresses never need an opt-in"},
		{"example.com:443", fakeIP, false, "a name is never connected to without an address"},
		{"198.18.25.43", fakeIP, false, "an address without a port is malformed"},
	} {
		err := outboundGuard(tc.allow)("tcp", tc.address, nil)
		if (err == nil) != tc.wantAllowed {
			t.Errorf("%s with %v: allowed=%v, want %v (%s)", tc.address, tc.allow, err == nil, tc.wantAllowed, tc.explanation)
		}
		if err != nil && tc.address != "198.18.25.43" && !errors.Is(err, errNotPublic) {
			t.Errorf("%s: a refusal must be the standard one, got %v", tc.address, err)
		}
	}
}

func TestOutboundAllowListReachesAnAllowedNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	target := mustURL(t, srv.URL)

	if got := probeHTTP(context.Background(), newSiteHTTPClient(nil), target, time.Now); got.OK || got.Error != "域名没有可安全访问的公网地址" {
		t.Fatalf("by default loopback is refused: %+v", got)
	}
	loopback := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	if got := probeHTTP(context.Background(), newSiteHTTPClient(loopback), target, time.Now); !got.OK || got.Status == nil || *got.Status != 200 {
		t.Fatalf("an opted-in network is reachable: %+v", got)
	}
	// An opt-in for one network does not open another.
	elsewhere := []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}
	if got := probeHTTP(context.Background(), newSiteHTTPClient(elsewhere), target, time.Now); got.OK {
		t.Fatalf("a different network was opened: %+v", got)
	}
}

func TestOpenValidatesTheAllowList(t *testing.T) {
	for _, tc := range []struct {
		network string
		ok      bool
	}{
		{"198.18.0.0/15", true}, {"10.0.0.0/8", true}, {"192.168.1.0/24", true}, {"10.1.2.3/8", true},
		{"0.0.0.0/0", false}, {"128.0.0.0/1", false}, {"10.0.0.0/7", false}, {"::/0", false}, {"2001:db8::/15", false}, {"fd00::/16", true},
	} {
		a, err := Open(Config{DataDir: t.TempDir(), OutboundAllow: []netip.Prefix{netip.MustParsePrefix(tc.network)}})
		if (err == nil) != tc.ok {
			t.Errorf("%s: accepted=%v, want %v (%v)", tc.network, err == nil, tc.ok, err)
		}
		if err == nil {
			if got := a.outboundAllow[0]; got != got.Masked() {
				t.Errorf("%s is kept unmasked as %s", tc.network, got)
			}
			a.Close()
		}
	}
	if _, err := Open(Config{DataDir: t.TempDir(), OutboundAllow: []netip.Prefix{{}}}); err == nil {
		t.Error("an empty network must be refused")
	}
}
