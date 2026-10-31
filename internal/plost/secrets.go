package plost

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const sealedPrefix = "v1:"

// The key lives beside the database, never inside it, so snapshots and backups
// do not carry the key that unlocks stored storage credentials.
//
// Only sealing creates the key; opening a value never invents a replacement
// for a key that was lost, which would hide the loss.
func (a *App) sealKey(create bool) ([]byte, error) {
	a.keyMu.Lock()
	defer a.keyMu.Unlock()
	if a.key != nil {
		return a.key, nil
	}
	path := filepath.Join(a.cfg.DataDir, "secrets.key")
	b, err := os.ReadFile(path)
	if err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(key) != 32 {
			return nil, errors.New("secrets.key is invalid")
		}
		a.key = key
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) || !create {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString(hex.EncodeToString(key) + "\n")
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	a.key = key
	return key, nil
}

func (a *App) aead(create bool) (cipher.AEAD, error) {
	key, err := a.sealKey(create)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal binds the ciphertext to its field name so stored values cannot be swapped between columns.
func (a *App) seal(plain, field string) (string, error) {
	if plain == "" {
		return "", nil
	}
	g, err := a.aead(true)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return sealedPrefix + base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plain), []byte(field))), nil
}

func (a *App) open(sealed, field string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	if !strings.HasPrefix(sealed, sealedPrefix) {
		return "", errors.New("unsupported sealed value")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, sealedPrefix))
	if err != nil {
		return "", err
	}
	g, err := a.aead(false)
	if err != nil {
		return "", err
	}
	if len(raw) < g.NonceSize() {
		return "", errors.New("sealed value is truncated")
	}
	plain, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], []byte(field))
	return string(plain), err
}
