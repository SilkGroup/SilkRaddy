package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

const minSecretLength = 10

type Config struct {
	DBDriver string
	DBDir    string
	DBFile   string

	PostgresURL string

	// FileStore — Phase C. Driver is "local" (default) or "gcs".
	FileStoreDriver         string
	FileStoreBucket         string
	FileStoreServiceAccount string

	// MultiTenant — Phase B foundation feature flag. When false, the legacy
	// shared-secret login + single-tenant behaviour is preserved. When true,
	// the new tenants/users/memberships tables are used and login switches
	// to email + password.
	MultiTenant bool

	TracksDir    string
	TmpDir       string
	PlayerDir    string
	StudioDir    string
	HTTPPort     string
	JWTSign      string
	SecretKey    string
	SecureCookie bool
}

// Load reads configuration from environment variables (with .env support for
// local dev). It returns an aggregated error if any required secret is missing
// or too short, so callers can log and exit cleanly instead of dying inside
// the config package.
func Load() (*Config, error) {
	_ = godotenv.Load() // For development

	var errs []error
	jwtSign, err := getSecret("AIRSTATION_JWT_SIGN")
	if err != nil {
		errs = append(errs, err)
	}
	secretKey, err := getSecret("AIRSTATION_SECRET_KEY")
	if err != nil {
		errs = append(errs, err)
	}

	cfg := &Config{
		DBDriver: getEnv("AIRSTATION_DB_DRIVER", "sqlite"),
		DBDir:    getEnv("AIRSTATION_DB_DIR", filepath.Join("storage")),
		DBFile:   getEnv("AIRSTATION_DB_FILE", "storage.db"),

		PostgresURL: getEnv("AIRSTATION_POSTGRES_URL", ""),

		FileStoreDriver:         getEnv("SILKRADDY_FILESTORE_DRIVER", "local"),
		FileStoreBucket:         getEnv("SILKRADDY_FILESTORE_BUCKET", ""),
		FileStoreServiceAccount: getEnv("SILKRADDY_FILESTORE_SERVICE_ACCOUNT", ""),

		MultiTenant: getEnvBool("SILKRADDY_MULTI_TENANT", false),

		TracksDir:    getEnv("AIRSTATION_TRACKS_DIR", filepath.Join("static", "tracks")),
		TmpDir:       getEnv("AIRSTATION_TMP_DIR", filepath.Join("static", "tmp")),
		PlayerDir:    getEnv("AIRSTATION_PLAYER_DIR", filepath.Join("web", "player", "dist")),
		StudioDir:    getEnv("AIRSTATION_STUDIO_DIR", filepath.Join("web", "studio", "dist")),
		HTTPPort:     getEnv("AIRSTATION_HTTP_PORT", getEnv("PORT", "7331")),
		JWTSign:      jwtSign,
		SecretKey:    secretKey,
		SecureCookie: getEnvBool("AIRSTATION_SECURE_COOKIE", false),
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}

	val = strings.ToLower(val)
	return val == "1" || val == "true" || val == "yes" || val == "on"
}

func getSecret(key string) (string, error) {
	secret := os.Getenv(key)
	if secret == "" {
		return "", fmt.Errorf("%s is not set", key)
	}
	if len(secret) < minSecretLength {
		return "", fmt.Errorf("%s is too short (need at least %d characters)", key, minSecretLength)
	}
	return secret, nil
}
