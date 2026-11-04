package plost

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"modernc.org/sqlite"
)

var uiLocations sync.Map

func uiLocation(zone string) (*time.Location, error) {
	if v, ok := uiLocations.Load(zone); ok {
		return v.(*time.Location), nil
	}
	loc, err := time.LoadLocation(zone)
	if err == nil {
		uiLocations.Store(zone, loc)
	}
	return loc, err
}

func init() {
	err := sqlite.RegisterDeterministicScalarFunction("plost_local_hour", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		ts, ok := args[0].(int64)
		if !ok {
			return nil, nil
		}
		zone, _ := args[1].(string)
		loc, err := uiLocation(zone)
		if err != nil {
			return nil, err
		}
		t := time.Unix(ts, 0).In(loc)
		dow := (int(t.Weekday())+6)%7 + 1
		return int64(dow*24 + t.Hour()), nil
	})
	if err != nil {
		panic(err)
	}
	// Local calendar day (days since 1970-01-01) times 24 plus the local hour, for per-date grids.
	err = sqlite.RegisterDeterministicScalarFunction("plost_local_day_hour", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		ts, ok := args[0].(int64)
		if !ok {
			return nil, nil
		}
		zone, _ := args[1].(string)
		loc, err := uiLocation(zone)
		if err != nil {
			return nil, err
		}
		t := time.Unix(ts, 0).In(loc)
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix() / 86400
		return day*24 + int64(t.Hour()), nil
	})
	if err != nil {
		panic(err)
	}
}

func (a *App) setting(ctx context.Context, key, fallback string) string {
	var value string
	if a.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&value) != nil {
		return fallback
	}
	return value
}
func (a *App) preferences(w http.ResponseWriter, r *http.Request) error {
	style, span := a.sparklineSettings(r.Context())
	return jsonResponse(w, map[string]string{"mapboxStyle": a.setting(r.Context(), "mapbox_style", "standard"), "googleMapStyle": a.setting(r.Context(), "google_map_style", "roadmap"), "sparklineStyle": style, "sparklineRange": span})
}

// The small trend on each site card in the site list: one of these styles, over the last 24 hours (hourly) or the
// last 30 days (daily). The frontend draws them (SiteSparkline); "area-line" is the original look.
var sparklineStyles = map[string]bool{
	"area-line": true, "smooth": true, "peak": true, "bars": true, "change": true, "heat": true,
}

// retiredSparklineStyles were dropped as too alike or hard to read (2026-11-03, and mirror, blocks, step, dots,
// bars-average and lollipop on 2026-11-04); an account that saved one keeps the nearest look.
var retiredSparklineStyles = map[string]string{
	"bars-today": "bars", "bars-recent": "bars", "bars-cap": "bars", "barcode": "bars",
	"line": "area-line", "line-faint": "area-line", "area": "smooth",
	"mirror": "bars", "blocks": "bars", "step": "area-line", "dots": "bars", "bars-average": "bars", "lollipop": "bars",
}

func (a *App) sparklineSettings(ctx context.Context) (style, span string) {
	style = a.setting(ctx, "sparkline_style", "area-line")
	if near, ok := retiredSparklineStyles[style]; ok {
		style = near
	} else if !sparklineStyles[style] {
		style = "area-line"
	}
	if span = a.setting(ctx, "sparkline_range", "24h"); span != "30d" {
		span = "24h"
	}
	return style, span
}

func (a *App) sparklineSettingsPut(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Style string `json:"style"`
		Range string `json:"range"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if !sparklineStyles[in.Style] || (in.Range != "24h" && in.Range != "30d") {
		return failure(400, "走势图样式无效")
	}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('sparkline_style',?),('sparkline_range',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", in.Style, in.Range)
	if err != nil {
		return err
	}
	return jsonResponse(w, in)
}
func (a *App) mapStyle(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		StyleID string `json:"styleId"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.StyleID != "standard" && in.StyleID != "streets-v12" && in.StyleID != "dark-v11" {
		return failure(400, "地图样式无效")
	}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('mapbox_style',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", in.StyleID)
	if err != nil {
		return err
	}
	return jsonResponse(w, in)
}
func (a *App) googleMapStyle(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		StyleID string `json:"styleId"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.StyleID != "standard" && in.StyleID != "roadmap" && in.StyleID != "dark" {
		return failure(400, "地图样式无效")
	}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('google_map_style',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", in.StyleID)
	if err != nil {
		return err
	}
	return jsonResponse(w, in)
}
func (a *App) mapboxSettings(w http.ResponseWriter, r *http.Request) error {
	// The token itself is only ever in the env file (env_keys.go); the database keeps the switch.
	token, _ := a.mapKeys()
	enabled := a.setting(r.Context(), "mapbox_enabled", "true") == "true"
	if r.Method == "GET" {
		return jsonResponse(w, a.mapKeyState(enabled, token))
	}
	var in struct {
		Enabled *bool   `json:"enabled"`
		Token   *string `json:"token"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Token != nil {
		if err := a.saveMapKey(EnvMapboxToken, *in.Token); err != nil {
			return err
		}
		token, _ = a.mapKeys()
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
		if enabled && token == "" {
			return failure(400, "请先填写并保存 Mapbox Token")
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	flag := "false"
	if enabled {
		flag = "true"
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('mapbox_enabled',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", flag); err != nil {
		return err
	}
	// One map service at a time: turning Mapbox on turns Google Maps off.
	if enabled {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('google_maps_enabled','false') ON CONFLICT(key) DO UPDATE SET value=excluded.value"); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if enabled {
		a.googleMaps.Store(false)
	}
	return jsonResponse(w, a.mapKeyState(enabled, token))
}

func (a *App) groupRows(ctx context.Context, site int64, rng Range, q url.Values, expression, extra string, visitors ...bool) ([]Row, error) {
	w, args := where(site, rng, q)
	count := "count(*)"
	if len(visitors) > 0 && visitors[0] {
		count = "count(DISTINCT " + visitorSQL + ")"
	}
	rows, err := a.rdb.QueryContext(ctx, "SELECT "+expression+","+count+" AS n FROM events_v2 WHERE "+w+extra+" GROUP BY 1 ORDER BY n DESC,1 LIMIT 200", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Name, &r.Value); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (a *App) reportDetails(ctx context.Context, site Site, rng Range, q url.Values) (map[string]any, error) {
	result := map[string]any{"mapboxToken": "", "googleMapsKey": "", "mapOutline": a.setting(ctx, mapOutline, "regions")}
	// The statistics pages show the chosen map service unless it is set to the overview only.
	if service, key := a.mapService(ctx); service != "" && a.setting(ctx, mapUseDetail, "true") == "true" {
		result[map[string]string{"mapbox": "mapboxToken", "google": "googleMapsKey"}[service]] = key
	}
	var mu sync.Mutex
	set := func(key string, value any) {
		mu.Lock()
		result[key] = value
		mu.Unlock()
	}
	list := func(key, query string, args []any) func(context.Context) error {
		return func(ctx context.Context) error {
			rows, err := collectRows(a.rdb.QueryContext(ctx, query, args...))
			if err == nil {
				set(key, rows)
			}
			return err
		}
	}
	w, args := where(site.ID, rng, q)
	tasks := []func(context.Context) error{
		list("channels", "SELECT "+channelSQL+",count(DISTINCT "+visitorSQL+") AS n FROM events_v2 WHERE "+w+" AND name='pageview' GROUP BY 1 ORDER BY n DESC,1", args),
		// Visitors, like the channel list and the column heading, not pageviews.
		func(ctx context.Context) error {
			rows, err := a.groupRows(ctx, site.ID, rng, q, "referrer_source", " AND name='pageview' AND ("+channelSQL+")='ai'", true)
			if err == nil {
				set("aiSources", rows)
			}
			return err
		},
		list("properties", "SELECT e.name||' · '||j.key||' · '||CAST(j.value AS TEXT),count(*) AS n FROM events_v2 e,json_each(e.props) j WHERE "+w+" AND name NOT IN ('pageview','404') AND j.type IN ('text','integer','real','true','false') GROUP BY 1 ORDER BY n DESC,1 LIMIT 200", args),
	}
	for _, group := range []struct{ name, expression, extra string }{{"utm", "utm_source", " AND name='pageview' AND utm_source<>''"}, {"utmMediums", "utm_medium", " AND name='pageview' AND utm_medium<>''"}, {"campaigns", "utm_campaign", " AND name='pageview' AND utm_campaign<>''"}, {"notFoundPages", "pathname", " AND name='404'"}, {"downloads", "COALESCE(json_extract(props,'$.url'),'')", " AND name='File Download'"}, {"outbound", "COALESCE(json_extract(props,'$.url'),'')", " AND name='Outbound Link: Click'"}, {"eventCounts", "name", " AND name NOT IN ('pageview','404')"}} {
		tasks = append(tasks, func(ctx context.Context) error {
			rows, err := a.groupRows(ctx, site.ID, rng, q, group.expression, group.extra)
			if err == nil {
				set(group.name, rows)
			}
			return err
		})
	}
	for _, dim := range []struct{ key, column string }{{"pageVisitors", "pathname"}, {"titles", "title"}, {"hostnames", "hostname"}} {
		tasks = append(tasks, list(dim.key, "SELECT "+dim.column+",count(DISTINCT "+visitorSQL+") AS n FROM events_v2 WHERE "+w+" AND name='pageview' AND "+dim.column+"<>'' GROUP BY 1 ORDER BY n DESC,1 LIMIT 200", args))
	}
	for _, entry := range []struct{ key, order string }{{"entryPages", "ASC"}, {"exitPages", "DESC"}} {
		tasks = append(tasks, list(entry.key, "SELECT pathname,count(DISTINCT visitor) AS n FROM (SELECT pathname,"+visitorSQL+" AS visitor,row_number() OVER (PARTITION BY "+visitSQL+" ORDER BY timestamp "+entry.order+",id "+entry.order+") AS rn FROM events_v2 WHERE "+w+" AND name='pageview') WHERE rn=1 GROUP BY pathname ORDER BY n DESC,pathname LIMIT 200", args))
	}
	// The visiting-hours grid always covers the last seven local days ending today,
	// whatever range is chosen, so every row is one real date and today is the last row.
	loc, err := uiLocation(site.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	firstDay := time.Date(now.Year(), now.Month(), now.Day()-6, 0, 0, 0, 0, loc)
	week := Range{From: firstDay, To: time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)}
	tasks = append(tasks, func(ctx context.Context) error {
		hw, ha := where(site.ID, week, q)
		heatArgs := append([]any{site.Timezone}, ha...)
		rows, err := a.rdb.QueryContext(ctx, "SELECT plost_local_day_hour(timestamp,?) AS slot,count(DISTINCT "+visitorSQL+") FROM events_v2 WHERE "+hw+" AND name='pageview' GROUP BY slot", heatArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		heat := []map[string]any{}
		for rows.Next() {
			var slot, count int64
			if err := rows.Scan(&slot, &count); err != nil {
				return err
			}
			day := time.Unix(slot/24*86400, 0).UTC()
			heat = append(heat, map[string]any{"date": day.Format("2006-01-02"), "dow": (int(day.Weekday())+6)%7 + 1, "hour": slot % 24, "value": count})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		set("heatmap", heat)
		return nil
	})
	if err := parallel(ctx, tasks...); err != nil {
		return nil, err
	}
	days := make([]map[string]any, 7)
	for i := range days {
		d := time.Date(firstDay.Year(), firstDay.Month(), firstDay.Day()+i, 0, 0, 0, 0, time.UTC)
		days[i] = map[string]any{"date": d.Format("2006-01-02"), "dow": (int(d.Weekday())+6)%7 + 1}
	}
	result["heatmapDays"] = days
	result["heatmapHour"] = now.Hour()
	return result, nil
}

type RecentVisitor struct {
	ID        string   `json:"id"`
	Pathname  string   `json:"pathname"`
	Source    string   `json:"source"`
	Country   string   `json:"country"`
	Browser   string   `json:"browser"`
	OS        string   `json:"os"`
	Device    string   `json:"device"`
	Screen    string   `json:"screen"`
	Language  string   `json:"language"`
	FirstSeen string   `json:"firstSeen"`
	LastSeen  string   `json:"lastSeen"`
	Active    bool     `json:"active"`
	Pageviews int64    `json:"pageviews"`
	Events    int64    `json:"events"`
	Pages     []string `json:"pages"`
}

func (a *App) visitors(w http.ResponseWriter, r *http.Request) error {
	site, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	// Aggregation is done in SQLite; the Go heap only holds the bounded result.
	rows, err := a.db.QueryContext(r.Context(), `SELECT user_id,min(timestamp),max(timestamp),sum(name='pageview'),count(*),json_group_array(DISTINCT pathname),
 (SELECT pathname FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT referrer_source FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT country_code FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT browser FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT os FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT device FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT screen FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1),
 (SELECT language FROM events_v2 last WHERE last.site_id=e.site_id AND last.user_id=e.user_id ORDER BY timestamp DESC,id DESC LIMIT 1)
 FROM events_v2 e WHERE site_id=? AND timestamp>=? GROUP BY user_id ORDER BY max(timestamp) DESC LIMIT 200`, site.ID, unixNow()-1800)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []RecentVisitor{}
	for rows.Next() {
		var v RecentVisitor
		var first, last int64
		var pages string
		if err = rows.Scan(&v.ID, &first, &last, &v.Pageviews, &v.Events, &pages, &v.Pathname, &v.Source, &v.Country, &v.Browser, &v.OS, &v.Device, &v.Screen, &v.Language); err != nil {
			return err
		}
		v.FirstSeen = time.Unix(first, 0).UTC().Format(time.RFC3339)
		v.LastSeen = time.Unix(last, 0).UTC().Format(time.RFC3339)
		v.Active = last >= unixNow()-300
		_ = json.Unmarshal([]byte(pages), &v.Pages)
		if len(v.Pages) > 20 {
			v.Pages = v.Pages[:20]
		}
		list = append(list, v)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	var total, online int
	if err = a.db.QueryRowContext(r.Context(), "SELECT count(DISTINCT user_id),count(DISTINCT CASE WHEN timestamp>=? THEN user_id END) FROM events_v2 WHERE site_id=? AND timestamp>=?", unixNow()-300, site.ID, unixNow()-1800).Scan(&total, &online); err != nil {
		return err
	}
	return jsonResponse(w, map[string]any{"rows": list, "total": total, "online": online, "timezone": site.Timezone, "fetchedAt": time.Now().UTC().Format(time.RFC3339)})
}

type cachedTrend struct {
	until  time.Time
	points []Point
}

// dailyTrend is a site's last 30 days, one point a day, for its card in the site list. Thirty days of events is far
// more to read than the 24 hours the card's numbers cover, and the list refreshes every half minute, so the result
// is kept for five minutes.
func (a *App) dailyTrend(ctx context.Context, s Site) ([]Point, error) {
	now := time.Now()
	a.trendMu.Lock()
	cached, ok := a.trends[s.ID]
	a.trendMu.Unlock()
	if ok && now.Before(cached.until) {
		return cached.points, nil
	}
	rng, err := parseRange(url.Values{"period": {"30d"}}, s.Timezone)
	if err != nil {
		return nil, err
	}
	points, err := a.series(ctx, s.ID, rng, nil)
	if err != nil {
		return nil, err
	}
	a.trendMu.Lock()
	if a.trends == nil {
		a.trends = map[int64]cachedTrend{}
	}
	a.trends[s.ID] = cachedTrend{until: now.Add(5 * time.Minute), points: points}
	a.trendMu.Unlock()
	return points, nil
}

// googleMapsKey is a browser key for the Maps JavaScript API, which Google issues as "AIza" and 35 more characters.
var googleMapsKey = regexp.MustCompile(`^AIza[0-9A-Za-z_-]{35}$`)

// googleMapsSettings is the Google Maps alternative to Mapbox for the visitor map. The key is sent to every
// dashboard and share page that shows the map, so it must be a browser key limited to this site's referrer.
func (a *App) googleMapsSettings(w http.ResponseWriter, r *http.Request) error {
	// The key itself is only ever in the env file (env_keys.go); the database keeps the switch.
	_, key := a.mapKeys()
	enabled := a.googleMaps.Load()
	if r.Method == "GET" {
		return jsonResponse(w, a.mapKeyState(enabled, key))
	}
	var in struct {
		Enabled *bool   `json:"enabled"`
		Key     *string `json:"key"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Key != nil {
		if err := a.saveMapKey(EnvGoogleMapsKey, *in.Key); err != nil {
			return err
		}
		_, key = a.mapKeys()
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if enabled && key == "" {
		return failure(400, "请先填写并保存 Google Maps API Key")
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	flag := "false"
	if enabled {
		flag = "true"
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('google_maps_enabled',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", flag); err != nil {
		return err
	}
	// One map service at a time: turning Google Maps on turns Mapbox off.
	if enabled {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('mapbox_enabled','false') ON CONFLICT(key) DO UPDATE SET value=excluded.value"); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	a.googleMaps.Store(enabled)
	return jsonResponse(w, a.mapKeyState(enabled, key))
}

// mapKeyState is what the settings page sees of a map service: whether it is on, whether a key is set (shown only
// by its start), and whether the page may save one into the env file.
func (a *App) mapKeyState(enabled bool, key string) map[string]any {
	return map[string]any{"enabled": enabled, "configured": key != "", "masked": maskKey(key), "editable": a.cfg.EnvFile != ""}
}

// Where the chosen map service (Mapbox or Google Maps) is used: on each site's statistics page, on the site
// overview's globe, or both. Wherever it is not used, or no service is on, the built-in map is shown.
const (
	mapUseDetail   = "map_use_detail"   // On by default: the statistics pages used the service before this setting.
	mapUseOverview = "map_use_overview" // Off by default: the overview keeps its dotted globe.
	// The built-in map's outlines: "regions" (countries with their provinces, the default) or "countries" only, a
	// quarter of the download and much less to draw, for those who want the page as light as it can be.
	mapOutline = "map_outline"
)

func (a *App) mapUsage(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "PUT" {
		var in struct {
			Detail   *bool   `json:"detail"`
			Overview *bool   `json:"overview"`
			Outline  *string `json:"outline"`
		}
		if err := readJSON(w, r, &in); err != nil {
			return err
		}
		if in.Outline != nil {
			if *in.Outline != "regions" && *in.Outline != "countries" {
				return failure(400, "未知的地图轮廓")
			}
			if _, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", mapOutline, *in.Outline); err != nil {
				return err
			}
		}
		for key, value := range map[string]*bool{mapUseDetail: in.Detail, mapUseOverview: in.Overview} {
			if value == nil {
				continue
			}
			if _, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, strconv.FormatBool(*value)); err != nil {
				return err
			}
		}
	}
	return jsonResponse(w, map[string]any{
		"detail":   a.setting(r.Context(), mapUseDetail, "true") == "true",
		"overview": a.setting(r.Context(), mapUseOverview, "false") == "true",
		"outline":  a.setting(r.Context(), mapOutline, "regions"),
	})
}

// mapService is the map service that is on and has its key, with that key, or "" for the built-in map.
func (a *App) mapService(ctx context.Context) (service, key string) {
	mapboxToken, googleKey := a.mapKeys()
	if a.googleMaps.Load() && googleKey != "" {
		return "google", googleKey
	}
	if a.setting(ctx, "mapbox_enabled", "true") == "true" && mapboxToken != "" {
		return "mapbox", mapboxToken
	}
	return "", ""
}

// overviewMap tells the site overview which map to draw: the service with its browser key when the overview is set to
// use it, otherwise nothing and the page keeps its built-in globe.
func (a *App) overviewMap(w http.ResponseWriter, r *http.Request) error {
	service, key := a.mapService(r.Context())
	if service == "" || a.setting(r.Context(), mapUseOverview, "false") != "true" {
		return jsonResponse(w, map[string]string{"service": ""})
	}
	return jsonResponse(w, map[string]string{"service": service, "key": key})
}
