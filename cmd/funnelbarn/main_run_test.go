package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// runEnv points every variable run reads at an isolated state: temp spool and
// database, no telemetry endpoints, no admin, a bound loopback port.
func runEnv(t *testing.T) (addr string) {
	t.Helper()
	for _, k := range []string{
		"FUNNELBARN_API_KEY", "FUNNELBARN_API_KEY_SHA256", "FUNNELBARN_ADMIN_USERNAME",
		"FUNNELBARN_ADMIN_PASSWORD", "FUNNELBARN_ADMIN_PASSWORD_BCRYPT", "FUNNELBARN_PUBLIC_URL",
		"FUNNELBARN_SELF_ENDPOINT", "FUNNELBARN_SELF_API_KEY", "FUNNELBARN_DOGFOOD_API_KEY",
		"FUNNELBARN_SPANBARN_ENDPOINT", "FUNNELBARN_SPANBARN_API_KEY", "FUNNELBARN_METRICS_TOKEN",
		"FUNNELBARN_GEOIP_CITY_DB", "FUNNELBARN_GEOIP_ASN_DB", "FUNNELBARN_REDIS_QUEUE_URL",
		"FUNNELBARN_INGEST_VIA_QUEUE", "FUNNELBARN_RECORDINGS_VIA_QUEUE", "FUNNELBARN_INTERNAL_TOKEN",
		"FUNNELBARN_WRITER_URL", "FUNNELBARN_OIDC_ISSUER", "FUNNELBARN_OIDC_CLIENT_ID",
		"FUNNELBARN_OIDC_CLIENT_SECRET", "FUNNELBARN_OIDC_REDIRECT_URL",
		"FUNNELBARN_OIDC_ORGANIZATION_ID", "FUNNELBARN_R2_BUCKET", "FUNNELBARN_R2_ENDPOINT",
		"FUNNELBARN_R2_ACCESS_KEY_ID", "FUNNELBARN_R2_SECRET_ACCESS_KEY",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	t.Setenv("FUNNELBARN_SPOOL_DIR", dir+"/spool")
	t.Setenv("FUNNELBARN_DB_PATH", dir+"/funnelbarn.db")
	t.Setenv("FUNNELBARN_ENVIRONMENT", "testing")
	t.Setenv("FUNNELBARN_MODE", "")
	t.Setenv("FUNNELBARN_VERSION", "v9.9.9-test")
	t.Setenv("FUNNELBARN_SESSION_SECRET", strings.Repeat("s", 32))

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr = l.Addr().String()
	_ = l.Close()
	t.Setenv("FUNNELBARN_ADDR", addr)
	return addr
}

// callRun runs run() with a clean argument list and its stderr captured. It
// returns run's error channel and a function that reads what was logged.
func callRun(t *testing.T) (<-chan error, func() string) {
	t.Helper()
	prevArgs, prevStderr, prevLogger := os.Args, os.Stderr, slog.Default()
	logFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("temp stderr: %v", err)
	}
	os.Args = []string{"funnelbarn"}
	os.Stderr = logFile
	t.Cleanup(func() {
		os.Args, os.Stderr = prevArgs, prevStderr
		slog.SetDefault(prevLogger)
		_ = logFile.Close()
	})

	done := make(chan error, 1)
	go func() { done <- run() }()
	return done, func() string {
		b, _ := os.ReadFile(logFile.Name())
		return string(b)
	}
}

func waitForStatus(t *testing.T, url string, want int, done <-chan error) []byte {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("run returned before serving: %v", err)
		default:
		}
		resp, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return body
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s never answered %d", url, want)
	return nil
}

// A standalone process comes up, serves the API with the injected version, logs
// its startup decisions, and returns nil after SIGTERM.
func TestRun_ServesAndStopsOnSIGTERM(t *testing.T) {
	addr := runEnv(t)
	t.Setenv("FUNNELBARN_SESSION_SECRET", "") // empty is tolerated: a random per-process secret is used
	done, logs := callRun(t)

	waitForStatus(t, "http://"+addr+"/api/v1/health", http.StatusOK, done)
	body := waitForStatus(t, "http://"+addr+"/api/v1/version", http.StatusOK, done)
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.Version != "v9.9.9-test" {
		t.Errorf("version body %q (err %v), want version v9.9.9-test", body, err)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run after SIGTERM: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return after SIGTERM")
	}

	out := logs()
	for _, want := range []string{
		"self-reporting disabled",
		"process mode",
		"authentication mechanisms",
		"/metrics is served without authentication",
		"no authentication mechanism configured",
		"funnelbarn starting",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("startup log lacks %q", want)
		}
	}
	for _, unwanted := range []string{"dogfood analytics", "spanbarn telemetry enabled", "geoip enabled"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("startup log has %q although it is not configured", unwanted)
		}
	}
}

// run refuses to start on a bad configuration, before it serves anything.
func TestRun_RefusesBadConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"unknown mode", map[string]string{"FUNNELBARN_MODE": "sideways"}, "FUNNELBARN_MODE"},
		{"reader needs the queue", map[string]string{"FUNNELBARN_MODE": "reader"}, "FUNNELBARN_REDIS_QUEUE_URL"},
		{"short session secret", map[string]string{"FUNNELBARN_SESSION_SECRET": "too-short"}, "at least 32 characters"},
		{"production without any auth", map[string]string{"FUNNELBARN_ENVIRONMENT": "production"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			done, _ := callRun(t)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("run returned nil, want an error")
				}
				if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q lacks %q", err, tc.wantErr)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("run did not return")
			}
		})
	}
}

// A dogfood key without a public URL disables backend self-tracking and says
// so; a configured key with a URL enables it.
func TestRun_DogfoodWithoutPublicURLWarns(t *testing.T) {
	runEnv(t)
	t.Setenv("FUNNELBARN_DOGFOOD_API_KEY", "dog")
	// A production run without auth fails closed after the wiring is done, so
	// the process stops by itself once the startup warnings are logged.
	t.Setenv("FUNNELBARN_ENVIRONMENT", "production")
	done, logs := callRun(t)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("run returned nil, want the fail-closed error")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return")
	}
	out := logs()
	if !strings.Contains(out, "dogfood analytics enabled") {
		t.Error("log lacks the dogfood announcement")
	}
	if !strings.Contains(out, "FUNNELBARN_PUBLIC_URL is unset; backend self-tracking disabled") {
		t.Error("log lacks the missing public URL warning")
	}
}

// Startup prunes web sessions past their absolute cap and keeps live ones.
func TestRun_PrunesExpiredWebSessionsAtStartup(t *testing.T) {
	addr := runEnv(t)
	dbPath := os.Getenv("FUNNELBARN_DB_PATH")
	seed, err := repository.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().Unix()
	for _, ws := range []repository.WebSession{
		{IDHash: "expired", Username: "u", AuthMethod: "local", CreatedAt: now - 7200, AbsoluteExpiresAt: now - 3600},
		{IDHash: "live", Username: "u", AuthMethod: "local", CreatedAt: now, AbsoluteExpiresAt: now + 3600},
	} {
		if err := seed.CreateWebSession(context.Background(), ws); err != nil {
			t.Fatalf("create session %s: %v", ws.IDHash, err)
		}
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	done, _ := callRun(t)
	waitForStatus(t, "http://"+addr+"/api/v1/health", http.StatusOK, done)
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run after SIGTERM: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return after SIGTERM")
	}

	check, err := repository.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer check.Close()
	if _, err := check.GetWebSession(context.Background(), "expired"); err == nil {
		t.Error("the expired session survived startup")
	}
	if _, err := check.GetWebSession(context.Background(), "live"); err != nil {
		t.Errorf("the live session was removed: %v", err)
	}
}
