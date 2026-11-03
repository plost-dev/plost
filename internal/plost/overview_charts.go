package plost

import (
	"net/http"
	"net/url"
)

// OverviewCharts feeds the key figures beside the globe on the site list: the last 24 hours against the 24 before,
// with the hours for the small trend lines. Every figure is the sum over all sites; each site keeps its own time zone
// for its buckets, and points are added up by position.
type OverviewCharts struct {
	Totals   Overview `json:"totals"`
	Previous Overview `json:"previous"`
	Points   []Point  `json:"points"`
}

func (a *App) overviewCharts(w http.ResponseWriter, r *http.Request) error {
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
	out := OverviewCharts{Points: []Point{}}
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
		points, err := a.series(r.Context(), s.ID, day, nil)
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
	return jsonResponse(w, out)
}
