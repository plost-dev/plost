package plost

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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
	"area-line": true, "bars": true, "bars-today": true, "bars-recent": true, "barcode": true, "bars-average": true, "bars-cap": true,
	"line": true, "line-faint": true, "area": true, "step": true, "dots": true, "heat": true,
}

func (a *App) sparklineSettings(ctx context.Context) (style, span string) {
	style = a.setting(ctx, "sparkline_style", "area-line")
	if !sparklineStyles[style] {
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
	token := a.setting(r.Context(), "mapbox_public_token", "")
	enabled := a.setting(r.Context(), "mapbox_enabled", "true") == "true"
	if r.Method == "GET" {
		return jsonResponse(w, map[string]any{"enabled": enabled, "configured": token != ""})
	}
	var in struct {
		Token   *string `json:"token"`
		Enabled *bool   `json:"enabled"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Token != nil {
		token = strings.TrimSpace(*in.Token)
		if token != "" && (!strings.HasPrefix(token, "pk.") || len(token) > 2048) {
			return failure(400, "只接受 Mapbox 公开 Token")
		}
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
		if enabled && token == "" {
			return failure(400, "请先填写 Mapbox Token")
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES('mapbox_public_token',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", token); err != nil {
		return err
	}
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
	return jsonResponse(w, map[string]bool{"enabled": enabled, "configured": token != ""})
}

func (a *App) groupRows(ctx context.Context, site int64, rng Range, q url.Values, expression, extra string) ([]Row, error) {
	w, args := where(site, rng, q)
	rows, err := a.db.QueryContext(ctx, "SELECT "+expression+",count(*) AS n FROM events_v2 WHERE "+w+extra+" GROUP BY 1 ORDER BY n DESC,1 LIMIT 200", args...)
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
	result := map[string]any{"mapboxToken": a.setting(ctx, "mapbox_public_token", ""), "googleMapsKey": ""}
	if a.setting(ctx, "mapbox_enabled", "true") != "true" {
		result["mapboxToken"] = ""
	}
	if a.setting(ctx, "google_maps_enabled", "false") == "true" {
		result["googleMapsKey"] = a.setting(ctx, "google_maps_key", "")
		result["mapboxToken"] = ""
	}
	channel := `CASE WHEN referrer_source IN ('','Direct') THEN 'direct' WHEN referrer_source IN ('google.com','bing.com','baidu.com','duckduckgo.com','yahoo.com','yandex.com','yandex.ru','search.brave.com','ecosia.org','sogou.com','so.com') THEN 'organic_search' WHEN referrer_source IN ('twitter.com','x.com','t.co','facebook.com','instagram.com','reddit.com','weibo.com','youtube.com','bsky.app','threads.net','tiktok.com') THEN 'organic_social' WHEN referrer_source IN ('chatgpt.com','chat.openai.com','perplexity.ai','claude.ai','gemini.google.com','copilot.microsoft.com','grok.com','deepseek.com','chat.deepseek.com') THEN 'ai' ELSE 'referral' END`
	w0, a0 := where(site.ID, rng, q)
	channelRows, err := a.db.QueryContext(ctx, "SELECT "+channel+",count(DISTINCT "+visitorSQL+") AS n FROM events_v2 WHERE "+w0+" AND name='pageview' GROUP BY 1 ORDER BY n DESC,1", a0...)
	if err != nil {
		return nil, err
	}
	channels := []Row{}
	for channelRows.Next() {
		var v Row
		if err = channelRows.Scan(&v.Name, &v.Value); err != nil {
			channelRows.Close()
			return nil, err
		}
		channels = append(channels, v)
	}
	err = channelRows.Err()
	channelRows.Close()
	if err != nil {
		return nil, err
	}
	result["channels"] = channels
	ai, err := a.groupRows(ctx, site.ID, rng, q, "referrer_source", " AND name='pageview' AND ("+channel+")='ai'")
	if err != nil {
		return nil, err
	}
	result["aiSources"] = ai
	for _, group := range []struct{ name, expression, extra string }{{"titles", "title", " AND name='pageview'"}, {"hostnames", "hostname", " AND name='pageview'"}, {"utm", "utm_source", " AND name='pageview' AND utm_source<>''"}, {"utmMediums", "utm_medium", " AND name='pageview' AND utm_medium<>''"}, {"campaigns", "utm_campaign", " AND name='pageview' AND utm_campaign<>''"}, {"notFoundPages", "pathname", " AND name='404'"}, {"downloads", "COALESCE(json_extract(props,'$.url'),'')", " AND name='File Download'"}, {"outbound", "COALESCE(json_extract(props,'$.url'),'')", " AND name='Outbound Link: Click'"}, {"eventCounts", "name", " AND name NOT IN ('pageview','404')"}} {
		rows, err := a.groupRows(ctx, site.ID, rng, q, group.expression, group.extra)
		if err != nil {
			return nil, err
		}
		result[group.name] = rows
	}
	w, args := where(site.ID, rng, q)
	for _, dim := range []struct{ key, column string }{{"pageVisitors", "pathname"}, {"titles", "title"}, {"hostnames", "hostname"}} {
		rows, err := a.db.QueryContext(ctx, "SELECT "+dim.column+",count(DISTINCT "+visitorSQL+") AS n FROM events_v2 WHERE "+w+" AND name='pageview' AND "+dim.column+"<>'' GROUP BY 1 ORDER BY n DESC,1 LIMIT 200", args...)
		if err != nil {
			return nil, err
		}
		list := []Row{}
		for rows.Next() {
			var v Row
			if err = rows.Scan(&v.Name, &v.Value); err != nil {
				rows.Close()
				return nil, err
			}
			list = append(list, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		result[dim.key] = list
	}
	properties, err := a.db.QueryContext(ctx, "SELECT e.name||' · '||j.key||' · '||CAST(j.value AS TEXT),count(*) AS n FROM events_v2 e,json_each(e.props) j WHERE "+w+" AND name NOT IN ('pageview','404') AND j.type IN ('text','integer','real','true','false') GROUP BY 1 ORDER BY n DESC,1 LIMIT 200", args...)
	if err != nil {
		return nil, err
	}
	propRows := []Row{}
	for properties.Next() {
		var v Row
		if err = properties.Scan(&v.Name, &v.Value); err != nil {
			properties.Close()
			return nil, err
		}
		propRows = append(propRows, v)
	}
	err = properties.Err()
	properties.Close()
	if err != nil {
		return nil, err
	}
	result["properties"] = propRows
	for _, entry := range []bool{true, false} {
		order := "ASC"
		key := "entryPages"
		if !entry {
			order = "DESC"
			key = "exitPages"
		}
		rows, err := a.db.QueryContext(ctx, "SELECT pathname,count(DISTINCT visitor) AS n FROM (SELECT pathname,"+visitorSQL+" AS visitor,row_number() OVER (PARTITION BY "+visitSQL+" ORDER BY timestamp "+order+",id "+order+") AS rn FROM events_v2 WHERE "+w+" AND name='pageview') WHERE rn=1 GROUP BY pathname ORDER BY n DESC,pathname LIMIT 200", args...)
		if err != nil {
			return nil, err
		}
		list := []Row{}
		for rows.Next() {
			var v Row
			if err = rows.Scan(&v.Name, &v.Value); err != nil {
				rows.Close()
				return nil, err
			}
			list = append(list, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		result[key] = list
	}
	heatArgs := append([]any{site.Timezone}, args...)
	rows, err := a.db.QueryContext(ctx, "SELECT plost_local_hour(timestamp,?) AS slot,count(DISTINCT "+visitorSQL+") FROM events_v2 WHERE "+w+" AND name='pageview' GROUP BY slot", heatArgs...)
	if err != nil {
		return nil, err
	}
	heat := []map[string]int64{}
	for rows.Next() {
		var slot, count int64
		if err = rows.Scan(&slot, &count); err != nil {
			rows.Close()
			return nil, err
		}
		heat = append(heat, map[string]int64{"dow": slot / 24, "hour": slot % 24, "value": count})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result["heatmap"] = heat
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
	key := a.setting(r.Context(), "google_maps_key", "")
	enabled := a.setting(r.Context(), "google_maps_enabled", "false") == "true"
	if r.Method == "GET" {
		return jsonResponse(w, googleMapsState(enabled, key))
	}
	var in struct {
		Key     *string `json:"key"`
		Enabled *bool   `json:"enabled"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Key != nil {
		key = strings.TrimSpace(*in.Key)
		if key != "" && !googleMapsKey.MatchString(key) {
			return failure(400, "Google 地图 API Key 格式不对，应以 AIza 开头、共 39 个字符")
		}
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if enabled && key == "" {
		return failure(400, "请先填写 Google 地图 API Key")
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
	for _, kv := range [][2]string{{"google_maps_key", key}, {"google_maps_enabled", flag}} {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", kv[0], kv[1]); err != nil {
			return err
		}
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
	return jsonResponse(w, googleMapsState(enabled, key))
}

// googleMapsState is what the settings page sees: whether Google Maps is on and whether a key is saved, never the key.
func googleMapsState(enabled bool, key string) map[string]any {
	return map[string]any{"enabled": enabled, "configured": key != ""}
}
