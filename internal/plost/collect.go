package plost

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Event struct {
	SiteID                                                                                                                                                                                  int64
	At                                                                                                                                                                                      int64
	Name, Hostname, Path, Source, Referrer, UserID, SessionID, SessionVisitor, Country, Region, City, CityZh, Browser, OS, Device, Language, Screen, Title, UTMSource, UTMMedium, UTMCampaign, Props, Via string
}
type ingest struct {
	event Event
	done  chan error
	epoch uint64
}

var visitorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var screenPattern = regexp.MustCompile(`^[0-9]{1,5}x[0-9]{1,5}$`)
var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// botPattern names crawlers, link previews, monitors and scripted clients. Most crawlers never run the tracker at all;
// this catches the ones that do and say who they are. Browsers driven by automation that pose as Chrome are left
// out by the tracker itself (navigator.webdriver).
var botPattern = regexp.MustCompile(`(?i)bot\b|spider|crawl|headless|lighthouse|pagespeed|gtmetrix|facebookexternalhit|facebookcatalog|meta-externalagent|preview|monitor|uptime|pingdom|statuscake|site24x7|slurp|bingpreview|yandex(?:bot|images|metrika)|baiduspider|bytespider|petalbot|semrush|ahrefs|mj12|dotbot|ia_archiver|archive\.org|gptbot|chatgpt-user|claudebot|anthropic-ai|perplexity|ccbot|amazonbot|cohere-ai|applebot|duckduckbot|googleother|google-inspectiontool|feedfetcher|embedly|whatsapp/|telegrambot|discordbot|slack-imgproxy|skypeuripreview|validator|scanner|python-requests|python-urllib|aiohttp|httpx|curl/|wget/|go-http-client|okhttp|java/|libwww|httpclient|axios/|node-fetch|undici|scrapy|phantomjs|puppeteer|playwright|selenium`)

// isBot reports whether a request comes from a client that should not count as a visitor. A request without a user
// agent never comes from a browser. "CUBOT" is a phone maker whose model names would otherwise read as "…bot".
func isBot(ua string) bool {
	if strings.TrimSpace(ua) == "" {
		return true
	}
	return botPattern.MatchString(strings.ReplaceAll(strings.ToLower(ua), "cubot", ""))
}

var eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,40}$`)

// When the tracker's normal request fails (a content blocker or a broken network refuses it), it sends the same
// event again to /e, first as an image and then as a Beacon, and says which with "x". The value is stored with the
// event so the share of events that only arrived that way can be counted. Normal requests to /api/send are stored as
// "send"; an empty value is the older /api/event request.
var retryTransports = map[string]bool{"img": true, "beacon": true}

// pixel is the 1×1 transparent GIF returned to image requests, so the tracker can tell a delivered event (load) from a
// refused one (error) and only then try a Beacon.
var pixel = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\x00\x00\x00!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")

// accepted answers a stored or quietly ignored event: 202 for the tracker's requests, the pixel for an image request.
func accepted(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "image/gif")
		w.WriteHeader(200)
		_, _ = w.Write(pixel)
		return
	}
	w.WriteHeader(202)
}

// recentEvents remembers the ids of events stored in the last quarter of an hour, so a retry of an event that did
// arrive (the tracker cannot always tell) is not stored twice.
type recentEvents struct {
	mu   sync.Mutex
	seen map[string]int64
}

const recentEventTTL = 15 * 60

func newRecentEvents() *recentEvents { return &recentEvents{seen: make(map[string]int64)} }

// claim reserves an id and reports whether it was free. A claim that does not end in a stored event is released.
func (r *recentEvents) claim(key string, now int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if at, ok := r.seen[key]; ok && now-at < recentEventTTL {
		return false
	}
	if len(r.seen) >= 200_000 {
		for k, at := range r.seen {
			if now-at >= recentEventTTL {
				delete(r.seen, k)
			}
		}
		if len(r.seen) >= 200_000 {
			return true // too busy to remember; storing a rare duplicate beats losing the event
		}
	}
	r.seen[key] = now
	return true
}

func (r *recentEvents) release(key string) {
	r.mu.Lock()
	delete(r.seen, key)
	r.mu.Unlock()
}

func short(s string, max int) string {
	if len(s) > max {
		s = s[:max]
	}
	return strings.ToValidUTF8(s, "")
}
func first(v map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		var s string
		if json.Unmarshal(v[name], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}
func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(host), "www."), ".")
}

func (a *App) collect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	// /api/send takes JSON like Umami's tracker, which needs a preflight; let browsers reuse it.
	w.Header().Set("Access-Control-Max-Age", "86400")
	// No credentials or private data are exposed by this ingestion-only endpoint.
	if o := r.Header.Get("Origin"); o != "" && o != "null" {
		w.Header().Set("Access-Control-Allow-Origin", o)
	}
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	var b []byte
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		// An image request carries the event in the query string instead of a body.
		b = []byte(r.URL.Query().Get("p"))
		if len(b) > 16<<10 {
			writeError(w, failure(413, "请求过大"))
			return
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var err error
		if b, err = io.ReadAll(r.Body); err != nil {
			writeError(w, failure(413, "请求过大"))
			return
		}
	}
	if r.URL.Path == "/api/send" {
		converted, err := fromUmami(b)
		if errors.Is(err, errUmamiIgnored) {
			accepted(w, r)
			return
		}
		if err != nil {
			writeError(w, failure(400, "请求格式无效"))
			return
		}
		b = converted
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(b, &data) != nil {
		writeError(w, failure(400, "请求格式无效"))
		return
	}
	ua := r.UserAgent()
	if isBot(ua) {
		accepted(w, r)
		return
	}
	u, err := url.Parse(first(data, "u", "url"))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || len(u.String()) > 8192 {
		writeError(w, failure(400, "页面地址无效"))
		return
	}
	domain := first(data, "d", "domain")
	if domain == "" {
		domain = u.Hostname()
	}
	domain, err = normalizeDomain(domain)
	if err != nil || canonicalHost(u.Hostname()) != domain {
		accepted(w, r)
		return
	}
	if o := r.Header.Get("Origin"); o != "" {
		origin, err := url.Parse(o)
		if err != nil || canonicalHost(origin.Hostname()) != domain || (origin.Scheme != "http" && origin.Scheme != "https") {
			accepted(w, r)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	a.mu.RLock()
	site, err := readSite(ctx, a.db, domain)
	if err != nil {
		a.mu.RUnlock()
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, err)
		} else {
			accepted(w, r)
		}
		return
	}
	loc, _ := time.LoadLocation(site.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now()
	visitor := first(data, "v", "visitor")
	if visitor != "" && !visitorPattern.MatchString(visitor) {
		a.mu.RUnlock()
		writeError(w, failure(400, "访客标识无效"))
		return
	}
	identity := visitor
	if identity == "" {
		addr, _ := a.visitorAddr(r)
		identity = addr.String() + "|" + ua + "|" + now.In(loc).Format("2006-01-02")
	}
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte(strconvID(site.ID) + "|" + identity))
	userID := hex.EncodeToString(h.Sum(nil))
	sessionVisitor := a.sessionVisitor(r, site.ID, ua, now)
	epoch := a.epoch
	a.mu.RUnlock()
	name := first(data, "n", "name")
	if name == "" {
		name = "pageview"
	}
	if len(name) > 255 {
		writeError(w, failure(400, "事件名称过长"))
		return
	}
	ref := first(data, "r", "referrer")
	source := "Direct"
	if parsed, e := url.Parse(ref); e == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		source = canonicalHost(parsed.Hostname())
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		ref = parsed.String()
	} else {
		ref = ""
	}
	if source == domain {
		source = "Direct"
	}
	q := u.Query()
	source = sourceName(source, q.Get("utm_source"))
	props := map[string]any{}
	for _, k := range []string{"p", "props", "m", "meta"} {
		if len(data[k]) > 0 {
			if json.Unmarshal(data[k], &props) != nil {
				writeError(w, failure(400, "事件属性无效"))
				return
			}
			break
		}
	}
	if len(props) > 32 {
		writeError(w, failure(400, "事件属性过多"))
		return
	}
	for k, v := range props {
		if len(k) > 300 {
			writeError(w, failure(400, "事件属性名称过长"))
			return
		}
		switch v := v.(type) {
		case string:
			props[k] = short(v, 2000)
		case nil, bool, float64:
		default:
			writeError(w, failure(400, "事件属性只接受简单值"))
			return
		}
	}
	propsJSON, _ := json.Marshal(props)
	country := a.requestCountry(r, now)
	browser, osName, device := parseUA(ua)
	var width int
	_ = json.Unmarshal(data["w"], &width)
	if width <= 0 {
		_ = json.Unmarshal(data["width"], &width)
	}
	if device != "Tablet" && device != "Mobile" && width > 0 && width < 768 {
		device = "Mobile"
	}
	screen := strings.ToLower(first(data, "s", "screen"))
	if !screenPattern.MatchString(screen) {
		screen = ""
	}
	pathname := u.EscapedPath()
	if decoded, e := url.PathUnescape(pathname); e == nil {
		pathname = decoded
	}
	if pathname == "" {
		pathname = "/"
	}
	place := a.requestPlace(r, country, now)
	ev := Event{SiteID: site.ID, At: now.Unix(), Name: name, Hostname: canonicalHost(u.Hostname()), Path: short(pathname, 2048), Source: source, Referrer: short(ref, 2048), UserID: userID, SessionVisitor: sessionVisitor, Country: country, Region: place.region, City: place.city, CityZh: place.cityZh, Browser: browser, OS: osName, Device: device, Language: short(first(data, "l", "language"), 64), Screen: screen, Title: short(first(data, "t", "title"), 500), UTMSource: short(q.Get("utm_source"), 200), UTMMedium: short(q.Get("utm_medium"), 200), UTMCampaign: short(q.Get("utm_campaign"), 200), Props: string(propsJSON)}
	if via := first(data, "x"); retryTransports[via] {
		ev.Via = via
	} else if r.URL.Path == "/api/send" {
		ev.Via = "send"
	}
	// An event that carries an id is stored once, however many of the tracker's retries reach us.
	claimed := ""
	if id := first(data, "i"); eventIDPattern.MatchString(id) {
		claimed = strconvID(site.ID) + ":" + id
		if !a.recent.claim(claimed, now.Unix()) {
			accepted(w, r)
			return
		}
	}
	release := func() {
		if claimed != "" {
			a.recent.release(claimed)
		}
	}
	if name == "pageview" {
		visitor := sessionVisitor
		if visitor == "" {
			visitor = userID
		}
		a.recordPlace(r, site.ID, visitor, country, now)
	}
	req := &ingest{event: ev, done: make(chan error, 1), epoch: epoch}
	select {
	case <-a.stop:
		release()
		writeError(w, failure(503, "服务正在关闭"))
		return
	default:
	}
	select {
	case a.queue <- req:
	default:
		release()
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
		_ = jsonResponse(w, map[string]string{"error": "event_buffer_full"})
		return
	}
	select {
	case err := <-req.done:
		if err != nil {
			release()
			writeError(w, err)
			return
		}
		accepted(w, r)
	case <-ctx.Done():
		// The worker may still store it, so the id stays claimed.
		writeError(w, failure(503, "采集超时"))
	}
}

func (a *App) ingestLoop() {
	defer a.workers.Done()
	for {
		select {
		case req := <-a.queue:
			batch := []*ingest{req}
			timer := time.NewTimer(4 * time.Millisecond)
		gather:
			for len(batch) < 64 {
				select {
				case next := <-a.queue:
					batch = append(batch, next)
				case <-timer.C:
					break gather
				case <-a.stop:
					break gather
				}
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			err := a.persistBatch(batch)
			for _, r := range batch {
				r.done <- err
			}
		case <-a.stop:
			for {
				select {
				case req := <-a.queue:
					err := a.persistBatch([]*ingest{req})
					req.done <- err
				default:
					return
				}
			}
		}
	}
}

func (a *App) persistBatch(batch []*ingest) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO events_v2(site_id,timestamp,name,hostname,pathname,referrer_source,referrer,user_id,session_id,session_visitor_id,country_code,region,city,city_zh,browser,os,device,language,screen,title,utm_source,utm_medium,utm_campaign,props,via) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, req := range batch {
		if req.epoch != a.epoch {
			return failure(503, "恢复切换期间请重试采集")
		}
		e := req.event
		var session string
		// A visit continues while the same visitor stays active: the visitor is the address hash when the event has one,
		// otherwise the browser id. A new address starts a new visit, which is what makes it a new visitor.
		if e.SessionVisitor != "" {
			err = tx.QueryRowContext(ctx, "SELECT session_id FROM events_v2 WHERE site_id=? AND session_visitor_id=? AND session_visitor_id<>'' AND timestamp>=? ORDER BY timestamp DESC,id DESC LIMIT 1", e.SiteID, e.SessionVisitor, e.At-1800).Scan(&session)
		} else {
			err = tx.QueryRowContext(ctx, "SELECT session_id FROM events_v2 WHERE site_id=? AND user_id=? AND timestamp>=? ORDER BY timestamp DESC,id DESC LIMIT 1", e.SiteID, e.UserID, e.At-1800).Scan(&session)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if session == "" {
			session = randomHex(16)
		}
		_, err = stmt.ExecContext(ctx, e.SiteID, e.At, e.Name, e.Hostname, e.Path, e.Source, e.Referrer, e.UserID, session, e.SessionVisitor, e.Country, e.Region, e.City, e.CityZh, e.Browser, e.OS, e.Device, e.Language, e.Screen, e.Title, e.UTMSource, e.UTMMedium, e.UTMCampaign, e.Props, e.Via)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.changes.notify()
	return nil
}

func parseUA(ua string) (browser, osName, device string) {
	browser = "Other"
	osName = "Other"
	device = "Desktop"
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "EdgA/") || strings.Contains(ua, "EdgiOS/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	switch {
	case strings.Contains(ua, "Android"):
		osName = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		osName = "iOS"
	case strings.Contains(ua, "Windows"):
		osName = "Windows"
	case strings.Contains(ua, "CrOS"):
		osName = "Chrome OS"
	case strings.Contains(ua, "Macintosh"):
		osName = "macOS"
	case strings.Contains(ua, "Linux"):
		osName = "Linux"
	}
	if strings.Contains(ua, "Mobile") || strings.Contains(ua, "iPhone") {
		device = "Mobile"
	}
	if strings.Contains(ua, "iPad") || (strings.Contains(ua, "Android") && !strings.Contains(ua, "Mobile")) {
		device = "Tablet"
	}
	return
}
