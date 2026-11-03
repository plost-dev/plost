package plost

import (
	"hash/maphash"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// The settings page checks that visitors' addresses are really theirs. Collection keeps a short memory of how the
// last events' addresses were found: only the kind of address and a hash of its network, never the address, and only
// in memory. From it the page warns about the usual mistakes: a proxy in front that is not trusted, CDN or proxy
// addresses taken for visitors, or most visitors sharing one network.

const ipSampleSize = 1000

type ipSample struct {
	via     string
	kind    string // Kind of the address taken for the visitor.
	ignored bool   // Forwarding headers came from a peer that is not trusted.
	peer    string // Kind of the peer that sent ignored headers.
	network uint64 // Hash of the visitor's /24 (IPv4) or /48 (IPv6).
}

type ipSamples struct {
	mu   sync.Mutex
	seed maphash.Seed
	ring [ipSampleSize]ipSample
	n    int
	next int
}

func newIPSamples() *ipSamples { return &ipSamples{seed: maphash.MakeSeed()} }

// addressKind sorts an address into what the checks care about.
func addressKind(addr netip.Addr) string {
	switch {
	case !addr.IsValid():
		return "invalid"
	case addr.IsLoopback():
		return "loopback"
	case addr.IsPrivate() || addr.IsLinkLocalUnicast():
		return "private"
	}
	for _, network := range cloudflareNets {
		if network.Contains(addr) {
			return "cloudflare"
		}
	}
	return "public"
}

func (s *ipSamples) networkHash(addr netip.Addr) uint64 {
	bits := 24
	if addr.Is6() {
		bits = 48
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return 0
	}
	b, _ := prefix.Addr().MarshalBinary()
	return maphash.Bytes(s.seed, b)
}

func (a *App) recordAddress(r *http.Request) {
	addr, via := a.clientAddr(r)
	sample := ipSample{via: via, kind: addressKind(addr), network: a.ipSamples.networkHash(addr)}
	if via == viaDirect && hasForwardingHeader(r) {
		sample.ignored = true
		sample.peer = addressKind(peerAddr(r))
	}
	s := a.ipSamples
	s.mu.Lock()
	s.ring[s.next] = sample
	s.next = (s.next + 1) % ipSampleSize
	s.n = min(s.n+1, ipSampleSize)
	s.mu.Unlock()
}

type ipWarning struct {
	Code    string  `json:"code"`
	Share   float64 `json:"share"`
	Suggest string  `json:"suggest,omitempty"` // A -trusted-proxies preset that would help, when one would.
}

type ipSummary struct {
	Total    int            `json:"total"`
	Via      map[string]int `json:"via"`
	Kinds    map[string]int `json:"kinds"`
	Ignored  int            `json:"ignored"`
	Networks int            `json:"networks"`
	TopShare float64        `json:"topShare"`
	Warnings []ipWarning    `json:"warnings"`
}

// Thresholds for the warnings; below minSamples events the shares say too little.
const (
	minSamples         = 30
	warnIgnoredShare   = 0.2
	warnProxyShare     = 0.5
	warnCDNShare       = 0.2
	warnTopShare       = 0.3
	minSamplesTopShare = 100
)

func (s *ipSamples) summary() ipSummary {
	s.mu.Lock()
	samples := make([]ipSample, s.n)
	copy(samples, s.ring[:s.n])
	s.mu.Unlock()
	out := ipSummary{Total: len(samples), Via: map[string]int{}, Kinds: map[string]int{}, Warnings: []ipWarning{}}
	networks := map[uint64]int{}
	peers := map[string]int{}
	for _, sample := range samples {
		out.Via[sample.via]++
		out.Kinds[sample.kind]++
		networks[sample.network]++
		if sample.ignored {
			out.Ignored++
			peers[sample.peer]++
		}
	}
	top := 0
	for _, n := range networks {
		top = max(top, n)
	}
	out.Networks = len(networks)
	if out.Total == 0 {
		return out
	}
	out.TopShare = float64(top) / float64(out.Total)
	if out.Total < minSamples {
		return out
	}
	share := func(n int) float64 { return float64(n) / float64(out.Total) }
	// An untrusted proxy in front also makes the addresses local and alike; only that cause is reported then.
	if share(out.Ignored) >= warnIgnoredShare {
		suggest, most := "", 0
		for kind, n := range peers {
			if n > most {
				suggest, most = kind, n
			}
		}
		if suggest == "public" || suggest == "invalid" {
			suggest = "" // The CDN's own networks, which only its owner can list.
		}
		out.Warnings = append(out.Warnings, ipWarning{Code: "untrusted_proxy", Share: share(out.Ignored), Suggest: suggest})
		return out
	}
	if n := out.Kinds["loopback"] + out.Kinds["private"]; share(n) >= warnProxyShare {
		out.Warnings = append(out.Warnings, ipWarning{Code: "proxy_addresses", Share: share(n), Suggest: "private"})
	}
	if n := out.Kinds["cloudflare"]; share(n) >= warnCDNShare {
		out.Warnings = append(out.Warnings, ipWarning{Code: "cdn_addresses", Share: share(n), Suggest: "cloudflare"})
	}
	// One network for most visitors is worth a warning of its own only when no cause above explains it, such as a CDN
	// whose networks are not built in.
	if len(out.Warnings) == 0 && out.Total >= minSamplesTopShare && out.TopShare >= warnTopShare {
		out.Warnings = append(out.Warnings, ipWarning{Code: "shared_network", Share: out.TopShare})
	}
	return out
}

// clientIPCheck shows the signed-in owner how this request's address was found, the settings it was found with and
// what the recent events looked like. The addresses shown are the owner's own and the proxies'.
func (a *App) clientIPCheck(w http.ResponseWriter, r *http.Request) error {
	addr, via := a.clientAddr(r)
	peer := peerAddr(r)
	headers := map[string]string{}
	for _, name := range append(append([]string{}, forwardingHeaders...), countryHeaders...) {
		if v := r.Header.Get(name); v != "" {
			headers[name] = short(v, 500)
		}
	}
	trusted := append([]string{}, a.cfg.TrustedProxyNames...)
	if a.cfg.TrustProxy {
		trusted = append([]string{"loopback (-trust-proxy)"}, trusted...)
	}
	return jsonResponse(w, map[string]any{
		"config": map[string]any{"trusted": trusted, "header": a.cfg.ClientIPHeader, "geoip": a.geo != nil},
		"request": map[string]any{
			"peer": peer.String(), "peerTrusted": peer.IsValid() && a.trustedProxy(peer), "headers": headers,
			"address": addr.String(), "kind": addressKind(addr), "via": via, "known": a.addressKnown(r), "country": a.requestCountry(r, time.Now()),
		},
		"recent": a.ipSamples.summary(),
	})
}
