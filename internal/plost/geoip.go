package plost

import (
	"errors"
	"log"
	"math"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// geoCheckEvery is how often the database file is looked at for a newer version. Providers publish a new file
// every few days, so there is no need to look more often.
const geoCheckEvery = time.Minute

// geoIP looks up the country of an address in a MaxMind-format database (GeoLite2 City or Country). The file
// is mapped rather than read, which keeps its tens of megabytes out of the Go heap and its memory limit, and it is
// opened again when its size or modification time changes. The mapping is released once the previous reader is no
// longer used. Updaters must replace the file by renaming a new one over it, as they all do; rewriting it in place
// while it is mapped is not safe.
type geoIP struct {
	path      string
	db        atomic.Pointer[geoDB]
	failed    atomic.Pointer[fileSig] // the file that last could not be loaded, so it is not retried until it changes
	checked   atomic.Int64            // unix time of the last look at the file
	reloading atomic.Bool
}

type geoDB struct {
	reader *maxminddb.Reader
	sig    fileSig
}

type fileSig struct {
	size int64
	mod  time.Time
}

func sigOf(info os.FileInfo) fileSig { return fileSig{info.Size(), info.ModTime()} }

func openGeoIP(path string) (*geoIP, error) {
	g := &geoIP{path: path}
	db, err := g.read()
	if err != nil {
		return nil, err
	}
	g.db.Store(db)
	g.checked.Store(time.Now().Unix())
	return g, nil
}

// newManagedGeoIP is for the database Plost downloads itself (geoip_update.go): the file may not exist yet, and
// lookups find nothing until the first download is in place.
func newManagedGeoIP(path string) *geoIP {
	g := &geoIP{path: path}
	if db, err := g.read(); err == nil {
		g.db.Store(db)
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("GeoIP database not loaded: %v", err)
	}
	g.checked.Store(time.Now().Unix())
	return g
}

// placesKnown reports whether visitors' addresses can be looked up: in a loaded database, or online.
func (a *App) placesKnown() bool { return a.geoOnline.Load() || a.geo.ready() }

// ready reports whether a database is loaded; a nil geoIP (no database configured) is not ready.
func (g *geoIP) ready() bool { return g != nil && g.db.Load() != nil }

// load switches to the file now, rather than when the next lookup notices it changed.
func (g *geoIP) load() error {
	db, err := g.read()
	if err != nil {
		return err
	}
	g.failed.Store(nil)
	g.db.Store(db)
	return nil
}

// unload stops using the database, whose mapping is released once no lookup holds it.
func (g *geoIP) unload() {
	g.failed.Store(nil)
	g.db.Store(nil)
}

func (g *geoIP) read() (*geoDB, error) {
	info, err := os.Stat(g.path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	reader, err := maxminddb.Open(g.path)
	if err != nil {
		return nil, err
	}
	return &geoDB{reader: reader, sig: sigOf(info)}, nil
}

// refresh loads the file again when it changed, without holding up the request that noticed. A file that cannot
// be read, for example one that is still being written, leaves the database in use as it was.
func (g *geoIP) refresh(now time.Time) {
	last := g.checked.Load()
	if now.Unix()-last < int64(geoCheckEvery/time.Second) || !g.checked.CompareAndSwap(last, now.Unix()) {
		return
	}
	info, err := os.Stat(g.path)
	if err != nil {
		return
	}
	sig := sigOf(info)
	if cur, failed := g.db.Load(), g.failed.Load(); (cur != nil && sig == cur.sig) || (failed != nil && sig == *failed) || !g.reloading.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer g.reloading.Store(false)
		db, err := g.read()
		if err != nil {
			log.Printf("GeoIP database not reloaded: %v", err)
			g.failed.Store(&sig)
			return
		}
		g.failed.Store(nil)
		g.db.Store(db)
	}()
}

type geoRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	// Only City databases name the province or state, largest first: France's region and then its department,
	// England and then its county (see mapSubdivisionLevel for which one is kept).
	Subdivisions []struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"subdivisions"`
	// Only City databases name the city, in several languages; "en" and "zh-CN" are the ones shown.
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	// Only City databases have a location: the middle of the city or area the address belongs to.
	Location struct {
		Latitude  *float64 `maxminddb:"latitude"`
		Longitude *float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

// location returns the approximate coordinates of a public address, rounded to two decimals (about a kilometre),
// or ok false when the database has none (a Country database, or an address it does not place).
func (g *geoIP) location(ip netip.Addr, now time.Time) (lat, lon float64, ok bool) {
	g.refresh(now)
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return 0, 0, false
	}
	db := g.db.Load()
	if db == nil {
		return 0, 0, false
	}
	var rec geoRecord
	if err := db.reader.Lookup(ip).Decode(&rec); err != nil || rec.Location.Latitude == nil || rec.Location.Longitude == nil {
		return 0, 0, false
	}
	round := func(v float64) float64 { return math.Round(v*100) / 100 }
	return round(*rec.Location.Latitude), round(*rec.Location.Longitude), true
}

// country returns the two-letter country code of a public address, or "" when the database does not know it.
func (g *geoIP) country(ip netip.Addr, now time.Time) string {
	g.refresh(now)
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return ""
	}
	db := g.db.Load()
	if db == nil {
		return ""
	}
	var rec geoRecord
	if err := db.reader.Lookup(ip).Decode(&rec); err != nil {
		return ""
	}
	code := strings.ToUpper(rec.Country.ISOCode)
	if !countryPattern.MatchString(code) {
		return ""
	}
	return code
}

// geoPlace is where an address is, below its country: the province or state and the city, each "" when unknown.
type geoPlace struct {
	region         string // an ISO 3166-2 code ("CN-GD"), or the name an online service gives
	city, cityZh string // the city's English name, and its Chinese name when one is known
}

// region returns the address's country and its province or state as an ISO 3166-2 code ("CN-GD", "US-NE", "FR-75"),
// and its city, with "" for what the database does not have (a Country database, or an address placed only by
// country).
func (g *geoIP) region(ip netip.Addr, now time.Time) (country string, place geoPlace) {
	g.refresh(now)
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return "", geoPlace{}
	}
	db := g.db.Load()
	if db == nil {
		return "", geoPlace{}
	}
	var rec geoRecord
	if err := db.reader.Lookup(ip).Decode(&rec); err != nil {
		return "", geoPlace{}
	}
	country = strings.ToUpper(rec.Country.ISOCode)
	if !countryPattern.MatchString(country) {
		return "", geoPlace{}
	}
	if n := len(rec.Subdivisions); n > 0 {
		level := rec.Subdivisions[0]
		if secondLevelMaps[country] {
			level = rec.Subdivisions[n-1]
		}
		if code := strings.ToUpper(level.ISOCode); regionCodePattern.MatchString(code) {
			place.region = country + "-" + code
		}
	}
	place.city = short(strings.TrimSpace(rec.City.Names["en"]), 100)
	place.cityZh = short(strings.TrimSpace(rec.City.Names["zh-CN"]), 100)
	if place.city == "" {
		place.city = place.cityZh
	}
	return country, place
}

// secondLevelMaps are the countries the visitor map divides into their smaller units (Natural Earth's provinces
// there): France's departments, England's counties, Italy's and Spain's provinces and so on. Elsewhere the map shows
// the first level, which is also all a City database has for most countries.
var secondLevelMaps = map[string]bool{"FR": true, "GB": true, "IT": true, "ES": true, "BE": true, "IE": true, "PH": true}

var regionCodePattern = regexp.MustCompile(`^[A-Z0-9]{1,3}$`)

// requestPlace names the province or state and the city of the visitor in country (the one requestCountry chose):
// the region as an ISO 3166-2 code from a City database, or the name the online service gives, which the page
// matches to a code. A lookup that places the address in another country than the CDN said gives nothing, rather
// than a wrong place. With the cnip.io option on, visitors in mainland China are placed by api.cnip.io instead,
// which knows Chinese cities far better than GeoLite2; the others keep the database.
func (a *App) requestPlace(r *http.Request, country string, now time.Time) geoPlace {
	if country == "" || !a.visitorKnown(r) {
		return geoPlace{}
	}
	ip, _ := a.visitorAddr(r)
	if !ip.IsValid() {
		return geoPlace{}
	}
	if country == "CN" && a.cnCity.Load() {
		if p := a.cnip.place(ip, now); p.country == "CN" && p.city != "" {
			return geoPlace{region: chinaRegionCode(p.region), city: p.city, cityZh: p.city}
		}
	}
	if !a.placesKnown() {
		return geoPlace{}
	}
	if a.geoOnline.Load() {
		if p := a.online.place(ip, now); p.country == country {
			return geoPlace{region: p.region, city: p.city, cityZh: p.city}
		}
		return geoPlace{}
	}
	if c, place := a.geo.region(ip, now); c == country {
		return place
	}
	return geoPlace{}
}

// chinaRegionCode turns the province cnip.io names ("江苏", "广西壮族自治区") into its ISO 3166-2 code, the form a
// City database stores, so both sources fall into the same rows; an unknown name is kept as it is.
func chinaRegionCode(name string) string {
	name = strings.TrimSpace(name)
	for _, suffix := range []string{"维吾尔自治区", "壮族自治区", "回族自治区", "自治区", "特别行政区", "省", "市"} {
		name = strings.TrimSuffix(name, suffix)
	}
	if code, ok := chinaProvinces[name]; ok {
		return "CN-" + code
	}
	return short(name, 100)
}

var chinaProvinces = map[string]string{
	"北京": "BJ", "天津": "TJ", "河北": "HE", "山西": "SX", "内蒙古": "NM", "辽宁": "LN", "吉林": "JL", "黑龙江": "HL",
	"上海": "SH", "江苏": "JS", "浙江": "ZJ", "安徽": "AH", "福建": "FJ", "江西": "JX", "山东": "SD", "河南": "HA",
	"湖北": "HB", "湖南": "HN", "广东": "GD", "广西": "GX", "海南": "HI", "重庆": "CQ", "四川": "SC", "贵州": "GZ",
	"云南": "YN", "西藏": "XZ", "陕西": "SN", "甘肃": "GS", "青海": "QH", "宁夏": "NX", "新疆": "XJ",
}

// requestCountry names the visitor's country: a CDN's country header when one counts (visitorCountryHeader),
// otherwise the visitor's address looked up in the GeoIP database (-geoip-db, or the one downloaded on the settings page), if the address
// is really the visitor's.
func (a *App) requestCountry(r *http.Request, now time.Time) string {
	if code := a.visitorCountryHeader(r); code != "" {
		return code
	}
	if !a.placesKnown() || !a.visitorKnown(r) {
		return ""
	}
	ip, _ := a.visitorAddr(r)
	if !ip.IsValid() {
		return ""
	}
	if a.geoOnline.Load() {
		return a.online.place(ip, now).country
	}
	return a.geo.country(ip, now)
}
