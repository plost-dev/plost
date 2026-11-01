package plost

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	DataDir    string
	Origin     string
	TrustProxy bool
	Version    string
	// OutboundAllow lists networks that site checks and cloud backups may connect to although they are
	// normally refused (private, loopback, reserved). It exists for setups such as a local proxy that
	// answers every name with a fake address, and is only ever set by whoever starts the program.
	OutboundAllow []netip.Prefix
}

type App struct {
	cfg      Config
	db       *sql.DB
	mu       sync.RWMutex // Quiesce all database access for an in-process restore.
	secret   []byte
	epoch    uint64
	queue    chan *ingest
	stop     chan struct{}
	workers  sync.WaitGroup
	limits   *limiter
	closed   sync.Once
	lockFile *os.File

	keyMu         sync.Mutex // Guards the lazily loaded key for stored backup credentials.
	key           []byte
	backupRunning atomic.Bool
	jobsMu        sync.Mutex // Orders job starts against Close.
	closing       bool
	jobCtx        context.Context
	cancelJobs    context.CancelFunc
	ceremonyMu    sync.Mutex // Guards pending passkey registrations and sign-ins.
	ceremonies    map[string]ceremony

	// Test seams: tests point cloud requests at a local fake and allow its loopback address.
	newCloud          func(cloudSettings) *s3Client
	allowPrivateCloud bool

	siteProbe     siteChecker // Network probes behind the site check; tests replace them.
	outboundAllow []netip.Prefix
}

func Open(cfg Config) (*App, error) {
	allow := make([]netip.Prefix, 0, len(cfg.OutboundAllow))
	for _, network := range cfg.OutboundAllow {
		widest := 16 // IPv6
		if network.Addr().Is4() || network.Addr().Is4In6() {
			widest = 8
		}
		if !network.IsValid() || network.Bits() < widest {
			return nil, errors.New("allow-outbound networks must be valid and no wider than /8 (IPv4) or /16 (IPv6)")
		}
		allow = append(allow, network.Masked())
	}
	cfg.OutboundAllow = allow
	if cfg.Origin != "" {
		u, err := url.Parse(cfg.Origin)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("origin must be an HTTP(S) origin without a path")
		}
		cfg.Origin = strings.TrimSuffix(cfg.Origin, "/")
	}
	dir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	cfg.DataDir = dir
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := lockDirectory(dir)
	if err != nil {
		return nil, err
	}
	opened := false
	defer func() {
		if !opened {
			unlockDirectory(lock)
		}
	}()
	path := filepath.Join(dir, "plost.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	var secret string
	if err = db.QueryRow("SELECT value FROM settings WHERE key='visitor_secret'").Scan(&secret); err != nil {
		db.Close()
		return nil, err
	}
	key, err := hex.DecodeString(secret)
	if err != nil || len(key) != 32 {
		db.Close()
		return nil, errors.New("invalid visitor secret")
	}
	a := &App{cfg: cfg, db: db, secret: key, queue: make(chan *ingest, 1024), stop: make(chan struct{}), limits: newLimiter(), lockFile: lock}
	a.jobCtx, a.cancelJobs = context.WithCancel(context.Background())
	a.outboundAllow = cfg.OutboundAllow
	a.siteProbe = newSiteChecker(cfg.OutboundAllow)
	a.workers.Add(3)
	go a.ingestLoop()
	go a.scheduleLoop()
	go a.siteCheckLoop()
	opened = true
	return a, nil
}

func openDB(path string) (*sql.DB, error) {
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)", "foreign_keys(ON)", "cache_size(-4096)", "temp_store(FILE)", "trusted_schema(OFF)"} {
		q.Add("_pragma", p)
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	// A single connection serializes writes and bounds cache memory. Queries have deadlines.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS users(
 id INTEGER PRIMARY KEY CHECK(id=1),email TEXT NOT NULL UNIQUE,name TEXT NOT NULL,
 password_hash TEXT NOT NULL,locale TEXT NOT NULL DEFAULT 'zh-CN',theme TEXT NOT NULL DEFAULT 'system',
 totp_secret TEXT NOT NULL DEFAULT '',totp_pending TEXT NOT NULL DEFAULT '',totp_last INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(
 token_hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 host TEXT NOT NULL,expires INTEGER NOT NULL,pending INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS recovery_codes(hash TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS sites(
 id INTEGER PRIMARY KEY AUTOINCREMENT,domain TEXT NOT NULL UNIQUE,name TEXT NOT NULL,
 timezone TEXT NOT NULL DEFAULT 'Etc/UTC',public INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS shared_links(
 id INTEGER PRIMARY KEY AUTOINCREMENT,site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
 name TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS events_v2(
 id INTEGER PRIMARY KEY AUTOINCREMENT,site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
 timestamp INTEGER NOT NULL,name TEXT NOT NULL,hostname TEXT NOT NULL,pathname TEXT NOT NULL,
 referrer_source TEXT NOT NULL DEFAULT '',referrer TEXT NOT NULL DEFAULT '',
 user_id TEXT NOT NULL,session_id TEXT NOT NULL,
 country_code TEXT NOT NULL DEFAULT '',browser TEXT NOT NULL DEFAULT '',os TEXT NOT NULL DEFAULT '',
 device TEXT NOT NULL DEFAULT '',language TEXT NOT NULL DEFAULT '',screen TEXT NOT NULL DEFAULT '',title TEXT NOT NULL DEFAULT '',
 utm_source TEXT NOT NULL DEFAULT '',utm_medium TEXT NOT NULL DEFAULT '',utm_campaign TEXT NOT NULL DEFAULT '',
 props TEXT NOT NULL DEFAULT '{}');
CREATE INDEX IF NOT EXISTS events_site_time ON events_v2(site_id,timestamp);
CREATE INDEX IF NOT EXISTS events_visitor_time ON events_v2(site_id,user_id,timestamp DESC);
CREATE INDEX IF NOT EXISTS events_session ON events_v2(site_id,session_id,timestamp);
CREATE TABLE IF NOT EXISTS backup_settings(
 id INTEGER PRIMARY KEY CHECK(id=1),provider TEXT NOT NULL DEFAULT 'r2',endpoint TEXT NOT NULL DEFAULT '',
 region TEXT NOT NULL DEFAULT 'auto',bucket TEXT NOT NULL DEFAULT '',prefix TEXT NOT NULL DEFAULT '',
 access_key TEXT NOT NULL DEFAULT '',secret_key TEXT NOT NULL DEFAULT '',backup_password TEXT NOT NULL DEFAULT '',
 cloud_enabled INTEGER NOT NULL DEFAULT 0,schedule TEXT NOT NULL DEFAULT 'off',
 hour INTEGER NOT NULL DEFAULT 3,minute INTEGER NOT NULL DEFAULT 0,weekday INTEGER NOT NULL DEFAULT 0,keep INTEGER NOT NULL DEFAULT 7,
 last_run_at INTEGER NOT NULL DEFAULT 0,last_attempt_at INTEGER NOT NULL DEFAULT 0,
 last_status TEXT NOT NULL DEFAULT '',last_error TEXT NOT NULL DEFAULT '',last_backup TEXT NOT NULL DEFAULT '');
INSERT OR IGNORE INTO backup_settings(id) VALUES(1);
CREATE TABLE IF NOT EXISTS site_check_settings(
 site_id INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,enabled INTEGER NOT NULL DEFAULT 0,
 path TEXT NOT NULL DEFAULT '/',version INTEGER NOT NULL DEFAULT 1,next_check_at INTEGER NOT NULL DEFAULT 0,
 last_started_at INTEGER NOT NULL DEFAULT 0,expiry TEXT NOT NULL DEFAULT '',expiry_host TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS site_check_results(
 id INTEGER PRIMARY KEY AUTOINCREMENT,site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
 target_url TEXT NOT NULL,checked_at INTEGER NOT NULL,ok INTEGER NOT NULL,status INTEGER NOT NULL DEFAULT 0,
 duration_ms INTEGER NOT NULL DEFAULT -1,error TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS site_check_results_site_time ON site_check_results(site_id,checked_at DESC);
CREATE TABLE IF NOT EXISTS passkeys(
 id INTEGER PRIMARY KEY AUTOINCREMENT,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 credential_id TEXT NOT NULL UNIQUE,name TEXT NOT NULL,credential TEXT NOT NULL,
 created_at INTEGER NOT NULL,last_used_at INTEGER NOT NULL DEFAULT 0);
PRAGMA user_version=1;
`

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v > 1 {
		return errors.New("database was created by a newer Plost version")
	}
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	for _, column := range []struct{ table, name, kind string }{{"users", "avatar", "TEXT NOT NULL DEFAULT ''"}, {"shared_links", "slug", "TEXT NOT NULL DEFAULT ''"}, {"events_v2", "source_id", "TEXT NOT NULL DEFAULT ''"}, {"events_v2", "session_visitor_id", "TEXT NOT NULL DEFAULT ''"}} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name=?", column.table, column.name).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := db.Exec("ALTER TABLE " + column.table + " ADD COLUMN " + column.name + " " + column.kind); err != nil {
				return err
			}
		}
	}
	// Imported history carries the id of its source row, which makes a repeated import a no-op and lets it be rolled back.
	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS events_source ON events_v2(site_id,source_id) WHERE source_id<>''"); err != nil {
		return err
	}
	_, err := db.Exec("INSERT INTO settings(key,value) VALUES('visitor_secret',?) ON CONFLICT(key) DO NOTHING", randomHex(32))
	return err
}

func (a *App) Close() error {
	var err error
	a.closed.Do(func() {
		a.jobsMu.Lock()
		a.closing = true
		a.jobsMu.Unlock()
		a.cancelJobs()
		close(a.stop)
		a.workers.Wait()
		a.mu.Lock()
		defer a.mu.Unlock()
		err = a.db.Close()
		unlockDirectory(a.lockFile)
	})
	return err
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("secure random source unavailable")
	}
	return hex.EncodeToString(b)
}

type Site struct {
	ID        int64  `json:"id"`
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Timezone  string `json:"timezone"`
	Public    bool   `json:"public"`
	CreatedAt int64  `json:"createdAt"`
}

func readSite(ctx context.Context, db *sql.DB, domain string) (Site, error) {
	var s Site
	err := db.QueryRowContext(ctx, "SELECT id,domain,name,timezone,public,created_at FROM sites WHERE domain=?", domain).Scan(&s.ID, &s.Domain, &s.Name, &s.Timezone, &s.Public, &s.CreatedAt)
	return s, err
}

func normalizeDomain(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Port() != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("请输入不含路径和端口的网站域名")
	}
	host := strings.TrimSuffix(strings.TrimPrefix(u.Hostname(), "www."), ".")
	if len(host) > 253 || !strings.Contains(host, ".") || strings.ContainsAny(host, " /\\@:%[]") {
		return "", errors.New("网站域名无效")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", errors.New("网站域名无效")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", errors.New("国际域名请使用 Punycode")
			}
		}
	}
	return host, nil
}

func unixNow() int64 { return time.Now().Unix() }
