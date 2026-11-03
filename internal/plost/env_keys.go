package plost

import (
	"fmt"
	"os"
	"strings"
)

// The environment variables that hold the keys, normally set in the service's env file. The settings page only
// says whether each is set and never stores one, so the keys survive a database restore or a new data directory.
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
	case cfg.MapboxToken != "" && (!strings.HasPrefix(cfg.MapboxToken, "pk.") || len(cfg.MapboxToken) > 2048):
		return fmt.Errorf("%s must be a Mapbox public token starting with pk.", EnvMapboxToken)
	case cfg.GoogleMapsKey != "" && !googleMapsKey.MatchString(cfg.GoogleMapsKey):
		return fmt.Errorf("%s must be a Google Maps API key: AIza and 35 more characters", EnvGoogleMapsKey)
	case !plainKey(cfg.BackupAccessKey) || !plainKey(cfg.BackupSecretKey):
		return fmt.Errorf("%s and %s cannot contain spaces or control characters", EnvBackupAccessKey, EnvBackupSecretKey)
	case cfg.BackupPassword != "" && (len(cfg.BackupPassword) < 12 || len(cfg.BackupPassword) > 256):
		return fmt.Errorf("%s must be 12 to 256 bytes", EnvBackupPassword)
	}
	return nil
}
