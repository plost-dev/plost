package plost

import (
	"net/http"
	"net/url"
)

// OverviewCharts feeds the key figures beside the globe on the site list: the last 24 hours against the 24 before,
// summed over all sites, each in its own time zone.
type OverviewCharts struct {
	Totals   Overview `json:"totals"`
	Previous Overview `json:"previous"`
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
	out := OverviewCharts{}
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
	}
	return jsonResponse(w, out)
}
