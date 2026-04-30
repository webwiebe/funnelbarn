package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// handleSetup — GET /api/v1/setup/{slug}
// ---------------------------------------------------------------------------

func TestHandleSetup_OK(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("setup: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type: want text/plain…, got %q", ct)
	}
}

func TestHandleSetup_ContainsSlugAndAPIKey(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("setup: want 200, got %d", w.Code)
	}

	body := w.Body.String()

	// The response must mention the slug.
	if !strings.Contains(body, "my-new-site") {
		t.Error("setup response does not contain the slug 'my-new-site'")
	}

	// The response must contain the deterministic API key.
	plaintext, _ := setupKey("test-secret", "my-new-site")
	if !strings.Contains(body, plaintext) {
		t.Errorf("setup response does not contain the expected API key %q", plaintext)
	}
}

func TestHandleSetup_Idempotent(t *testing.T) {
	srv, _ := newTestServer(t)

	// First call.
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first setup call: want 200, got %d", w1.Code)
	}

	// Extract the key from the first response.
	plaintext, _ := setupKey("test-secret", "my-new-site")
	if !strings.Contains(w1.Body.String(), plaintext) {
		t.Fatalf("first response missing expected key %q", plaintext)
	}

	// Second call — must succeed and return the same API key.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second setup call: want 200, got %d", w2.Code)
	}

	if !strings.Contains(w2.Body.String(), plaintext) {
		t.Errorf("second response has a different API key — endpoint is not idempotent")
	}
}

func TestHandleSetup_MissingSlug(t *testing.T) {
	// The router only matches when a slug segment is present, so hitting the
	// bare path falls through to the mux's 404. Verify it is not a 500.
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// A missing slug should not produce a 500. 404 or 400 are both acceptable.
	if w.Code == http.StatusInternalServerError {
		t.Errorf("missing slug: want non-500, got 500 (body: %s)", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// setupKey — deterministic key derivation
// ---------------------------------------------------------------------------

func TestSetupKey_Deterministic(t *testing.T) {
	pt1, hash1 := setupKey("my-secret", "acme-corp")
	pt2, hash2 := setupKey("my-secret", "acme-corp")

	if pt1 != pt2 {
		t.Errorf("plaintext not deterministic: %q != %q", pt1, pt2)
	}
	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash1, hash2)
	}
}

func TestSetupKey_DifferentSlugsProduceDifferentKeys(t *testing.T) {
	pt1, _ := setupKey("my-secret", "site-a")
	pt2, _ := setupKey("my-secret", "site-b")

	if pt1 == pt2 {
		t.Error("different slugs should produce different keys")
	}
}

func TestSetupKey_DifferentSecretsProduceDifferentKeys(t *testing.T) {
	pt1, _ := setupKey("secret-one", "my-site")
	pt2, _ := setupKey("secret-two", "my-site")

	if pt1 == pt2 {
		t.Error("different secrets should produce different keys")
	}
}

func TestSetupKey_EmptySecretUsesFallback(t *testing.T) {
	// An empty secret must not panic and must produce the same key as the
	// explicit fallback string.
	ptEmpty, _ := setupKey("", "test-slug")
	ptFallback, _ := setupKey(setupInsecureFallback, "test-slug")

	if ptEmpty == "" {
		t.Error("empty secret: expected non-empty plaintext key")
	}
	if ptEmpty != ptFallback {
		t.Errorf("empty secret did not use the fallback: %q != %q", ptEmpty, ptFallback)
	}
}

func TestSetupKey_PlaintextLength(t *testing.T) {
	// The plaintext key is always 40 hex chars (first 40 of a 64-char SHA-256 hex).
	plaintext, _ := setupKey("any-secret", "any-slug")
	if len(plaintext) != 40 {
		t.Errorf("plaintext length: want 40, got %d (%q)", len(plaintext), plaintext)
	}
}

func TestSetupKey_HashIsHex(t *testing.T) {
	_, keySHA256 := setupKey("any-secret", "any-slug")
	if len(keySHA256) != 64 {
		t.Errorf("keySHA256 length: want 64 hex chars, got %d (%q)", len(keySHA256), keySHA256)
	}
	for _, c := range keySHA256 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("keySHA256 contains non-hex character %q", c)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// setCORSHeaders — allowedOrigins list
// ---------------------------------------------------------------------------

func TestCORS_AllowedOriginMatches(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.allowedOrigins = []string{"https://allowed.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://allowed.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "https://allowed.com" {
		t.Errorf("allowed origin: want 'https://allowed.com', got %q", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Errorf("Vary: want 'Origin', got %q", w.Header().Get("Vary"))
	}
}

func TestCORS_BlockedOriginNotSet(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.allowedOrigins = []string{"https://allowed.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://blocked.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("blocked origin: want empty ACAO header, got %q", got)
	}
}

func TestCORS_NoOriginHeaderProducesNoCORSHeaders(t *testing.T) {
	srv, _ := newTestServer(t)
	// Even with an open allowedOrigins, no Origin header → no CORS headers.

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	// No Origin header set.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no origin: want empty ACAO header, got %q", got)
	}
}

func TestCORS_WildcardAllowedOriginsAllowsAny(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.allowedOrigins = []string{"*"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://any-random-origin.io")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got == "" {
		t.Error("wildcard allowedOrigins: expected ACAO header to be set for any origin")
	}
}

func TestCORS_EmptyAllowedOriginsAllowsAll(t *testing.T) {
	// newTestServer passes nil allowedOrigins → open CORS (returns "*").
	srv, _ := newTestServer(t)
	// allowedOrigins is nil/empty by default in newTestServer.

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "*" {
		t.Errorf("empty allowedOrigins: want ACAO=*, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// requireSession — various states
// ---------------------------------------------------------------------------

func TestRequireSession_NilSessionManager_Passthrough(t *testing.T) {
	// When the session manager is nil and user auth is disabled, the endpoint
	// must be accessible without any credentials.
	srv, _ := newTestServer(t)
	srv.sessionManager = nil // force nil manager

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// The me handler itself will 401 because there is no cookie, but the
	// middleware layer (requireSession) must not intercept. The 401 here
	// comes from handleMe — not from the "unauthorized" middleware response.
	// We distinguish by checking the error message.
	if w.Code == http.StatusUnauthorized {
		body := w.Body.String()
		// "unauthorized" is the middleware message; "not authenticated" comes from handleMe.
		if strings.Contains(body, `"unauthorized"`) {
			t.Error("requireSession middleware blocked the request even though sessionManager is nil")
		}
	}
}

func TestRequireSession_ExpiredToken_Returns401(t *testing.T) {
	srv, _ := newAuthedServer(t)

	// Craft a cookie with an obviously invalid/expired token value.
	badCookie := &http.Cookie{
		Name:  "funnelbarn_session",
		Value: "this.is.not.a.valid.token",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(badCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: want 401, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestRequireSession_NoCookie_Returns401(t *testing.T) {
	srv, _ := newAuthedServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: want 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// CORS preflight — OPTIONS
// ---------------------------------------------------------------------------

func TestCORSPreflight_EventsEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS /api/v1/events: want 204, got %d", w.Code)
	}
	if len(w.Body.Bytes()) != 0 {
		t.Errorf("OPTIONS response should have no body, got %q", w.Body.String())
	}
}

func TestCORSPreflight_AllowMethodsHeaderPresent(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS: want 204, got %d", w.Code)
	}
	if am := w.Header().Get("Access-Control-Allow-Methods"); am == "" {
		t.Error("OPTIONS: Access-Control-Allow-Methods header missing")
	}
	if ah := w.Header().Get("Access-Control-Allow-Headers"); ah == "" {
		t.Error("OPTIONS: Access-Control-Allow-Headers header missing")
	}
}
