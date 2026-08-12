package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

	// CORS. Comma-separated origin allowlist. Empty preserves the legacy
	// permissive default (any origin) for backward compatibility with
	// self-host deploys where the studio and player are on the same host as
	// the API. Production deploys should set this.
	CORSAllowedOrigins string

	// MaxUploadBytes caps request body size for track uploads to prevent
	// disk-fill DoS. 0 preserves the legacy no-limit behaviour.
	MaxUploadBytes int64

	// LoginRateBurst / LoginRateRefillPerMin control the per-IP login
	// rate limiter. Defaults are chosen for a human retrying a password.
	LoginRateBurst       int
	LoginRateRefillPerMin int

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

		CORSAllowedOrigins: getEnv("SILKRADDY_CORS_ORIGINS", ""),

		// Default: 2 GiB total per multipart upload — big enough for a full
		// live-set recording, small enough to protect ephemeral disk. Set 0
		// via env to disable.
		MaxUploadBytes:        getEnvInt64("SILKRADDY_MAX_UPLOAD_BYTES", 2*1024*1024*1024),
		LoginRateBurst:        int(getEnvInt64("SILKRADDY_LOGIN_RATE_BURST", 10)),
		LoginRateRefillPerMin: int(getEnvInt64("SILKRADDY_LOGIN_RATE_REFILL_PER_MIN", 5)),

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

func getEnvInt64(key string, defaultValue int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
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
