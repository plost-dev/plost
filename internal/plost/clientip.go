package plost

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

// Two readings of the client's address. clientIP is strict and guards rate limits and the like: it takes forwarding
// headers only from proxies declared with -trusted-proxies (or -trust-proxy). visitorAddr is for the statistics: with
// proxies declared it is the same strict reading, and without any it finds the visitor on its own, as Umami does, so
// that direct, reverse-proxied and CDN setups all count right with no configuration. A visitor can forge the headers
// that reading believes, but that only changes how their own visits are counted, never what anyone may do.
//
// In the strict reading the visitor's address is only taken from forwarding headers that a trusted proxy handed over. A request whose
// peer is not trusted is the visitor itself, whatever headers it carries, so nobody can claim another address by
// connecting to the program directly. Behind trusted proxies the X-Forwarded-For chain (or RFC 7239 Forwarded, or
// X-Real-IP) is read from the right: every proxy appends the address it saw, so the first entry that is not a
// trusted proxy is the visitor. This works for any CDN that appends to X-Forwarded-For, which the common ones do.

// Proxy presets that -trusted-proxies accepts by name.
var proxyPresets = map[string][]string{
	"loopback": {"127.0.0.0/8", "::1/128"},
	// Private networks: Docker, Kubernetes and LAN reverse proxies.
	"private": {"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"},
	// https://www.cloudflare.com/ips/ (checked 2026-11-03).
	"cloudflare": {
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18", "108.162.192.0/18",
		"190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
	},
}

func mustPrefixes(list []string) []netip.Prefix {
	out := make([]netip.Prefix, len(list))
	for i, s := range list {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// ParseTrustedProxies reads -trusted-proxies: a comma-separated list of preset names (loopback, private,
// cloudflare), networks or single addresses, and @file entries naming a file with one of those per line (# starts a
// comment). It returns the networks and, for the settings page, the entries as written.
func ParseTrustedProxies(spec string) ([]netip.Prefix, []string, error) {
	var nets []netip.Prefix
	var names []string
	var add func(entry string, fromFile bool) error
	add = func(entry string, fromFile bool) error {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil
		}
		if preset, ok := proxyPresets[strings.ToLower(entry)]; ok {
			nets = append(nets, mustPrefixes(preset)...)
			names = append(names, strings.ToLower(entry))
			return nil
		}
		if path, ok := strings.CutPrefix(entry, "@"); ok && !fromFile {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			lines := bufio.NewScanner(f)
			for lines.Scan() {
				line, _, _ := strings.Cut(lines.Text(), "#")
				if err := add(line, true); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
			}
			if err := lines.Err(); err != nil {
				return err
			}
			names = append(names, entry)
			return nil
		}
		network, err := netip.ParsePrefix(entry)
		if err != nil {
			addr, addrErr := netip.ParseAddr(entry)
			if addrErr != nil {
				return fmt.Errorf("%q is neither loopback, private, cloudflare, a network such as 203.0.113.0/24 nor an address", entry)
			}
			addr = addr.Unmap()
			network = netip.PrefixFrom(addr, addr.BitLen())
		}
		if network.Addr().Is4In6() {
			network = netip.PrefixFrom(network.Addr().Unmap(), max(0, network.Bits()-96))
		}
		if network.Bits() == 0 {
			return fmt.Errorf("%q would trust every address, so anyone could claim any address", entry)
		}
		nets = append(nets, network.Masked())
		if !fromFile {
			names = append(names, network.Masked().String())
		}
		return nil
	}
	for _, entry := range strings.Split(spec, ",") {
		if err := add(entry, false); err != nil {
			return nil, nil, err
		}
	}
	return nets, names, nil
}

// clientIPHeaders are the headers that -client-ip-header may name; each is set by one CDN or proxy to the visitor's
// address alone.
var clientIPHeaders = map[string]bool{
	"CF-Connecting-IP": true, "True-Client-IP": true, "X-Real-IP": true, "Fastly-Client-IP": true,
	"Ali-CDN-Real-IP": true, "EO-Connecting-IP": true, "X-Client-IP": true,
}

// CanonicalClientIPHeader checks a -client-ip-header value and returns its canonical spelling.
func CanonicalClientIPHeader(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	for known := range clientIPHeaders {
		if strings.EqualFold(known, name) {
			return known, nil
		}
	}
	return "", errors.New("client-ip-header must be one of CF-Connecting-IP, True-Client-IP, X-Real-IP, Fastly-Client-IP, Ali-CDN-Real-IP, EO-Connecting-IP or X-Client-IP")
}

// How the visitor's address was found.
const (
	viaDirect    = "direct"    // The peer is not a trusted proxy, so it is the visitor.
	viaHeader    = "header"    // The header named by -client-ip-header.
	viaForwarded = "forwarded" // X-Forwarded-For or RFC 7239 Forwarded.
	viaRealIP    = "x-real-ip" // X-Real-IP from a trusted proxy that sent no chain.
	viaProxy     = "proxy"     // A trusted proxy that sent no address; the proxy itself.
)

// forwardingHeaders are the headers whose presence shows that a proxy sits in front, trusted or not.
var forwardingHeaders = []string{"X-Forwarded-For", "Forwarded", "X-Real-IP", "CF-Connecting-IP", "True-Client-IP", "Ali-CDN-Real-IP", "EO-Connecting-IP", "Fastly-Client-IP"}

func (a *App) trustedProxy(addr netip.Addr) bool {
	addr = addr.Unmap()
	if a.cfg.TrustProxy && addr.IsLoopback() {
		return true
	}
	for _, network := range a.cfg.TrustedProxies {
		if network.Contains(addr) {
			return true
		}
	}
	return false
}

func peerAddr(r *http.Request) netip.Addr {
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return ap.Addr().Unmap().WithZone("")
	}
	addr, _ := netip.ParseAddr(r.RemoteAddr)
	return addr.Unmap().WithZone("")
}

// parseHostAddr reads one address as proxies write it: bare, with a port, or bracketed IPv6 with or without one.
func parseHostAddr(s string) (netip.Addr, bool) {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap().WithZone(""), true
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	addr, err := netip.ParseAddr(strings.Trim(s, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

// forwardedChain lists the addresses in X-Forwarded-For, or failing that the for= parameters of Forwarded, from the
// visitor's end to the proxy nearest to this program. An entry that is not an address (an obfuscated identifier,
// "unknown") drops it and everything before it, since nothing to its left can be checked.
func forwardedChain(r *http.Request) []netip.Addr {
	var chain []netip.Addr
	var entries []string
	if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
		for _, v := range values {
			entries = append(entries, strings.Split(v, ",")...)
		}
	} else {
		for _, v := range r.Header.Values("Forwarded") {
			for _, element := range strings.Split(v, ",") {
				for _, pair := range strings.Split(element, ";") {
					if key, value, ok := strings.Cut(strings.TrimSpace(pair), "="); ok && strings.EqualFold(key, "for") {
						entries = append(entries, value)
					}
				}
			}
		}
	}
	for _, e := range entries {
		if strings.TrimSpace(e) == "" {
			continue
		}
		addr, ok := parseHostAddr(e)
		if !ok {
			chain = chain[:0]
			continue
		}
		chain = append(chain, addr)
	}
	return chain
}

// fromTrustedEdge reports whether the request reached the program through a trusted proxy all the way from the
// outside, so that headers which only the outermost proxy (the CDN) sets can be believed. A trusted peer on a public
// address is that proxy itself. A trusted peer on this machine or a private network is a local reverse proxy, which
// passes such headers on from anyone: then the hop it received the request from, the last X-Forwarded-For entry,
// must be trusted too. A local proxy that sends no chain at all is taken at its word.
func (a *App) fromTrustedEdge(r *http.Request) bool {
	peer := peerAddr(r)
	if !peer.IsValid() || !a.trustedProxy(peer) {
		return false
	}
	if !peer.IsLoopback() && !peer.IsPrivate() {
		return true
	}
	chain := forwardedChain(r)
	return len(chain) == 0 || a.trustedProxy(chain[len(chain)-1])
}

// clientAddr finds the visitor's address and says how.
func (a *App) clientAddr(r *http.Request) (netip.Addr, string) {
	peer := peerAddr(r)
	if !peer.IsValid() || !a.trustedProxy(peer) {
		return peer, viaDirect
	}
	if a.cfg.ClientIPHeader != "" && a.fromTrustedEdge(r) {
		if addr, ok := parseHostAddr(r.Header.Get(a.cfg.ClientIPHeader)); ok {
			return addr, viaHeader
		}
	}
	chain := forwardedChain(r)
	for i := len(chain) - 1; i >= 0; i-- {
		if !a.trustedProxy(chain[i]) {
			return chain[i], viaForwarded
		}
	}
	if len(chain) > 0 {
		// Every hop is a trusted network, so the visitor is inside one of them (a LAN visitor, say).
		return chain[0], viaForwarded
	}
	if addr, ok := parseHostAddr(r.Header.Get("X-Real-IP")); ok {
		return addr, viaRealIP
	}
	return peer, viaProxy
}

func (a *App) clientIP(r *http.Request) string {
	addr, _ := a.clientAddr(r)
	if !addr.IsValid() {
		return r.RemoteAddr
	}
	return addr.String()
}

// addressKnown reports whether clientIP is the visitor's own address. It is not when the peer is a proxy the
// program was not told to trust: one on this machine (loopback), or any peer that sends forwarding headers. Hashing
// such an address would merge every visitor into the proxy, so those events count by browser id instead.
func (a *App) addressKnown(r *http.Request) bool {
	addr, via := a.clientAddr(r)
	if via != viaDirect {
		return true
	}
	if !addr.IsValid() || addr.IsLoopback() {
		return false
	}
	return !hasForwardingHeader(r)
}

func hasForwardingHeader(r *http.Request) bool {
	for _, header := range forwardingHeaders {
		if r.Header.Get(header) != "" {
			return true
		}
	}
	return false
}

// countryHeaders are set by a CDN to the visitor's country. They count when the request came through trusted
// proxies from the outside (fromTrustedEdge). With the older -trust-proxy alone, a proxy on this machine is believed
// as before, so it must remove them from requests that did not come through the CDN (see the README).
var countryHeaders = []string{"CF-IPCountry", "CloudFront-Viewer-Country"}

func (a *App) headerCountry(r *http.Request) string {
	if !(a.cfg.TrustProxy && peerAddr(r).IsLoopback()) && !a.fromTrustedEdge(r) {
		return ""
	}
	for _, header := range countryHeaders {
		if code := strings.ToUpper(strings.TrimSpace(r.Header.Get(header))); countryPattern.MatchString(code) {
			return code
		}
	}
	return ""
}

// singleAddressHeaders are set by CDNs and proxies to the visitor's address alone, in the order visitorAddr tries
// them when no proxies are declared.
var singleAddressHeaders = []string{"CF-Connecting-IP", "True-Client-IP", "Ali-CDN-Real-IP", "EO-Connecting-IP", "Fastly-Client-IP"}

func (a *App) strictProxies() bool { return a.cfg.TrustProxy || len(a.cfg.TrustedProxies) > 0 }

// publicAddr reports whether an address can be a visitor's own on the internet.
func publicAddr(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsGlobalUnicast() && !addr.IsPrivate()
}

// visitorAddr finds the visitor's address for the statistics, and says how.
func (a *App) visitorAddr(r *http.Request) (netip.Addr, string) {
	if a.strictProxies() {
		return a.clientAddr(r)
	}
	for _, header := range singleAddressHeaders {
		if addr, ok := parseHostAddr(r.Header.Get(header)); ok {
			return addr, viaHeader
		}
	}
	// Each proxy appends what it saw, so the visitor is the first public address in the chain; later ones are
	// CDN nodes and proxies.
	if chain := forwardedChain(r); len(chain) > 0 {
		for _, addr := range chain {
			if publicAddr(addr) {
				return addr, viaForwarded
			}
		}
		return chain[0], viaForwarded
	}
	if addr, ok := parseHostAddr(r.Header.Get("X-Real-IP")); ok {
		return addr, viaRealIP
	}
	return peerAddr(r), viaDirect
}

// visitorKnown reports whether visitorAddr is the visitor's own address. It is not when a proxy on this machine
// passed the request on without saying whom it came from: hashing the proxy would merge every visitor into one.
func (a *App) visitorKnown(r *http.Request) bool {
	if a.strictProxies() {
		return a.addressKnown(r)
	}
	addr, via := a.visitorAddr(r)
	return addr.IsValid() && !(via == viaDirect && addr.IsLoopback())
}

// visitorCountryHeader is a CDN's country header for the statistics: from trusted proxies only in the strict reading,
// otherwise whenever one is present.
func (a *App) visitorCountryHeader(r *http.Request) string {
	if a.strictProxies() {
		return a.headerCountry(r)
	}
	for _, header := range countryHeaders {
		if code := strings.ToUpper(strings.TrimSpace(r.Header.Get(header))); countryPattern.MatchString(code) {
			return code
		}
	}
	return ""
}
