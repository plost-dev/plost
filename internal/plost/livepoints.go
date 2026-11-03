package plost

import (
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Where the visitors online now are, for the live dots on the map. A City GeoIP database places each pageview's
// address at the middle of its city or area; only those rounded coordinates, the country and the time are kept, in
// memory, for the five minutes a visitor counts as online, and never written to the database. Each visitor (the
// session hash, or the browser id) keeps only their latest place.

const liveWindow = 5 * time.Minute
const livePerSite = 5000 // A ceiling on memory for a very busy site; the oldest places go first.

type livePlace struct {
	at       int64
	lat, lon float64
	country  string
}

type livePlaces struct {
	mu    sync.Mutex
	sites map[int64]map[string]livePlace
}

func newLivePlaces() *livePlaces { return &livePlaces{sites: map[int64]map[string]livePlace{}} }

func (l *livePlaces) record(site int64, visitor string, place livePlace) {
	l.mu.Lock()
	defer l.mu.Unlock()
	places := l.sites[site]
	if places == nil {
		places = map[string]livePlace{}
		l.sites[site] = places
	}
	places[visitor] = place
	if len(places) > livePerSite {
		l.pruneLocked(site, place.at)
		for len(places) > livePerSite {
			oldest, at := "", int64(math.MaxInt64)
			for key, p := range places {
				if p.at < at {
					oldest, at = key, p.at
				}
			}
			delete(places, oldest)
		}
	}
}

func (l *livePlaces) pruneLocked(site int64, now int64) {
	for key, p := range l.sites[site] {
		if now-p.at > int64(liveWindow/time.Second) {
			delete(l.sites[site], key)
		}
	}
}

// LivePoint is one place on the map with the number of visitors online there.
type LivePoint struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Country string  `json:"country"`
	Count   int     `json:"count"`
}

// points groups the site's visitors online now by place, the busiest first. A country filter narrows it; other
// report filters do not apply, since a place is not tied to a page or a source.
func (l *livePlaces) points(site int64, now time.Time, country string) []LivePoint {
	l.mu.Lock()
	l.pruneLocked(site, now.Unix())
	counts := map[[2]float64]*LivePoint{}
	for _, p := range l.sites[site] {
		if country != "" && p.country != country {
			continue
		}
		key := [2]float64{p.lat, p.lon}
		if counts[key] == nil {
			counts[key] = &LivePoint{Lat: p.lat, Lon: p.lon, Country: p.country}
		}
		counts[key].Count++
	}
	l.mu.Unlock()
	out := make([]LivePoint, 0, len(counts))
	for _, p := range counts {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Lat < out[j].Lat || (out[i].Lat == out[j].Lat && out[i].Lon < out[j].Lon)
	})
	return out
}

// recordPlace notes where a pageview's visitor is, when the GeoIP database can place their address.
func (a *App) recordPlace(r *http.Request, site int64, visitor, country string, now time.Time) {
	if a.geo == nil || !a.visitorKnown(r) {
		return
	}
	addr, _ := a.visitorAddr(r)
	lat, lon, ok := a.geo.location(addr, now)
	if !ok {
		return
	}
	a.live.record(site, visitor, livePlace{at: now.Unix(), lat: lat, lon: lon, country: country})
}
