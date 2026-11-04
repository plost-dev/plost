package plost

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Moving history in from Umami is an offline operation: the importer owns the data directory
// (so Plost must be stopped), copies only what Plost can express, and proves the result by
// comparing per-day aggregates computed independently on both sides before it commits.

// ImportWindow bounds the events read from Umami. Since is inclusive and Until exclusive; zero means open.
type ImportWindow struct{ Since, Until time.Time }

type UmamiWebsite struct{ ID, Name, Domain string }

// UmamiProp is one row of Umami's event_data. Type follows Umami: 1 string, 2 number, 3 boolean, 4 date, 5 array.
type UmamiProp struct {
	Key    string
	Type   int
	String *string
	Number *float64
	Date   *time.Time
	Bool   *bool
}

type UmamiEvent struct {
	ID, WebsiteID, SessionID, VisitID                                                            string
	CreatedAt                                                                                    time.Time
	Type                                                                                         int
	Name, Hostname, Path, ReferrerDomain, ReferrerPath, Title, UTMSource, UTMMedium, UTMCampaign string
	Browser, OS, Device, Screen, Language, Country                                               string
	Props                                                                                        []UmamiProp
}

// UmamiDay holds aggregates computed by Umami's own database for one local calendar day.
// Visitors and Visits count pageviews only, as Plost does.
type UmamiDay struct {
	Day                                    string
	Pageviews, CustomRaw, Visitors, Visits int64
}

// UmamiSource reads one consistent snapshot of an Umami database.
type UmamiSource interface {
	Websites(ctx context.Context) ([]UmamiWebsite, error)
	// Events streams pageviews (type 1) and custom events (type 2) ordered by time and id.
	Events(ctx context.Context, websiteID string, w ImportWindow, fn func(UmamiEvent) error) error
	// EventTypeCounts counts every event type, including those that are not imported.
	EventTypeCounts(ctx context.Context, websiteID string, w ImportWindow) (map[int]int64, error)
	// DayCounts groups by calendar day in timezone and must not depend on the import's own conversion.
	DayCounts(ctx context.Context, websiteID, timezone string, w ImportWindow) ([]UmamiDay, error)
}

type ImportOptions struct {
	Sites        map[string]string // Umami website id -> Plost domain; sites not listed are not imported
	CreateSites  bool
	Timezone     string // for sites created by the import
	Window       ImportWindow
	Apply        bool
	Strict       bool // fail instead of skipping events Plost cannot represent
	AllowOverlap bool // accept live Plost events inside the imported window
}

type DayStats struct {
	Day       string `json:"day"`
	Pageviews int64  `json:"pageviews"`
	Custom    int64  `json:"customEvents"`
	Visitors  int64  `json:"visitors"`
	Visits    int64  `json:"visits"`
}

type SiteReport struct {
	WebsiteID       string           `json:"websiteId"`
	Name            string           `json:"name"`
	Domain          string           `json:"domain"`
	Timezone        string           `json:"timezone"`
	CreatedSite     bool             `json:"createdSite"`
	Read            int64            `json:"read"`
	Inserted        int64            `json:"inserted"`
	AlreadyImported int64            `json:"alreadyImported"`
	Pageviews       int64            `json:"pageviews"`
	Custom          int64            `json:"customEvents"`
	Skipped         map[string]int64 `json:"skipped"`
	OtherTypes      map[string]int64 `json:"otherEventTypes"`
	WithoutVisitID  int64            `json:"withoutVisitId"`
	DroppedProps    int64            `json:"droppedProperties"`
	FirstEvent      string           `json:"firstEvent,omitempty"`
	LastEvent       string           `json:"lastEvent,omitempty"`
	Days            []DayStats       `json:"verifiedDays"`
	Problems        []string         `json:"problems,omitempty"`
}

type UnmappedSite struct {
	WebsiteID  string           `json:"websiteId"`
	Name       string           `json:"name"`
	Domain     string           `json:"domain"`
	EventTypes map[string]int64 `json:"eventTypes"`
}

type ImportReport struct {
	Apply    bool           `json:"apply"`
	Since    string         `json:"since,omitempty"`
	Until    string         `json:"until,omitempty"`
	Snapshot string         `json:"snapshot,omitempty"`
	Sites    []SiteReport   `json:"sites"`
	Unmapped []UnmappedSite `json:"notImported"`
}

// ErrReconcile means the data did not add up; nothing was written.
var ErrReconcile = errors.New("对账失败，没有写入任何数据")

type Importer struct {
	dir   string
	db    *sql.DB
	lock  *os.File
	write bool
}

// OpenImporter prepares the data directory of a stopped Plost instance. Without write it only reads.
func OpenImporter(dir string, write bool) (*Importer, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "plost.db")
	if _, err := os.Stat(path); err != nil {
		return nil, errors.New("数据目录里没有 plost.db，请先启动一次 Plost 并完成初始化")
	}
	imp := &Importer{dir: dir, write: write}
	if !write {
		u := &url.URL{Scheme: "file", Path: path}
		q := u.Query()
		q.Add("mode", "ro")
		q.Add("_pragma", "busy_timeout(5000)")
		u.RawQuery = q.Encode()
		if imp.db, err = sql.Open("sqlite", u.String()); err != nil {
			return nil, err
		}
		imp.db.SetMaxOpenConns(1)
		if err = imp.db.Ping(); err != nil {
			imp.db.Close()
			return nil, err
		}
		return imp, nil
	}
	if imp.lock, err = lockDirectory(dir); err != nil {
		return nil, errors.New("Plost 正在使用这个数据目录，请先停止它再导入")
	}
	if imp.db, err = openDB(path); err == nil {
		err = migrate(imp.db)
	}
	if err != nil {
		imp.Close()
		return nil, err
	}
	return imp, nil
}

func (imp *Importer) Close() {
	if imp.db != nil {
		_ = imp.db.Close()
	}
	unlockDirectory(imp.lock)
}

func (imp *Importer) hasSourceColumn(ctx context.Context) bool {
	var n int
	return imp.db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('events_v2') WHERE name='source_id'").Scan(&n) == nil && n > 0
}

func (imp *Importer) snapshot(ctx context.Context) (string, error) {
	dir := filepath.Join(imp.dir, "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "before-import-"+time.Now().UTC().Format("20060102T150405")+"-"+randomHex(4)+".db")
	if _, err := imp.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", err
	}
	return path, os.Chmod(path, 0600)
}

func umamiPrefix(websiteID string) (from, to string) {
	return "umami:" + websiteID + ":", "umami:" + websiteID + ";" // ';' follows ':' so the range covers exactly this site
}

// Plost groups by its own small vocabulary, so Umami's labels are folded into it. Samsung, Yandex and
// other Chromium builds are Chrome here because that is how live collection classifies them.
func umamiBrowser(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(v, "edge"):
		return "Edge"
	case strings.Contains(v, "opera") || v == "opr":
		return "Opera"
	case strings.Contains(v, "firefox") || v == "fxios":
		return "Firefox"
	case strings.Contains(v, "chrome") || strings.Contains(v, "chromium") || strings.Contains(v, "crios") || strings.Contains(v, "samsung") || strings.Contains(v, "yandex"):
		return "Chrome"
	case v == "safari" || v == "ios":
		return "Safari"
	}
	return "Other"
}

func umamiOS(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(v, "android"):
		return "Android"
	case strings.Contains(v, "chrome os") || strings.Contains(v, "chromeos") || strings.Contains(v, "cros"):
		return "Chrome OS"
	case strings.Contains(v, "ios") || strings.Contains(v, "iphone") || strings.Contains(v, "ipad"):
		return "iOS"
	case strings.Contains(v, "windows"):
		return "Windows"
	case strings.Contains(v, "mac"):
		return "macOS"
	case strings.Contains(v, "linux") || strings.Contains(v, "ubuntu") || strings.Contains(v, "debian") || strings.Contains(v, "fedora"):
		return "Linux"
	}
	return "Other"
}

func umamiDevice(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "mobile":
		return "Mobile"
	case "tablet":
		return "Tablet"
	}
	return "Desktop"
}

// umamiProps keeps what Plost can store: at most 32 simple values with bounded keys and strings.
func umamiProps(props []UmamiProp) (string, int) {
	sorted := append([]UmamiProp(nil), props...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	out := map[string]any{}
	dropped := 0
	for _, p := range sorted {
		if p.Key == "" || len(p.Key) > 300 || len(out) >= 32 {
			dropped++
			continue
		}
		var v any
		switch p.Type {
		case 2:
			if p.Number != nil {
				v = *p.Number
			}
		case 3:
			if p.Bool != nil {
				v = *p.Bool
			} else if p.String != nil {
				switch strings.ToLower(strings.TrimSpace(*p.String)) {
				case "true":
					v = true
				case "false":
					v = false
				}
			}
		case 4:
			if p.Date != nil {
				v = p.Date.UTC().Format(time.RFC3339)
			}
		}
		if v == nil { // strings, arrays (kept as their JSON text) and rows whose typed column is empty
			switch {
			case p.String != nil:
				v = short(*p.String, 2000)
			case p.Number != nil:
				v = *p.Number
			case p.Bool != nil:
				v = *p.Bool
			case p.Date != nil:
				v = p.Date.UTC().Format(time.RFC3339)
			}
		}
		if f, ok := v.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
			v = nil
		}
		if v == nil {
			dropped++
			continue
		}
		out[p.Key] = v
	}
	b, _ := json.Marshal(out)
	return string(b), dropped
}

// convertUmami maps one Umami event to a Plost event with the same rules live collection applies.
// The second result is the reason an event cannot be represented, or "" when it was converted.
func convertUmami(e UmamiEvent, domain string) (Event, string, int) {
	name := "pageview"
	switch e.Type {
	case 1:
	case 2:
		name = strings.TrimSpace(e.Name)
		switch {
		case name == "":
			return Event{}, "empty_event_name", 0
		case strings.EqualFold(name, "pageview"):
			return Event{}, "reserved_event_name", 0
		case len(name) > 255:
			return Event{}, "event_name_too_long", 0
		}
	default:
		return Event{}, fmt.Sprintf("event_type_%d", e.Type), 0
	}
	host := canonicalHost(e.Hostname)
	if host == "" {
		host = domain
	}
	path := e.Path
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if path == "" {
		path = "/"
	}
	source, referrer := "Direct", ""
	if domainRef := strings.ToLower(strings.TrimSpace(e.ReferrerDomain)); domainRef != "" {
		source = canonicalHost(domainRef)
		referrer = "https://" + domainRef + e.ReferrerPath
	}
	if source == domain {
		source = "Direct"
	}
	source = sourceName(source, e.UTMSource)
	country := strings.ToUpper(strings.TrimSpace(e.Country))
	if !countryPattern.MatchString(country) {
		country = ""
	}
	screen := strings.ToLower(strings.TrimSpace(e.Screen))
	if !screenPattern.MatchString(screen) {
		screen = ""
	}
	props, dropped := umamiProps(e.Props)
	return Event{
		At: e.CreatedAt.Unix(), Name: name, Hostname: host, Path: short(path, 2048), Source: source, Referrer: short(referrer, 2048),
		UserID: e.SessionID, SessionID: e.VisitID, Country: country,
		Browser: umamiBrowser(e.Browser), OS: umamiOS(e.OS), Device: umamiDevice(e.Device),
		Language: short(e.Language, 64), Screen: screen, Title: short(e.Title, 500),
		UTMSource: short(e.UTMSource, 200), UTMMedium: short(e.UTMMedium, 200), UTMCampaign: short(e.UTMCampaign, 200), Props: props,
	}, "", dropped
}

type dayAgg struct{ pageviews, custom, skippedCustom int64 }

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func importDay(ctx context.Context, q queryer, siteID int64, websiteID string, loc *time.Location, day string, w ImportWindow) (DayStats, error) {
	d, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return DayStats{}, err
	}
	start, end := d.Unix(), time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, loc).Unix()
	if !w.Since.IsZero() && w.Since.Unix() > start {
		start = w.Since.Unix()
	}
	if !w.Until.IsZero() && w.Until.Unix() < end {
		end = w.Until.Unix()
	}
	from, to := umamiPrefix(websiteID)
	s := DayStats{Day: day}
	err = q.QueryRowContext(ctx, `SELECT COALESCE(sum(name='pageview'),0),COALESCE(sum(name<>'pageview'),0),
 count(DISTINCT CASE WHEN name='pageview' THEN user_id END),count(DISTINCT CASE WHEN name='pageview' THEN session_id END)
 FROM events_v2 WHERE site_id=? AND source_id>=? AND source_id<? AND timestamp>=? AND timestamp<?`, siteID, from, to, start, end).
		Scan(&s.Pageviews, &s.Custom, &s.Visitors, &s.Visits)
	return s, err
}

const insertImported = `INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,referrer_source,referrer,user_id,session_id,country_code,browser,os,device,language,screen,title,utm_source,utm_medium,utm_campaign,props,source_id)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(site_id,source_id) WHERE source_id<>'' DO NOTHING`

// Run converts and (with Apply) writes the mapped sites in one transaction. A mismatch anywhere rolls everything back.
func (imp *Importer) Run(ctx context.Context, src UmamiSource, opts ImportOptions) (ImportReport, error) {
	rep := ImportReport{Apply: opts.Apply, Sites: []SiteReport{}, Unmapped: []UnmappedSite{}}
	if !opts.Window.Since.IsZero() {
		rep.Since = opts.Window.Since.UTC().Format(time.RFC3339)
	}
	if !opts.Window.Until.IsZero() {
		rep.Until = opts.Window.Until.UTC().Format(time.RFC3339)
	}
	if opts.Apply && !imp.write {
		return rep, errors.New("导入需要以写入模式打开数据目录")
	}
	if !opts.Window.Since.IsZero() && !opts.Window.Until.IsZero() && !opts.Window.Since.Before(opts.Window.Until) {
		return rep, errors.New("起始时间必须早于结束时间")
	}
	websites, err := src.Websites(ctx)
	if err != nil {
		return rep, err
	}
	byID := map[string]UmamiWebsite{}
	for _, w := range websites {
		byID[w.ID] = w
	}
	targets := map[string]string{}
	var ids []string
	for id, domain := range opts.Sites {
		if _, ok := byID[id]; !ok {
			return rep, fmt.Errorf("Umami 中没有站点 %s", id)
		}
		d, err := normalizeDomain(domain)
		if err != nil {
			return rep, fmt.Errorf("站点 %s 的目标域名无效：%w", id, err)
		}
		for other, taken := range targets {
			if taken == d {
				return rep, fmt.Errorf("Umami 站点 %s 和 %s 不能导入到同一个域名 %s", other, id, d)
			}
		}
		targets[id] = d
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return rep, errors.New("没有指定要导入的站点")
	}
	for _, w := range websites {
		if _, ok := targets[w.ID]; ok {
			continue
		}
		types, err := src.EventTypeCounts(ctx, w.ID, opts.Window)
		if err != nil {
			return rep, err
		}
		rep.Unmapped = append(rep.Unmapped, UnmappedSite{WebsiteID: w.ID, Name: w.Name, Domain: w.Domain, EventTypes: typeNames(types)})
	}

	// Resolve every target and check for overlap before anything is written.
	type plan struct {
		site    Site
		created bool
		loc     *time.Location
		website UmamiWebsite
	}
	plans := map[string]*plan{}
	hasSource := imp.hasSourceColumn(ctx)
	for _, id := range ids {
		p := &plan{website: byID[id]}
		p.site, err = readSite(ctx, imp.db, targets[id])
		switch {
		case err == nil:
		case errors.Is(err, sql.ErrNoRows):
			if !opts.CreateSites {
				return rep, fmt.Errorf("Plost 里还没有站点 %s，请先添加，或使用创建站点选项", targets[id])
			}
			tz := opts.Timezone
			if tz == "" {
				tz = "Etc/UTC"
			}
			p.site, p.created = Site{Domain: targets[id], Name: firstNonEmpty(p.website.Name, targets[id]), Timezone: tz}, true
		default:
			return rep, err
		}
		if p.loc, err = time.LoadLocation(p.site.Timezone); err != nil {
			return rep, fmt.Errorf("时区 %q 无效", p.site.Timezone)
		}
		if !p.created && !opts.AllowOverlap {
			query := "SELECT COALESCE(min(timestamp),0) FROM events_v2 WHERE site_id=?"
			if hasSource {
				query += " AND source_id=''"
			}
			var earliest int64
			if err = imp.db.QueryRowContext(ctx, query, p.site.ID).Scan(&earliest); err != nil {
				return rep, err
			}
			if earliest > 0 && (opts.Window.Until.IsZero() || opts.Window.Until.Unix() > earliest) {
				return rep, fmt.Errorf("站点 %s 已有 Plost 实时采集的数据（最早 %s），导入范围与它重叠会造成重复计数；请把结束时间设为切换统计脚本的时刻，或确认无误后允许重叠",
					p.site.Domain, time.Unix(earliest, 0).UTC().Format(time.RFC3339))
			}
		}
		plans[id] = p
	}

	var tx *sql.Tx
	var stmt *sql.Stmt
	if opts.Apply {
		if rep.Snapshot, err = imp.snapshot(ctx); err != nil {
			return rep, fmt.Errorf("无法在导入前保存快照：%w", err)
		}
		if tx, err = imp.db.BeginTx(ctx, nil); err != nil {
			return rep, err
		}
		defer tx.Rollback()
		if stmt, err = tx.PrepareContext(ctx, insertImported); err != nil {
			return rep, err
		}
		defer stmt.Close()
	}

	failed := false
	for _, id := range ids {
		p := plans[id]
		sr := SiteReport{WebsiteID: id, Name: p.website.Name, Domain: p.site.Domain, Timezone: p.site.Timezone, CreatedSite: p.created,
			Skipped: map[string]int64{}, OtherTypes: map[string]int64{}, Days: []DayStats{}}
		if tx != nil && p.created {
			res, err := tx.ExecContext(ctx, "INSERT INTO sites(domain,name,timezone,public,created_at) VALUES(?,?,?,0,?)", p.site.Domain, p.site.Name, p.site.Timezone, unixNow())
			if err != nil {
				return rep, err
			}
			if p.site.ID, err = res.LastInsertId(); err != nil {
				return rep, err
			}
		}
		days := map[string]*dayAgg{}
		var first, last int64
		prefix, _ := umamiPrefix(id)
		err := src.Events(ctx, id, opts.Window, func(e UmamiEvent) error {
			sr.Read++
			ev, skip, dropped := convertUmami(e, p.site.Domain)
			day := e.CreatedAt.In(p.loc).Format("2006-01-02")
			agg := days[day]
			if agg == nil {
				agg = &dayAgg{}
				days[day] = agg
			}
			if skip != "" {
				sr.Skipped[skip]++
				if e.Type == 2 {
					agg.skippedCustom++
				}
				return nil
			}
			if e.VisitID == "" { // events from before Umami recorded visits count as one visit per session
				ev.SessionID = e.SessionID
				sr.WithoutVisitID++
			}
			sr.DroppedProps += int64(dropped)
			if e.Type == 1 {
				sr.Pageviews++
				agg.pageviews++
			} else {
				sr.Custom++
				agg.custom++
			}
			if first == 0 || ev.At < first {
				first = ev.At
			}
			if ev.At > last {
				last = ev.At
			}
			if stmt == nil {
				return nil
			}
			res, err := stmt.ExecContext(ctx, p.site.ID, ev.At, ev.Name, ev.Hostname, ev.Path, ev.Source, ev.Referrer, ev.UserID, ev.SessionID, ev.Country,
				ev.Browser, ev.OS, ev.Device, ev.Language, ev.Screen, ev.Title, ev.UTMSource, ev.UTMMedium, ev.UTMCampaign, ev.Props, prefix+e.ID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				sr.Inserted++
			} else {
				sr.AlreadyImported++
			}
			return nil
		})
		if err != nil {
			return rep, fmt.Errorf("读取 Umami 站点 %s 失败：%w", id, err)
		}
		if first > 0 {
			sr.FirstEvent, sr.LastEvent = time.Unix(first, 0).UTC().Format(time.RFC3339), time.Unix(last, 0).UTC().Format(time.RFC3339)
		}
		types, err := src.EventTypeCounts(ctx, id, opts.Window)
		if err != nil {
			return rep, err
		}
		for t, n := range types {
			if t != 1 && t != 2 && n > 0 {
				sr.OtherTypes[fmt.Sprint(t)] = n
			}
		}
		sourceDays, err := src.DayCounts(ctx, id, p.site.Timezone, opts.Window)
		if err != nil {
			return rep, fmt.Errorf("读取 Umami 站点 %s 的按日统计失败：%w", id, err)
		}
		bySource := map[string]UmamiDay{}
		keys := map[string]bool{}
		for _, d := range sourceDays {
			bySource[d.Day] = d
			keys[d.Day] = true
		}
		for d := range days {
			keys[d] = true
		}
		var ordered []string
		for d := range keys {
			ordered = append(ordered, d)
		}
		sort.Strings(ordered)
		for _, day := range ordered {
			conv, s := days[day], bySource[day]
			if conv == nil {
				conv = &dayAgg{}
			}
			bad := func(format string, args ...any) {
				sr.Problems = append(sr.Problems, day+"："+fmt.Sprintf(format, args...))
			}
			if s.Pageviews != conv.pageviews {
				bad("Umami 有 %d 条浏览，转换了 %d 条", s.Pageviews, conv.pageviews)
			}
			if s.CustomRaw != conv.custom+conv.skippedCustom {
				bad("Umami 有 %d 条自定义事件，转换 %d 条、跳过 %d 条", s.CustomRaw, conv.custom, conv.skippedCustom)
			}
			if tx != nil {
				t, err := importDay(ctx, tx, p.site.ID, id, p.loc, day, opts.Window)
				if err != nil {
					return rep, err
				}
				if t.Pageviews != conv.pageviews || t.Custom != conv.custom {
					bad("写入后 Plost 有 %d 条浏览、%d 条自定义事件，应为 %d、%d", t.Pageviews, t.Custom, conv.pageviews, conv.custom)
				}
				if t.Visitors != s.Visitors || t.Visits != s.Visits {
					bad("写入后 Plost 有 %d 位访客、%d 次访问，Umami 为 %d、%d", t.Visitors, t.Visits, s.Visitors, s.Visits)
				}
			}
			sr.Days = append(sr.Days, DayStats{Day: day, Pageviews: conv.pageviews, Custom: conv.custom, Visitors: s.Visitors, Visits: s.Visits})
		}
		if opts.Strict {
			if len(sr.Skipped) > 0 || len(sr.OtherTypes) > 0 {
				sr.Problems = append(sr.Problems, "严格模式：存在 Plost 无法表示而被跳过的事件")
			}
		}
		if len(sr.Problems) > 0 {
			failed = true
		}
		rep.Sites = append(rep.Sites, sr)
	}
	if failed {
		return rep, ErrReconcile
	}
	if tx != nil {
		if err = tx.Commit(); err != nil {
			return rep, err
		}
		// A run that changed nothing, such as repeating an import, does not leave a copy of the database behind.
		changed := false
		for _, s := range rep.Sites {
			changed = changed || s.Inserted > 0 || s.CreatedSite
		}
		if !changed && rep.Snapshot != "" {
			_ = os.Remove(rep.Snapshot)
			rep.Snapshot = ""
		}
	}
	return rep, nil
}

func typeNames(m map[int]int64) map[string]int64 {
	out := map[string]int64{}
	for t, n := range m {
		out[fmt.Sprint(t)] = n
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

type RollbackReport struct {
	Domain   string `json:"domain"`
	Imported int64  `json:"importedEvents"`
	Removed  int64  `json:"removed"`
	Snapshot string `json:"snapshot,omitempty"`
}

// RollbackUmami removes only events that came from Umami for one site; live events are never touched.
func (imp *Importer) RollbackUmami(ctx context.Context, domain string, apply bool) (RollbackReport, error) {
	domain, err := normalizeDomain(domain)
	rep := RollbackReport{Domain: domain}
	if err != nil {
		return rep, err
	}
	if apply && !imp.write {
		return rep, errors.New("回滚需要以写入模式打开数据目录")
	}
	site, err := readSite(ctx, imp.db, domain)
	if err != nil {
		return rep, fmt.Errorf("Plost 里没有站点 %s", domain)
	}
	if !imp.hasSourceColumn(ctx) {
		return rep, nil
	}
	if err = imp.db.QueryRowContext(ctx, "SELECT count(*) FROM events_v2 WHERE site_id=? AND source_id>='umami:' AND source_id<'umami;'", site.ID).Scan(&rep.Imported); err != nil || !apply || rep.Imported == 0 {
		return rep, err
	}
	if rep.Snapshot, err = imp.snapshot(ctx); err != nil {
		return rep, err
	}
	res, err := imp.db.ExecContext(ctx, "DELETE FROM events_v2 WHERE site_id=? AND source_id>='umami:' AND source_id<'umami;'", site.ID)
	if err != nil {
		return rep, err
	}
	rep.Removed, _ = res.RowsAffected()
	return rep, nil
}
