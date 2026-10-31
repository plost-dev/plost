package plost

import "testing"

// A passkey requires user verification, so it counts as two factors: it signs in without a
// verifier code, while the password path still stops at the code prompt.
func TestPasskeySignInSkipsTOTPButPasswordDoesNot(t *testing.T) {
	e := newPKEnv(t)
	key := newSoftKey(t)
	e.enroll(t, key)
	if _, err := e.x.app.db.Exec("UPDATE users SET totp_secret=?,totp_last=0 WHERE id=1", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
		t.Fatal(err)
	}

	password := e.client()
	login := parseMap(t, e.json(t, password, "POST", "/api/login", map[string]any{"email": "owner@example.test", "password": testPassword}, 200))
	if login["requiresTOTP"] != true {
		t.Fatalf("password sign-in should ask for the verifier code: %v", login)
	}
	e.do(t, password, "GET", "/api/account", nil, e.origin, 401)

	passkey := e.client()
	options := parseMap(t, e.json(t, passkey, "POST", "/api/login/passkey/begin", map[string]any{}, 200))
	e.do(t, passkey, "POST", "/api/login/passkey/finish", key.assert(t, options, e.origin, "localhost", 1), e.origin, 200)
	e.do(t, passkey, "GET", "/api/account", nil, e.origin, 200)
}
