package plost

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// The settings page can keep a GeoLite2 database up to date by itself, so a deployer need not run geoipupdate:
// it downloads either from the Plost mirror (ipdata.plost.io, no account needed) or from MaxMind with the
// deployer's own account. A download is checked against its published SHA-256, unpacked, opened and verified
// before it is renamed over the database in use; the previous version and every leftover download are then
// deleted, so the data directory holds one database at a time. -geoip-db turns all of this off: the file it
// names is the deployer's to update.

const (
	EnvMaxMindAccount = "PLOST_MAXMIND_ACCOUNT_ID"
	EnvMaxMindKey     = "PLOST_MAXMIND_LICENSE_KEY"

	geoMirrorBase  = "https://ipdata.plost.io/"
	geoMaxMindBase = "https://download.maxmind.com/geoip/databases/"

	geoCheckDaily   = 24 * time.Hour // how often a working database is checked for a new version
	geoRetryAfter   = time.Hour      // how soon a failed check is tried again
	geoDownloadMax  = 256 << 20      // the City archive is about 28 MB
	geoUnpackedMax  = 512 << 20      // the City database is about 50 MB
	geoDownloadTime = 10 * time.Minute
)

// geoFirstCheck is how long after start the first check waits; tests push it out so they never reach the network.
var geoFirstCheck = 20 * time.Second

var (
	geoEditions        = map[string]bool{"GeoLite2-City": true, "GeoLite2-Country": true}
	geoSources         = map[string]bool{"plost": true, "maxmind": true, "off": true}
	maxMindAccountForm = regexp.MustCompile(`^[0-9]{1,10}$`)
	maxMindKeyForm     = regexp.MustCompile(`^[A-Za-z0-9_]{16,64}$`)
	sha256Form         = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// geoState is what the settings page shows about the downloaded database, kept in settings.geoip_state.
type geoState struct {
	Source    string `json:"source,omitempty"`  // where the database in use came from
	Edition   string `json:"edition,omitempty"` // which database it is
	SHA256    string `json:"sha256,omitempty"`  // of the archive it came in, compared with the published one
	BuiltAt   int64  `json:"builtAt,omitempty"` // when MaxMind built it
	Size      int64  `json:"size,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"` // when it was put in place
	CheckedAt int64  `json:"checkedAt,omitempty"` // the last look for a new version, successful or not
	Error     string `json:"error,omitempty"`     // why that look failed
}

func (a *App) geoManaged() bool { return a.cfg.GeoIPPath == "" }

func (a *App) geoDir() string { return filepath.Join(a.cfg.DataDir, "geoip") }

func (a *App) geoFile() string { return filepath.Join(a.geoDir(), "GeoLite2.mmdb") }

func (a *App) maxMindKeys() (account, key string) {
	a.keysMu.RLock()
	defer a.keysMu.RUnlock()
	return a.cfg.MaxMindAccount, a.cfg.MaxMindKey
}

// geoSettings reads the chosen source and edition. Without a choice the Plost mirror's City database is used, so a
// new instance places its visitors without any setup.
func (a *App) geoSettings(ctx context.Context) (source, edition string, state geoState) {
	source = a.setting(ctx, "geoip_source", "plost")
	if !geoSources[source] {
		source = "plost"
	}
	edition = a.setting(ctx, "geoip_edition", "GeoLite2-City")
	if !geoEditions[edition] {
		edition = "GeoLite2-City"
	}
	_ = json.Unmarshal([]byte(a.setting(ctx, "geoip_state", "{}")), &state)
	return source, edition, state
}

func (a *App) saveGeoState(state geoState) {
	data, _ := json.Marshal(state)
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, err := a.db.ExecContext(context.Background(), "INSERT INTO settings(key,value) VALUES('geoip_state',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(data)); err != nil {
		log.Printf("GeoIP state not saved: %v", err)
	}
}

// geoDue reports whether the background check should look for a new version now.
func geoDue(source, edition string, state geoState, loaded bool, now time.Time) bool {
	if source == "off" {
		return loaded || state.Source != ""
	}
	if !loaded || state.Source != source || state.Edition != edition {
		return state.Error == "" || now.Unix()-state.CheckedAt >= int64(geoRetryAfter/time.Second)
	}
	wait := geoCheckDaily
	if state.Error != "" {
		wait = geoRetryAfter
	}
	return now.Unix()-state.CheckedAt >= int64(wait/time.Second)
}

func (a *App) geoipLoop() {
	defer a.workers.Done()
	if !a.geoManaged() {
		return
	}
	timer := time.NewTimer(geoFirstCheck)
	defer timer.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-timer.C:
			a.mu.RLock()
			source, edition, state := a.geoSettings(a.jobCtx)
			a.mu.RUnlock()
			if geoDue(source, edition, state, a.geo.ready(), time.Now()) {
				_, _ = a.updateGeoIP(a.jobCtx)
			}
			timer.Reset(10 * time.Minute)
		}
	}
}

// updateGeoIP brings the database in line with the settings: it downloads a new version when the published one
// differs from the one in use, or removes the database when GeoIP is off. A failure keeps the database in use.
func (a *App) updateGeoIP(ctx context.Context) (geoState, error) {
	if !a.geoUpdating.CompareAndSwap(false, true) {
		return geoState{}, failure(409, "IP 数据库正在更新，请稍后再试")
	}
	defer a.geoUpdating.Store(false)
	return a.runGeoUpdate(ctx)
}

// runGeoUpdate does the work of updateGeoIP; the caller holds geoUpdating.
func (a *App) runGeoUpdate(ctx context.Context) (geoState, error) {
	a.mu.RLock()
	source, edition, state := a.geoSettings(ctx)
	a.mu.RUnlock()
	now := time.Now()
	if source == "off" {
		a.geo.unload()
		err := a.clearGeoDir("")
		state = geoState{}
		if err != nil {
			state.Error = "删除 IP 数据库失败：" + err.Error()
		}
		a.saveGeoState(state)
		return state, nil
	}
	next, err := a.downloadGeoIP(ctx, source, edition, state)
	if err != nil {
		state.CheckedAt, state.Error = now.Unix(), err.Error()
		a.saveGeoState(state)
		return state, failure(502, err.Error())
	}
	next.CheckedAt = time.Now().Unix()
	a.saveGeoState(next)
	return next, nil
}

// geoURLs names the archive and its checksum for the source.
func (a *App) geoURLs(source, edition string) (archive, sum string) {
	if source == "maxmind" {
		base := a.geoMaxMindURL
		if base == "" {
			base = geoMaxMindBase
		}
		archive = base + edition + "/download?suffix=tar.gz"
		return archive, archive + ".sha256"
	}
	base := a.geoMirrorURL
	if base == "" {
		base = geoMirrorBase
	}
	return base + edition + ".tar.gz", base + edition + ".tar.gz.sha256"
}

func (a *App) geoClient() *http.Client {
	if a.geoHTTP != nil {
		return a.geoHTTP
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: outboundGuard(a.outboundAllow)}
	return &http.Client{Timeout: geoDownloadTime, Transport: &http.Transport{
		// Environment proxies would make the dial-time address check see the proxy instead of the download host.
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableKeepAlives:     true,
	}}
}

// geoGet fetches a URL from the source. MaxMind wants the account and licence key; it answers with a redirect to
// its storage, and the client drops the credentials on the way there because the host differs.
func (a *App) geoGet(ctx context.Context, source, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Plost/"+a.cfg.Version)
	if source == "maxmind" {
		account, key := a.maxMindKeys()
		if account == "" || key == "" {
			return nil, errors.New("请先填写并保存 MaxMind 账号 ID 和 License Key")
		}
		req.SetBasicAuth(account, key)
	}
	resp, err := a.geoClient().Do(req)
	if err != nil {
		if errors.Is(err, errNotPublic) {
			return nil, errors.New("下载地址没有解析到公网地址")
		}
		return nil, errors.New("无法连接下载服务器，请检查服务器的网络")
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	resp.Body.Close()
	switch {
	case source == "maxmind" && resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("MaxMind 账号 ID 或 License Key 不正确")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("下载次数过多，请稍后再试")
	}
	return nil, fmt.Errorf("下载服务器返回 HTTP %d", resp.StatusCode)
}

func (a *App) downloadGeoIP(ctx context.Context, source, edition string, state geoState) (geoState, error) {
	archiveURL, sumURL := a.geoURLs(source, edition)
	resp, err := a.geoGet(ctx, source, sumURL)
	if err != nil {
		return state, err
	}
	line, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	resp.Body.Close()
	fields := strings.Fields(string(line))
	if len(fields) == 0 || !sha256Form.MatchString(strings.ToLower(fields[0])) {
		return state, errors.New("下载服务器没有给出有效的 SHA-256 校验值")
	}
	want := strings.ToLower(fields[0])
	if want == state.SHA256 && source == state.Source && edition == state.Edition && a.geo.ready() {
		state.Error = ""
		return state, nil
	}
	if err := os.MkdirAll(a.geoDir(), 0o700); err != nil {
		return state, fmt.Errorf("无法创建 IP 数据库目录：%w", err)
	}
	// Leftovers from a download that was cut off go first, so a failed run cannot pile them up.
	_ = a.clearGeoDir(a.geoFile())

	archive, err := os.CreateTemp(a.geoDir(), ".download-*.tmp")
	if err != nil {
		return state, err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	if resp, err = a.geoGet(ctx, source, archiveURL); err != nil {
		return state, err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, geoDownloadMax+1))
	resp.Body.Close()
	if err != nil {
		return state, errors.New("下载中断，请重试")
	}
	if n > geoDownloadMax {
		return state, errors.New("下载的文件过大")
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return state, errors.New("下载的文件与 SHA-256 校验值不符，已丢弃")
	}

	unpacked, err := os.CreateTemp(a.geoDir(), ".unpack-*.tmp")
	if err != nil {
		return state, err
	}
	defer os.Remove(unpacked.Name())
	size, err := unpackGeoIP(archive, unpacked, edition)
	if cerr := unpacked.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return state, err
	}
	built, err := verifyGeoIP(unpacked.Name(), edition)
	if err != nil {
		return state, err
	}
	if err := os.Chmod(unpacked.Name(), 0o600); err != nil {
		return state, err
	}
	// Renaming replaces the previous version in one step; lookups keep the old mapping until they switch.
	if err := os.Rename(unpacked.Name(), a.geoFile()); err != nil {
		return state, fmt.Errorf("无法替换 IP 数据库：%w", err)
	}
	if err := a.geo.load(); err != nil {
		return state, fmt.Errorf("新的 IP 数据库无法打开：%w", err)
	}
	_ = a.clearGeoDir(a.geoFile())
	return geoState{Source: source, Edition: edition, SHA256: want, BuiltAt: built, Size: size, UpdatedAt: time.Now().Unix()}, nil
}

// unpackGeoIP copies the edition's .mmdb out of a MaxMind archive (a .tar.gz holding one dated folder).
func unpackGeoIP(archive *os.File, out io.Writer, edition string) (int64, error) {
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return 0, errors.New("下载的文件不是有效的压缩包")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return 0, fmt.Errorf("压缩包里没有 %s.mmdb", edition)
		}
		if err != nil {
			return 0, errors.New("下载的文件不是有效的压缩包")
		}
		if h.Typeflag != tar.TypeReg || path.Base(h.Name) != edition+".mmdb" {
			continue
		}
		n, err := io.Copy(out, io.LimitReader(tr, geoUnpackedMax+1))
		if err != nil {
			return 0, errors.New("解压失败，请重试")
		}
		if n > geoUnpackedMax {
			return 0, errors.New("解压后的数据库过大")
		}
		return n, nil
	}
}

// verifyGeoIP opens a downloaded database and checks that it is the edition asked for and is sound throughout,
// returning when it was built.
func verifyGeoIP(file, edition string) (int64, error) {
	reader, err := maxminddb.Open(file)
	if err != nil {
		return 0, errors.New("下载的 IP 数据库无法打开")
	}
	defer reader.Close()
	if reader.Metadata.DatabaseType != edition {
		return 0, fmt.Errorf("下载的是 %s，不是 %s", reader.Metadata.DatabaseType, edition)
	}
	if err := reader.Verify(); err != nil {
		log.Printf("GeoIP download not verified: %v", err)
		return 0, errors.New("下载的 IP 数据库校验失败")
	}
	return int64(reader.Metadata.BuildEpoch), nil
}

// clearGeoDir deletes everything in the GeoIP directory except keep (empty: delete everything), which takes away
// any earlier version and any download that did not finish.
func (a *App) clearGeoDir(keep string) error {
	entries, err := os.ReadDir(a.geoDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var first error
	for _, entry := range entries {
		name := filepath.Join(a.geoDir(), entry.Name())
		if name == keep || entry.IsDir() {
			continue
		}
		if err := os.Remove(name); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type geoView struct {
	Managed  bool     `json:"managed"`  // false: -geoip-db names the database and this page only reports it
	FlagPath string   `json:"flagPath"` // the -geoip-db file
	Loaded   bool     `json:"loaded"`   // a database is in use
	Source   string   `json:"source"`   // plost, maxmind or off
	Edition  string   `json:"edition"`  // GeoLite2-City or GeoLite2-Country
	Account  string   `json:"account"`  // the MaxMind account ID, shown by its start
	Key      string   `json:"key"`      // the MaxMind licence key, shown by its start
	Editable bool     `json:"editable"` // the keys can be saved from this page (-env-file)
	Updating bool     `json:"updating"` // a download is running
	State    geoState `json:"state"`
}

func (a *App) geoViewOf(ctx context.Context) geoView {
	source, edition, state := a.geoSettings(ctx)
	account, key := a.maxMindKeys()
	return geoView{Managed: a.geoManaged(), FlagPath: a.cfg.GeoIPPath, Loaded: a.geo.ready(), Source: source, Edition: edition,
		Account: maskKey(account), Key: maskSecret(key), Editable: a.cfg.EnvFile != "", Updating: a.geoUpdating.Load(), State: state}
}

// maskSecret shows only the first four characters of the licence key, which unlike a map token is secret.
func maskSecret(value string) string {
	if len(value) < 8 {
		return maskKey(value)
	}
	return value[:4] + strings.Repeat("•", 8)
}

func (a *App) geoipGet(w http.ResponseWriter, r *http.Request) error {
	return jsonResponse(w, a.geoViewOf(r.Context()))
}

// geoipPut saves the source, the edition or the MaxMind keys, then brings the database in line in the background.
func (a *App) geoipPut(w http.ResponseWriter, r *http.Request) error {
	if !a.geoManaged() {
		return failure(400, "IP 数据库由启动参数 -geoip-db 指定，请在服务器上更新该文件")
	}
	var in struct {
		Source  *string `json:"source"`
		Edition *string `json:"edition"`
		Account *string `json:"account"`
		Key     *string `json:"key"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Account != nil || in.Key != nil {
		if in.Account == nil || in.Key == nil {
			return failure(400, "请同时填写 MaxMind 账号 ID 和 License Key")
		}
		if err := a.saveMaxMindKeys(strings.TrimSpace(*in.Account), strings.TrimSpace(*in.Key)); err != nil {
			return err
		}
	}
	source, edition, _ := a.geoSettings(r.Context())
	if in.Source != nil {
		if !geoSources[*in.Source] {
			return failure(400, "未知的 IP 数据来源")
		}
		source = *in.Source
	}
	if in.Edition != nil {
		if !geoEditions[*in.Edition] {
			return failure(400, "未知的 IP 数据库类型")
		}
		edition = *in.Edition
	}
	if account, key := a.maxMindKeys(); source == "maxmind" && (account == "" || key == "") {
		return failure(400, "请先填写并保存 MaxMind 账号 ID 和 License Key")
	}
	for k, v := range map[string]string{"geoip_source": source, "geoip_edition": edition} {
		if _, err := a.db.ExecContext(r.Context(), "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", k, v); err != nil {
			return err
		}
	}
	a.startGeoUpdate()
	view := a.geoViewOf(r.Context())
	view.Updating = true
	return jsonResponse(w, view)
}

// geoipUpdate looks for a new version now and waits for the download.
func (a *App) geoipUpdate(w http.ResponseWriter, r *http.Request) error {
	if !a.geoManaged() {
		return failure(400, "IP 数据库由启动参数 -geoip-db 指定，请在服务器上更新该文件")
	}
	if _, err := a.updateGeoIP(r.Context()); err != nil {
		return err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return jsonResponse(w, a.geoViewOf(r.Context()))
}

// startGeoUpdate runs an update as a background job, which Close waits for. It is marked as running before this
// returns, so the page sees it at once; settings saved while one runs are applied by another run right after.
func (a *App) startGeoUpdate() {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	if a.closing {
		return
	}
	if !a.geoUpdating.CompareAndSwap(false, true) {
		a.geoAgain.Store(true)
		return
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		for {
			a.geoAgain.Store(false)
			_, _ = a.runGeoUpdate(a.jobCtx)
			if a.geoAgain.Load() && a.jobCtx.Err() == nil {
				continue
			}
			a.geoUpdating.Store(false)
			// A save that came in between the check and the release starts no run of its own; pick it up here.
			if !a.geoAgain.Load() || !a.geoUpdating.CompareAndSwap(false, true) {
				return
			}
		}
	}()
}

func (a *App) saveMaxMindKeys(account, key string) error {
	if a.cfg.EnvFile == "" {
		return failure(400, "启动 Plost 时没有指定 env 文件（-env-file），请直接在 env 文件里修改 "+EnvMaxMindAccount+" 和 "+EnvMaxMindKey)
	}
	if !maxMindAccountForm.MatchString(account) {
		return failure(400, "MaxMind 账号 ID 是一串数字")
	}
	if !maxMindKeyForm.MatchString(key) {
		return failure(400, "请填写 MaxMind 后台生成的 License Key")
	}
	a.keysMu.Lock()
	defer a.keysMu.Unlock()
	if err := writeEnvKey(a.cfg.EnvFile, EnvMaxMindAccount, account); err != nil {
		return failure(500, "写入 env 文件失败："+err.Error())
	}
	if err := writeEnvKey(a.cfg.EnvFile, EnvMaxMindKey, key); err != nil {
		return failure(500, "写入 env 文件失败："+err.Error())
	}
	a.cfg.MaxMindAccount, a.cfg.MaxMindKey = account, key
	return nil
}
