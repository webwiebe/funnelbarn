package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func TestHandleSetup_ShowsPlaceholderInsteadOfAKey(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("setup: want 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "my-new-site") {
		t.Error("setup response does not contain the slug 'my-new-site'")
	}
	if !strings.Contains(body, setupKeyPlaceholder) {
		t.Errorf("setup response does not show the key placeholder %q", setupKeyPlaceholder)
	}
	if k := regexp.MustCompile(`\b[0-9a-f]{40}\b`).FindString(body); k != "" {
		t.Errorf("setup response contains something shaped like an API key: %q", k)
	}
}

// The guide used to upsert a key derived from the slug on every fetch, so a
// revoked setup key came back as soon as anyone opened the guide again.
func TestHandleSetup_CreatesNoAPIKey(t *testing.T) {
	srv, store := newTestServer(t)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("setup call %d: want 200, got %d", i+1, w.Code)
		}
	}

	keys, err := store.ListAllAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAllAPIKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("setup guide created %d API key(s), want none", len(keys))
	}
}

// Anyone could fetch a working ingest key for any slug while the guide was
// public. It now needs a dashboard session.
func TestHandleSetup_RequiresSession(t *testing.T) {
	srv, _ := newAuthedServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: want 401, got %d (body: %s)", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/setup/my-new-site", nil)
	req.AddCookie(sessionCookieFor(t, srv, "admin"))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("with session: want 200, got %d (body: %s)", w.Code, w.Body.String())
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

func TestCORS_EmptyAllowedOriginsDeniesDashboard(t *testing.T) {
	// CORS-1: an empty allowlist must mean "same-origin only" for the
	// credentialed dashboard API, NOT "reflect any origin". A dashboard route
	// (here /api/v1/health, a non-ingest path) must get no CORS headers.
	srv, _ := newTestServer(t)
	// allowedOrigins is nil/empty by default in newTestServer.

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("empty allowedOrigins on dashboard route: want no ACAO header, got %q", got)
	}
}

// TestCORS_IngestEndpointOpenNoCredentials verifies CORS-1: the public ingest
// endpoints allow any origin (browser SDKs live on arbitrary customer sites)
// but must NOT set Allow-Credentials — they authenticate with an API-key
// header, never cookies.
func TestCORS_IngestEndpointOpenNoCredentials(t *testing.T) {
	srv, _ := newTestServer(t)
	// No allowlist configured — ingest must still be reachable cross-origin.

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://any-customer-site.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://any-customer-site.example" {
		t.Errorf("ingest ACAO: want reflected origin, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("ingest must not allow credentials, got %q", got)
	}
}

// TestCORS_IngestReflectsRequestedHeaders is the regression guard for the
// custom-CNAME ingest CORS break: a browser SDK preflights /api/v1/events with
// headers we don't control (notably `traceparent` from distributed tracing),
// and the non-credentialed ingest path must echo them back in
// Access-Control-Allow-Headers, or the browser blocks the real request. A fixed
// allowlist that omits `traceparent` regressed this.
func TestCORS_IngestReflectsRequestedHeaders(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://brandtrace.net")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type,traceparent,x-funnelbarn-api-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	allow := w.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(strings.ToLower(allow), "traceparent") {
		t.Errorf("ingest preflight must echo requested headers incl. traceparent, got %q", allow)
	}
	// Still non-credentialed.
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("ingest must not allow credentials, got %q", got)
	}
}

// TestCORS_DashboardDoesNotReflectRequestedHeaders verifies the credentialed
// dashboard API keeps the strict fixed allowlist (never reflects arbitrary
// requested headers) — the security posture CORS-1 established.
func TestCORS_DashboardDoesNotReflectRequestedHeaders(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.allowedOrigins = []string{"https://dash.example"}

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/projects", nil)
	req.Header.Set("Origin", "https://dash.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "x-evil-injected-header")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	allow := w.Header().Get("Access-Control-Allow-Headers")
	if strings.Contains(strings.ToLower(allow), "x-evil-injected-header") {
		t.Errorf("dashboard CORS must not reflect arbitrary requested headers, got %q", allow)
	}
	if allow != corsAllowHeaders {
		t.Errorf("dashboard Allow-Headers: want fixed allowlist %q, got %q", corsAllowHeaders, allow)
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
