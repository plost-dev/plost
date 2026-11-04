package plost

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image/jpeg"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "litestats_session"

type User struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Locale string `json:"locale"`
	Theme  string `json:"theme"`
	TOTP   bool   `json:"totp"`
	Avatar string `json:"avatar"`
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func (a *App) user(r *http.Request) (*User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, err
	}
	var u User
	err = a.db.QueryRowContext(r.Context(), `SELECT u.id,u.email,u.name,u.locale,u.theme,u.totp_secret<>'',u.avatar
 FROM sessions s JOIN users u ON s.user_id=u.id WHERE s.token_hash=? AND s.host=? AND s.expires>? AND s.pending=0`, digest(c.Value), r.Host, unixNow()).Scan(&u.ID, &u.Email, &u.Name, &u.Locale, &u.Theme, &u.TOTP, &u.Avatar)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (a *App) setSession(w http.ResponseWriter, r *http.Request, pending bool) error {
	token := randomHex(32)
	lifetime := 14 * 24 * time.Hour
	if pending {
		lifetime = 5 * time.Minute
	}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,user_id,host,expires,pending) VALUES(?,1,?,?,?)", digest(token), r.Host, time.Now().Add(lifetime).Unix(), pending)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.secure(r), MaxAge: int(lifetime.Seconds())})
	_, _ = a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE expires<?", unixNow())
	return nil
}

func (a *App) secure(r *http.Request) bool {
	return strings.HasPrefix(a.cfg.Origin, "https://") || r.TLS != nil
}
func (a *App) authStatus(w http.ResponseWriter, r *http.Request) error {
	var count int
	if err := a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return err
	}
	u, _ := a.user(r)
	return jsonResponse(w, map[string]any{"needsSetup": count == 0, "user": u, "version": a.cfg.Version, "passkey": a.hasPasskeys(r.Context())})
}

type credentials struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

// validPassword leaves the choice to the person: at least eight characters of any kind, digits only included,
// and no more than the 72 bytes bcrypt can use. Length is counted in characters, so a password in Chinese is not penalised.
func validPassword(p string) bool {
	return utf8.RuneCountInString(p) >= 8 && len(p) <= 72
}

func validEmail(s string) bool {
	m, e := mail.ParseAddress(s)
	return e == nil && m.Address == s && len(s) <= 254
}

func (a *App) setup(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("setup", a.clientIP(r), 10, 15*time.Minute) {
		return failure(429, "尝试次数过多，请稍后再试")
	}
	var in credentials
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if !validEmail(in.Email) || in.Name == "" || len(in.Name) > 100 || !validPassword(in.Password) {
		return failure(400, "请填写姓名、有效邮箱，以及至少 8 个字符的密码（最长 72 字节）")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	// The singleton primary key makes two simultaneous setup requests mutually exclusive.
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO users(id,email,name,password_hash,created_at) VALUES(1,?,?,?,?)", in.Email, in.Name, string(hash), unixNow())
	if err != nil {
		var n int
		_ = a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&n)
		if n > 0 {
			return failure(409, "实例已经初始化，请登录")
		}
		return err
	}
	if err := a.setSession(w, r, false); err != nil {
		return err
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("login", a.clientIP(r), 12, 15*time.Minute) || !a.limits.allow("login-global", "all", 120, 15*time.Minute) {
		return failure(429, "登录尝试过多，请稍后再试")
	}
	var in credentials
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if len(in.Password) > 72 {
		return failure(401, "邮箱或密码错误")
	}
	var password, secret string
	err := a.db.QueryRowContext(r.Context(), "SELECT password_hash,totp_secret FROM users WHERE id=1 AND email=?", strings.ToLower(strings.TrimSpace(in.Email))).Scan(&password, &secret)
	if err != nil {
		password = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5N2z8BH0qQ.4oZ6szSjFJmM7MRmqw1i"
	}
	match := bcrypt.CompareHashAndPassword([]byte(password), []byte(in.Password))
	if err != nil || match != nil {
		return failure(401, "邮箱或密码错误")
	}
	if err := a.setSession(w, r, secret != ""); err != nil {
		return err
	}
	return jsonResponse(w, map[string]bool{"ok": true, "requiresTOTP": secret != ""})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) error {
	if c, e := r.Cookie(sessionCookie); e == nil {
		_, _ = a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", digest(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.secure(r)})
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) profile(w http.ResponseWriter, r *http.Request) error {
	u, err := a.user(r)
	if err != nil {
		return failure(401, "请先登录")
	}
	if r.Method == "GET" {
		return jsonResponse(w, u)
	}
	var in struct {
		Name            string          `json:"name"`
		Email           string          `json:"email"`
		Locale          string          `json:"locale"`
		Theme           string          `json:"theme"`
		CurrentPassword string          `json:"currentPassword"`
		Password        string          `json:"password"`
		Avatar          json.RawMessage `json:"avatar"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	avatar := u.Avatar
	if in.Avatar != nil {
		if string(in.Avatar) == "null" {
			avatar = ""
		} else if json.Unmarshal(in.Avatar, &avatar) != nil {
			return failure(400, "头像格式无效")
		}
		if avatar != "" {
			const prefix = "data:image/jpeg;base64,"
			if !strings.HasPrefix(avatar, prefix) || len(avatar) > 200000 {
				return failure(400, "头像格式无效或过大")
			}
			b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(avatar, prefix))
			if err != nil {
				return failure(400, "头像格式无效")
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
			if err != nil || cfg.Width > 256 || cfg.Height > 256 {
				return failure(400, "头像尺寸无效")
			}
		}
	}
	if in.Name == "" || len(in.Name) > 100 || !validEmail(in.Email) || !uiLocales[in.Locale] || (in.Theme != "system" && in.Theme != "dark" && in.Theme != "light") {
		return failure(400, "账号设置无效")
	}
	passwordChange := in.Password != ""
	emailChange := in.Email != u.Email
	var hash string
	if passwordChange || emailChange {
		if !a.limits.allow("profile", a.clientIP(r), 10, 15*time.Minute) {
			return failure(429, "尝试次数过多")
		}
		if err := a.db.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=1").Scan(&hash); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.CurrentPassword)) != nil {
			return failure(400, "当前密码错误")
		}
	}
	if passwordChange {
		if !validPassword(in.Password) {
			return failure(400, "新密码至少需要 8 个字符（最长 72 字节）")
		}
		b, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hash = string(b)
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "UPDATE users SET name=?,email=?,locale=?,theme=?,avatar=? WHERE id=1", in.Name, in.Email, in.Locale, in.Theme, avatar); err != nil {
		return err
	}
	if passwordChange {
		if _, err = tx.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=1", hash); err != nil {
			return err
		}
	}
	if passwordChange || emailChange {
		if _, err = tx.ExecContext(r.Context(), "DELETE FROM sessions"); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if passwordChange || emailChange {
		if err = a.setSession(w, r, false); err != nil {
			return err
		}
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func totpStep(secret, code string, now time.Time) int64 {
	if len(code) != 6 {
		return -1
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return -1
	}
	for delta := int64(-1); delta <= 1; delta++ {
		step := now.Unix()/30 + delta
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(step))
		h := hmac.New(sha1.New, key)
		h.Write(b[:])
		sum := h.Sum(nil)
		offset := sum[19] & 15
		n := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
		candidate := strconv.Itoa(int(n))
		candidate = strings.Repeat("0", 6-len(candidate)) + candidate
		if hmac.Equal([]byte(candidate), []byte(code)) {
			return step
		}
	}
	return -1
}

func consumeCode(tx *sql.Tx, code string) bool {
	var secret string
	var last int64
	if tx.QueryRow("SELECT totp_secret,totp_last FROM users WHERE id=1").Scan(&secret, &last) != nil {
		return false
	}
	if step := totpStep(secret, code, time.Now()); step > last {
		res, err := tx.Exec("UPDATE users SET totp_last=? WHERE id=1 AND totp_last<?", step, step)
		if err != nil {
			return false
		}
		n, _ := res.RowsAffected()
		return n == 1
	}
	res, err := tx.Exec("DELETE FROM recovery_codes WHERE hash=?", digest(code))
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (a *App) verifyTOTP(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("totp", a.clientIP(r), 10, 5*time.Minute) {
		return failure(429, "验证尝试过多")
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return failure(401, "请重新登录")
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if tx.QueryRowContext(r.Context(), "SELECT count(*) FROM sessions WHERE token_hash=? AND host=? AND expires>? AND pending=1", digest(c.Value), r.Host, unixNow()).Scan(&n) != nil || n != 1 {
		return failure(401, "请重新登录")
	}
	if !consumeCode(tx, strings.TrimSpace(in.Code)) {
		return failure(400, "验证码错误或已使用")
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", digest(c.Value)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = a.setSession(w, r, false); err != nil {
		return err
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) manageTOTP(w http.ResponseWriter, r *http.Request) error {
	u, err := a.user(r)
	if err != nil {
		return failure(401, "请先登录")
	}
	if !a.limits.allow("security", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "尝试次数过多")
	}
	var in struct {
		Action   string `json:"action"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	var password string
	_ = a.db.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=1").Scan(&password)
	if bcrypt.CompareHashAndPassword([]byte(password), []byte(in.Password)) != nil {
		return failure(400, "当前密码错误")
	}
	if in.Action == "begin" {
		if u.TOTP {
			return failure(409, "两步验证已经开启")
		}
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(randomHex(16)))
		if _, err := a.db.ExecContext(r.Context(), "UPDATE users SET totp_pending=? WHERE id=1", secret); err != nil {
			return err
		}
		uri := "otpauth://totp/" + url.PathEscape("Plost:"+u.Email) + "?secret=" + secret + "&issuer=Plost"
		return jsonResponse(w, map[string]string{"secret": secret, "uri": uri})
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if in.Action == "enable" {
		var secret string
		if err = tx.QueryRowContext(r.Context(), "SELECT totp_pending FROM users WHERE id=1 AND totp_secret='' ").Scan(&secret); err != nil {
			return failure(409, "请重新开始设置")
		}
		step := totpStep(secret, in.Code, time.Now())
		if secret == "" || step < 0 {
			return failure(400, "验证码错误")
		}
		if _, err = tx.ExecContext(r.Context(), "UPDATE users SET totp_secret=totp_pending,totp_pending='',totp_last=? WHERE id=1", step); err != nil {
			return err
		}
	} else if in.Action == "disable" || in.Action == "recovery" {
		if !u.TOTP || !consumeCode(tx, strings.TrimSpace(in.Code)) {
			return failure(400, "验证码错误或已使用")
		}
		if in.Action == "disable" {
			if _, err = tx.ExecContext(r.Context(), "UPDATE users SET totp_secret='',totp_pending='',totp_last=0 WHERE id=1"); err != nil {
				return err
			}
		}
	} else {
		return failure(400, "操作无效")
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM recovery_codes"); err != nil {
		return err
	}
	codes := []string{}
	if in.Action != "disable" {
		for i := 0; i < 8; i++ {
			code := randomHex(8)
			codes = append(codes, code)
			if _, err = tx.ExecContext(r.Context(), "INSERT INTO recovery_codes(hash) VALUES(?)", digest(code)); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM sessions"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = a.setSession(w, r, false); err != nil {
		return err
	}
	return jsonResponse(w, map[string]any{"ok": true, "recoveryCodes": codes})
}

type limitEntry struct {
	count int
	reset time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]limitEntry
}

func newLimiter() *limiter { return &limiter{entries: make(map[string]limitEntry)} }
func (l *limiter) allow(scope, key string, max int, period time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	k := scope + ":" + digest(key)
	if len(l.entries) >= 4096 {
		for k, v := range l.entries {
			if !now.Before(v.reset) {
				delete(l.entries, k)
			}
		}
		if len(l.entries) >= 4096 {
			return false
		}
	}
	v := l.entries[k]
	if !now.Before(v.reset) {
		v = limitEntry{reset: now.Add(period)}
	}
	v.count++
	l.entries[k] = v
	return v.count <= max
}

// uiLocales are the interface languages the frontend ships (frontend/src/lib/i18n.tsx LOCALES).
var uiLocales = map[string]bool{
	"zh-CN": true, "zh-TW": true, "en": true, "ru": true, "vi": true, "id": true, "pt-BR": true, "fa": true, "ar": true, "ja": true,
	"ko": true, "th": true, "es": true, "fr": true, "de": true, "tr": true, "ur": true, "it": true, "pl": true, "nl": true,
}
