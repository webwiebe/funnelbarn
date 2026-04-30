package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func openMemoryStore(t *testing.T) *repository.Store {
	t.Helper()
	s, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("repository.Open(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestSpool(t *testing.T) *spool.Spool {
	t.Helper()
	dir, err := os.MkdirTemp("", "api-spool-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("spool.New: %v", err)
	}
	t.Cleanup(func() { sp.Close() })
	return sp
}

// newTestServer creates an API server with no auth (open access) for most tests.
func newTestServer(t *testing.T) (*Server, *repository.Store) {
	t.Helper()
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ingestHandler := ingest.NewHandler(auth.New("test-key"), sp, 0)
	sm := auth.NewSessionManager("test-secret", time.Hour)

	// Use disabled user auth so requireSession lets all requests through.
	userAuth, _ := auth.NewUserAuthenticator("", "", "")

	srv := NewServer(ingestHandler,
		service.NewProjectService(store), service.NewFunnelService(store),
		service.NewABTestService(store), service.NewEventService(store),
		service.NewSessionService(store), service.NewAPIKeyService(store),
		userAuth, sm, nil, "test-secret", "http://localhost", 1000, 1000, 1000, 1000, 1000, 1000, store, "test")
	return srv, store
}

// newAuthedServer creates a server with user auth enabled.
func newAuthedServer(t *testing.T) (*Server, *repository.Store) {
	t.Helper()
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ingestHandler := ingest.NewHandler(auth.New("test-key"), sp, 0)
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("admin", "password", "")

	srv := NewServer(ingestHandler,
		service.NewProjectService(store), service.NewFunnelService(store),
		service.NewABTestService(store), service.NewEventService(store),
		service.NewSessionService(store), service.NewAPIKeyService(store),
		userAuth, sm, nil, "test-secret", "http://localhost", 1000, 1000, 1000, 1000, 1000, 1000, store, "test")
	return srv, store
}

// sessionCookieFor creates a valid session cookie for the given server.
func sessionCookieFor(t *testing.T, srv *Server, username string) *http.Cookie {
	t.Helper()
	token, expires, err := srv.sessionManager.Create(username)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	return auth.SessionCookie(token, expires, false)
}

func postJSON(t *testing.T, srv *Server, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func getJSON(t *testing.T, srv *Server, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func deleteReq(t *testing.T, srv *Server, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/health", nil)
	if w.Code != http.StatusOK {
		t.Errorf("health: expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("health status: want ok, got %v", resp["status"])
	}
}

// failPinger is a Pinger that always returns an error.
type failPinger struct{ err error }

func (f *failPinger) Ping(_ context.Context) error { return f.err }

func TestHandleHealth_DBDown(t *testing.T) {
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ingestHandler := ingest.NewHandler(auth.New("test-key"), sp, 0)
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("", "", "")

	brokenPinger := &failPinger{err: errors.New("connection refused")}

	srv := NewServer(ingestHandler,
		service.NewProjectService(store), service.NewFunnelService(store),
		service.NewABTestService(store), service.NewEventService(store),
		service.NewSessionService(store), service.NewAPIKeyService(store),
		userAuth, sm, nil, "test-secret", "http://localhost", 1000, 1000, 1000, 1000, 1000, 1000, brokenPinger, "test")

	w := getJSON(t, srv, "/api/v1/health", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("health DB down: expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "unhealthy" {
		t.Errorf("health status: want unhealthy, got %v", resp["status"])
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

func TestCORSOptions(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: expected 204, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Login / Logout
// ---------------------------------------------------------------------------

func TestHandleLogin_Success(t *testing.T) {
	srv, _ := newAuthedServer(t)

	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "admin",
		"password": "password",
	}, nil)

	if w.Code != http.StatusOK {
		t.Errorf("login: expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	srv, _ := newAuthedServer(t)

	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "admin",
		"password": "wrong",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("bad login: expected 401, got %d", w.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("logout: expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

func TestHandleCreateProject(t *testing.T) {
	srv, _ := newTestServer(t)

	w := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name": "My Site",
		"slug": "my-site",
	}, nil)

	if w.Code != http.StatusCreated {
		t.Errorf("create project: expected 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var p repository.Project
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.Name != "My Site" {
		t.Errorf("Name: want My Site, got %q", p.Name)
	}
}

func TestHandleCreateProject_MissingName(t *testing.T) {
	srv, _ := newTestServer(t)
	// Empty name should now return 422 Unprocessable Entity (service-layer validation).
	w := postJSON(t, srv, "/api/v1/projects", map[string]string{"slug": "test"}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleListProjects(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, "Site A", "site-a")
	_, _ = store.CreateProject(ctx, "Site B", "site-b")

	w := getJSON(t, srv, "/api/v1/projects", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	projects, _ := resp["projects"].([]any)
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestHandleDeleteProject(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ToDelete", "to-delete")

	w := deleteReq(t, srv, "/api/v1/projects/"+p.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleUpdateProject(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "Old", "old")

	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+p.ID, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("update project: expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

func TestHandleListAPIKeys(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/apikeys", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleCreateAndDeleteAPIKey(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "KeySite", "keysite")

	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": p.ID,
		"name":       "my-key",
		"scope":      "ingest",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Errorf("create api key: expected 201, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	apiKey, _ := resp["api_key"].(map[string]any)
	keyID, _ := apiKey["id"].(string)
	if keyID == "" {
		t.Fatal("expected non-empty key ID in response")
	}

	// Delete the key.
	wd := deleteReq(t, srv, "/api/v1/apikeys/"+keyID, nil)
	if wd.Code != http.StatusNoContent {
		t.Errorf("delete api key: expected 204, got %d", wd.Code)
	}
}

func TestHandleCreateAPIKey_InvalidScope(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ScopeSite", "scope-site")

	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": p.ID,
		"name":       "bad-scope-key",
		"scope":      "superadmin",
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid scope, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// requireSession middleware
// ---------------------------------------------------------------------------

func TestRequireSession_Blocked(t *testing.T) {
	srv, _ := newAuthedServer(t)

	// Without a session cookie, should get 401.
	w := getJSON(t, srv, "/api/v1/me", nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without session, got %d", w.Code)
	}
}

func TestRequireSession_Allowed(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie := sessionCookieFor(t, srv, "admin")

	w := getJSON(t, srv, "/api/v1/me", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid session, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// toSlug helper
// ---------------------------------------------------------------------------

func TestToSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Site", "my-site"},
		{"Hello, World!", "hello-world"},
		{"example.com", "example-com"},
		{"  spaces  ", "spaces"},
		{"ALL CAPS", "all-caps"},
		{"a--b", "a-b"},
	}

	for _, tc := range tests {
		got := toSlug(tc.input)
		if got != tc.want {
			t.Errorf("toSlug(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Sessions endpoints
// ---------------------------------------------------------------------------

func TestHandleListSessions(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "SessListSite", "sess-list-site")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/sessions", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Events endpoint
// ---------------------------------------------------------------------------

func TestHandleListEvents(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "EvtListSite", "evt-list-site")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/events", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Funnel endpoints
// ---------------------------------------------------------------------------

func TestHandleFunnelCRUD(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FunnelSite", "funnel-site")

	// List (empty).
	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list funnels: expected 200, got %d", w.Code)
	}

	// Create.
	wc := postJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels", map[string]any{
		"name": "My Funnel",
		"steps": []map[string]string{
			{"event_name": "signup"},
			{"event_name": "payment"},
		},
	}, nil)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create funnel: expected 201, got %d (body: %s)", wc.Code, wc.Body.String())
	}

	var created repository.Funnel
	_ = json.Unmarshal(wc.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("expected funnel ID in response")
	}

	// Delete.
	wd := deleteReq(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+created.ID, nil)
	if wd.Code != http.StatusNoContent {
		t.Errorf("delete funnel: expected 204, got %d", wd.Code)
	}
}

// ---------------------------------------------------------------------------
// Approve project
// ---------------------------------------------------------------------------

func TestHandleApproveProject(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.EnsureProjectPending(ctx, "Pending", "pending-slug")

	w := postJSON(t, srv, "/api/v1/projects/"+p.ID+"/approve", nil, nil)
	if w.Code != http.StatusOK {
		t.Errorf("approve project: expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var updated repository.Project
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Status != "active" {
		t.Errorf("expected status=active, got %q", updated.Status)
	}
}

// ---------------------------------------------------------------------------
// jsonError / writeJSON helpers (indirect tests)
// ---------------------------------------------------------------------------

func TestJSONError_Format(t *testing.T) {
	w := httptest.NewRecorder()
	jsonError(w, "something went wrong", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 422, got %d", w.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "something went wrong" {
		t.Errorf("error message: want 'something went wrong', got %q", resp["error"])
	}
}


// ---------------------------------------------------------------------------
// Domain error mapping
// ---------------------------------------------------------------------------

func TestHandleCreateProject_EmptyName_Returns422(t *testing.T) {
	srv, _ := newTestServer(t)
	// Sending an empty name should return 422 via service-layer validation.
	w := postJSON(t, srv, "/api/v1/projects", map[string]string{"name": "", "slug": "my-slug"}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty name, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleCreateProject_DuplicateSlug_Returns409(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create the first project successfully.
	w1 := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name": "First Project",
		"slug": "dup-slug",
	}, nil)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d (body: %s)", w1.Code, w1.Body.String())
	}

	// Creating with the same slug should return 409 Conflict.
	w2 := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name": "Second Project",
		"slug": "dup-slug",
	}, nil)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate slug: expected 409, got %d (body: %s)", w2.Code, w2.Body.String())
	}
}

func TestHandleGetProject_UnknownID_Returns404(t *testing.T) {
	srv, _ := newTestServer(t)
	// Use a non-existent project ID in a project-scoped route.
	w := getJSON(t, srv, "/api/v1/projects/nonexistent-id/sessions", nil)
	// The endpoint returns 200 with empty sessions (it doesn't look up the project).
	// Use the funnels endpoint which calls GetFunnel internally on a specific funnel ID.
	// Instead, test via the approve endpoint which calls GetProject.
	_ = w

	// POST approve on a non-existent project ID.
	wa := postJSON(t, srv, "/api/v1/projects/nonexistent-id/approve", nil, nil)
	if wa.Code != http.StatusNotFound {
		t.Errorf("unknown project approve: expected 404, got %d (body: %s)", wa.Code, wa.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Active session count
// ---------------------------------------------------------------------------

func TestHandleActiveSessionCount(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ActiveSite", "active-site")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/sessions/active", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}
