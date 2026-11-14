package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// loadEnvKeys lists every variable Load reads. A test that calls Load clears
// all of them first so the machine's environment and config files cannot leak
// in.
var loadEnvKeys = []string{
	"FUNNELBARN_ADDR", "FUNNELBARN_ADMIN_PASSWORD", "FUNNELBARN_ADMIN_PASSWORD_BCRYPT",
	"FUNNELBARN_ADMIN_USERNAME", "FUNNELBARN_ALLOWED_ORIGINS", "FUNNELBARN_API_KEY",
	"FUNNELBARN_API_KEY_SHA256", "FUNNELBARN_API_RATE_BURST", "FUNNELBARN_API_RATE_PER_MINUTE",
	"FUNNELBARN_DB_PATH", "FUNNELBARN_DOGFOOD_API_KEY", "FUNNELBARN_DOGFOOD_PROJECT",
	"FUNNELBARN_ENVIRONMENT", "FUNNELBARN_EVENT_RETENTION_DAYS",
	"FUNNELBARN_FLAG_AUTOREGISTER_MAX", "FUNNELBARN_FLAG_AUTOREGISTER_TTL_DAYS",
	"FUNNELBARN_FLAG_CONFIG_CACHE_SECONDS", "FUNNELBARN_GEOIP_ASN_DB", "FUNNELBARN_GEOIP_CITY_DB",
	"FUNNELBARN_GITHUB_WEBHOOK_PROJECT", "FUNNELBARN_GITHUB_WEBHOOK_SECRET",
	"FUNNELBARN_INGEST_QUEUE_MAX_LEN", "FUNNELBARN_INGEST_RATE_BURST",
	"FUNNELBARN_INGEST_RATE_PER_MINUTE", "FUNNELBARN_INGEST_VIA_QUEUE", "FUNNELBARN_INTERNAL_TOKEN",
	"FUNNELBARN_LOG_LEVEL", "FUNNELBARN_LOGIN_RATE_BURST", "FUNNELBARN_LOGIN_RATE_PER_MINUTE",
	"FUNNELBARN_MAX_BODY_BYTES", "FUNNELBARN_MAX_SPOOL_BYTES", "FUNNELBARN_MCP_RESOURCE_URL",
	"FUNNELBARN_METRICS_TOKEN", "FUNNELBARN_MODE", "FUNNELBARN_OIDC_CLIENT_ID",
	"FUNNELBARN_OIDC_CLIENT_SECRET", "FUNNELBARN_OIDC_ISSUER", "FUNNELBARN_OIDC_ORGANIZATION_ID",
	"FUNNELBARN_OIDC_REDIRECT_URL", "FUNNELBARN_OIDC_REFRESH_GRACE_SECONDS",
	"FUNNELBARN_POST_LOGOUT_REDIRECT_URI", "FUNNELBARN_PUBLIC_URL", "FUNNELBARN_R2_ACCESS_KEY_ID",
	"FUNNELBARN_R2_ACCOUNT_ID", "FUNNELBARN_R2_BUCKET", "FUNNELBARN_R2_ENDPOINT",
	"FUNNELBARN_R2_SECRET_ACCESS_KEY", "FUNNELBARN_RECORDINGS_QUEUE_MAX_LEN",
	"FUNNELBARN_RECORDINGS_VIA_QUEUE", "FUNNELBARN_REDIS_QUEUE_URL", "FUNNELBARN_SELF_API_KEY",
	"FUNNELBARN_SELF_ENDPOINT", "FUNNELBARN_SELF_PROJECT", "FUNNELBARN_SESSION_SECRET",
	"FUNNELBARN_SESSION_TTL_SECONDS", "FUNNELBARN_SETUP_RATE_BURST", "FUNNELBARN_SETUP_RATE_PER_MINUTE",
	"FUNNELBARN_SPANBARN_API_KEY", "FUNNELBARN_SPANBARN_ENDPOINT", "FUNNELBARN_SPANBARN_LOG_LEVEL",
	"FUNNELBARN_SPOOL_DIR", "FUNNELBARN_TRUSTED_PROXIES", "FUNNELBARN_VERSION",
	"FUNNELBARN_WRITER_URL",
}

// cleanLoadEnv empties every variable Load reads and points HOME at an empty
// directory, so no user config file applies.
func cleanLoadEnv(t *testing.T) {
	t.Helper()
	for _, k := range loadEnvKeys {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", t.TempDir())
}

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

// With nothing set, Load returns exactly these defaults.
func TestLoad_AllDefaults(t *testing.T) {
	cleanLoadEnv(t)

	want := Config{
		Addr:                    ":8080",
		SessionTTL:              12 * time.Hour,
		SpoolDir:                ".data/spool",
		DBPath:                  ".data/funnelbarn.db",
		MaxBodyBytes:            1 << 20,
		SelfProject:             "funnelbarn",
		SelfEnvironment:         "production",
		DogfoodProject:          "funnelbarn",
		EventRetentionDays:      90,
		AutoRegisterMaxFlags:    100,
		AutoRegisterTTLDays:     30,
		ConfigFlagCacheSeconds:  60,
		LoginRatePerMinute:      20,
		LoginRateBurst:          20,
		APIRatePerMinute:        300,
		APIRateBurst:            60,
		IngestRatePerMinute:     500,
		IngestRateBurst:         100,
		SetupRatePerMinute:      10,
		SetupRateBurst:          5,
		LogLevel:                slog.LevelInfo,
		SpanBarnLogLevel:        slog.LevelInfo,
		OIDCRefreshGraceSeconds: 3600,
		IngestQueueMaxLen:       10000,
		RecordingsQueueMaxLen:   5000,
		Mode:                    ModeStandalone,
		GitHubWebhookProject:    "agents",
	}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("defaults differ\n got: %+v\nwant: %+v", got, want)
	}
}

// With every variable set, Load maps each one to its field.
func TestLoad_AllOverrides(t *testing.T) {
	cleanLoadEnv(t)
	setEnv(t, map[string]string{
		"FUNNELBARN_ADDR":                       ":9999",
		"FUNNELBARN_API_KEY":                    "k",
		"FUNNELBARN_API_KEY_SHA256":             "sha",
		"FUNNELBARN_ADMIN_USERNAME":             "root",
		"FUNNELBARN_ADMIN_PASSWORD":             "pw",
		"FUNNELBARN_ADMIN_PASSWORD_BCRYPT":      "bc",
		"FUNNELBARN_SESSION_SECRET":             "sec",
		"FUNNELBARN_SESSION_TTL_SECONDS":        "60",
		"FUNNELBARN_ALLOWED_ORIGINS":            "https://a.test, ,https://b.test",
		"FUNNELBARN_SPOOL_DIR":                  "/spool",
		"FUNNELBARN_DB_PATH":                    "/db.sqlite",
		"FUNNELBARN_MAX_BODY_BYTES":             "2048",
		"FUNNELBARN_MAX_SPOOL_BYTES":            "4096",
		"FUNNELBARN_PUBLIC_URL":                 "https://fb.test/",
		"FUNNELBARN_SELF_ENDPOINT":              "https://self.test",
		"FUNNELBARN_SELF_API_KEY":               "selfkey",
		"FUNNELBARN_SELF_PROJECT":               "selfproj",
		"FUNNELBARN_ENVIRONMENT":                "staging",
		"FUNNELBARN_VERSION":                    "v9.9.9",
		"FUNNELBARN_DOGFOOD_API_KEY":            "dogkey",
		"FUNNELBARN_DOGFOOD_PROJECT":            "dogproj",
		"FUNNELBARN_EVENT_RETENTION_DAYS":       "0",
		"FUNNELBARN_FLAG_AUTOREGISTER_MAX":      "7",
		"FUNNELBARN_FLAG_AUTOREGISTER_TTL_DAYS": "8",
		"FUNNELBARN_FLAG_CONFIG_CACHE_SECONDS":  "9",
		"FUNNELBARN_LOGIN_RATE_PER_MINUTE":      "1.5",
		"FUNNELBARN_LOGIN_RATE_BURST":           "2.5",
		"FUNNELBARN_API_RATE_PER_MINUTE":        "3.5",
		"FUNNELBARN_API_RATE_BURST":             "4.5",
		"FUNNELBARN_INGEST_RATE_PER_MINUTE":     "5.5",
		"FUNNELBARN_INGEST_RATE_BURST":          "6.5",
		"FUNNELBARN_SETUP_RATE_PER_MINUTE":      "7.5",
		"FUNNELBARN_SETUP_RATE_BURST":           "8.5",
		"FUNNELBARN_METRICS_TOKEN":              "metrics",
		"FUNNELBARN_LOG_LEVEL":                  "error",
		"FUNNELBARN_SPANBARN_ENDPOINT":          "https://sb.test",
		"FUNNELBARN_SPANBARN_API_KEY":           "sbkey",
		"FUNNELBARN_SPANBARN_LOG_LEVEL":         "warning",
		"FUNNELBARN_TRUSTED_PROXIES":            "10.0.0.0/8 , ,192.168.0.1",
		"FUNNELBARN_POST_LOGOUT_REDIRECT_URI":   "https://fb.test/bye",
		"FUNNELBARN_OIDC_ISSUER":                "https://iam.test",
		"FUNNELBARN_OIDC_CLIENT_ID":             "cid",
		"FUNNELBARN_OIDC_CLIENT_SECRET":         "csec",
		"FUNNELBARN_OIDC_REDIRECT_URL":          "https://fb.test/cb",
		"FUNNELBARN_OIDC_ORGANIZATION_ID":       "org",
		"FUNNELBARN_OIDC_REFRESH_GRACE_SECONDS": "120",
		"FUNNELBARN_MCP_RESOURCE_URL":           "https://fb.test/mcp",
		"FUNNELBARN_GEOIP_CITY_DB":              "/city.mmdb",
		"FUNNELBARN_GEOIP_ASN_DB":               "/asn.mmdb",
		"FUNNELBARN_R2_ACCOUNT_ID":              "acct",
		"FUNNELBARN_R2_ACCESS_KEY_ID":           "akid",
		"FUNNELBARN_R2_SECRET_ACCESS_KEY":       "asec",
		"FUNNELBARN_R2_BUCKET":                  "bucket",
		"FUNNELBARN_R2_ENDPOINT":                "https://r2.test",
		"FUNNELBARN_REDIS_QUEUE_URL":            "redis://q:6379/0",
		"FUNNELBARN_INGEST_VIA_QUEUE":           "true",
		"FUNNELBARN_INGEST_QUEUE_MAX_LEN":       "11",
		"FUNNELBARN_RECORDINGS_VIA_QUEUE":       "true",
		"FUNNELBARN_RECORDINGS_QUEUE_MAX_LEN":   "12",
		"FUNNELBARN_MODE":                       " Reader ",
		"FUNNELBARN_WRITER_URL":                 "http://writer:8080//",
		"FUNNELBARN_INTERNAL_TOKEN":             "internal",
		"FUNNELBARN_GITHUB_WEBHOOK_SECRET":      "ghsec",
		"FUNNELBARN_GITHUB_WEBHOOK_PROJECT":     "ghproj",
	})

	want := Config{
		Addr:                    ":9999",
		APIKey:                  "k",
		APIKeySHA256:            "sha",
		AdminUsername:           "root",
		AdminPassword:           "pw",
		AdminPasswordBcrypt:     "bc",
		SessionSecret:           "sec",
		SessionTTL:              60 * time.Second,
		AllowedOrigins:          []string{"https://a.test", "https://b.test"},
		SpoolDir:                "/spool",
		DBPath:                  "/db.sqlite",
		MaxBodyBytes:            2048,
		MaxSpoolBytes:           4096,
		PublicURL:               "https://fb.test/",
		SelfEndpoint:            "https://self.test",
		SelfAPIKey:              "selfkey",
		SelfProject:             "selfproj",
		SelfEnvironment:         "staging",
		Version:                 "v9.9.9",
		DogfoodAPIKey:           "dogkey",
		DogfoodProject:          "dogproj",
		EventRetentionDays:      0,
		AutoRegisterMaxFlags:    7,
		AutoRegisterTTLDays:     8,
		ConfigFlagCacheSeconds:  9,
		LoginRatePerMinute:      1.5,
		LoginRateBurst:          2.5,
		APIRatePerMinute:        3.5,
		APIRateBurst:            4.5,
		IngestRatePerMinute:     5.5,
		IngestRateBurst:         6.5,
		SetupRatePerMinute:      7.5,
		SetupRateBurst:          8.5,
		MetricsToken:            "metrics",
		LogLevel:                slog.LevelError,
		SpanBarnEndpoint:        "https://sb.test",
		SpanBarnAPIKey:          "sbkey",
		SpanBarnLogLevel:        slog.LevelWarn,
		TrustedProxies:          []string{"10.0.0.0/8", "192.168.0.1"},
		PostLogoutRedirectURI:   "https://fb.test/bye",
		OIDCIssuer:              "https://iam.test",
		OIDCClientID:            "cid",
		OIDCClientSecret:        "csec",
		OIDCRedirectURL:         "https://fb.test/cb",
		OIDCOrganizationID:      "org",
		OIDCRefreshGraceSeconds: 120,
		MCPResourceURL:          "https://fb.test/mcp",
		GeoIPCityDB:             "/city.mmdb",
		GeoIPASNDB:              "/asn.mmdb",
		R2AccountID:             "acct",
		R2AccessKeyID:           "akid",
		R2SecretAccessKey:       "asec",
		R2Bucket:                "bucket",
		R2Endpoint:              "https://r2.test",
		RedisQueueURL:           "redis://q:6379/0",
		IngestViaQueue:          true,
		IngestQueueMaxLen:       11,
		RecordingsViaQueue:      true,
		RecordingsQueueMaxLen:   12,
		Mode:                    ModeReader,
		WriterURL:               "http://writer:8080",
		InternalToken:           "internal",
		GitHubWebhookSecret:     "ghsec",
		GitHubWebhookProject:    "ghproj",
	}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("overrides differ\n got: %+v\nwant: %+v", got, want)
	}
}

// A rate that is not a positive number keeps its default.
func TestLoad_RateVariablesIgnoreBadValues(t *testing.T) {
	cases := []struct {
		key  string
		get  func(Config) float64
		want float64
	}{
		{"FUNNELBARN_LOGIN_RATE_PER_MINUTE", func(c Config) float64 { return c.LoginRatePerMinute }, 20},
		{"FUNNELBARN_LOGIN_RATE_BURST", func(c Config) float64 { return c.LoginRateBurst }, 20},
		{"FUNNELBARN_API_RATE_PER_MINUTE", func(c Config) float64 { return c.APIRatePerMinute }, 300},
		{"FUNNELBARN_API_RATE_BURST", func(c Config) float64 { return c.APIRateBurst }, 60},
		{"FUNNELBARN_INGEST_RATE_PER_MINUTE", func(c Config) float64 { return c.IngestRatePerMinute }, 500},
		{"FUNNELBARN_INGEST_RATE_BURST", func(c Config) float64 { return c.IngestRateBurst }, 100},
		{"FUNNELBARN_SETUP_RATE_PER_MINUTE", func(c Config) float64 { return c.SetupRatePerMinute }, 10},
		{"FUNNELBARN_SETUP_RATE_BURST", func(c Config) float64 { return c.SetupRateBurst }, 5},
	}
	for _, tc := range cases {
		for _, bad := range []string{"abc", "0", "-3"} {
			t.Run(tc.key+"="+bad, func(t *testing.T) {
				cleanLoadEnv(t)
				t.Setenv(tc.key, bad)
				if got := tc.get(Load()); got != tc.want {
					t.Errorf("%s=%q: got %v, want default %v", tc.key, bad, got, tc.want)
				}
			})
		}
	}
}

// Integer settings fall back on bad values; zero disables where the setting
// allows it and falls back where it does not.
func TestLoad_IntegerVariablesIgnoreBadValues(t *testing.T) {
	cases := []struct {
		key, val string
		get      func(Config) int64
		want     int64
	}{
		{"FUNNELBARN_MAX_BODY_BYTES", "0", func(c Config) int64 { return c.MaxBodyBytes }, 1 << 20},
		{"FUNNELBARN_MAX_SPOOL_BYTES", "-1", func(c Config) int64 { return c.MaxSpoolBytes }, 0},
		{"FUNNELBARN_SESSION_TTL_SECONDS", "0", func(c Config) int64 { return int64(c.SessionTTL / time.Second) }, 12 * 3600},
		{"FUNNELBARN_EVENT_RETENTION_DAYS", "-1", func(c Config) int64 { return int64(c.EventRetentionDays) }, 90},
		{"FUNNELBARN_EVENT_RETENTION_DAYS", "0", func(c Config) int64 { return int64(c.EventRetentionDays) }, 0},
		{"FUNNELBARN_FLAG_AUTOREGISTER_MAX", "x", func(c Config) int64 { return int64(c.AutoRegisterMaxFlags) }, 100},
		{"FUNNELBARN_FLAG_AUTOREGISTER_TTL_DAYS", "0", func(c Config) int64 { return int64(c.AutoRegisterTTLDays) }, 0},
		{"FUNNELBARN_FLAG_CONFIG_CACHE_SECONDS", "-5", func(c Config) int64 { return int64(c.ConfigFlagCacheSeconds) }, 60},
		{"FUNNELBARN_OIDC_REFRESH_GRACE_SECONDS", "0", func(c Config) int64 { return int64(c.OIDCRefreshGraceSeconds) }, 3600},
		{"FUNNELBARN_INGEST_QUEUE_MAX_LEN", "0", func(c Config) int64 { return c.IngestQueueMaxLen }, 10000},
		{"FUNNELBARN_RECORDINGS_QUEUE_MAX_LEN", "abc", func(c Config) int64 { return c.RecordingsQueueMaxLen }, 5000},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			cleanLoadEnv(t)
			t.Setenv(tc.key, tc.val)
			if got := tc.get(Load()); got != tc.want {
				t.Errorf("%s=%q: got %d, want %d", tc.key, tc.val, got, tc.want)
			}
		})
	}
}

// The queue switches turn on for the exact string "true" only.
func TestLoad_QueueSwitchesNeedExactTrue(t *testing.T) {
	for _, v := range []string{"1", "TRUE", "yes", "false", ""} {
		t.Run(v, func(t *testing.T) {
			cleanLoadEnv(t)
			t.Setenv("FUNNELBARN_INGEST_VIA_QUEUE", v)
			t.Setenv("FUNNELBARN_RECORDINGS_VIA_QUEUE", v)
			cfg := Load()
			if cfg.IngestViaQueue || cfg.RecordingsViaQueue {
				t.Errorf("value %q switched a queue on", v)
			}
		})
	}
}

// Unknown or empty log levels fall back to info; the post-logout redirect and
// MCP resource URL derive from the public URL when unset.
func TestLoad_LevelsAndDerivedURLs(t *testing.T) {
	cleanLoadEnv(t)
	t.Setenv("FUNNELBARN_LOG_LEVEL", "loud")
	t.Setenv("FUNNELBARN_SPANBARN_LOG_LEVEL", "DEBUG")
	t.Setenv("FUNNELBARN_PUBLIC_URL", "https://fb.test///")
	cfg := Load()
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.SpanBarnLogLevel != slog.LevelDebug {
		t.Errorf("SpanBarnLogLevel = %v, want debug", cfg.SpanBarnLogLevel)
	}
	if want := "https://fb.test/api/v1/auth/oidc/logged-out"; cfg.PostLogoutRedirectURI != want {
		t.Errorf("PostLogoutRedirectURI = %q, want %q", cfg.PostLogoutRedirectURI, want)
	}
	if want := "https://fb.test/api/v1/mcp"; cfg.MCPResourceURL != want {
		t.Errorf("MCPResourceURL = %q, want %q", cfg.MCPResourceURL, want)
	}
}

// Values in a user config file fill variables that are empty in the
// environment, and the environment wins over the file.
func TestLoad_UserConfigFile(t *testing.T) {
	cleanLoadEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeUserConf(t, home, "FUNNELBARN_ADDR=:7777\nFUNNELBARN_DB_PATH=\"/file.db\"\n")
	t.Setenv("FUNNELBARN_DB_PATH", "/env.db")

	cfg := Load()
	if cfg.Addr != ":7777" {
		t.Errorf("Addr = %q, want the file value", cfg.Addr)
	}
	if cfg.DBPath != "/env.db" {
		t.Errorf("DBPath = %q, want the environment value", cfg.DBPath)
	}
}

func writeUserConf(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "funnelbarn")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "funnelbarn.conf"), []byte(content), 0o600); err != nil {
		t.Fatalf("write conf: %v", err)
	}
}
