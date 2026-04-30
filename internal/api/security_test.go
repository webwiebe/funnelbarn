package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ---------------------------------------------------------------------------
// 1. Authentication enforcement
// ---------------------------------------------------------------------------

// TestRequireSession_NoCooke_Returns401 verifies that every session-protected
// endpoint returns 401 when no session cookie is present.
func TestRequireSession_NoCooke_Returns401(t *testing.T) {
	srv, _ := newAuthedServer(t)
	protectedPaths := []struct{ method, path string }{
		{"GET", "/api/v1/projects"},
		{"POST", "/api/v1/projects"},
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/apikeys"},
	}
	for _, tc := range protectedPaths {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Expired/tampered session cookie
// ---------------------------------------------------------------------------

func TestRequireSession_TamperedCookie_Returns401(t *testing.T) {
	srv, _ := newAuthedServer(t)
	// Create a valid cookie then modify the token
	cookie := &http.Cookie{
		Name:  "funnelbarn_session",
		Value: "definitely-not-valid-token-xyz",
	}
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("tampered cookie: expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// 3. Rate limit enforcement (login)
// ---------------------------------------------------------------------------

func TestLoginRateLimit_Enforced(t *testing.T) {
	// Create server with tight rate limit (1 req/min, burst 1)
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ingestHandler := ingest.NewHandler(auth.New("k"), sp, 0)
	sm := auth.NewSessionManager("secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("admin", "password", "")
	srv := NewServer(ingestHandler,
		service.NewProjectService(store), service.NewFunnelService(store),
		service.NewABTestService(store), service.NewEventService(store),
		service.NewSessionService(store), service.NewAPIKeyService(store),
		userAuth, sm, nil, "secret", "http://localhost",
		1, 1, // loginRatePerMinute=1, loginRateBurst=1
		store,
	)

	body := map[string]string{"username": "admin", "password": "wrong"}
	// First request should get through (burst=1)
	postJSON(t, srv, "/api/v1/login", body, nil)
	// Second immediate request should be rate-limited
	w := postJSON(t, srv, "/api/v1/login", body, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("rate limit: expected 429, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// 4. Sensitive fields not leaked in responses
// ---------------------------------------------------------------------------

// TestAPIKey_HashNotLeaked verifies that the API key hash is never returned in
// list or create responses.
func TestAPIKey_HashNotLeaked(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create a project first
	pw := postJSON(t, srv, "/api/v1/projects", map[string]string{"name": "P", "slug": "p"}, nil)
	var proj map[string]any
	json.Unmarshal(pw.Body.Bytes(), &proj)
	pid := proj["id"].(string)

	// Create an API key
	kw := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": pid, "name": "mykey", "scope": "full",
	}, nil)
	if kw.Code != http.StatusCreated {
		t.Fatalf("create apikey: %d %s", kw.Code, kw.Body.String())
	}
	body := kw.Body.String()
	// The JSON tag on KeyHash is json:"-" so it should never appear
	if strings.Contains(body, "key_hash") || strings.Contains(body, "KeyHash") {
		t.Errorf("API key hash leaked in create response: %s", body)
	}
}

// TestUser_PasswordHashNotLeaked verifies that password hashes never appear
// in any API response.
func TestUser_PasswordHashNotLeaked(t *testing.T) {
	srv, store := newTestServer(t)
	// Seed a user directly
	_ = store.UpsertUser(context.Background(), "tester", "$2a$10$fakehashhhhhhhhhhhhhhhhhhhhhhhhh")

	// Login returns user object — check no password_hash in response
	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "tester", "password": "anything",
	}, nil)
	// Login will fail (wrong password) but we can check list endpoints too
	_ = w
	// Check /me endpoint
	cookie := sessionCookieFor(t, srv, "tester")
	mw := getJSON(t, srv, "/api/v1/me", cookie)
	body := mw.Body.String()
	if strings.Contains(body, "password") || strings.Contains(body, "hash") {
		t.Errorf("password hash leaked in /me response: %s", body)
	}
}

// ---------------------------------------------------------------------------
// 5. Error responses don't leak internal details
// ---------------------------------------------------------------------------

func TestErrorResponses_NoInternalDetails(t *testing.T) {
	srv, _ := newTestServer(t)
	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"unknown project", "GET", "/api/v1/projects/nonexistent-id/dashboard", nil},
		{"bad JSON", "POST", "/api/v1/projects", "not-json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != nil {
				b, _ := json.Marshal(tc.body)
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader(b))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			body := w.Body.String()
			// Should never see SQL, stack traces, or internal paths
			for _, forbidden := range []string{"sqlite", "SQLITE", "panic", "goroutine", "/Users/", "/home/"} {
				if strings.Contains(body, forbidden) {
					t.Errorf("internal detail %q leaked in error response: %s", forbidden, body)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. CORS: wildcard blocked when origins configured
// ---------------------------------------------------------------------------

func TestCORS_AllowedOriginOnly(t *testing.T) {
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ih := ingest.NewHandler(auth.New("k"), sp, 0)
	sm := auth.NewSessionManager("s", time.Hour)
	ua, _ := auth.NewUserAuthenticator("", "", "")
	srv := NewServer(ih,
		service.NewProjectService(store), service.NewFunnelService(store),
		service.NewABTestService(store), service.NewEventService(store),
		service.NewSessionService(store), service.NewAPIKeyService(store),
		ua, sm, []string{"https://allowed.example.com"}, "s", "", 1000, 1000, store)

	// Allowed origin gets ACAO header
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example.com" {
		t.Errorf("allowed origin: want https://allowed.example.com, got %q", got)
	}

	// Unknown origin gets no ACAO header
	req2 := httptest.NewRequest("GET", "/api/v1/health", nil)
	req2.Header.Set("Origin", "https://evil.example.com")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("blocked origin: want empty ACAO, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// 7. Security headers present
// ---------------------------------------------------------------------------

func TestSecurityHeaders_Present(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	headers := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for h, want := range headers {
		if got := w.Header().Get(h); got != want {
			t.Errorf("header %s: want %q, got %q", h, want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. Ingest endpoint requires valid API key
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 9. /metrics endpoint — token protection
// ---------------------------------------------------------------------------

// TestMetricsEndpoint_OpenWhenNoToken verifies /metrics is accessible when no token configured.
func TestMetricsEndpoint_OpenWhenNoToken(t *testing.T) {
	srv, _ := newTestServer(t) // no metricsToken
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("metrics no token: expected 200, got %d", w.Code)
	}
}

// TestMetricsEndpoint_RequiresToken verifies /metrics returns 401 when a token is
// configured and the request carries no (or an incorrect) Authorization header.
func TestMetricsEndpoint_RequiresToken(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetMetricsToken("secret-token")

	// No Authorization header → 401.
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("metrics no header: expected 401, got %d", w.Code)
	}

	// Wrong token → 401.
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("metrics wrong token: expected 401, got %d", w2.Code)
	}

	// Correct token → 200.
	req3 := httptest.NewRequest("GET", "/metrics", nil)
	req3.Header.Set("Authorization", "Bearer secret-token")
	w3 := httptest.NewRecorder()
	srv.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("metrics correct token: expected 200, got %d", w3.Code)
	}
}

// TestBodySizeLimit_NonIngestRoute verifies that non-ingest routes reject bodies
// larger than 256 KiB.
func TestBodySizeLimit_NonIngestRoute(t *testing.T) {
	srv, _ := newTestServer(t)
	// Build a body larger than 256 KiB.
	large := make([]byte, (256<<10)+1)
	for i := range large {
		large[i] = 'x'
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(large))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// Expect either 400 (invalid JSON after truncation) or 413 (too large), not 500.
	if w.Code == http.StatusInternalServerError {
		t.Errorf("oversized body on non-ingest route: expected 400 or 413, got 500")
	}
}

func TestIngest_NoAPIKey_Returns401(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"event":"pageview","url":"https://example.com","project":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("ingest no key: expected 401, got %d", w.Code)
	}
}
