package plost

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// With the "online" source no database is kept here: each visitor's address is looked up at geo.plost.io, the
// Plost geolocation service, and the answer is kept in memory for a day so a returning visitor is not asked about
// again. A lookup that is slow or fails leaves that event without a country, as with no database at all.

const (
	geoOnlineBase    = "https://geo.plost.io/v1/"
	geoOnlineTimeout = 1500 * time.Millisecond
	geoOnlineKeep    = 24 * time.Hour // how long an answer is reused
	geoOnlineMiss    = time.Minute    // how long a failed lookup is not retried
	geoOnlineEntries = 20000          // answers kept in memory at most
	geoOnlineWaiting = 16             // lookups in flight at most; more go without a country
	geoOnlineBody    = 4 << 10        // the answer is about 150 bytes
)

type onlineGeo struct {
	base   string
	client *http.Client
	mu     sync.Mutex
	cache  map[netip.Addr]onlinePlace
	flight map[netip.Addr]chan struct{}
}

type onlinePlace struct {
	country  string
	lat, lon float64
	placed   bool // the answer had coordinates
	until    time.Time
}

func newOnlineGeo(base string, client *http.Client) *onlineGeo {
	return &onlineGeo{base: base, client: client, cache: map[netip.Addr]onlinePlace{}, flight: map[netip.Addr]chan struct{}{}}
}

func newOnlineClient(allow []netip.Prefix) *http.Client {
	dialer := &net.Dialer{Timeout: geoOnlineTimeout, Control: outboundGuard(allow)}
	return &http.Client{Timeout: geoOnlineTimeout, Transport: &http.Transport{
		Proxy:               nil, // the dial-time address check must see the service, not a proxy
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: geoOnlineTimeout,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}}
}

// place answers from memory, or asks the service once for an address many requests want at the same moment.
func (o *onlineGeo) place(ip netip.Addr, now time.Time) onlinePlace {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return onlinePlace{}
	}
	o.mu.Lock()
	if p, ok := o.cache[ip]; ok && now.Before(p.until) {
		o.mu.Unlock()
		return p
	}
	if wait, ok := o.flight[ip]; ok {
		o.mu.Unlock()
		select {
		case <-wait:
		case <-time.After(geoOnlineTimeout):
		}
		o.mu.Lock()
		p := o.cache[ip]
		o.mu.Unlock()
		return p
	}
	if len(o.flight) >= geoOnlineWaiting {
		o.mu.Unlock()
		return onlinePlace{}
	}
	done := make(chan struct{})
	o.flight[ip] = done
	o.mu.Unlock()

	p, ok := o.ask(ip)
	if ok {
		p.until = now.Add(geoOnlineKeep)
	} else {
		p = onlinePlace{until: now.Add(geoOnlineMiss)}
	}
	o.mu.Lock()
	if len(o.cache) >= geoOnlineEntries {
		for k, old := range o.cache { // drop expired answers first, then any one
			if now.After(old.until) {
				delete(o.cache, k)
			}
		}
		for k := range o.cache {
			if len(o.cache) < geoOnlineEntries {
				break
			}
			delete(o.cache, k)
		}
	}
	o.cache[ip] = p
	delete(o.flight, ip)
	o.mu.Unlock()
	close(done)
	return p
}

func (o *onlineGeo) ask(ip netip.Addr) (onlinePlace, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), geoOnlineTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", o.base+ip.String(), nil)
	if err != nil {
		return onlinePlace{}, false
	}
	req.Header.Set("Accept", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return onlinePlace{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, geoOnlineBody))
		return onlinePlace{}, false
	}
	var answer struct {
		Country   string   `json:"country_code"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, geoOnlineBody)).Decode(&answer) != nil {
		return onlinePlace{}, false
	}
	p := onlinePlace{country: strings.ToUpper(answer.Country)}
	if !countryPattern.MatchString(p.country) {
		p.country = ""
	}
	if lat, lon := answer.Latitude, answer.Longitude; lat != nil && lon != nil && *lat >= -90 && *lat <= 90 && *lon >= -180 && *lon <= 180 {
		p.lat, p.lon, p.placed = roundCoord(*lat), roundCoord(*lon), true
	}
	return p, true
}

// clear forgets every answer, when the source changes away from online.
func (o *onlineGeo) clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cache = map[netip.Addr]onlinePlace{}
}

// roundCoord keeps two decimals, about a kilometre, as the local database lookups do.
func roundCoord(v float64) float64 { return math.Round(v*100) / 100 }
