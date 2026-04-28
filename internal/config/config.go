package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for Trailpost.
type Config struct {
	Addr                string
	APIKey              string
	APIKeySHA256        string
	AdminUsername       string
	AdminPassword       string
	AdminPasswordBcrypt string
	SessionSecret       string
	SessionTTL          time.Duration
	AllowedOrigins      []string
	SpoolDir            string
	DBPath              string
	MaxBodyBytes        int64
	MaxSpoolBytes       int64
	PublicURL           string
	SelfEndpoint        string
	SelfAPIKey          string
}

// Load reads config from config files and environment variables.
// Environment variables always win over file values.
func Load() Config {
	loadConfigFiles()

	cfg := Config{
		Addr:                getenv("TRAILPOST_ADDR", ":8080"),
		APIKey:              os.Getenv("TRAILPOST_API_KEY"),
		APIKeySHA256:        os.Getenv("TRAILPOST_API_KEY_SHA256"),
		AdminUsername:       os.Getenv("TRAILPOST_ADMIN_USERNAME"),
		AdminPassword:       os.Getenv("TRAILPOST_ADMIN_PASSWORD"),
		AdminPasswordBcrypt: os.Getenv("TRAILPOST_ADMIN_PASSWORD_BCRYPT"),
		SessionSecret:       os.Getenv("TRAILPOST_SESSION_SECRET"),
		SessionTTL:          12 * time.Hour,
		SpoolDir:            getenv("TRAILPOST_SPOOL_DIR", ".data/spool"),
		DBPath:              getenv("TRAILPOST_DB_PATH", ".data/trailpost.db"),
		MaxBodyBytes:        1 << 20, // 1 MiB
		PublicURL:           os.Getenv("TRAILPOST_PUBLIC_URL"),
		SelfEndpoint:        os.Getenv("TRAILPOST_SELF_ENDPOINT"),
		SelfAPIKey:          os.Getenv("TRAILPOST_SELF_API_KEY"),
	}

	if raw := os.Getenv("TRAILPOST_ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, trimmed)
			}
		}
	}
	if raw := os.Getenv("TRAILPOST_MAX_BODY_BYTES"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			cfg.MaxBodyBytes = parsed
		}
	}
	if raw := os.Getenv("TRAILPOST_MAX_SPOOL_BYTES"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			cfg.MaxSpoolBytes = parsed
		}
	}
	if raw := os.Getenv("TRAILPOST_SESSION_TTL_SECONDS"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			cfg.SessionTTL = time.Duration(parsed) * time.Second
		}
	}

	return cfg
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// loadConfigFiles applies KEY=VALUE config files to the process environment.
// Files are read in order: system-wide first, then user-specific. Values from
// later files win over earlier ones, but env vars already set in the environment
// always take precedence over values in any file.
//
// Supported locations:
//   - /etc/trailpost/trailpost.conf       (Linux system-wide)
//   - ~/.config/trailpost/trailpost.conf  (XDG user config, Linux + macOS)
func loadConfigFiles() {
	candidates := []string{
		"/etc/trailpost/trailpost.conf",
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "trailpost", "trailpost.conf"))
	}
	for _, path := range candidates {
		if err := applyConfigFile(path); err != nil && !os.IsNotExist(err) {
			// Non-fatal: config file is optional.
			_ = err
		}
	}
}

// applyConfigFile reads KEY=VALUE pairs and sets them as environment variables
// for keys not already set. Blank lines and # comments are ignored.
// Values may optionally be wrapped in single or double quotes.
func applyConfigFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key != "" && os.Getenv(key) == "" {
			os.Setenv(key, val) //nolint:errcheck
		}
	}
	return scanner.Err()
}
