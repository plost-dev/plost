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
	if !a.placesKnown() || !a.visitorKnown(r) {
		return
	}
	addr, _ := a.visitorAddr(r)
	var lat, lon float64
	var ok bool
	if a.geoOnline.Load() {
		p := a.online.place(addr, now)
		lat, lon, ok = p.lat, p.lon, p.placed
	} else {
		lat, lon, ok = a.geo.location(addr, now)
	}
	if !ok {
		return
	}
	a.live.record(site, visitor, livePlace{at: now.Unix(), lat: lat, lon: lon, country: country})
}

// OverviewPlace is one place on the site overview's globe: the visitors online there now, across all sites.
// Without a City GeoIP database a place is a whole country and carries no coordinates.
type OverviewPlace struct {
	Lat      *float64       `json:"lat,omitempty"`
	Lon      *float64       `json:"lon,omitempty"`
	Country  string         `json:"country"`
	Visitors int            `json:"visitors"`
	Sites    []OverviewSite `json:"sites"`
}

type OverviewSite struct {
	Name     string `json:"name"`
	Visitors int    `json:"visitors"`
}

// overviewLive places the visitors online now on the site overview's globe, busiest place first.
func (a *App) overviewLive(w http.ResponseWriter, r *http.Request) error {
	now := time.Now()
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,COALESCE(NULLIF(name,''),domain) FROM sites")
	if err != nil {
		return err
	}
	names := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		names[id] = name
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	type key struct {
		lat, lon float64
		country  string
	}
	places := map[key]*OverviewPlace{}
	add := func(k key, coords bool, site string, n int) {
		p := places[k]
		if p == nil {
			p = &OverviewPlace{Country: k.country}
			if coords {
				lat, lon := k.lat, k.lon
				p.Lat, p.Lon = &lat, &lon
			}
			places[k] = p
		}
		p.Visitors += n
		p.Sites = append(p.Sites, OverviewSite{site, n})
	}
	// Visitors the City database placed sit at their city; the rest of a country's online visitors (no
	// database, or an address it knows only the country of) sit at the country itself.
	placed := map[int64]map[string]int{}
	if a.placesKnown() {
		for id, name := range names {
			placed[id] = map[string]int{}
			for _, pt := range a.live.points(id, now, "") {
				add(key{pt.Lat, pt.Lon, pt.Country}, true, name, pt.Count)
				placed[id][pt.Country] += pt.Count
			}
		}
	}
	// CROSS JOIN keeps sites as the outer loop, so each site reads only its recent rows through events_site_time.
	rows, err = a.db.QueryContext(r.Context(), `SELECT s.id,e.country_code,count(DISTINCT `+visitorSQL+`)
		FROM sites s CROSS JOIN events_v2 e
		WHERE e.site_id=s.id AND e.timestamp>=? AND e.country_code<>''
		GROUP BY s.id,e.country_code`, now.Add(-liveWindow).Unix())
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var country string
		var n int
		if err := rows.Scan(&id, &country, &n); err != nil {
			rows.Close()
			return err
		}
		if rest := n - placed[id][country]; rest > 0 {
			add(key{country: country}, false, names[id], rest)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	out := make([]OverviewPlace, 0, len(places))
	for _, p := range places {
		sort.Slice(p.Sites, func(i, j int) bool { return p.Sites[i].Visitors > p.Sites[j].Visitors })
		if len(p.Sites) > 3 {
			p.Sites = p.Sites[:3]
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Visitors != out[j].Visitors {
			return out[i].Visitors > out[j].Visitors
		}
		return out[i].Country < out[j].Country
	})
	if len(out) > 500 {
		out = out[:500]
	}
	return jsonResponse(w, out)
}
