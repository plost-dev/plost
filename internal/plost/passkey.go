package plost

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	passkeyCookie  = "plost_webauthn"
	maxPasskeys    = 20
	ceremonyWindow = 5 * time.Minute
	verifyHint     = "无法验证 Passkey。请通过 HTTPS 域名访问，并在启动参数中设置 -origin 为该域名。"
)

type passkeyUser struct {
	handle      []byte
	email, name string
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.handle }
func (u *passkeyUser) WebAuthnName() string                       { return u.email }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// ceremony is the server-side state of one registration or sign-in. Only a random id travels in the cookie.
type ceremony struct {
	kind    string
	session webauthn.SessionData
	expires time.Time
}

// relyingParty is bound to the origin the browser actually used, which -origin fixes in production.
func (a *App) relyingParty(r *http.Request) (*webauthn.WebAuthn, error) {
	origin := a.cfg.Origin
	if origin == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		origin = scheme + "://" + r.Host
	}
	u, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	return webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPDisplayName: "Plost", RPOrigins: []string{origin}})
}

func (a *App) saveCeremony(w http.ResponseWriter, r *http.Request, kind string, session *webauthn.SessionData) error {
	a.ceremonyMu.Lock()
	defer a.ceremonyMu.Unlock()
	if a.ceremonies == nil {
		a.ceremonies = map[string]ceremony{}
	}
	now := time.Now()
	for id, c := range a.ceremonies {
		if now.After(c.expires) {
			delete(a.ceremonies, id)
		}
	}
	if len(a.ceremonies) >= 256 {
		return failure(429, "尝试次数过多，请稍后再试")
	}
	id := randomHex(24)
	a.ceremonies[id] = ceremony{kind: kind, session: *session, expires: now.Add(ceremonyWindow)}
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: id, Path: "/api/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secure(r), MaxAge: int(ceremonyWindow.Seconds())})
	return nil
}

// takeCeremony consumes the state: a challenge can be answered once.
func (a *App) takeCeremony(w http.ResponseWriter, r *http.Request, kind string) (webauthn.SessionData, bool) {
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Path: "/api/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secure(r)})
	c, err := r.Cookie(passkeyCookie)
	if err != nil {
		return webauthn.SessionData{}, false
	}
	a.ceremonyMu.Lock()
	defer a.ceremonyMu.Unlock()
	state, ok := a.ceremonies[c.Value]
	delete(a.ceremonies, c.Value)
	if !ok || state.kind != kind || time.Now().After(state.expires) {
		return webauthn.SessionData{}, false
	}
	return state.session, true
}

// loadPasskeyUser must be called with the read lock held.
func (a *App) loadPasskeyUser(ctx context.Context) (*passkeyUser, error) {
	u := &passkeyUser{}
	if err := a.db.QueryRowContext(ctx, "SELECT email,name FROM users WHERE id=1").Scan(&u.email, &u.name); err != nil {
		return nil, err
	}
	if _, err := a.db.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('passkey_handle',?) ON CONFLICT(key) DO NOTHING", randomHex(32)); err != nil {
		return nil, err
	}
	var handle string
	if err := a.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='passkey_handle'").Scan(&handle); err != nil {
		return nil, err
	}
	var err error
	if u.handle, err = hex.DecodeString(handle); err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(ctx, "SELECT credential FROM passkeys WHERE user_id=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var c webauthn.Credential
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		u.credentials = append(u.credentials, c)
	}
	return u, rows.Err()
}

func (a *App) hasPasskeys(ctx context.Context) bool {
	var found bool
	return a.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM passkeys)").Scan(&found) == nil && found
}

func (a *App) passkeyList(w http.ResponseWriter, r *http.Request) error {
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,name,created_at,last_used_at FROM passkeys WHERE user_id=1 ORDER BY id DESC")
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, created, used int64
		var name string
		if err := rows.Scan(&id, &name, &created, &used); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "name": name, "createdAt": created, "lastUsedAt": used})
	}
	return jsonResponse(w, list)
}

func (a *App) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("passkey", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "尝试次数过多，请稍后再试")
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	// Adding a sign-in method needs the account password, so a stolen session cannot plant its own passkey.
	if !a.checkPassword(r.Context(), in.Password) {
		return failure(400, "当前密码错误")
	}
	user, err := a.loadPasskeyUser(r.Context())
	if err != nil {
		return err
	}
	if len(user.credentials) >= maxPasskeys {
		return failure(409, "最多可添加 20 把 Passkey")
	}
	wa, err := a.relyingParty(r)
	if err != nil {
		return err
	}
	creation, session, err := wa.BeginRegistration(user,
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		}))
	if err != nil {
		return failure(400, verifyHint)
	}
	if err = a.saveCeremony(w, r, "register", session); err != nil {
		return err
	}
	return jsonResponse(w, creation.Response)
}

func (a *App) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("passkey", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "尝试次数过多，请稍后再试")
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "Passkey " + time.Now().Format("2006-01-02")
	}
	if len(name) > 60 || strings.ContainsFunc(name, func(c rune) bool { return c < ' ' || c == 0x7f }) {
		return failure(400, "Passkey 名称无效")
	}
	session, ok := a.takeCeremony(w, r, "register")
	if !ok {
		return failure(400, "Passkey 挑战已过期，请重新开始")
	}
	user, err := a.loadPasskeyUser(r.Context())
	if err != nil {
		return err
	}
	wa, err := a.relyingParty(r)
	if err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	credential, err := wa.FinishRegistration(user, session, r)
	if err != nil {
		return failure(400, verifyHint)
	}
	stored, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if _, err = a.db.ExecContext(r.Context(), "INSERT INTO passkeys(user_id,credential_id,name,credential,created_at) VALUES(1,?,?,?,?)",
		hex.EncodeToString(credential.ID), name, string(stored), unixNow()); err != nil {
		var n int
		if a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM passkeys WHERE credential_id=?", hex.EncodeToString(credential.ID)).Scan(&n) == nil && n > 0 {
			return failure(409, "这把 Passkey 已经添加")
		}
		return err
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) passkeyDelete(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("passkey", a.clientIP(r), 12, 5*time.Minute) {
		return failure(429, "尝试次数过多，请稍后再试")
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return failure(400, "Passkey 无效")
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if !a.checkPassword(r.Context(), in.Password) {
		return failure(400, "当前密码错误")
	}
	res, err := a.db.ExecContext(r.Context(), "DELETE FROM passkeys WHERE id=? AND user_id=1", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return failure(404, "Passkey 不存在")
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}

func (a *App) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("passkey-login", a.clientIP(r), 30, 15*time.Minute) {
		return failure(429, "登录尝试过多，请稍后再试")
	}
	if !a.hasPasskeys(r.Context()) {
		return failure(409, "尚未配置 Passkey")
	}
	wa, err := a.relyingParty(r)
	if err != nil {
		return err
	}
	assertion, session, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return failure(400, verifyHint)
	}
	if err = a.saveCeremony(w, r, "login", session); err != nil {
		return err
	}
	return jsonResponse(w, assertion.Response)
}

func (a *App) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) error {
	if !a.limits.allow("passkey-login", a.clientIP(r), 30, 15*time.Minute) || !a.limits.allow("login-global", "all", 120, 15*time.Minute) {
		return failure(429, "登录尝试过多，请稍后再试")
	}
	fail := failure(401, "Passkey 登录失败")
	session, ok := a.takeCeremony(w, r, "login")
	if !ok {
		return failure(400, "Passkey 挑战已过期，请重新开始")
	}
	user, err := a.loadPasskeyUser(r.Context())
	if err != nil {
		return fail
	}
	wa, err := a.relyingParty(r)
	if err != nil {
		return err
	}
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		if subtle.ConstantTimeCompare(userHandle, user.handle) != 1 {
			return nil, errors.New("unknown user handle")
		}
		return user, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	_, credential, err := wa.FinishPasskeyLogin(handler, session, r)
	// A counter that did not advance means the credential may have been copied.
	if err != nil || credential == nil || credential.Authenticator.CloneWarning {
		return fail
	}
	stored, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if _, err = a.db.ExecContext(r.Context(), "UPDATE passkeys SET credential=?,last_used_at=? WHERE credential_id=?", string(stored), unixNow(), hex.EncodeToString(credential.ID)); err != nil {
		return err
	}
	// User verification is required, so a passkey already counts as two factors and skips the TOTP step.
	if err = a.setSession(w, r, false); err != nil {
		return err
	}
	return jsonResponse(w, map[string]bool{"ok": true})
}
