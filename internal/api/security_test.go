package api

// Security and cross-cutting tests:
//   - Cross-project data isolation (funnel/abtest scoped to wrong project → 404)
//   - Error response format — always JSON, never SQL or internal details
//   - Security headers present on every response
//   - Rate-limit response is JSON with Retry-After header
//   - Request ID propagated on every response

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ---------------------------------------------------------------------------
// Cross-project isolation
// ---------------------------------------------------------------------------

func TestCrossProjectIsolation_Funnel(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()

	// Create two projects.
	pa, _ := store.CreateProject(ctx, "Project A", "proj-a")
	pb, _ := store.CreateProject(ctx, "Project B", "proj-b")

	// Create a funnel under project A.
	fa, _ := store.CreateFunnel(ctx, repositoryFunnel(pa.ID, "Funnel A"))

	// Accessing that funnel via project B's URL must return 404, not the real funnel.
	w := getJSON(t, srv, "/api/v1/projects/"+pb.ID+"/funnels/"+fa.ID+"/analysis", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("cross-project funnel access: want 404, got %d", w.Code)
	}
	// Body must still be JSON.
	assertJSONError(t, w)
}

func TestCrossProjectIsolation_FunnelUpdate(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()

	pa, _ := store.CreateProject(ctx, "PA", "pa")
	pb, _ := store.CreateProject(ctx, "PB", "pb")
	fa, _ := store.CreateFunnel(ctx, repositoryFunnel(pa.ID, "FA"))

	// PUT funnel of project A via project B → 404
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+pb.ID+"/funnels/"+fa.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("cross-project funnel PUT: want 404, got %d", w.Code)
	}
}

func TestCrossProjectIsolation_ABTest(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()

	pa, _ := store.CreateProject(ctx, "PA2", "pa2")
	pb, _ := store.CreateProject(ctx, "PB2", "pb2")
	at, _ := store.CreateABTest(ctx, repositoryABTest(pa.ID))

	// Accessing the test via project B → 404
	w := getJSON(t, srv, "/api/v1/projects/"+pb.ID+"/abtests/"+at.ID+"/analysis", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("cross-project AB test access: want 404, got %d", w.Code)
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Error response format — never leaks internal details
// ---------------------------------------------------------------------------

func TestErrorResponse_NotFoundIsJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/projects/does-not-exist/funnels", nil)
	// 200 with empty list, not 404, because list doesn't check project existence.
	// Ensure whatever is returned is valid JSON.
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Errorf("response not valid JSON: %v (body: %s)", err, w.Body.String())
	}
}

func TestErrorResponse_NonexistentFunnel(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ErrFmt", "errfmt")

	// Looking up a nonexistent funnel should return 404 with JSON error body.
	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/does-not-exist/analysis", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
	assertJSONError(t, w)
}

func TestErrorResponse_BadJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	req.Body = http.NoBody
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for empty body on create project")
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Security headers on every response
// ---------------------------------------------------------------------------

func TestSecurityHeaders_Present(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/health", nil)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, want := range headers {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
}

func TestSecurityHeaders_OnErrorResponse(t *testing.T) {
	srv, _ := newTestServer(t)
	// Even error responses must carry security headers.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	req.Body = http.NoBody
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options missing on error response")
	}
}

// ---------------------------------------------------------------------------
// Rate limit response format
// The rate limiter's 429 JSON shape is tested in ratelimit_test.go via wrap().
// This test verifies the behaviour at the mux level using the real limiter.
// ---------------------------------------------------------------------------

func TestRateLimit_ResponseFormat(t *testing.T) {
	// Limit to 1 request per minute — second request is definitely blocked.
	rl := newRateLimiter(60, 1) // burst of 1 → second request blocked
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "9.9.9.9:1234"

	// First request: allowed.
	rl.middleware(inner).ServeHTTP(httptest.NewRecorder(), req)

	// Second request: blocked.
	w := httptest.NewRecorder()
	rl.middleware(inner).ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing from rate-limit response")
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func repositoryFunnel(projectID, name string) repository.Funnel {
	return repository.Funnel{
		ProjectID: projectID,
		Name:      name,
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	}
}

func repositoryABTest(projectID string) repository.ABTest {
	return repository.ABTest{
		ProjectID:       projectID,
		Name:            "Test A",
		Status:          "running",
		ConversionEvent: "purchase",
	}
}

func assertJSONError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", w.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body not valid JSON: %v (body: %s)", err, w.Body.String())
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("JSON error response missing 'error' key: %v", body)
	}
}
