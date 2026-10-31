package plost

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// softKey is a software authenticator: it builds the same registration and assertion messages a
// browser would forward, so the server's verification runs end to end.
type softKey struct {
	priv   *ecdsa.PrivateKey
	id     []byte
	handle []byte
	count  uint32
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func newSoftKey(t *testing.T) *softKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	rand.Read(id)
	return &softKey{priv: priv, id: id}
}

func clientData(typ, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin})
	return b
}

func (k *softKey) register(t *testing.T, options map[string]any, origin, rpID string) []byte {
	t.Helper()
	user := options["user"].(map[string]any)
	k.handle, _ = base64.RawURLEncoding.DecodeString(user["id"].(string))
	x, y := k.priv.PublicKey.X.FillBytes(make([]byte, 32)), k.priv.PublicKey.Y.FillBytes(make([]byte, 32))
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		t.Fatal(err)
	}
	rp := sha256.Sum256([]byte(rpID))
	data := append([]byte{}, rp[:]...)
	data = append(data, 0x45) // user present, user verified, attested credential data
	data = binary.BigEndian.AppendUint32(data, 0)
	data = append(data, make([]byte, 16)...)
	data = binary.BigEndian.AppendUint16(data, uint16(len(k.id)))
	data = append(data, k.id...)
	data = append(data, cose...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": data})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"attestationObject": b64(attestation), "clientDataJSON": b64(clientData("webauthn.create", options["challenge"].(string), origin)), "transports": []string{"internal"}}})
	return out
}

func (k *softKey) assert(t *testing.T, options map[string]any, origin, rpID string, count uint32) []byte {
	t.Helper()
	k.count = count
	cd := clientData("webauthn.get", options["challenge"].(string), origin)
	rp := sha256.Sum256([]byte(rpID))
	data := append([]byte{}, rp[:]...)
	data = append(data, 0x05) // user present, user verified
	data = binary.BigEndian.AppendUint32(data, count)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, data...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"authenticatorData": b64(data), "clientDataJSON": b64(cd), "signature": b64(sig), "userHandle": b64(k.handle)}})
	return out
}

// pkEnv talks to the app as the host "localhost", the only name browsers accept for a passkey over plain HTTP.
type pkEnv struct {
	x      *testApp
	origin string
}

func newPKEnv(t *testing.T) *pkEnv {
	t.Helper()
	x := newTestApp(t)
	u, _ := url.Parse(x.server.URL)
	return &pkEnv{x: x, origin: "http://localhost:" + u.Port()}
}

func (e *pkEnv) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 15 * time.Second}
}

func (e *pkEnv) do(t *testing.T, c *http.Client, method, path string, body []byte, origin string, expected int) []byte {
	t.Helper()
	req, err := http.NewRequest(method, e.x.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = strings.TrimPrefix(e.origin, "http://")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Plost-Request", "1")
	req.Header.Set("Origin", origin)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expected {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, expected, out)
	}
	return out
}

func (e *pkEnv) json(t *testing.T, c *http.Client, method, path string, v any, expected int) []byte {
	b, _ := json.Marshal(v)
	return e.do(t, c, method, path, b, e.origin, expected)
}

func parseMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON: %s", b)
	}
	return m
}

// enroll registers key through the real endpoints and returns the signed-in client.
func (e *pkEnv) enroll(t *testing.T, key *softKey) *http.Client {
	t.Helper()
	owner := e.client()
	e.json(t, owner, "POST", "/api/setup", map[string]any{"email": "owner@example.test", "name": "Owner", "password": testPassword}, 200)
	options := parseMap(t, e.json(t, owner, "POST", "/api/passkeys/register/begin", map[string]any{"password": testPassword}, 200))
	e.do(t, owner, "POST", "/api/passkeys/register/finish?name=Laptop", key.register(t, options, e.origin, "localhost"), e.origin, 200)
	return owner
}

func TestPasskeyRegistrationAndManagement(t *testing.T) {
	e := newPKEnv(t)
	key := newSoftKey(t)
	owner := e.client()
	anonymous := e.client()
	for _, tc := range [][2]string{{"GET", "/api/passkeys"}, {"POST", "/api/passkeys/register/begin"}, {"POST", "/api/passkeys/register/finish"}, {"DELETE", "/api/passkeys/1"}} {
		e.json(t, anonymous, tc[0], tc[1], map[string]any{}, 401)
	}
	if parseMap(t, e.do(t, anonymous, "GET", "/api/auth", nil, e.origin, 200))["passkey"] != false {
		t.Fatal("passkey flag set before any passkey exists")
	}
	e.json(t, anonymous, "POST", "/api/login/passkey/begin", map[string]any{}, 409)

	e.json(t, owner, "POST", "/api/setup", map[string]any{"email": "owner@example.test", "name": "Owner", "password": testPassword}, 200)
	e.json(t, owner, "POST", "/api/passkeys/register/begin", map[string]any{"password": "wrong-account-password"}, 400)
	// A challenge answered with nothing, or without having been issued, is refused.
	e.do(t, owner, "POST", "/api/passkeys/register/finish", []byte(`{}`), e.origin, 400)

	options := parseMap(t, e.json(t, owner, "POST", "/api/passkeys/register/begin", map[string]any{"password": testPassword}, 200))
	selection := options["authenticatorSelection"].(map[string]any)
	if selection["userVerification"] != "required" || selection["residentKey"] != "required" {
		t.Fatalf("passkeys must be discoverable and user-verified: %v", selection)
	}
	response := key.register(t, options, e.origin, "localhost")
	e.do(t, owner, "POST", "/api/passkeys/register/finish?name="+url.QueryEscape("Work laptop"), bad(response, "origin"), e.origin, 400)
	// The failed attempt consumed the challenge; a fresh one is needed.
	options = parseMap(t, e.json(t, owner, "POST", "/api/passkeys/register/begin", map[string]any{"password": testPassword}, 200))
	response = key.register(t, options, e.origin, "localhost")
	e.do(t, owner, "POST", "/api/passkeys/register/finish?name="+url.QueryEscape("Work laptop"), response, e.origin, 200)

	var list []map[string]any
	json.Unmarshal(e.do(t, owner, "GET", "/api/passkeys", nil, e.origin, 200), &list)
	if len(list) != 1 || list[0]["name"] != "Work laptop" {
		t.Fatalf("unexpected list %v", list)
	}
	body := string(e.do(t, owner, "GET", "/api/passkeys", nil, e.origin, 200))
	if strings.Contains(body, b64(key.id)) || strings.Contains(body, "publicKey") {
		t.Fatal("list exposes credential material")
	}
	if parseMap(t, e.do(t, anonymous, "GET", "/api/auth", nil, e.origin, 200))["passkey"] != true {
		t.Fatal("sign-in page cannot tell a passkey exists")
	}

	// The same authenticator is excluded from a second registration.
	options = parseMap(t, e.json(t, owner, "POST", "/api/passkeys/register/begin", map[string]any{"password": testPassword}, 200))
	if excluded, _ := options["excludeCredentials"].([]any); len(excluded) != 1 {
		t.Fatalf("existing credential not excluded: %v", options["excludeCredentials"])
	}
	e.do(t, owner, "POST", "/api/passkeys/register/finish", key.register(t, options, e.origin, "localhost"), e.origin, 409)

	var id float64 = list[0]["id"].(float64)
	path := "/api/passkeys/" + trimFloat(id)
	e.json(t, owner, "DELETE", path, map[string]any{"password": "wrong-account-password"}, 400)
	e.json(t, owner, "DELETE", path, map[string]any{"password": testPassword}, 200)
	e.json(t, owner, "DELETE", path, map[string]any{"password": testPassword}, 404)
	e.json(t, anonymous, "POST", "/api/login/passkey/begin", map[string]any{}, 409)
}

func trimFloat(f float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(jsonNumber(f), ".0"), ".00")
}
func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// bad returns the registration response with its client data pointed at another origin.
func bad(response []byte, field string) []byte {
	var m map[string]any
	json.Unmarshal(response, &m)
	inner := m["response"].(map[string]any)
	raw, _ := base64.RawURLEncoding.DecodeString(inner["clientDataJSON"].(string))
	var cd map[string]any
	json.Unmarshal(raw, &cd)
	cd[field] = "http://evil.example"
	raw, _ = json.Marshal(cd)
	inner["clientDataJSON"] = b64(raw)
	out, _ := json.Marshal(m)
	return out
}

func TestPasskeySignInAndAttacks(t *testing.T) {
	e := newPKEnv(t)
	key := newSoftKey(t)
	e.enroll(t, key)

	begin := func(c *http.Client) map[string]any {
		return parseMap(t, e.json(t, c, "POST", "/api/login/passkey/begin", map[string]any{}, 200))
	}
	signedIn := func(c *http.Client, expected int) { e.do(t, c, "GET", "/api/account", nil, e.origin, expected) }

	// Normal sign-in, twice: the counter moves forward each time.
	first := e.client()
	options := begin(first)
	if options["userVerification"] != "required" {
		t.Fatalf("sign-in must require user verification: %v", options["userVerification"])
	}
	signedIn(first, 401)
	e.do(t, first, "POST", "/api/login/passkey/finish", key.assert(t, options, e.origin, "localhost", 1), e.origin, 200)
	signedIn(first, 200)
	second := e.client()
	e.do(t, second, "POST", "/api/login/passkey/finish", key.assert(t, begin(second), e.origin, "localhost", 2), e.origin, 200)
	signedIn(second, 200)
	var used int64
	if err := e.x.app.db.QueryRow("SELECT last_used_at FROM passkeys").Scan(&used); err != nil || used == 0 {
		t.Fatal("last use was not recorded")
	}

	// A response is single-use: its challenge is gone, and an old response fails against a new challenge.
	replay := e.client()
	oldOptions := begin(replay)
	good := key.assert(t, oldOptions, e.origin, "localhost", 3)
	e.do(t, replay, "POST", "/api/login/passkey/finish", good, e.origin, 200)
	e.do(t, replay, "POST", "/api/login/passkey/finish", good, e.origin, 400)
	attacker := e.client()
	begin(attacker)
	e.do(t, attacker, "POST", "/api/login/passkey/finish", good, e.origin, 401)
	signedIn(attacker, 401)

	// No ceremony cookie, wrong ceremony kind, wrong origin, unknown key, broken body.
	e.do(t, e.client(), "POST", "/api/login/passkey/finish", good, e.origin, 400)
	owner := e.client()
	e.json(t, owner, "POST", "/api/login", map[string]any{"email": "owner@example.test", "password": testPassword}, 200)
	e.json(t, owner, "POST", "/api/passkeys/register/begin", map[string]any{"password": testPassword}, 200)
	e.do(t, owner, "POST", "/api/login/passkey/finish", good, e.origin, 400)

	wrongOrigin := e.client()
	e.do(t, wrongOrigin, "POST", "/api/login/passkey/finish", key.assert(t, begin(wrongOrigin), "http://evil.example", "localhost", 4), e.origin, 401)
	wrongRP := e.client()
	e.do(t, wrongRP, "POST", "/api/login/passkey/finish", key.assert(t, begin(wrongRP), e.origin, "evil.example", 4), e.origin, 401)
	stranger := newSoftKey(t)
	stranger.handle = key.handle
	unknown := e.client()
	e.do(t, unknown, "POST", "/api/login/passkey/finish", stranger.assert(t, begin(unknown), e.origin, "localhost", 1), e.origin, 401)
	otherUser := newSoftKey(t)
	otherUser.id, otherUser.handle = key.id, []byte("someone-else-entirely")
	handleMismatch := e.client()
	e.do(t, handleMismatch, "POST", "/api/login/passkey/finish", otherUser.assert(t, begin(handleMismatch), e.origin, "localhost", 5), e.origin, 401)
	garbage := e.client()
	begin(garbage)
	e.do(t, garbage, "POST", "/api/login/passkey/finish", []byte(`{"id":"x"`), e.origin, 401)
	for _, c := range []*http.Client{wrongOrigin, wrongRP, unknown, handleMismatch, garbage} {
		signedIn(c, 401)
	}

	// A counter that stops advancing means the credential may have been cloned.
	cloned := e.client()
	options = begin(cloned)
	e.do(t, cloned, "POST", "/api/login/passkey/finish", key.assert(t, options, e.origin, "localhost", 3), e.origin, 401)
	signedIn(cloned, 401)

	// CSRF protections still apply to these endpoints.
	req, _ := http.NewRequest("POST", e.x.server.URL+"/api/login/passkey/begin", strings.NewReader("{}"))
	req.Host = strings.TrimPrefix(e.origin, "http://")
	req.Header.Set("Origin", e.origin)
	resp, err := e.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("request without the CSRF header got %d", resp.StatusCode)
	}
}

func TestPasskeyChallengesExpireAndStayBounded(t *testing.T) {
	e := newPKEnv(t)
	key := newSoftKey(t)
	e.enroll(t, key)
	c := e.client()
	options := parseMap(t, e.json(t, c, "POST", "/api/login/passkey/begin", map[string]any{}, 200))
	response := key.assert(t, options, e.origin, "localhost", 1)
	e.x.app.ceremonyMu.Lock()
	for id, state := range e.x.app.ceremonies {
		state.expires = time.Now().Add(-time.Second)
		e.x.app.ceremonies[id] = state
	}
	e.x.app.ceremonyMu.Unlock()
	e.do(t, c, "POST", "/api/login/passkey/finish", response, e.origin, 400)

	// Unauthenticated begin calls cannot grow memory without bound.
	e.x.app.ceremonyMu.Lock()
	for i := 0; i < 256; i++ {
		e.x.app.ceremonies[randomHex(8)] = ceremony{kind: "login", expires: time.Now().Add(time.Minute)}
	}
	e.x.app.ceremonyMu.Unlock()
	e.x.app.limits = newLimiter()
	e.json(t, e.client(), "POST", "/api/login/passkey/begin", map[string]any{}, 429)
}

func TestPasskeysSurviveBackupRestore(t *testing.T) {
	e := newPKEnv(t)
	key := newSoftKey(t)
	owner := e.enroll(t, key)
	backup := e.do(t, owner, "POST", "/api/backup", mustJSON(map[string]string{"password": testBackupPassword, "currentPassword": testPassword}), e.origin, 200)
	var count int
	e.x.app.db.QueryRow("SELECT count(*) FROM passkeys").Scan(&count)
	if count != 1 {
		t.Fatal("setup failed")
	}
	if _, err := e.x.app.db.Exec("DELETE FROM passkeys"); err != nil {
		t.Fatal(err)
	}
	e.restore(t, owner, backup)
	e.x.app.db.QueryRow("SELECT count(*) FROM passkeys").Scan(&count)
	if count != 1 {
		t.Fatal("restore lost the passkey")
	}
	c := e.client()
	options := parseMap(t, e.json(t, c, "POST", "/api/login/passkey/begin", map[string]any{}, 200))
	e.do(t, c, "POST", "/api/login/passkey/finish", key.assert(t, options, e.origin, "localhost", 1), e.origin, 200)
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// restore uploads a backup as the signed-in client, using the same host the session is bound to.
func (e *pkEnv) restore(t *testing.T, c *http.Client, backup []byte) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	f, _ := mw.CreateFormFile("file", "test.plostbk")
	f.Write(backup)
	mw.WriteField("password", testBackupPassword)
	mw.WriteField("currentPassword", testPassword)
	mw.WriteField("confirmation", "RESTORE")
	mw.Close()
	req, _ := http.NewRequest("POST", e.x.server.URL+"/api/backup/restore", &b)
	req.Host = strings.TrimPrefix(e.origin, "http://")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Plost-Request", "1")
	req.Header.Set("Origin", e.origin)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out, _ := io.ReadAll(resp.Body); resp.StatusCode != 200 {
		t.Fatalf("restore: status %d: %s", resp.StatusCode, out)
	}
}
