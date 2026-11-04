package plost

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	siteCheckInterval  = 5 * time.Minute
	siteCheckRetention = 91 * 24 * time.Hour // the 90-day uptime row plus today's partial day
	uptimeDays         = 90
)

type SiteCheckReport struct {
	Enabled bool            `json:"enabled"`
	Path    string          `json:"path"`
	URL     string          `json:"url"`
	Expiry  *ExpirySnapshot `json:"expiry"`
	Latest  *HTTPCheck      `json:"latest"`
	Checks  []HTTPCheck     `json:"checks"`
	Uptime  SiteUptime      `json:"uptime"`
	Now     string          `json:"now"`
}

// SiteUptime is one entry per calendar day in the site's timezone, oldest first, ending today.
// Percent is null on a day without any check.
type SiteUptime struct {
	Days []UptimeDay `json:"days"`
}

type UptimeDay struct {
	Date    string   `json:"date"`
	Percent *float64 `json:"percent"`
	Total   int      `json:"total"`
	Failed  int      `json:"failed"`
}

// siteUptime counts the checks of the last 90 days per local calendar day.
func (a *App) siteUptime(ctx context.Context, site Site, target string, now time.Time) (SiteUptime, error) {
	loc, err := time.LoadLocation(site.Timezone)
	if err != nil {
		loc = time.UTC
	}
	today := now.In(loc)
	first := time.Date(today.Year(), today.Month(), today.Day()-(uptimeDays-1), 0, 0, 0, 0, loc)
	days := make([]UptimeDay, uptimeDays)
	index := map[string]int{}
	for i := range days {
		date := time.Date(first.Year(), first.Month(), first.Day()+i, 0, 0, 0, 0, loc).Format("2006-01-02")
		days[i].Date = date
		index[date] = i
	}
	rows, err := a.db.QueryContext(ctx, "SELECT checked_at,ok FROM site_check_results WHERE site_id=? AND target_url=? AND checked_at>=?", site.ID, target, first.Unix())
	if err != nil {
		return SiteUptime{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		var ok bool
		if err := rows.Scan(&at, &ok); err != nil {
			return SiteUptime{}, err
		}
		if i, found := index[time.Unix(at, 0).In(loc).Format("2006-01-02")]; found {
			days[i].Total++
			if !ok {
				days[i].Failed++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return SiteUptime{}, err
	}
	for i := range days {
		if days[i].Total > 0 {
			percent := float64(days[i].Total-days[i].Failed) / float64(days[i].Total) * 100
			days[i].Percent = &percent
		}
	}
	return SiteUptime{Days: days}, nil
}

func parseExpiry(text string) *ExpirySnapshot {
	if text == "" {
		return nil
	}
	var s ExpirySnapshot
	if json.Unmarshal([]byte(text), &s) != nil {
		return nil
	}
	return &s
}

// siteCheckReport is what the dashboard's status badge and uptime card show: settings, the last 24 hours of
// checks, daily uptime for 90 days and the latest expiry lookup. The caller holds a.mu.
func (a *App) siteCheckReport(ctx context.Context, site Site) (SiteCheckReport, error) {
	enabled, path, expiryText, expiryHost := false, "/", "", ""
	err := a.db.QueryRowContext(ctx, "SELECT enabled,path,expiry,expiry_host FROM site_check_settings WHERE site_id=?", site.ID).Scan(&enabled, &path, &expiryText, &expiryHost)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SiteCheckReport{}, err
	}
	target, err := siteCheckURL(site.Domain, path)
	if err != nil {
		path = "/"
		target, _ = siteCheckURL(site.Domain, path)
	}
	now := time.Now()
	report := SiteCheckReport{Enabled: enabled, Path: path, URL: target.String(), Checks: []HTTPCheck{}, Now: isoTime(now)}
	if expiryHost == site.Domain {
		report.Expiry = parseExpiry(expiryText)
	}
	rows, err := a.db.QueryContext(ctx, `SELECT checked_at,ok,status,duration_ms,error FROM site_check_results
 WHERE site_id=? AND target_url=? AND checked_at>? ORDER BY checked_at DESC LIMIT 1440`, site.ID, target.String(), now.Add(-24*time.Hour).Unix())
	if err != nil {
		return report, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		var ok bool
		var status, duration int
		var message string
		if err := rows.Scan(&at, &ok, &status, &duration, &message); err != nil {
			return report, err
		}
		check := HTTPCheck{CheckedAt: isoTime(time.Unix(at, 0)), OK: ok, Error: message}
		if status > 0 {
			check.Status = &status
		}
		if duration >= 0 {
			check.DurationMs = &duration
		}
		report.Checks = append(report.Checks, check)
	}
	if err := rows.Err(); err != nil {
		return report, err
	}
	for i, j := 0, len(report.Checks)-1; i < j; i, j = i+1, j-1 { // newest first in SQL, oldest first for the chart
		report.Checks[i], report.Checks[j] = report.Checks[j], report.Checks[i]
	}
	if n := len(report.Checks); n > 0 {
		report.Latest = &report.Checks[n-1]
	}
	if report.Uptime, err = a.siteUptime(ctx, site, target.String(), now); err != nil {
		return report, err
	}
	return report, nil
}

// certificateExpiries lists the certificate expiry of every site whose latest lookup matches its current domain.
func (a *App) certificateExpiries(ctx context.Context) map[int64]string {
	out := map[int64]string{}
	rows, err := a.db.QueryContext(ctx, "SELECT c.site_id,c.expiry FROM site_check_settings c JOIN sites s ON s.id=c.site_id WHERE c.expiry_host=s.domain AND c.expiry<>''")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var text string
		if rows.Scan(&id, &text) == nil {
			if s := parseExpiry(text); s != nil && s.Certificate.ExpiresAt != "" {
				out[id] = s.Certificate.ExpiresAt
			}
		}
	}
	return out
}

func (a *App) saveSiteCheck(ctx context.Context, site Site, enabled bool, path string) error {
	if _, err := siteCheckURL(site.Domain, path); err != nil {
		return failure(400, err.Error())
	}
	// A new version drops results that a probe still in flight would otherwise write for the old settings.
	_, err := a.db.ExecContext(ctx, `INSERT INTO site_check_settings(site_id,enabled,path,next_check_at) VALUES(?,?,?,?)
 ON CONFLICT(site_id) DO UPDATE SET enabled=excluded.enabled,path=excluded.path,version=version+1,next_check_at=excluded.next_check_at`,
		site.ID, enabled, path, unixNow())
	return err
}

// runSiteCheck claims one check atomically, so scheduled and manual runs never overlap, then probes and records it.
// It reports false when nothing was due or the previous run started less than a minute ago.
func (a *App) runSiteCheck(ctx context.Context, siteID int64, manual bool) (bool, error) {
	now := time.Now()
	a.mu.RLock()
	res, err := a.db.ExecContext(ctx, `UPDATE site_check_settings SET last_started_at=?,next_check_at=?
 WHERE site_id=? AND enabled=1 AND last_started_at<? AND (?=1 OR next_check_at<=?)`,
		now.Unix(), now.Add(siteCheckInterval).Unix(), siteID, now.Add(-time.Minute).Unix(), manual, now.Unix())
	if err != nil {
		a.mu.RUnlock()
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		a.mu.RUnlock()
		return false, nil
	}
	var path, expiryText, expiryHost, domain string
	var version int64
	err = a.db.QueryRowContext(ctx, `SELECT c.path,c.version,c.expiry,c.expiry_host,s.domain FROM site_check_settings c JOIN sites s ON s.id=c.site_id WHERE c.site_id=?`, siteID).
		Scan(&path, &version, &expiryText, &expiryHost, &domain)
	a.mu.RUnlock()
	if err != nil {
		return false, err
	}
	target, err := siteCheckURL(domain, path)
	if err != nil {
		return false, err
	}
	var previous *ExpirySnapshot
	if expiryHost == domain {
		previous = parseExpiry(expiryText)
	}
	var check HTTPCheck
	var expiry *ExpirySnapshot
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); check = a.siteProbe.http(ctx, target) }()
	if manual || expiryRefreshDue(previous, now) {
		wg.Add(1)
		go func() { defer wg.Done(); s := a.siteProbe.expiry(ctx, target.Hostname()); expiry = &s }()
	}
	wg.Wait()

	// Changing the settings, pausing, or renaming the site while a request runs invalidates its result.
	a.mu.RLock()
	defer a.mu.RUnlock()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var valid int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM site_check_settings c JOIN sites s ON s.id=c.site_id WHERE c.site_id=? AND c.version=? AND c.enabled=1 AND s.domain=?`,
		siteID, version, domain).Scan(&valid); err != nil {
		return false, err
	}
	if valid == 1 {
		at, err := time.Parse(time.RFC3339Nano, check.CheckedAt)
		if err != nil {
			at = now
		}
		status, duration := 0, -1
		if check.Status != nil {
			status = *check.Status
		}
		if check.DurationMs != nil {
			duration = *check.DurationMs
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO site_check_results(site_id,target_url,checked_at,ok,status,duration_ms,error) VALUES(?,?,?,?,?,?,?)",
			siteID, target.String(), at.Unix(), check.OK, status, duration, check.Error); err != nil {
			return false, err
		}
		if expiry != nil {
			merged, _ := json.Marshal(mergeExpiry(previous, *expiry))
			if _, err = tx.ExecContext(ctx, "UPDATE site_check_settings SET expiry=?,expiry_host=? WHERE site_id=?", string(merged), domain, siteID); err != nil {
				return false, err
			}
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM site_check_results WHERE site_id=? AND checked_at<?", siteID, now.Add(-siteCheckRetention).Unix()); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// runDueSiteChecks runs what is due with two bounded workers.
func (a *App) runDueSiteChecks() {
	a.mu.RLock()
	rows, err := a.db.QueryContext(a.jobCtx, "SELECT site_id FROM site_check_settings WHERE enabled=1 AND next_check_at<=? ORDER BY next_check_at LIMIT 20", unixNow())
	var ids []int64
	if err == nil {
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	a.mu.RUnlock()
	if len(ids) == 0 {
		return
	}
	queue := make(chan int64, len(ids))
	for _, id := range ids {
		queue <- id
	}
	close(queue)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range queue {
				if _, err := a.runSiteCheck(a.jobCtx, id, false); err != nil && a.jobCtx.Err() == nil {
					log.Print("Plost site check job failed")
				}
			}
		}()
	}
	wg.Wait()
}

func (a *App) siteCheckLoop() {
	defer a.workers.Done()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-timer.C:
			a.runDueSiteChecks()
			timer.Reset(30 * time.Second)
		}
	}
}

func (a *App) siteCheckGet(w http.ResponseWriter, r *http.Request) error {
	site, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	report, err := a.siteCheckReport(r.Context(), site)
	if err != nil {
		return err
	}
	return jsonResponse(w, map[string]any{"report": report, "canManage": true})
}

func (a *App) siteCheckPut(w http.ResponseWriter, r *http.Request) error {
	site, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	if err != nil {
		return failure(404, "网站不存在")
	}
	if !a.limits.allow("site-check", a.clientIP(r), 30, 5*time.Minute) {
		return failure(429, "操作过于频繁")
	}
	var in struct {
		Enabled bool   `json:"enabled"`
		Path    string `json:"path"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Path == "" {
		in.Path = "/"
	}
	if err := a.saveSiteCheck(r.Context(), site, in.Enabled, in.Path); err != nil {
		return err
	}
	if in.Enabled { // the first result appears within seconds without making the save wait for the network
		a.startJob(func(ctx context.Context) { _, _ = a.runSiteCheck(ctx, site.ID, true) })
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) siteCheckRun(w http.ResponseWriter, r *http.Request) error {
	a.mu.RLock()
	site, err := readSite(r.Context(), a.db, r.PathValue("domain"))
	a.mu.RUnlock()
	if err != nil {
		return failure(404, "网站不存在")
	}
	if !a.limits.allow("site-check-run", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "检查过于频繁，请稍后再试")
	}
	ran, err := a.runSiteCheck(r.Context(), site.ID, true)
	if err != nil {
		return err
	}
	if !ran {
		return failure(409, "请开启检查，并与上次检查间隔至少一分钟")
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}
