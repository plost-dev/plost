package plost

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// The environment variables that hold the keys, normally set in the service's env file. None is ever kept in the
// database, so the keys survive a database restore or a new data directory. The two map keys can also be saved on
// the settings page, which writes them into that env file (-env-file).
const (
	EnvMapboxToken     = "PLOST_MAPBOX_TOKEN"
	EnvGoogleMapsKey   = "PLOST_GOOGLE_MAPS_KEY"
	EnvBackupAccessKey = "PLOST_BACKUP_ACCESS_KEY"
	EnvBackupSecretKey = "PLOST_BACKUP_SECRET_KEY"
	EnvBackupPassword  = "PLOST_BACKUP_PASSWORD"
)

// EnvKeys reads the keys from the environment into cfg and checks their form, so a mistyped key stops the
// program at start instead of failing later on a page.
func EnvKeys(cfg *Config) error {
	get := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	cfg.MapboxToken, cfg.GoogleMapsKey = get(EnvMapboxToken), get(EnvGoogleMapsKey)
	cfg.BackupAccessKey, cfg.BackupSecretKey, cfg.BackupPassword = get(EnvBackupAccessKey), get(EnvBackupSecretKey), os.Getenv(EnvBackupPassword)
	switch {
	case cfg.MapboxToken != "" && !validMapKey(EnvMapboxToken, cfg.MapboxToken):
		return fmt.Errorf("%s must be a Mapbox public token starting with pk.", EnvMapboxToken)
	case cfg.GoogleMapsKey != "" && !validMapKey(EnvGoogleMapsKey, cfg.GoogleMapsKey):
		return fmt.Errorf("%s must be a Google Maps API key: AIza and 35 more characters", EnvGoogleMapsKey)
	case !plainKey(cfg.BackupAccessKey) || !plainKey(cfg.BackupSecretKey):
		return fmt.Errorf("%s and %s cannot contain spaces or control characters", EnvBackupAccessKey, EnvBackupSecretKey)
	case cfg.BackupPassword != "" && (len(cfg.BackupPassword) < 12 || len(cfg.BackupPassword) > 256):
		return fmt.Errorf("%s must be 12 to 256 bytes", EnvBackupPassword)
	}
	return nil
}

func validMapKey(name, value string) bool {
	if name == EnvMapboxToken {
		return strings.HasPrefix(value, "pk.") && len(value) <= 2048 && plainKey(value)
	}
	return googleMapsKey.MatchString(value)
}

// maskKey shows the start of a key and hides the rest, so the settings page can tell which key is set.
func maskKey(value string) string {
	if value == "" {
		return ""
	}
	return value[:min(10, len(value)/2)] + strings.Repeat("•", 8)
}

// mapKeys are the map keys the pages use now: those read at start, or saved on the settings page since.
func (a *App) mapKeys() (mapbox, google string) {
	a.keysMu.RLock()
	defer a.keysMu.RUnlock()
	return a.cfg.MapboxToken, a.cfg.GoogleMapsKey
}

// saveMapKey writes a map key from the settings page into the env file and uses it at once, without a restart.
func (a *App) saveMapKey(name, value string) error {
	value = strings.TrimSpace(value)
	if a.cfg.EnvFile == "" {
		return failure(400, "启动 Plost 时没有指定 env 文件（-env-file），请直接在 env 文件里修改 "+name)
	}
	if !validMapKey(name, value) {
		if name == EnvMapboxToken {
			return failure(400, "请填写以 pk. 开头的 Mapbox 公开 Token")
		}
		return failure(400, "请填写以 AIza 开头的 Google Maps API Key")
	}
	a.keysMu.Lock()
	defer a.keysMu.Unlock()
	if err := writeEnvKey(a.cfg.EnvFile, name, value); err != nil {
		return failure(500, "写入 env 文件失败："+err.Error())
	}
	if name == EnvMapboxToken {
		a.cfg.MapboxToken = value
	} else {
		a.cfg.GoogleMapsKey = value
	}
	return nil
}

// writeEnvKey sets name=value in an env file, replacing the key's line or adding one and leaving every other line
// as it was. The file is rewritten in place, so it keeps its owner and mode and may be the only path the service
// can write outside its data directory.
func writeEnvKey(path, name, value string) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var lines []string
	found := false
	if text := strings.TrimRight(string(old), "\n"); text != "" {
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimPrefix(strings.TrimSpace(line), "export "), name+"=") {
				if found {
					continue
				}
				line, found = name+"="+value, true
			}
			lines = append(lines, line)
		}
	}
	if !found {
		lines = append(lines, name+"="+value)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(strings.Join(lines, "\n") + "\n"); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
