package plost

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func strconvID(id int64) string { return strconv.FormatInt(id, 10) }

type Range struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Period string    `json:"period"`
}
type Overview struct {
	Visitors   int64   `json:"visitors"`
	Visits     int64   `json:"visits"`
	Pageviews  int64   `json:"pageviews"`
	Online     int64   `json:"online"`
	BounceRate float64 `json:"bounceRate"`
	Duration   float64 `json:"duration"`
}
type Row struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}
type RegionRow struct {
	Country string `json:"country"`
	Name    string `json:"name"`
	Value   int64  `json:"value"`
}
type Point struct {
	At         int64   `json:"at"`
	Visitors   int64   `json:"visitors"`
	Pageviews  int64   `json:"pageviews"`
	Visits     int64   `json:"visits"`
	BounceRate float64 `json:"bounceRate"`
	Duration   float64 `json:"duration"`
}
type Report struct {
	Site      Site     `json:"site"`
	Range     Range    `json:"range"`
	Overview  Overview `json:"overview"`
	Previous  Overview `json:"previous"`
	Series    []Point  `json:"series"`
	Sources   []Row    `json:"sources"`
	Pages     []Row    `json:"pages"`
	Countries []Row    `json:"countries"`
	// Regions counts visitors by province or state, for the map: an ISO 3166-2 code, or the name the online GeoIP
	// service gave, under its country.
	Regions   []RegionRow `json:"regions"`
	Browsers  []Row       `json:"browsers"`
	OS        []Row       `json:"os"`
	Devices   []Row       `json:"devices"`
	Languages []Row       `json:"languages"`
	Screens   []Row       `json:"screens"`
	Events    []Row       `json:"events"`
	Readonly  bool        `json:"readonly"`
	// Live counts the visitors online now (the last five minutes, like Overview.Online) by country, for the map, and
	// LivePoints places them where a City GeoIP database could (livepoints.go).
	Live []Row `json:"live"`
	// LiveRegions counts the same visitors by province or state where one was recorded, so the map can place those it
	// has no city for at their province instead of the middle of their country.
	LiveRegions []RegionRow    `json:"liveRegions"`
	LivePoints  []LivePoint    `json:"livePoints"`
	Details     map[string]any `json:"details,omitempty"`
}

func parseRange(q url.Values, zone string) (Range, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	r := Range{From: midnight, To: now.Add(time.Second), Period: q.Get("period")}
	if r.Period == "" {
		r.Period = "today"
	}
	switch r.Period {
	case "today":
	case "yesterday":
		r.From = midnight.AddDate(0, 0, -1)
		r.To = midnight
	case "24h":
		r.From = now.Add(-24 * time.Hour)
	case "realtime":
		r.From = now.Add(-30 * time.Minute)
	case "7d":
		r.From = midnight.AddDate(0, 0, -6)
	case "30d", "28d":
		days := 29
		if r.Period == "28d" {
			days = 27
		}
		r.From = midnight.AddDate(0, 0, -days)
	case "all":
		r.From = time.Unix(0, 0).In(loc)
	case "91d":
		r.From = midnight.AddDate(0, 0, -90)
	case "month":
		r.From = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	case "last_month":
		r.To = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		r.From = r.To.AddDate(0, -1, 0)
	case "year":
		r.From = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
	case "12mo":
		r.From = midnight.AddDate(-1, 0, 1)
	case "custom":
		from, e := time.ParseInLocation("2006-01-02", q.Get("from"), loc)
		if e != nil {
			return r, failure(400, "日期无效")
		}
		to, e := time.ParseInLocation("2006-01-02", q.Get("to"), loc)
		if e != nil || to.Before(from) || to.Sub(from) > 3660*24*time.Hour {
			return r, failure(400, "日期范围无效")
		}
		r.From = from
		r.To = to.AddDate(0, 0, 1)
	default:
		return r, failure(400, "不支持的统计时间范围")
	}
	return r, nil
}

var filterColumns = map[string]string{"source": "referrer_source", "page": "pathname", "country": "country_code", "region": "region", "browser": "browser", "os": "os", "device": "device", "language": "language", "screen": "screen", "hostname": "hostname"}

func where(site int64, r Range, q url.Values) (string, []any) {
	w := "site_id=? AND timestamp>=? AND timestamp<?"
	args := []any{site, r.From.Unix(), r.To.Unix()}
	for _, k := range []string{"source", "page", "country", "region", "browser", "os", "device", "language", "screen", "hostname"} {
		if values, ok := q[k]; ok && len(values) > 0 {
			w += " AND " + filterColumns[k] + "=?"
			args = append(args, short(values[0], 2048))
		}
	}
	return w, args
}

func (a *App) overview(ctx context.Context, site int64, r Range, q url.Values) (Overview, error) {
	w, args := where(site, r, q)
	var o Overview
	err := a.db.QueryRowContext(ctx, "SELECT count(DISTINCT "+visitorSQL+"),count(DISTINCT "+visitSQL+"),count(*) FROM events_v2 WHERE "+w+" AND name='pageview'", args...).Scan(&o.Visitors, &o.Visits, &o.Pageviews)
	if err != nil {
		return o, err
	}
	if o.Visits > 0 {
		err = a.db.QueryRowContext(ctx, `SELECT COALESCE(100.0*sum(pv=1)/count(*),0),COALESCE(avg(dur),0) FROM (SELECT count(*) AS pv,max(timestamp)-min(timestamp) AS dur FROM events_v2 WHERE `+w+` AND name='pageview' GROUP BY `+visitSQL+`)`, args...).Scan(&o.BounceRate, &o.Duration)
		if err != nil {
			return o, err
		}
	}
	live := Range{From: time.Now().Add(-5 * time.Minute), To: time.Now().Add(time.Second)}
	lw, la := where(site, live, q)
	err = a.db.QueryRowContext(ctx, "SELECT count(DISTINCT "+visitorSQL+") FROM events_v2 WHERE "+lw, la...).Scan(&o.Online)
	return o, err
}

func (a *App) rows(ctx context.Context, site int64, r Range, q url.Values, column string, pageviews bool, events bool) ([]Row, error) {
	allowed := map[string]bool{"referrer_source": true, "pathname": true, "country_code": true, "browser": true, "os": true, "device": true, "language": true, "screen": true, "name": true}
	if !allowed[column] {
		return nil, fmt.Errorf("invalid dimension")
	}
	w, args := where(site, r, q)
	metric := "count(DISTINCT " + visitorSQL + ")"
	if pageviews || events {
		metric = "count(*)"
	}
	if events {
		w += " AND name<>'pageview'"
	} else {
		w += " AND name='pageview'"
	}
	rows, err := a.db.QueryContext(ctx, "SELECT "+column+","+metric+" AS n FROM events_v2 WHERE "+w+" GROUP BY "+column+" ORDER BY n DESC,"+column+" LIMIT 50", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var v Row
		if err = rows.Scan(&v.Name, &v.Value); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (a *App) series(ctx context.Context, site int64, r Range, q url.Values) ([]Point, error) {
	start := r.From
	if r.Period == "all" {
		var earliest sql.NullInt64
		if err := a.db.QueryRowContext(ctx, "SELECT min(timestamp) FROM events_v2 WHERE site_id=?", site).Scan(&earliest); err != nil {
			return nil, err
		}
		if !earliest.Valid {
			return []Point{}, nil
		}
		t := time.Unix(earliest.Int64, 0).In(start.Location())
		start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	span := r.To.Sub(start)
	hourly := span <= 26*time.Hour
	monthly := span > 93*24*time.Hour
	interval := q.Get("interval")
	if interval != "" {
		hourly = interval == "hour" || interval == "minute"
		monthly = interval == "month"
	}
	if hourly {
		minute := 0
		if interval == "minute" {
			minute = start.Minute()
		}
		start = time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), minute, 0, 0, start.Location())
	}
	if monthly {
		start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	}
	bounds := []time.Time{start}
	for bounds[len(bounds)-1].Before(r.To) && len(bounds) < 3000 {
		last := bounds[len(bounds)-1]
		next := last.AddDate(0, 0, 1)
		if hourly {
			next = last.Add(time.Hour)
			if interval == "minute" {
				next = last.Add(time.Minute)
			}
		} else if monthly {
			next = last.AddDate(0, 1, 0)
		} else if interval == "week" {
			next = last.AddDate(0, 0, 7)
		}
		bounds = append(bounds, next)
	}
	if bounds[len(bounds)-1].Before(r.To) {
		return nil, failure(400, "图表间隔过小，请扩大间隔")
	}
	points := []Point{}
	for _, b := range bounds[:len(bounds)-1] {
		points = append(points, Point{At: b.Unix()})
	}
	if len(points) == 0 {
		return points, nil
	}
	// Explicit calendar boundaries keep daily grouping correct across DST and non-hour timezones.
	caseSQL := "CASE"
	args := []any{}
	for i, b := range bounds[1:] {
		caseSQL += " WHEN timestamp<? THEN " + strconv.Itoa(i)
		args = append(args, b.Unix())
	}
	caseSQL += " ELSE -1 END"
	w, wa := where(site, r, q)
	args = append(args, wa...)
	// Pageviews fall in the bucket they happened in. A visit belongs to the bucket it started in, so a visit that runs
	// past a bucket boundary is counted once and the buckets add up to the overview's visit total; its bounce and
	// duration are measured over the whole visit. Within a day (hour and minute buckets) a visitor is counted in the
	// bucket of their first pageview in the range, so the buckets also add up to the visitor total and the gap to
	// visits shows the return visits. Longer buckets keep the usual distinct visitors per day, week or month.
	visitorBucket := "SELECT bucket,count(DISTINCT visitor) AS n FROM ev GROUP BY bucket"
	if hourly {
		visitorBucket = "SELECT bucket,count(*) AS n FROM (SELECT min(bucket) AS bucket FROM ev GROUP BY visitor) GROUP BY bucket"
	}
	query := "WITH ev AS (SELECT " + caseSQL + " AS bucket," + visitorSQL + " AS visitor," + visitSQL + " AS visit,timestamp FROM events_v2 WHERE " + w + " AND name='pageview'), " +
		"vi AS (SELECT min(bucket) AS bucket,count(*) AS pv,max(timestamp)-min(timestamp) AS dur FROM ev GROUP BY visit), " +
		"b AS (SELECT bucket,count(*) AS pv FROM ev GROUP BY bucket), " +
		"va AS (SELECT bucket,count(*) AS n,100.0*sum(pv=1)/count(*) AS bounce,avg(dur) AS dur FROM vi GROUP BY bucket), " +
		"vu AS (" + visitorBucket + ") " +
		"SELECT b.bucket,COALESCE(vu.n,0),b.pv,COALESCE(va.n,0),COALESCE(va.bounce,0),COALESCE(va.dur,0) FROM b LEFT JOIN vu ON vu.bucket=b.bucket LEFT JOIN va ON va.bucket=b.bucket"
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var i int
		var uv, pv int64
		var visits int64
		var bounce, duration float64
		if err := rows.Scan(&i, &uv, &pv, &visits, &bounce, &duration); err != nil {
			return nil, err
		}
		if i >= 0 && i < len(points) {
			points[i].Visitors = uv
			points[i].Pageviews = pv
			points[i].Visits = visits
			points[i].BounceRate = bounce
			points[i].Duration = duration
		}
	}
	return points, rows.Err()
}

func (a *App) report(r *http.Request, site Site, readonly bool) (Report, error) {
	q := r.URL.Query()
	rng, err := parseRange(q, site.Timezone)
	if err != nil {
		return Report{}, err
	}
	result := Report{Site: site, Range: rng, Readonly: readonly}
	result.Overview, err = a.overview(r.Context(), site.ID, rng, q)
	if err != nil {
		return result, err
	}
	if rng.Period != "all" {
		prev := Range{From: rng.From.Add(-rng.To.Sub(rng.From)), To: rng.From, Period: rng.Period}
		// Full calendar periods compare with full preceding calendar periods.
		if rng.Period == "yesterday" {
			prev.From = rng.From.AddDate(0, 0, -1)
		}
		result.Previous, err = a.overview(r.Context(), site.ID, prev, q)
		if err != nil {
			return result, err
		}
	}
	result.Series, err = a.series(r.Context(), site.ID, rng, q)
	if err != nil {
		return result, err
	}
	for _, v := range []struct {
		col        string
		out        *[]Row
		pv, events bool
	}{
		{"referrer_source", &result.Sources, false, false}, {"pathname", &result.Pages, true, false}, {"country_code", &result.Countries, false, false}, {"browser", &result.Browsers, false, false}, {"os", &result.OS, false, false}, {"device", &result.Devices, false, false}, {"language", &result.Languages, false, false}, {"screen", &result.Screens, false, false}, {"name", &result.Events, false, true},
	} {
		*v.out, err = a.rows(r.Context(), site.ID, rng, q, v.col, v.pv, v.events)
		if err != nil {
			return result, err
		}
	}
	result.Regions, err = a.regions(r.Context(), site.ID, rng, q)
	if err != nil {
		return result, err
	}
	result.Live, err = a.liveCountries(r.Context(), site.ID, q)
	if err != nil {
		return result, err
	}
	result.LiveRegions, err = a.liveRegions(r.Context(), site.ID, q)
	if err != nil {
		return result, err
	}
	result.LivePoints = a.live.points(site.ID, time.Now(), strings.ToUpper(q.Get("country")))
	if q.Get("ui") == "1" {
		result.Details, err = a.reportDetails(r.Context(), site, rng, q)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (a *App) siteReport(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	u, _ := a.user(r)
	if u == nil && !s.Public {
		return failure(401, "请先登录，或使用分享链接")
	}
	report, err := a.report(r, s, u == nil)
	if err != nil {
		return err
	}
	return jsonResponse(w, report)
}
func (a *App) shareReport(w http.ResponseWriter, r *http.Request) error {
	s, err := a.sharedSite(r, r.PathValue("token"))
	if err != nil {
		return failure(404, "分享不存在或已撤销")
	}
	report, err := a.report(r, s, true)
	if err != nil {
		return err
	}
	return jsonResponse(w, report)
}

func (a *App) activity(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	rng, err := parseRange(r.URL.Query(), s.Timezone)
	if err != nil {
		return err
	}
	where, args := where(s.ID, rng, r.URL.Query())
	rows, err := a.db.QueryContext(r.Context(), "SELECT timestamp,name,pathname,referrer_source,country_code,browser,os,device,screen FROM events_v2 WHERE "+where+" ORDER BY timestamp DESC,id DESC LIMIT 201", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var at int64
		var name, page, source, country, browser, osName, device, screen string
		if err := rows.Scan(&at, &name, &page, &source, &country, &browser, &osName, &device, &screen); err != nil {
			return err
		}
		result = append(result, map[string]any{"at": at, "name": name, "page": page, "source": source, "country": country, "browser": browser, "os": osName, "device": device, "screen": screen})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return jsonResponse(w, result)
}

func (a *App) statsAPI(w http.ResponseWriter, r *http.Request) error {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") {
		return failure(401, "需要 Bearer API Key")
	}
	// An API key, or a share link's credential as before API keys existed.
	s, err := a.apiKeySite(r.Context(), token)
	if err != nil {
		s, err = a.sharedSite(r, token)
	}
	if err != nil {
		return failure(401, "API Key 无效或已删除")
	}
	if !a.limits.allow("stats", token, 60, time.Minute) {
		w.Header().Set("Retry-After", "60")
		return failure(429, "请求过于频繁")
	}
	for key, values := range r.URL.Query() {
		if (key != "period" && key != "domain") || len(values) != 1 {
			return failure(400, "只支持单个 period 和 domain 参数")
		}
	}
	if d := r.URL.Query().Get("domain"); d != "" && d != s.Domain {
		return failure(403, "凭证不允许读取这个网站")
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "summary"
	}
	if period != "summary" && period != "live" && period != "today" && period != "yesterday" {
		return failure(400, "不支持的统计时间范围")
	}
	today, _ := parseRange(url.Values{"period": {"today"}}, s.Timezone)
	yesterday, _ := parseRange(url.Values{"period": {"yesterday"}}, s.Timezone)
	t, err := a.overview(r.Context(), s.ID, today, nil)
	if err != nil {
		return err
	}
	y, err := a.overview(r.Context(), s.ID, yesterday, nil)
	if err != nil {
		return err
	}
	todayData := map[string]any{"date": today.From.Format("2006-01-02"), "visitors": t.Visitors, "visits": t.Visits, "pageviews": t.Pageviews}
	yesterdayData := map[string]any{"date": yesterday.From.Format("2006-01-02"), "visitors": y.Visitors, "visits": y.Visits, "pageviews": y.Pageviews}
	var data any = map[string]any{"online": t.Online, "today": todayData, "yesterday": yesterdayData}
	switch period {
	case "live":
		data = map[string]int64{"online": t.Online}
	case "today":
		data = todayData
	case "yesterday":
		data = yesterdayData
	}
	return jsonResponse(w, map[string]any{"apiVersion": "1", "site": map[string]string{"domain": s.Domain, "timezone": s.Timezone}, "period": period, "generatedAt": time.Now().UTC().Format(time.RFC3339), "onlineWindowSeconds": 300, "data": data})
}

func (a *App) exportCSV(w http.ResponseWriter, r *http.Request) error {
	s, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	report, err := a.report(r, s, false)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="plost-report.csv"`)
	c := csv.NewWriter(w)
	defer c.Flush()
	_ = c.Write([]string{"dimension", "name", "value"})
	for _, section := range []struct {
		name string
		rows []Row
	}{{"sources", report.Sources}, {"pages", report.Pages}, {"countries", report.Countries}, {"browsers", report.Browsers}, {"os", report.OS}, {"devices", report.Devices}, {"languages", report.Languages}, {"screens", report.Screens}} {
		for _, row := range section.rows {
			name := row.Name
			if strings.HasPrefix(name, "=") || strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "\t") || strings.HasPrefix(name, "\r") {
				name = "'" + name
			}
			if err := c.Write([]string{section.name, name, strconvID(row.Value)}); err != nil {
				return err
			}
		}
	}
	c.Flush()
	return c.Error()
}

func (a *App) openapi(w http.ResponseWriter, r *http.Request) {
	_ = jsonResponse(w, map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "Plost Go statistics API", "version": "0.1.0"}, "paths": map[string]any{"/api/v1/stats": map[string]any{"get": map[string]any{"summary": "Online, today and yesterday metrics for the API key's site", "security": []any{map[string]any{"bearerAuth": []string{}}}, "responses": map[string]any{"200": map[string]any{"description": "Site statistics"}, "401": map[string]any{"description": "Missing, invalid or deleted API key"}}}}}, "components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"}}}})
}

// regions counts the visitors of each province or state with a recorded one. The busiest come first; the map only
// shades so many, and a country filter leaves just that country's.
func (a *App) regions(ctx context.Context, site int64, r Range, q url.Values) ([]RegionRow, error) {
	w, args := where(site, r, q)
	rows, err := a.db.QueryContext(ctx, "SELECT country_code,region,count(DISTINCT "+visitorSQL+") AS n FROM events_v2 WHERE "+w+" AND name='pageview' AND region<>'' AND country_code<>'' GROUP BY 1,2 ORDER BY n DESC,1,2 LIMIT 1000", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RegionRow{}
	for rows.Next() {
		var v RegionRow
		if err := rows.Scan(&v.Country, &v.Name, &v.Value); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// liveRegions counts the visitors of the last five minutes by country and province, with the report's filters; the
// region is empty for visitors without one.
func (a *App) liveRegions(ctx context.Context, site int64, q url.Values) ([]RegionRow, error) {
	live := Range{From: time.Now().Add(-5 * time.Minute), To: time.Now().Add(time.Second)}
	w, args := where(site, live, q)
	rows, err := a.db.QueryContext(ctx, "SELECT country_code,region,count(DISTINCT "+visitorSQL+") FROM events_v2 WHERE "+w+" AND country_code<>'' GROUP BY 1,2 ORDER BY 3 DESC LIMIT 1000", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RegionRow{}
	for rows.Next() {
		var v RegionRow
		if err := rows.Scan(&v.Country, &v.Name, &v.Value); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// liveCountries counts the visitors of the last five minutes by country, with the report's filters.
func (a *App) liveCountries(ctx context.Context, site int64, q url.Values) ([]Row, error) {
	live := Range{From: time.Now().Add(-5 * time.Minute), To: time.Now().Add(time.Second)}
	w, args := where(site, live, q)
	rows, err := a.db.QueryContext(ctx, "SELECT country_code, count(DISTINCT "+visitorSQL+") FROM events_v2 WHERE "+w+" AND country_code<>'' GROUP BY 1 ORDER BY 2 DESC LIMIT 250", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Name, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
