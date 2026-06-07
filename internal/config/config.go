package config

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for FunnelBarn.
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
	SelfProject         string
	SelfEnvironment     string
	DogfoodAPIKey       string
	DogfoodProject      string
	EventRetentionDays  int // 0 = disabled; default 90
	LoginRatePerMinute  float64
	LoginRateBurst      float64
	APIRatePerMinute    float64
	APIRateBurst        float64
	MetricsToken        string
	LogLevel            slog.Level
	IngestRatePerMinute float64
	IngestRateBurst     float64
	SpanBarnEndpoint    string
	SpanBarnAPIKey      string
	TrustedProxies      []string
	SetupRatePerMinute  float64
	SetupRateBurst      float64

	IAMBarnClientID string
	IAMBarnIssuer   string

	OIDCIssuer        string // FUNNELBARN_OIDC_ISSUER — when all four OIDC vars are set, OIDC login is offered alongside local auth
	OIDCClientID      string // FUNNELBARN_OIDC_CLIENT_ID
	OIDCClientSecret  string // FUNNELBARN_OIDC_CLIENT_SECRET
	OIDCRedirectURL   string // FUNNELBARN_OIDC_REDIRECT_URL
	OIDCRequiredGroup string // FUNNELBARN_OIDC_REQUIRED_GROUP — defaults to "funnelbarn-users"

	GeoIPCityDB string // path to GeoLite2-City.mmdb; empty = geo disabled
	GeoIPASNDB  string // path to GeoLite2-ASN.mmdb; empty = skip ASN enrichment

	R2AccountID       string // FUNNELBARN_R2_ACCOUNT_ID — Cloudflare account ID
	R2AccessKeyID     string // FUNNELBARN_R2_ACCESS_KEY_ID
	R2SecretAccessKey string // FUNNELBARN_R2_SECRET_ACCESS_KEY
	R2Bucket          string // FUNNELBARN_R2_BUCKET — bucket name for recordings
	R2Endpoint        string // FUNNELBARN_R2_ENDPOINT — full endpoint URL (required for EU jurisdiction buckets)
}

// Load reads config from config files and environment variables.
// Environment variables always win over file values.
func Load() Config {
	loadConfigFiles()

	cfg := Config{
		Addr:                getenv("FUNNELBARN_ADDR", ":8080"),
		APIKey:              os.Getenv("FUNNELBARN_API_KEY"),
		APIKeySHA256:        os.Getenv("FUNNELBARN_API_KEY_SHA256"),
		AdminUsername:       os.Getenv("FUNNELBARN_ADMIN_USERNAME"),
		AdminPassword:       os.Getenv("FUNNELBARN_ADMIN_PASSWORD"),
		AdminPasswordBcrypt: os.Getenv("FUNNELBARN_ADMIN_PASSWORD_BCRYPT"),
		SessionSecret:       os.Getenv("FUNNELBARN_SESSION_SECRET"),
		SessionTTL:          12 * time.Hour,
		SpoolDir:            getenv("FUNNELBARN_SPOOL_DIR", ".data/spool"),
		DBPath:              getenv("FUNNELBARN_DB_PATH", ".data/funnelbarn.db"),
		MaxBodyBytes:        1 << 20, // 1 MiB
		PublicURL:           os.Getenv("FUNNELBARN_PUBLIC_URL"),
		SelfEndpoint:        os.Getenv("FUNNELBARN_SELF_ENDPOINT"),
		SelfAPIKey:          os.Getenv("FUNNELBARN_SELF_API_KEY"),
		SelfProject:         getenv("FUNNELBARN_SELF_PROJECT", "funnelbarn"),
		SelfEnvironment:     getenv("FUNNELBARN_ENVIRONMENT", "production"),
		DogfoodAPIKey:       os.Getenv("FUNNELBARN_DOGFOOD_API_KEY"),
		DogfoodProject:      getenv("FUNNELBARN_DOGFOOD_PROJECT", "funnelbarn"),
	}

	if raw := os.Getenv("FUNNELBARN_ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, trimmed)
			}
		}
	}
	if raw := os.Getenv("FUNNELBARN_MAX_BODY_BYTES"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			cfg.MaxBodyBytes = parsed
		}
	}
	if raw := os.Getenv("FUNNELBARN_MAX_SPOOL_BYTES"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			cfg.MaxSpoolBytes = parsed
		}
	}
	if raw := os.Getenv("FUNNELBARN_SESSION_TTL_SECONDS"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			cfg.SessionTTL = time.Duration(parsed) * time.Second
		}
	}
	cfg.EventRetentionDays = 90
	if raw := os.Getenv("FUNNELBARN_EVENT_RETENTION_DAYS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			cfg.EventRetentionDays = parsed
		}
	}

	// Login rate limit — default 20/min burst 20.
	// Set higher (e.g. 1000) in test environments to avoid blocking E2E suites.
	cfg.LoginRatePerMinute = 20
	cfg.LoginRateBurst = 20
	if raw := os.Getenv("FUNNELBARN_LOGIN_RATE_PER_MINUTE"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.LoginRatePerMinute = parsed
		}
	}
	if raw := os.Getenv("FUNNELBARN_LOGIN_RATE_BURST"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.LoginRateBurst = parsed
		}
	}

	// API rate limit — default 300/min burst 60.
	cfg.APIRatePerMinute = 300
	cfg.APIRateBurst = 60
	if raw := os.Getenv("FUNNELBARN_API_RATE_PER_MINUTE"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.APIRatePerMinute = parsed
		}
	}
	if raw := os.Getenv("FUNNELBARN_API_RATE_BURST"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.APIRateBurst = parsed
		}
	}

	cfg.MetricsToken = os.Getenv("FUNNELBARN_METRICS_TOKEN")

	// Ingest rate limit — default 500/min burst 100.
	cfg.IngestRatePerMinute = 500
	cfg.IngestRateBurst = 100
	if raw := os.Getenv("FUNNELBARN_INGEST_RATE_PER_MINUTE"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.IngestRatePerMinute = parsed
		}
	}
	if raw := os.Getenv("FUNNELBARN_INGEST_RATE_BURST"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.IngestRateBurst = parsed
		}
	}

	cfg.SpanBarnEndpoint = os.Getenv("FUNNELBARN_SPANBARN_ENDPOINT")
	cfg.SpanBarnAPIKey = os.Getenv("FUNNELBARN_SPANBARN_API_KEY")

	if raw := os.Getenv("FUNNELBARN_TRUSTED_PROXIES"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cfg.TrustedProxies = append(cfg.TrustedProxies, trimmed)
			}
		}
	}

	cfg.IAMBarnClientID = os.Getenv("FUNNELBARN_IAMBARN_CLIENT_ID")
	cfg.IAMBarnIssuer = getenv("FUNNELBARN_IAMBARN_ISSUER", "https://iam.wiebe.xyz")

	cfg.OIDCIssuer = os.Getenv("FUNNELBARN_OIDC_ISSUER")
	cfg.OIDCClientID = os.Getenv("FUNNELBARN_OIDC_CLIENT_ID")
	cfg.OIDCClientSecret = os.Getenv("FUNNELBARN_OIDC_CLIENT_SECRET")
	cfg.OIDCRedirectURL = os.Getenv("FUNNELBARN_OIDC_REDIRECT_URL")
	cfg.OIDCRequiredGroup = getenv("FUNNELBARN_OIDC_REQUIRED_GROUP", "funnelbarn-users")

	cfg.GeoIPCityDB = os.Getenv("FUNNELBARN_GEOIP_CITY_DB")
	cfg.GeoIPASNDB = os.Getenv("FUNNELBARN_GEOIP_ASN_DB")

	cfg.R2AccountID = os.Getenv("FUNNELBARN_R2_ACCOUNT_ID")
	cfg.R2AccessKeyID = os.Getenv("FUNNELBARN_R2_ACCESS_KEY_ID")
	cfg.R2SecretAccessKey = os.Getenv("FUNNELBARN_R2_SECRET_ACCESS_KEY")
	cfg.R2Bucket = os.Getenv("FUNNELBARN_R2_BUCKET")
	cfg.R2Endpoint = os.Getenv("FUNNELBARN_R2_ENDPOINT")

	cfg.SetupRatePerMinute = 10
	cfg.SetupRateBurst = 5
	if raw := os.Getenv("FUNNELBARN_SETUP_RATE_PER_MINUTE"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.SetupRatePerMinute = parsed
		}
	}
	if raw := os.Getenv("FUNNELBARN_SETUP_RATE_BURST"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.SetupRateBurst = parsed
		}
	}

	cfg.LogLevel = slog.LevelInfo
	if raw := os.Getenv("FUNNELBARN_LOG_LEVEL"); raw != "" {
		switch strings.ToLower(raw) {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "warn", "warning":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
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
//   - /etc/funnelbarn/funnelbarn.conf       (Linux system-wide)
//   - ~/.config/funnelbarn/funnelbarn.conf  (XDG user config, Linux + macOS)
func loadConfigFiles() {
	candidates := []string{
		"/etc/funnelbarn/funnelbarn.conf",
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "funnelbarn", "funnelbarn.conf"))
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
