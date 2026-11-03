package plost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// cloudSettings is the S3-compatible destination used for uploads and listing.
type cloudSettings struct {
	Provider, Endpoint, Region, Bucket, Prefix, AccessKey, SecretKey string
}

type backupSettings struct {
	cloudSettings
	BackupPassword                    string
	CloudEnabled                      bool
	Schedule                          string
	Hour, Minute, Weekday, Keep       int
	LastRunAt, LastAttemptAt          int64
	LastStatus, LastError, LastBackup string
}

// cloudInput is what the browser sends. Blank credentials mean "keep the saved ones".
type cloudInput struct {
	Provider  string `json:"provider"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type backupInput struct {
	cloudInput
	BackupPassword  string `json:"backupPassword"`
	CurrentPassword string `json:"currentPassword"`
	CloudEnabled    bool   `json:"cloudEnabled"`
	Schedule        string `json:"schedule"`
	Hour            int    `json:"hour"`
	Minute          int    `json:"minute"`
	Weekday         int    `json:"weekday"`
	Keep            int    `json:"keep"`
}

const defaultBackupPrefix = "plost"

var (
	regionPattern  = regexp.MustCompile(`^[a-z0-9-]*$`)
	bucketPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	r2HostPattern  = regexp.MustCompile(`^[a-z0-9]+(?:\.(?:eu|fedramp))?\.r2\.cloudflarestorage\.com$`)
	blockedHost    = regexp.MustCompile(`^(localhost|.*\.(localhost|local|internal|invalid|test|onion))$`)
	backupNameExpr = regexp.MustCompile(`^plost-go-\d{8}T\d{6}Z(-[0-9a-f]{8})?\.plostbk$`)
)

func (a *App) loadSettings(ctx context.Context) (backupSettings, error) {
	var s backupSettings
	err := a.db.QueryRowContext(ctx, `SELECT provider,endpoint,region,bucket,prefix,cloud_enabled,
 schedule,hour,minute,weekday,keep,last_run_at,last_attempt_at,last_status,last_error,last_backup FROM backup_settings WHERE id=1`).
		Scan(&s.Provider, &s.Endpoint, &s.Region, &s.Bucket, &s.Prefix, &s.CloudEnabled,
			&s.Schedule, &s.Hour, &s.Minute, &s.Weekday, &s.Keep, &s.LastRunAt, &s.LastAttemptAt, &s.LastStatus, &s.LastError, &s.LastBackup)
	// The credentials and the backup password are only ever in the env file (env_keys.go).
	s.AccessKey, s.SecretKey, s.BackupPassword = a.cfg.BackupAccessKey, a.cfg.BackupSecretKey, a.cfg.BackupPassword
	return s, err
}

// saveSettings stores everything but the credentials and the backup password, which stay in the env file; the old
// columns are cleared.
func (a *App) saveSettings(ctx context.Context, s backupSettings) error {
	_, err := a.db.ExecContext(ctx, `UPDATE backup_settings SET provider=?,endpoint=?,region=?,bucket=?,prefix=?,access_key='',secret_key='',backup_password='',
 cloud_enabled=?,schedule=?,hour=?,minute=?,weekday=?,keep=? WHERE id=1`,
		s.Provider, s.Endpoint, s.Region, s.Bucket, s.Prefix, s.CloudEnabled, s.Schedule, s.Hour, s.Minute, s.Weekday, s.Keep)
	return err
}

// recordBackup stores only a generic outcome; provider response bodies and URLs never reach the database.
func (a *App) recordBackup(reason, name string, failed error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.mu.RLock()
	defer a.mu.RUnlock()
	now := unixNow()
	if failed != nil {
		_, _ = a.db.ExecContext(ctx, "UPDATE backup_settings SET last_attempt_at=?,last_status='error',last_error=? WHERE id=1", now, backupFailureMessage(failed))
		return
	}
	_, _ = a.db.ExecContext(ctx, "UPDATE backup_settings SET last_attempt_at=?,last_run_at=?,last_status=?,last_error='',last_backup=? WHERE id=1", now, now, reason, name)
}

func scheduleZone() string {
	name, offset := time.Now().Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return fmt.Sprintf("%s UTC%s%02d:%02d", name, sign, offset/3600, offset%3600/60)
}

func (a *App) publicSettings(s backupSettings) map[string]any {
	configured := requireCloud(s.cloudSettings) == nil
	prefix := strings.Trim(s.Prefix, "/")
	if prefix == "" {
		prefix = defaultBackupPrefix
	}
	return map[string]any{
		"provider": s.Provider, "endpoint": s.Endpoint, "region": s.Region, "bucket": s.Bucket, "prefix": s.Prefix,
		"hasAccessKey": s.AccessKey != "", "hasSecretKey": s.SecretKey != "", "hasBackupPassword": s.BackupPassword != "",
		"cloudEnabled": s.CloudEnabled, "cloudConfigured": configured,
		"schedule": s.Schedule, "hour": s.Hour, "minute": s.Minute, "weekday": s.Weekday, "keep": s.Keep,
		"lastRunAt": s.LastRunAt, "lastAttemptAt": s.LastAttemptAt, "lastStatus": s.LastStatus, "lastBackup": s.LastBackup,
		"lastError": s.LastError, "effectivePrefix": prefix, "scheduleTimezone": scheduleZone(), "running": a.backupRunning.Load(),
	}
}

// validateEndpoint accepts only public HTTPS origins; the dialer repeats the address check at connect time.
func validateEndpoint(provider, value string, allowPrivate bool) (string, error) {
	if value == "" {
		return "", nil
	}
	bad := failure(400, "请填写有效的公网 HTTPS S3 Endpoint，不能包含凭据、查询参数或片段")
	if provider == "r2" {
		bad = failure(400, "请填写有效的 HTTPS R2 S3 API Endpoint")
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Hostname() == "" || strings.ContainsAny(value, "?#") {
		return "", bad
	}
	host := strings.ToLower(u.Hostname())
	if !allowPrivate {
		if ip, err := netip.ParseAddr(host); err == nil && !isPublicAddress(ip) {
			return "", bad
		}
		if blockedHost.MatchString(host) {
			return "", bad
		}
	}
	path := strings.TrimRight(u.Path, "/")
	if provider == "r2" && (!r2HostPattern.MatchString(host) || u.Port() != "" || path != "") {
		return "", bad
	}
	if strings.Contains(path, "//") {
		return "", bad
	}
	return "https://" + strings.ToLower(u.Host) + path, nil
}

// destination identifies where signed requests go, so saved credentials are never reused for another place.
func destination(c cloudSettings) string {
	if c.Endpoint != "" {
		return c.Provider + ":" + strings.ToLower(c.Endpoint)
	}
	region := c.Region
	if region == "" {
		region = "us-east-1"
	}
	return c.Provider + ":aws:" + region
}

func plainKey(s string) bool {
	return len(s) <= 1024 && !strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == 0x7f })
}

func resolveCloud(in cloudInput, saved cloudSettings, allowPrivate bool) (cloudSettings, error) {
	c := cloudSettings{Provider: strings.TrimSpace(in.Provider), Region: strings.TrimSpace(in.Region), Bucket: strings.TrimSpace(in.Bucket)}
	if c.Provider != "r2" && c.Provider != "s3" {
		return c, failure(400, "请选择有效的存储服务")
	}
	if len(c.Region) > 64 || !regionPattern.MatchString(c.Region) {
		return c, failure(400, "Region 只能包含小写字母、数字和连字符")
	}
	if c.Bucket != "" && !bucketPattern.MatchString(c.Bucket) {
		return c, failure(400, "请填写有效的存储桶名称")
	}
	prefix := strings.Trim(strings.TrimSpace(in.Prefix), "/")
	if len(prefix) > 1024 || strings.ContainsFunc(prefix, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return c, failure(400, "前缀不能包含控制字符")
	}
	for _, segment := range strings.Split(prefix, "/") {
		if prefix != "" && (segment == "" || segment == "." || segment == "..") {
			return c, failure(400, "前缀不能包含空目录、. 或 ..")
		}
	}
	c.Prefix = prefix
	endpoint, err := validateEndpoint(c.Provider, strings.TrimSpace(in.Endpoint), allowPrivate)
	if err != nil {
		return c, err
	}
	c.Endpoint = endpoint
	if c.Provider == "r2" && c.Region == "" {
		c.Region = "auto"
	}
	access, secret := strings.TrimSpace(in.AccessKey), strings.TrimSpace(in.SecretKey)
	if !plainKey(access) || !plainKey(secret) {
		return c, failure(400, "Access Key 和 Secret Key 不能包含空白或控制字符")
	}
	same := destination(c) == destination(saved)
	if !same && (saved.AccessKey != "" || saved.SecretKey != "") && (access == "" || secret == "") {
		return c, failure(400, "存储服务或 Endpoint 已更改，请重新填写 Access Key 和 Secret Key")
	}
	c.AccessKey, c.SecretKey = access, secret
	if same {
		if c.AccessKey == "" {
			c.AccessKey = saved.AccessKey
		}
		if c.SecretKey == "" {
			c.SecretKey = saved.SecretKey
		}
	}
	return c, nil
}

func requireCloud(c cloudSettings) error {
	switch {
	case c.Provider == "r2" && c.Endpoint == "":
		return errors.New("请填写 R2 S3 API Endpoint")
	case c.Provider == "s3" && (c.Region == "" || c.Region == "auto"):
		return errors.New("请填写 S3 Region，例如 us-east-1")
	case c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "":
		return errors.New("请填写存储桶、Access Key 和 Secret Key")
	}
	return nil
}

func (c cloudSettings) objectPrefix() string {
	prefix := strings.Trim(c.Prefix, "/")
	if prefix == "" {
		prefix = defaultBackupPrefix
	}
	return prefix + "/backups/"
}

func (a *App) backupSettingsGet(w http.ResponseWriter, r *http.Request) error {
	s, err := a.loadSettings(r.Context())
	if err != nil {
		return err
	}
	return jsonResponse(w, a.publicSettings(s))
}

func (a *App) backupSettingsPut(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("backup-settings", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "操作过于频繁")
	}
	var in backupInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	// Changing where backups go or which password protects them needs the account password,
	// so a stolen session cannot redirect encrypted copies of the database.
	if !a.checkPassword(r.Context(), in.CurrentPassword) {
		return failure(400, "当前账号密码错误")
	}
	if in.AccessKey != "" || in.SecretKey != "" || in.BackupPassword != "" {
		return failure(400, "Access Key、Secret Key 和计划备份密码请写在 env 文件（"+EnvBackupAccessKey+"、"+EnvBackupSecretKey+"、"+EnvBackupPassword+"），然后重启服务")
	}
	saved, err := a.loadSettings(r.Context())
	if err != nil {
		return err
	}
	in.AccessKey, in.SecretKey = saved.AccessKey, saved.SecretKey
	next := saved
	if next.cloudSettings, err = resolveCloud(in.cloudInput, saved.cloudSettings, a.allowPrivateCloud); err != nil {
		return err
	}
	next.CloudEnabled = in.CloudEnabled
	if next.CloudEnabled {
		if err = requireCloud(next.cloudSettings); err != nil {
			return failure(400, err.Error())
		}
	}
	if (in.Schedule != "off" && in.Schedule != "hourly" && in.Schedule != "daily" && in.Schedule != "weekly") ||
		in.Hour < 0 || in.Hour > 23 || in.Minute < 0 || in.Minute > 59 || in.Weekday < 0 || in.Weekday > 6 || in.Keep < 0 || in.Keep > 90 {
		return failure(400, "自动备份设置无效")
	}
	next.Schedule, next.Hour, next.Minute, next.Weekday, next.Keep = in.Schedule, in.Hour, in.Minute, in.Weekday, in.Keep
	if next.Schedule != "off" {
		if len(next.BackupPassword) < 12 {
			return failure(400, "请先在 env 文件里设置 "+EnvBackupPassword+"，自动备份需要它加密文件")
		}
		if next.Keep == 0 && !next.CloudEnabled {
			return failure(400, "本地保留 0 份时，需要启用云存储上传")
		}
	}
	if err = a.saveSettings(r.Context(), next); err != nil {
		return err
	}
	saved, err = a.loadSettings(r.Context())
	if err != nil {
		return err
	}
	return jsonResponse(w, a.publicSettings(saved))
}
