package plost

import (
	"net/http"
	"net/url"
)

// OverviewCharts feeds the chart beside the globe on the site list. Every figure is the sum over all sites; each
// site keeps its own time zone for its buckets, and points are added up by position.
type OverviewCharts struct {
	Totals   Overview           `json:"totals"`
	Previous Overview           `json:"previous"`
	Points   []Point            `json:"points"`
	Heatmap  []map[string]int64 `json:"heatmap"`
}

// overviewCharts answers ?view=hourly|metrics (the last 24 hours by hour), daily (the last 30 days by day) or
// heatmap (the last 30 days by weekday and hour); sites needs only the totals, which every view carries.
func (a *App) overviewCharts(w http.ResponseWriter, r *http.Request) error {
	view := r.URL.Query().Get("view")
	switch view {
	case "hourly", "metrics", "daily", "heatmap", "sites":
	default:
		return failure(400, "图表类型无效")
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,domain,name,timezone,public,created_at FROM sites ORDER BY created_at,id")
	if err != nil {
		return err
	}
	sites := []Site{}
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.ID, &s.Domain, &s.Name, &s.Timezone, &s.Public, &s.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		sites = append(sites, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	out := OverviewCharts{Points: []Point{}, Heatmap: []map[string]int64{}}
	heat := map[int64]int64{}
	add := func(to *Overview, o Overview) {
		to.Visitors += o.Visitors
		to.Visits += o.Visits
		to.Pageviews += o.Pageviews
		to.Online += o.Online
	}
	for _, s := range sites {
		day, err := parseRange(url.Values{"period": {"24h"}}, s.Timezone)
		if err != nil {
			return err
		}
		over, err := a.overview(r.Context(), s.ID, day, nil)
		if err != nil {
			return err
		}
		old, err := a.overview(r.Context(), s.ID, Range{From: day.From.Add(-day.To.Sub(day.From)), To: day.From}, nil)
		if err != nil {
			return err
		}
		add(&out.Totals, over)
		add(&out.Previous, old)
		var points []Point
		switch view {
		case "hourly", "metrics":
			points, err = a.series(r.Context(), s.ID, day, nil)
		case "daily":
			var month Range
			if month, err = parseRange(url.Values{"period": {"30d"}}, s.Timezone); err == nil {
				points, err = a.series(r.Context(), s.ID, month, nil)
			}
		case "heatmap":
			err = a.addHeatmap(r, s, heat)
		}
		if err != nil {
			return err
		}
		for i, p := range points {
			if i == len(out.Points) {
				out.Points = append(out.Points, Point{At: p.At})
			}
			out.Points[i].Visitors += p.Visitors
			out.Points[i].Visits += p.Visits
			out.Points[i].Pageviews += p.Pageviews
		}
	}
	for slot := int64(0); slot < 8*24; slot++ {
		if n := heat[slot]; n > 0 {
			out.Heatmap = append(out.Heatmap, map[string]int64{"dow": slot / 24, "hour": slot % 24, "value": n})
		}
	}
	return jsonResponse(w, out)
}

// addHeatmap counts a site's visitors over the last 30 days per local weekday and hour, the same way the site's
// own heatmap does.
func (a *App) addHeatmap(r *http.Request, s Site, into map[int64]int64) error {
	month, err := parseRange(url.Values{"period": {"30d"}}, s.Timezone)
	if err != nil {
		return err
	}
	w, args := where(s.ID, month, nil)
	rows, err := a.db.QueryContext(r.Context(), "SELECT plost_local_hour(timestamp,?) AS slot,count(DISTINCT "+visitorSQL+") FROM events_v2 WHERE "+w+" AND name='pageview' GROUP BY slot", append([]any{s.Timezone}, args...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var slot, n int64
		if err := rows.Scan(&slot, &n); err != nil {
			return err
		}
		into[slot] += n
	}
	return rows.Err()
}
