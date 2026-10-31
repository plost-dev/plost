package plost

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxRestoreBytes mirrors the plaintext limit enforced when a backup is decrypted for restore.
const maxRestoreBytes = 1 << 30

// safeError carries a message that is already fit to show and store.
type safeError string

func (e safeError) Error() string { return string(e) }

var errBackupBusy = safeError("已有备份正在执行，请稍后重试")

func backupFailureMessage(err error) string {
	var s safeError
	if errors.As(err, &s) {
		return string(s)
	}
	return "备份失败，请检查存储配置、磁盘空间和权限后重试"
}

func (a *App) scheduledDir() string { return filepath.Join(a.cfg.DataDir, "backups", "scheduled") }

func (a *App) cloudClient(c cloudSettings) *s3Client {
	if a.newCloud != nil {
		return a.newCloud(c)
	}
	return newS3Client(c, a.outboundAllow)
}

// beginBackup claims the single backup slot; the claimant must call runBackup.
func (a *App) beginBackup() bool { return a.backupRunning.CompareAndSwap(false, true) }

// startJob runs fn on a worker that Close waits for and cancels.
func (a *App) startJob(fn func(ctx context.Context)) bool {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	if a.closing {
		return false
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		fn(a.jobCtx)
	}()
	return true
}

func (a *App) runBackup(ctx context.Context, reason string) (string, error) {
	defer a.backupRunning.Store(false)
	name, err := a.writeBackup(ctx, reason)
	a.recordBackup(reason, name, err)
	return name, err
}

func encryptFile(src, dst, password string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = encryptBackup(out, in, password); err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}

func (a *App) writeBackup(ctx context.Context, reason string) (string, error) {
	local := safeError("无法写入本地备份目录，请检查磁盘空间和权限")
	a.mu.RLock()
	s, err := a.loadSettings(ctx)
	a.mu.RUnlock()
	switch {
	case err != nil:
		return "", safeError("无法读取备份设置")
	case s.Unreadable:
		return "", safeError("已保存的备份凭据无法解密，请在备份设置中重新填写")
	case len(s.BackupPassword) < 12:
		return "", safeError("请先在备份设置中设置计划备份密码")
	case s.Keep == 0 && !s.CloudEnabled:
		return "", safeError("本地保留 0 份时需要启用云存储上传")
	}
	if s.CloudEnabled {
		if err = requireCloud(s.cloudSettings); err != nil {
			return "", safeError(err.Error())
		}
	}
	dir := a.scheduledDir()
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", local
	}
	// Whatever happens below, the retention setting decides which local files remain.
	defer pruneLocal(dir, s.Keep)
	snapshot, err := temporaryPath(a.cfg.DataDir, ".snapshot-*.db")
	if err != nil {
		return "", local
	}
	defer os.Remove(snapshot)
	a.mu.RLock()
	err = a.snapshot(ctx, snapshot)
	a.mu.RUnlock()
	if err != nil {
		return "", local
	}
	if info, err := os.Stat(snapshot); err != nil {
		return "", local
	} else if info.Size() > maxRestoreBytes {
		return "", safeError("数据库已超过 1 GiB，当前版本无法恢复这样的备份，已取消备份")
	}
	name := "plost-go-" + time.Now().UTC().Format("20060102T150405Z") + "-" + randomHex(4) + ".plostbk"
	final := filepath.Join(dir, name)
	if err = encryptFile(snapshot, final+".part", s.BackupPassword); err != nil {
		return "", local
	}
	if err = os.Rename(final+".part", final); err != nil {
		_ = os.Remove(final + ".part")
		return "", local
	}
	if s.CloudEnabled {
		f, err := os.Open(final)
		if err != nil {
			return "", local
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return "", local
		}
		upload, cancel := context.WithTimeout(ctx, 2*time.Hour)
		defer cancel()
		if err = a.cloudClient(s.cloudSettings).put(upload, s.objectPrefix()+name, f, info.Size()); err != nil {
			return "", safeError("上传到云存储失败：" + cloudFailureMessage(err))
		}
	}
	return name, nil
}

// pruneLocal keeps the newest keep scheduled backups. Only files created by this feature are touched.
func pruneLocal(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".part"):
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
				_ = os.Remove(filepath.Join(dir, name))
			}
		case e.Type().IsRegular() && backupNameExpr.MatchString(name):
			names = append(names, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := keep; i < len(names); i++ {
		_ = os.Remove(filepath.Join(dir, names[i]))
	}
}

// backupDue decides in the server's local time zone. A missed slot runs once the server is back.
func backupDue(now time.Time, s backupSettings) bool {
	if s.LastStatus == "error" && now.Sub(time.Unix(s.LastAttemptAt, 0)) < 15*time.Minute {
		return false
	}
	last := time.Unix(s.LastRunAt, 0)
	at := func(day time.Time) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), s.Hour, s.Minute, 0, 0, now.Location())
	}
	switch s.Schedule {
	case "hourly":
		return s.LastRunAt == 0 || now.Sub(last) >= time.Hour
	case "daily":
		slot := at(now)
		return !now.Before(slot) && last.Before(slot)
	case "weekly":
		slot := at(now.AddDate(0, 0, -((int(now.Weekday()) - s.Weekday + 7) % 7)))
		if slot.After(now) {
			slot = at(slot.AddDate(0, 0, -7))
		}
		return !now.Before(slot) && last.Before(slot)
	}
	return false
}

func (a *App) scheduleTick(now time.Time) {
	ctx, cancel := context.WithTimeout(a.jobCtx, 5*time.Second)
	a.mu.RLock()
	s, err := a.loadSettings(ctx)
	a.mu.RUnlock()
	cancel()
	if err != nil || s.Schedule == "off" || !backupDue(now, s) || !a.beginBackup() {
		return
	}
	_, _ = a.runBackup(a.jobCtx, "schedule")
}

func (a *App) scheduleLoop() {
	defer a.workers.Done()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-timer.C:
			a.scheduleTick(time.Now())
			timer.Reset(30 * time.Second)
		}
	}
}

func (a *App) runNow(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("backup-run", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "备份操作过于频繁")
	}
	if !a.beginBackup() {
		return failure(409, string(errBackupBusy))
	}
	if !a.startJob(func(ctx context.Context) { _, _ = a.runBackup(ctx, "manual") }) {
		a.backupRunning.Store(false)
		return failure(503, "服务正在停止")
	}
	return jsonResponse(w, map[string]bool{"started": true})
}

type backupFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

func (a *App) localList(w http.ResponseWriter, r *http.Request) error {
	files := []backupFile{}
	entries, err := os.ReadDir(a.scheduledDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && e.Type().IsRegular() && backupNameExpr.MatchString(e.Name()) {
			files = append(files, backupFile{e.Name(), info.Size(), info.ModTime().Unix()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	if len(files) > 100 {
		files = files[:100]
	}
	return jsonResponse(w, files)
}

func (a *App) localDownload(w http.ResponseWriter, r *http.Request) error {
	name := r.PathValue("name")
	if !backupNameExpr.MatchString(name) {
		return failure(404, "备份不存在")
	}
	f, err := os.Open(filepath.Join(a.scheduledDir(), name))
	if err != nil {
		return failure(404, "备份不存在")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	// Headers are sent, so a broken connection can only truncate the body.
	_, _ = io.Copy(w, f)
	return nil
}

func (a *App) cloudTest(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("cloud-test", a.clientIP(r), 10, 5*time.Minute) {
		return failure(429, "测试过于频繁，请稍后再试")
	}
	var in cloudInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	a.mu.RLock()
	saved, err := a.loadSettings(r.Context())
	a.mu.RUnlock()
	if err != nil {
		return err
	}
	c, err := resolveCloud(in, saved.cloudSettings, a.allowPrivateCloud)
	if err != nil {
		return err
	}
	if err = requireCloud(c); err != nil {
		return failure(400, err.Error())
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	// A bounded list proves the connection and read permission only; nothing is written or deleted.
	if _, err = a.cloudClient(c).list(ctx, c.objectPrefix(), 1); err != nil {
		return failure(502, cloudFailureMessage(err))
	}
	return jsonResponse(w, map[string]any{"ok": true, "checkedAt": unixNow(), "bucket": c.Bucket, "prefix": c.objectPrefix(), "verified": []string{"list"}})
}

// savedCloud returns the stored destination for scanning and restoring; unsaved form values are never used.
func (a *App) savedCloud(ctx context.Context) (cloudSettings, error) {
	a.mu.RLock()
	s, err := a.loadSettings(ctx)
	a.mu.RUnlock()
	if err != nil {
		return cloudSettings{}, err
	}
	if s.Unreadable {
		return cloudSettings{}, failure(409, "已保存的云存储凭据无法解密，请重新填写并保存")
	}
	if err = requireCloud(s.cloudSettings); err != nil {
		return cloudSettings{}, failure(400, "请先保存云存储设置")
	}
	return s.cloudSettings, nil
}

func (a *App) cloudScan(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("cloud-scan", a.clientIP(r), 20, 5*time.Minute) {
		return failure(429, "扫描过于频繁，请稍后再试")
	}
	c, err := a.savedCloud(r.Context())
	if err != nil {
		return err
	}
	objects, err := a.cloudClient(c).list(r.Context(), c.objectPrefix(), 0)
	if err != nil {
		return failure(502, cloudFailureMessage(err))
	}
	files := []backupFile{}
	for _, o := range objects {
		if name := strings.TrimPrefix(o.Key, c.objectPrefix()); name != o.Key && backupNameExpr.MatchString(name) {
			files = append(files, backupFile{name, o.Size, o.Modified.Unix()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	if len(files) > 50 {
		files = files[:50]
	}
	return jsonResponse(w, files)
}

// openBackupSource opens a listed local or cloud backup so it goes through the same decrypt and validation as an upload.
func (a *App) openBackupSource(ctx context.Context, source string) (io.ReadCloser, error) {
	kind, name, _ := strings.Cut(source, ":")
	if !backupNameExpr.MatchString(name) {
		return nil, failure(400, "备份文件名无效")
	}
	switch kind {
	case "local":
		f, err := os.Open(filepath.Join(a.scheduledDir(), name))
		if err != nil {
			return nil, failure(404, "本地备份不存在")
		}
		return f, nil
	case "cloud":
		c, err := a.savedCloud(ctx)
		if err != nil {
			return nil, err
		}
		body, err := a.cloudClient(c).get(ctx, c.objectPrefix()+name)
		if err != nil {
			return nil, failure(502, cloudFailureMessage(err))
		}
		return body, nil
	}
	return nil, failure(400, "备份来源无效")
}
