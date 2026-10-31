package plost

import (
	"strings"
	"testing"
)

func TestValidPasswordRule(t *testing.T) {
	for password, ok := range map[string]bool{
		"12345678":              true, // digits only is the person's choice
		"abcdefgh":              true,
		"        ":              true, // any characters at all
		"密码密码密码密码":              true, // eight characters, though 24 bytes
		strings.Repeat("a", 72): true,
		strings.Repeat("密", 24): true, // exactly 72 bytes
		"1234567":               false,
		"":                      false,
		"密码密码密码":                false,
		strings.Repeat("a", 73): false, // bcrypt cannot use more than 72 bytes
		strings.Repeat("密", 25): false,
	} {
		if got := validPassword(password); got != ok {
			t.Errorf("%q (%d characters, %d bytes): accepted=%v, want %v", password, len([]rune(password)), len(password), got, ok)
		}
	}
}

func TestSetupAndPasswordChangeAcceptShortSimplePasswords(t *testing.T) {
	x := newTestApp(t)
	creds := func(password string) map[string]any {
		return map[string]any{"email": "owner@example.test", "name": "Owner", "password": password}
	}
	x.call(t, "POST", "/api/setup", creds("1234567"), 400)
	body := string(x.call(t, "POST", "/api/setup", creds("short"), 400))
	if !strings.Contains(body, "至少 8 个字符") {
		t.Fatalf("the message should say what is needed: %s", body)
	}
	x.call(t, "POST", "/api/setup", creds("12345678"), 200)
	x.call(t, "POST", "/api/logout", nil, 200)
	x.call(t, "POST", "/api/login", map[string]string{"email": "owner@example.test", "password": "12345678"}, 200)

	change := func(password string, expected int) []byte {
		return x.call(t, "PUT", "/api/account", map[string]any{"name": "Owner", "email": "owner@example.test", "locale": "zh-CN", "theme": "system",
			"currentPassword": "12345678", "password": password}, expected)
	}
	if body := string(change("abc1234", 400)); !strings.Contains(body, "至少需要 8 个字符") {
		t.Fatalf("a new password that is too short: %s", body)
	}
	change(strings.Repeat("a", 73), 400)
	change("密码密码密码密码", 200)
	x.call(t, "POST", "/api/logout", nil, 200)
	x.call(t, "POST", "/api/login", map[string]string{"email": "owner@example.test", "password": "12345678"}, 401)
	x.call(t, "POST", "/api/login", map[string]string{"email": "owner@example.test", "password": "密码密码密码密码"}, 200)
}
