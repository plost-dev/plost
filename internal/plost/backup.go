package plost

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

const backupMagic = "PLOSTGO1"
const chunkSize = 64 << 10

func backupCipher(password string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(password), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// Independently authenticated, bounded chunks avoid loading an entire database into RAM.
// An authenticated zero-length final frame detects truncated or appended backups.
func encryptBackup(w io.Writer, r io.Reader, password string) error {
	header := make([]byte, 28)
	copy(header, backupMagic)
	if _, err := rand.Read(header[8:]); err != nil {
		return err
	}
	aead, err := backupCipher(password, header[8:24])
	if err != nil {
		return err
	}
	if _, err = w.Write(header); err != nil {
		return err
	}
	buf := make([]byte, chunkSize)
	var counter uint64
	for {
		n, readErr := io.ReadFull(r, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if n > 0 {
			if err := writeFrame(w, aead, header, counter, buf[:n]); err != nil {
				return err
			}
			counter++
		}
		if readErr != nil {
			return writeFrame(w, aead, header, counter, nil)
		}
	}
}
func writeFrame(w io.Writer, a cipher.AEAD, header []byte, counter uint64, b []byte) error {
	nonce := make([]byte, 12)
	copy(nonce, header[24:])
	binary.BigEndian.PutUint64(nonce[4:], counter)
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(b)))
	aad := append(append([]byte{}, header...), nonce[4:]...)
	aad = append(aad, length...)
	sealed := a.Seal(nil, nonce, b, aad)
	if _, err := w.Write(length); err != nil {
		return err
	}
	_, err := w.Write(sealed)
	return err
}
func decryptBackup(w io.Writer, r io.Reader, password string) error {
	header := make([]byte, 28)
	if _, err := io.ReadFull(r, header); err != nil {
		return errors.New("备份文件不完整")
	}
	if string(header[:8]) != backupMagic {
		return errors.New("仅支持 Go 版备份；旧 PostgreSQL/ClickHouse 备份不能直接恢复")
	}
	a, err := backupCipher(password, header[8:24])
	if err != nil {
		return err
	}
	var total int64
	for counter := uint64(0); ; counter++ {
		length := make([]byte, 4)
		if _, err := io.ReadFull(r, length); err != nil {
			return errors.New("备份文件不完整")
		}
		n := binary.BigEndian.Uint32(length)
		if n > chunkSize {
			return errors.New("备份块无效")
		}
		total += int64(n)
		if total > 1<<30 {
			return errors.New("备份解压后超过 1 GiB")
		}
		sealed := make([]byte, int(n)+a.Overhead())
		if _, err := io.ReadFull(r, sealed); err != nil {
			return errors.New("备份文件不完整")
		}
		nonce := make([]byte, 12)
		copy(nonce, header[24:])
		binary.BigEndian.PutUint64(nonce[4:], counter)
		aad := append(append([]byte{}, header...), nonce[4:]...)
		aad = append(aad, length...)
		plain, err := a.Open(nil, nonce, sealed, aad)
		if err != nil {
			return errors.New("备份密码错误或文件已损坏")
		}
		if n == 0 {
			extra := make([]byte, 1)
			if _, err := r.Read(extra); err != io.EOF {
				return errors.New("备份包含多余内容")
			}
			return nil
		}
		if _, err = w.Write(plain); err != nil {
			return err
		}
	}
}

func (a *App) snapshot(ctx context.Context, path string) error {
	// VACUUM INTO creates a transactionally consistent snapshot including committed WAL pages.
	if _, err := a.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
func temporaryPath(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	p := f.Name()
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = os.Remove(p); err != nil {
		return "", err
	}
	return p, nil
}

func (a *App) checkPassword(ctx context.Context, password string) bool {
	var hash string
	return len(password) <= 72 && a.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id=1").Scan(&hash) == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (a *App) downloadBackup(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("backup", a.clientIP(r), 6, 5*time.Minute) {
		return failure(429, "备份操作过于频繁")
	}
	var in struct {
		Password        string `json:"password"`
		CurrentPassword string `json:"currentPassword"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if len(in.Password) < 12 || len(in.Password) > 256 {
		return failure(400, "请设置至少 12 字节的备份密码")
	}
	if !a.checkPassword(r.Context(), in.CurrentPassword) {
		return failure(400, "当前账号密码错误")
	}
	p, err := temporaryPath(a.cfg.DataDir, ".snapshot-*.db")
	if err != nil {
		return err
	}
	defer os.Remove(p)
	if err = a.snapshot(r.Context(), p); err != nil {
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="plost-go-`+time.Now().UTC().Format("20060102T150405Z")+`.plostbk"`)
	return encryptBackup(w, f, in.Password)
}

type upload struct{ path, password, confirmation string }

// readBackupUpload decrypts and validates an uploaded or listed backup into a temporary database.
// The read lock covers only database access, not the transfer, so a slow upload or cloud download
// does not stall every other request behind a waiting restore.
func (a *App) readBackupUpload(w http.ResponseWriter, r *http.Request) (upload, error) {
	a.mu.RLock()
	_, err := a.user(r)
	a.mu.RUnlock()
	if err != nil {
		return upload{}, failure(401, "请先登录")
	}
	if !a.limits.allow("restore", a.clientIP(r), 6, 5*time.Minute) {
		return upload{}, failure(429, "恢复操作过于频繁")
	}
	r.Body = http.MaxBytesReader(w, r.Body, (1<<30)+(64<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return upload{}, failure(400, "上传格式无效或文件过大")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	a.mu.RLock()
	authorized := a.checkPassword(r.Context(), r.FormValue("currentPassword"))
	a.mu.RUnlock()
	if !authorized {
		return upload{}, failure(400, "当前账号密码错误")
	}
	password := r.FormValue("password")
	if len(password) < 12 || len(password) > 256 {
		return upload{}, failure(400, "备份密码无效")
	}
	var src io.Reader
	if source := r.FormValue("source"); source != "" {
		// A listed backup skips the browser upload but passes through the same decryption and validation.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(31 * time.Minute))
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		body, err := a.openBackupSource(ctx, source)
		if err != nil {
			return upload{}, err
		}
		defer body.Close()
		src = body
	} else {
		f, _, err := r.FormFile("file")
		if err != nil {
			return upload{}, failure(400, "请选择 Go 版备份文件")
		}
		defer f.Close()
		src = f
	}
	out, err := os.CreateTemp(a.cfg.DataDir, ".restore-*.db")
	if err != nil {
		return upload{}, err
	}
	p := out.Name()
	clean := true
	defer func() {
		out.Close()
		if clean {
			os.Remove(p)
		}
	}()
	if err := decryptBackup(out, src, password); err != nil {
		return upload{}, failure(400, err.Error())
	}
	if err := out.Sync(); err != nil {
		return upload{}, err
	}
	if err := out.Close(); err != nil {
		return upload{}, err
	}
	if err := validateDatabase(r.Context(), p); err != nil {
		return upload{}, failure(400, err.Error())
	}
	clean = false
	return upload{path: p, password: password, confirmation: r.FormValue("confirmation")}, nil
}

func validateDatabase(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	magic := make([]byte, 16)
	_, err = io.ReadFull(f, magic)
	f.Close()
	if err != nil || !bytes.Equal(magic, []byte("SQLite format 3\x00")) {
		return errors.New("备份不是 SQLite 数据库")
	}
	db, err := openDB(path)
	if err != nil {
		return errors.New("无法打开备份数据库")
	}
	defer db.Close()
	var integrity string
	var version, count, unexpected int
	if db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity) != nil || integrity != "ok" {
		return errors.New("数据库完整性校验失败")
	}
	if db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version) != nil || version != 1 {
		return errors.New("不支持的数据库版本")
	}
	if db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE id=1").Scan(&count) != nil || count != 1 {
		return errors.New("备份必须包含一个实例管理员")
	}
	if db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count) != nil || count != 1 {
		return errors.New("个人版备份只允许一个实例管理员")
	}
	if db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type IN ('trigger','view')").Scan(&unexpected) != nil || unexpected != 0 {
		return errors.New("备份包含不支持的数据库对象")
	}
	if db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_foreign_key_check").Scan(&count) != nil || count != 0 {
		return errors.New("数据库关联校验失败")
	}
	var secret string
	if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='visitor_secret'").Scan(&secret) != nil {
		return errors.New("备份缺少访客密钥")
	}
	key, err := hex.DecodeString(secret)
	if err != nil || len(key) != 32 {
		return errors.New("备份中的访客密钥无效")
	}
	for table, columns := range map[string]string{"sites": "id,domain,name,timezone,public,created_at", "sessions": "token_hash,user_id,host,expires,pending", "shared_links": "id,site_id,name,token_hash,created_at", "events_v2": "id,site_id,timestamp,name,hostname,pathname,referrer_source,referrer,user_id,session_id,country_code,browser,os,device,language,screen,title,utm_source,utm_medium,utm_campaign,props", "users": "id,email,name,password_hash,locale,theme,totp_secret,totp_pending,totp_last,created_at", "recovery_codes": "hash"} {
		rows, err := db.QueryContext(ctx, "SELECT "+columns+" FROM "+table+" LIMIT 0")
		if err != nil {
			return errors.New("数据库表结构不兼容")
		}
		rows.Close()
	}
	return nil
}

func (a *App) validateBackup(w http.ResponseWriter, r *http.Request) {
	u, err := a.readBackupUpload(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer os.Remove(u.path)
	_ = jsonResponse(w, map[string]any{"ok": true, "format": "Plost Go 1", "message": "备份密码、完整性和表结构校验通过"})
}

func (a *App) restoreBackup(w http.ResponseWriter, r *http.Request) {
	u, err := a.readBackupUpload(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer os.Remove(u.path)
	if u.confirmation != "RESTORE" {
		writeError(w, failure(400, "请输入 RESTORE 确认完整恢复"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Recheck auth after upload: a concurrent restore/password change may have revoked it.
	if _, err := a.user(r); err != nil {
		writeError(w, failure(401, "请重新登录"))
		return
	}
	dir := filepath.Join(a.cfg.DataDir, "backups")
	if err = os.MkdirAll(dir, 0700); err != nil {
		writeError(w, err)
		return
	}
	safety := filepath.Join(dir, "before-restore-"+time.Now().UTC().Format("20060102T150405")+"-"+randomHex(4)+".db")
	if err = a.snapshot(r.Context(), safety); err != nil {
		writeError(w, err)
		return
	}
	if err = a.replaceDatabase(u.path, safety); err != nil {
		writeError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.secure(r)})
	_ = jsonResponse(w, map[string]any{"ok": true, "safetyBackup": filepath.Base(safety)})
}

func (a *App) replaceDatabase(incoming, safety string) error {
	path := filepath.Join(a.cfg.DataDir, "plost.db")
	if err := a.db.Close(); err != nil {
		return err
	}
	rollback := func(original error) error {
		src, err := os.Open(safety)
		if err != nil {
			return fmt.Errorf("restore and rollback failed")
		}
		defer src.Close()
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, src)
		out.Close()
		if err != nil {
			return err
		}
		_ = os.Remove(path + "-wal")
		_ = os.Remove(path + "-shm")
		a.db, err = openDB(path)
		if err != nil {
			return err
		}
		return original
	}
	// All application DB users are stopped by mu; no external DB service or privileged socket is needed.
	if err := os.Rename(incoming, path); err != nil {
		return rollback(err)
	}
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	db, err := openDB(path)
	if err != nil {
		return rollback(err)
	}
	if err = migrate(db); err != nil {
		db.Close()
		return rollback(err)
	}
	if _, err = db.Exec("DELETE FROM sessions; UPDATE users SET totp_pending='' WHERE id=1"); err != nil {
		db.Close()
		return rollback(err)
	}
	var secret string
	if err = db.QueryRow("SELECT value FROM settings WHERE key='visitor_secret'").Scan(&secret); err != nil {
		db.Close()
		return rollback(err)
	}
	key, err := hex.DecodeString(secret)
	if err != nil || len(key) != 32 {
		db.Close()
		return rollback(errors.New("invalid visitor secret"))
	}
	a.db = db
	a.secret = key
	a.epoch++
	return nil
}
