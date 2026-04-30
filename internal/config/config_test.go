package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Unset all FUNNELBARN_ vars to test defaults.
	unset := []string{
		"FUNNELBARN_ADDR", "FUNNELBARN_API_KEY", "FUNNELBARN_API_KEY_SHA256",
		"FUNNELBARN_ADMIN_USERNAME", "FUNNELBARN_ADMIN_PASSWORD",
		"FUNNELBARN_ADMIN_PASSWORD_BCRYPT", "FUNNELBARN_SESSION_SECRET",
		"FUNNELBARN_ALLOWED_ORIGINS", "FUNNELBARN_SPOOL_DIR", "FUNNELBARN_DB_PATH",
		"FUNNELBARN_MAX_BODY_BYTES", "FUNNELBARN_MAX_SPOOL_BYTES",
		"FUNNELBARN_SESSION_TTL_SECONDS", "FUNNELBARN_PUBLIC_URL",
		"FUNNELBARN_SELF_ENDPOINT", "FUNNELBARN_SELF_API_KEY", "FUNNELBARN_ENVIRONMENT",
	}
	for _, k := range unset {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.Addr != ":8080" {
		t.Errorf("Addr default: got %q", cfg.Addr)
	}
	if cfg.SpoolDir != ".data/spool" {
		t.Errorf("SpoolDir default: got %q", cfg.SpoolDir)
	}
	if cfg.DBPath != ".data/funnelbarn.db" {
		t.Errorf("DBPath default: got %q", cfg.DBPath)
	}
	if cfg.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes default: got %d", cfg.MaxBodyBytes)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("SessionTTL default: got %v", cfg.SessionTTL)
	}
	if cfg.SelfEnvironment != "production" {
		t.Errorf("SelfEnvironment default: got %q", cfg.SelfEnvironment)
	}
}

func TestLoad_EnvVars(t *testing.T) {
	t.Setenv("FUNNELBARN_ADDR", ":9090")
	t.Setenv("FUNNELBARN_API_KEY", "test-key")
	t.Setenv("FUNNELBARN_ADMIN_USERNAME", "admin")
	t.Setenv("FUNNELBARN_ALLOWED_ORIGINS", "https://a.com, https://b.com")
	t.Setenv("FUNNELBARN_MAX_BODY_BYTES", "2097152")
	t.Setenv("FUNNELBARN_MAX_SPOOL_BYTES", "10485760")
	t.Setenv("FUNNELBARN_SESSION_TTL_SECONDS", "3600")
	t.Setenv("FUNNELBARN_ENVIRONMENT", "staging")

	cfg := Load()

	if cfg.Addr != ":9090" {
		t.Errorf("Addr: got %q", cfg.Addr)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey: got %q", cfg.APIKey)
	}
	if cfg.AdminUsername != "admin" {
		t.Errorf("AdminUsername: got %q", cfg.AdminUsername)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Errorf("AllowedOrigins len: got %d", len(cfg.AllowedOrigins))
	}
	if cfg.AllowedOrigins[0] != "https://a.com" {
		t.Errorf("AllowedOrigins[0]: got %q", cfg.AllowedOrigins[0])
	}
	if cfg.MaxBodyBytes != 2097152 {
		t.Errorf("MaxBodyBytes: got %d", cfg.MaxBodyBytes)
	}
	if cfg.MaxSpoolBytes != 10485760 {
		t.Errorf("MaxSpoolBytes: got %d", cfg.MaxSpoolBytes)
	}
	if cfg.SessionTTL != time.Hour {
		t.Errorf("SessionTTL: got %v", cfg.SessionTTL)
	}
	if cfg.SelfEnvironment != "staging" {
		t.Errorf("SelfEnvironment: got %q", cfg.SelfEnvironment)
	}
}

func TestLoad_InvalidNumbers(t *testing.T) {
	t.Setenv("FUNNELBARN_MAX_BODY_BYTES", "not-a-number")
	t.Setenv("FUNNELBARN_MAX_SPOOL_BYTES", "not-a-number")
	t.Setenv("FUNNELBARN_SESSION_TTL_SECONDS", "not-a-number")

	cfg := Load()

	if cfg.MaxBodyBytes != 1<<20 {
		t.Errorf("invalid MaxBodyBytes should use default: got %d", cfg.MaxBodyBytes)
	}
	if cfg.MaxSpoolBytes != 0 {
		t.Errorf("invalid MaxSpoolBytes should stay 0: got %d", cfg.MaxSpoolBytes)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("invalid SessionTTL should use default: got %v", cfg.SessionTTL)
	}
}

func TestApplyConfigFile_Basic(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "test.conf")

	content := strings.Join([]string{
		"# This is a comment",
		"",
		"KEY_ONE=value1",
		`KEY_TWO="quoted value"`,
		"KEY_THREE='single quoted'",
		"KEY_FOUR = spaced",
		"INVALID_LINE_NO_EQUALS",
	}, "\n")

	if err := os.WriteFile(confPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}

	// Unset keys so applyConfigFile can set them.
	t.Setenv("KEY_ONE", "")
	t.Setenv("KEY_TWO", "")
	t.Setenv("KEY_THREE", "")
	t.Setenv("KEY_FOUR", "")

	if err := applyConfigFile(confPath); err != nil {
		t.Fatalf("applyConfigFile: %v", err)
	}

	if got := os.Getenv("KEY_ONE"); got != "value1" {
		t.Errorf("KEY_ONE: got %q", got)
	}
	if got := os.Getenv("KEY_TWO"); got != "quoted value" {
		t.Errorf("KEY_TWO: got %q", got)
	}
	if got := os.Getenv("KEY_THREE"); got != "single quoted" {
		t.Errorf("KEY_THREE: got %q", got)
	}
	if got := os.Getenv("KEY_FOUR"); got != "spaced" {
		t.Errorf("KEY_FOUR: got %q", got)
	}
}

func TestApplyConfigFile_NotFound(t *testing.T) {
	err := applyConfigFile("/does/not/exist.conf")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist, got %v", err)
	}
}

func TestApplyConfigFile_DoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "test.conf")

	if err := os.WriteFile(confPath, []byte("EXISTING_KEY=from-file\n"), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}

	t.Setenv("EXISTING_KEY", "from-env")

	if err := applyConfigFile(confPath); err != nil {
		t.Fatalf("applyConfigFile: %v", err)
	}

	if got := os.Getenv("EXISTING_KEY"); got != "from-env" {
		t.Errorf("existing env var should not be overridden: got %q", got)
	}
}
