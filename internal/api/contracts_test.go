package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ---------------------------------------------------------------------------
// Additional test helpers
// ---------------------------------------------------------------------------

func putJSON(t *testing.T, srv *Server, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPut, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status: want %d, got %d (body: %s)", want, w.Code, w.Body.String())
	}
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, w.Body.String())
	}
}

func assert(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Error(msg)
	}
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

func TestProjectCRUD(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create
	w := postJSON(t, srv, "/api/v1/projects", map[string]string{"name": "CRUD Test", "slug": "crud-test"}, nil)
	assertStatus(t, w, 201)
	var proj repository.Project
	decodeJSON(t, w, &proj)
	assert(t, proj.ID != "", "project has ID")
	assert(t, proj.Name == "CRUD Test", "project name correct")

	// List
	w = getJSON(t, srv, "/api/v1/projects", nil)
	assertStatus(t, w, 200)
	var listResp map[string]any
	json.NewDecoder(w.Body).Decode(&listResp) //nolint:errcheck
	projects, _ := listResp["projects"].([]any)
	assert(t, len(projects) >= 1, "at least one project in list")

	// Update
	w = putJSON(t, srv, "/api/v1/projects/"+proj.ID, map[string]string{"name": "Updated"}, nil)
	assertStatus(t, w, 200)
	var updated repository.Project
	decodeJSON(t, w, &updated)
	assert(t, updated.Name == "Updated", "project name updated")

	// Delete
	w = deleteReq(t, srv, "/api/v1/projects/"+proj.ID, nil)
	assertStatus(t, w, 204)

	// After delete → approve returns 404 (confirms project is gone)
	w = postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/approve", nil, nil)
	assertStatus(t, w, 404)
}

// ---------------------------------------------------------------------------
// Funnels
// ---------------------------------------------------------------------------

func TestFunnelCRUD(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "Funnel Test", "funnel-test")

	// Create funnel
	body := map[string]any{
		"name": "Onboarding",
		"steps": []map[string]string{
			{"event_name": "signup"},
			{"event_name": "verified"},
		},
	}
	w := postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels", body, nil)
	assertStatus(t, w, 201)
	var f repository.Funnel
	decodeJSON(t, w, &f)
	assert(t, f.ID != "", "funnel has ID")
	assert(t, f.Name == "Onboarding", "funnel name correct")
	assert(t, len(f.Steps) == 2, "funnel has 2 steps")

	// List
	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels", nil)
	assertStatus(t, w, 200)
	var listResp map[string]any
	json.NewDecoder(w.Body).Decode(&listResp) //nolint:errcheck
	funnels, _ := listResp["funnels"].([]any)
	assert(t, len(funnels) == 1, "one funnel in list")

	// Analysis (empty data → 200 with results)
	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels/"+f.ID+"/analysis", nil)
	assertStatus(t, w, 200)
	var analysisResp map[string]any
	json.NewDecoder(w.Body).Decode(&analysisResp) //nolint:errcheck
	assert(t, analysisResp["funnel"] != nil, "analysis response has funnel field")

	// Update
	body["name"] = "Onboarding v2"
	w = putJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels/"+f.ID, body, nil)
	assertStatus(t, w, 200)
	var updatedFunnel repository.Funnel
	decodeJSON(t, w, &updatedFunnel)
	assert(t, updatedFunnel.Name == "Onboarding v2", "funnel name updated")

	// Delete
	w = deleteReq(t, srv, "/api/v1/projects/"+proj.ID+"/funnels/"+f.ID, nil)
	assertStatus(t, w, 204)

	// After delete → get analysis returns 404
	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels/"+f.ID+"/analysis", nil)
	assertStatus(t, w, 404)
}

// TestFunnelWrongProject verifies that funnel endpoints respect project ownership.
func TestFunnelWrongProject(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj1, _ := store.CreateProject(ctx, "Proj1", "proj1")
	proj2, _ := store.CreateProject(ctx, "Proj2", "proj2")

	// Create a funnel for proj1
	w := postJSON(t, srv, "/api/v1/projects/"+proj1.ID+"/funnels", map[string]any{
		"name":  "Funnel 1",
		"steps": []map[string]string{{"event_name": "click"}},
	}, nil)
	assertStatus(t, w, 201)
	var f repository.Funnel
	decodeJSON(t, w, &f)

	// Accessing it via proj2 should return 404
	w = getJSON(t, srv, "/api/v1/projects/"+proj2.ID+"/funnels/"+f.ID+"/analysis", nil)
	assertStatus(t, w, 404)
}

// ---------------------------------------------------------------------------
// AB Tests
// ---------------------------------------------------------------------------

func TestABTestCreate(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "ABTest", "abtest-proj")

	body := map[string]any{
		"name":             "Button color",
		"conversion_event": "purchase",
		"control_filter":   "",
		"variant_filter":   "",
	}
	w := postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/abtests", body, nil)
	assertStatus(t, w, 201)
	var test repository.ABTest
	decodeJSON(t, w, &test)
	assert(t, test.ID != "", "ab test has ID")
	assert(t, test.Name == "Button color", "ab test name correct")

	// List
	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/abtests", nil)
	assertStatus(t, w, 200)
	var listResp map[string]any
	json.NewDecoder(w.Body).Decode(&listResp) //nolint:errcheck
	tests, _ := listResp["tests"].([]any)
	assert(t, len(tests) == 1, "one ab test in list")

	// Analysis (empty data → 200)
	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/abtests/"+test.ID+"/analysis", nil)
	assertStatus(t, w, 200)
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

func TestAPIKeyCRUD(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "Keys Project", "keys-proj")

	// Create
	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": proj.ID, "name": "Test Key", "scope": "full",
	}, nil)
	assertStatus(t, w, 201)

	// The create response wraps the key: {"api_key": {...}, "key": "..."}
	var createResp map[string]any
	decodeJSON(t, w, &createResp)
	apiKeyObj, _ := createResp["api_key"].(map[string]any)
	assert(t, apiKeyObj != nil, "response has api_key field")
	keyID, _ := apiKeyObj["id"].(string)
	assert(t, keyID != "", "key has ID")
	// key_hash must not appear in the response
	body := w.Body.String()
	assert(t, !containsStr(body, "key_hash"), "key_hash not in create response")

	// Plaintext key is present once
	plaintextKey, _ := createResp["key"].(string)
	assert(t, plaintextKey != "", "plaintext key returned on create")

	// List
	w = getJSON(t, srv, "/api/v1/apikeys?project_id="+proj.ID, nil)
	assertStatus(t, w, 200)
	var listResp map[string]any
	json.NewDecoder(w.Body).Decode(&listResp) //nolint:errcheck
	apiKeys, _ := listResp["api_keys"].([]any)
	assert(t, len(apiKeys) == 1, "one api key in list")

	// List response must not contain key_hash
	listBody := w.Body.String()
	assert(t, !containsStr(listBody, "key_hash"), "key_hash not in list response")

	// Delete
	w = deleteReq(t, srv, "/api/v1/apikeys/"+keyID, nil)
	assertStatus(t, w, 204)

	// After delete → list is empty
	w = getJSON(t, srv, "/api/v1/apikeys?project_id="+proj.ID, nil)
	assertStatus(t, w, 200)
	var afterResp map[string]any
	json.NewDecoder(w.Body).Decode(&afterResp) //nolint:errcheck
	afterKeys, _ := afterResp["api_keys"].([]any)
	assert(t, len(afterKeys) == 0, "no api keys after delete")
}

// containsStr is a helper to check for substring without importing strings in test assertions.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func TestSessionsEndpoints(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "Sess", "sess-proj")

	w := getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/sessions", nil)
	assertStatus(t, w, 200)

	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/sessions/active", nil)
	assertStatus(t, w, 200)
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func TestDashboardEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "Dash", "dash-proj")

	w := getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/dashboard", nil)
	assertStatus(t, w, 200)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	assert(t, resp["project_id"] != nil, "dashboard has project_id")
	assert(t, resp["total_events"] != nil, "dashboard has total_events")
	assert(t, resp["unique_sessions"] != nil, "dashboard has unique_sessions")
}

// ---------------------------------------------------------------------------
// Validation errors
// ---------------------------------------------------------------------------

func TestValidationErrors(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "Val", "val-proj")

	// Empty funnel name → 422 (service-layer validation)
	w := postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels", map[string]any{
		"name": "", "steps": []map[string]string{{"event_name": "click"}},
	}, nil)
	assertStatus(t, w, 422)
	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp) //nolint:errcheck
	assert(t, errResp["error"] != "", "422 response has error field")

	// Funnel with no steps → 422 (service-layer validation)
	w = postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels", map[string]any{
		"name": "NoSteps", "steps": []any{},
	}, nil)
	assertStatus(t, w, 422)

	// Invalid API key scope → 422 (handler-layer validation)
	w = postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": proj.ID, "name": "k", "scope": "invalid-scope",
	}, nil)
	assertStatus(t, w, 422)
}

// TestProjectValidationErrors covers project-level input validation.
func TestProjectValidationErrors(t *testing.T) {
	srv, _ := newTestServer(t)

	// Empty name → 422
	w := postJSON(t, srv, "/api/v1/projects", map[string]string{"name": "", "slug": "some-slug"}, nil)
	assertStatus(t, w, 422)

	// Missing name field entirely → 422
	w = postJSON(t, srv, "/api/v1/projects", map[string]string{"slug": "only-slug"}, nil)
	assertStatus(t, w, 422)
}

// ---------------------------------------------------------------------------
// AB Test analysis — unknown test ID returns error (not 2xx)
// ---------------------------------------------------------------------------

func TestABTestAnalysis_UnknownID_ReturnsError(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "ABProj", "ab-proj-404")

	w := getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/abtests/nonexistent-abtest-id/analysis", nil)
	// The handler returns a non-2xx code when the test is not found.
	assert(t, w.Code >= 400, "unknown ab test analysis returns error status")
}

// ---------------------------------------------------------------------------
// Funnel segments endpoint
// ---------------------------------------------------------------------------

func TestFunnelSegmentsEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "SegProj", "seg-proj")

	// Create a funnel first
	w := postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels", map[string]any{
		"name":  "SegFunnel",
		"steps": []map[string]string{{"event_name": "page_view"}},
	}, nil)
	assertStatus(t, w, 201)
	var f repository.Funnel
	decodeJSON(t, w, &f)

	// Segments endpoint returns 200
	w = getJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels/"+f.ID+"/segments", nil)
	assertStatus(t, w, 200)
}

// ---------------------------------------------------------------------------
// Response shape contracts
// ---------------------------------------------------------------------------

// TestProjectResponseShape verifies that create and list endpoints return
// the documented fields.
func TestProjectResponseShape(t *testing.T) {
	srv, _ := newTestServer(t)

	w := postJSON(t, srv, "/api/v1/projects", map[string]string{"name": "Shape Test", "slug": "shape-test"}, nil)
	assertStatus(t, w, 201)

	var proj map[string]any
	decodeJSON(t, w, &proj)
	for _, field := range []string{"id", "name", "slug", "status", "created_at"} {
		assert(t, proj[field] != nil, "project response has field: "+field)
	}
	assert(t, proj["id"].(string) != "", "id is non-empty string")
	assert(t, proj["name"].(string) == "Shape Test", "name matches")
}

// TestFunnelResponseShape verifies that the funnel create response includes
// all expected fields including steps.
func TestFunnelResponseShape(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "ShapeProj", "shape-proj")

	w := postJSON(t, srv, "/api/v1/projects/"+proj.ID+"/funnels", map[string]any{
		"name": "Shape Funnel",
		"steps": []map[string]string{
			{"event_name": "start"},
			{"event_name": "end"},
		},
	}, nil)
	assertStatus(t, w, 201)

	var f map[string]any
	decodeJSON(t, w, &f)
	for _, field := range []string{"id", "project_id", "name", "steps", "created_at"} {
		assert(t, f[field] != nil, "funnel response has field: "+field)
	}
	steps, _ := f["steps"].([]any)
	assert(t, len(steps) == 2, "funnel response has 2 steps")
}

// TestAPIKeyResponseShape verifies that create response includes the plaintext
// key and api_key object, and that list response uses the safe shape.
func TestAPIKeyResponseShape(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	proj, _ := store.CreateProject(ctx, "KeyShape", "key-shape")

	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": proj.ID,
		"name":       "shape-key",
		"scope":      "ingest",
	}, nil)
	assertStatus(t, w, 201)

	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert(t, resp["key"] != nil, "create response has plaintext key field")
	assert(t, resp["api_key"] != nil, "create response has api_key object")

	apiKey, _ := resp["api_key"].(map[string]any)
	for _, field := range []string{"id", "name", "scope", "created_at"} {
		assert(t, apiKey[field] != nil, "api_key object has field: "+field)
	}

	// List uses safeKey shape: id, name, scope, created_at — no project_id or key_hash
	w = getJSON(t, srv, "/api/v1/apikeys?project_id="+proj.ID, nil)
	assertStatus(t, w, 200)
	var listResp map[string]any
	json.NewDecoder(w.Body).Decode(&listResp) //nolint:errcheck
	keys, _ := listResp["api_keys"].([]any)
	assert(t, len(keys) == 1, "one key in list")
	keyObj, _ := keys[0].(map[string]any)
	for _, field := range []string{"id", "name", "scope", "created_at"} {
		assert(t, keyObj[field] != nil, "list api_key has field: "+field)
	}
}

// TestErrorResponseShape verifies that error responses always have an "error" field.
func TestErrorResponseShape(t *testing.T) {
	srv, _ := newTestServer(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"missing project name", "POST", "/api/v1/projects", map[string]string{"slug": "x"}},
		{"nonexistent approve", "POST", "/api/v1/projects/no-such-id/approve", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.body != nil {
				w = postJSON(t, srv, tc.path, tc.body, nil)
			} else {
				req := httptest.NewRequest(tc.method, tc.path, nil)
				rec := httptest.NewRecorder()
				srv.ServeHTTP(rec, req)
				w = rec
			}
			var errResp map[string]string
			json.NewDecoder(w.Body).Decode(&errResp) //nolint:errcheck
			assert(t, errResp["error"] != "", "error response has non-empty error field")
		})
	}
}
