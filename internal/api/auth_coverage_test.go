package api

// Additional coverage for api handlers:
// - 404 paths (nonexistent resource IDs)
// - handleLogin via DB fallback (when env-var auth is disabled)
// - handleCreateProject slug-from-domain
// - handleCreateAPIKey project_id requirement and binding
// - handleListAPIKeys with ?project_id= filter
// - funnel analysis with all segment types (exercises segmentClause + escapeSQLLiteral)

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// 404 / not-found paths
// ---------------------------------------------------------------------------

func TestHandleUpdateProject_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	w := putJSON(t, srv, "/api/v1/projects/does-not-exist", map[string]string{"name": "X"}, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for nonexistent project, got %d", w.Code)
	}
}

func TestHandleApproveProject_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	w := postJSON(t, srv, "/api/v1/projects/does-not-exist/approve", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestHandleDeleteFunnel_NotFound(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "DFNotFound", "dfnotfound")

	w := deleteReq(t, srv, "/api/v1/projects/"+p.ID+"/funnels/does-not-exist", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for nonexistent funnel, got %d", w.Code)
	}
}

func TestHandleUpdateFunnel_NotFound(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "UFNotFound", "ufnotfound")

	w := putJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/does-not-exist",
		map[string]any{"name": "X", "steps": []map[string]string{{"event_name": "ev"}}}, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestHandleFunnelSegments_NotFound(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FSNotFound", "fsnotfound")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/does-not-exist/segments", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestHandleFunnelAnalysis_NotFound(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FANotFound", "fanotfound")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/does-not-exist/analysis", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleLogin — DB fallback (env-var auth disabled → uses DB users)
// ---------------------------------------------------------------------------

func TestHandleLogin_DBFallback_Success(t *testing.T) {
	srv, store := newTestServer(t) // disabled env-var auth
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if err := store.UpsertUser(ctx, "dbuser", string(hash)); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "dbuser",
		"password": "password",
	}, nil)
	if w.Code != http.StatusOK {
		t.Errorf("DB login: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleLogin_DBFallback_WrongPassword(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	if err := store.UpsertUser(ctx, "dbuser2", string(hash)); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "dbuser2",
		"password": "wrong",
	}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: want 401, got %d", w.Code)
	}
}

func TestHandleLogin_DBFallback_UnknownUser(t *testing.T) {
	srv, _ := newTestServer(t) // no users in DB

	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "nobody",
		"password": "anything",
	}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no DB user: want 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleCreateProject — slug derived from domain field
// ---------------------------------------------------------------------------

func TestHandleCreateProject_SlugFromDomain(t *testing.T) {
	srv, _ := newTestServer(t)
	w := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name":   "My Site",
		"domain": "my-site.example.com",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Errorf("slug from domain: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleCreateAPIKey — project_id is required
// ---------------------------------------------------------------------------

// A key with no named project used to be bound to whichever project sorted
// first, so a caller that forgot one walked away with a credential for the
// wrong project that looked entirely healthy until it 403'd.
func TestHandleCreateAPIKey_RequiresProject(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, "AutoPick", "autopick")

	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"name":  "auto-key",
		"scope": "ingest",
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing project_id: want 400, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// The project named in the request is the project the key is bound to — not
// the first one on the instance. This is the regression the dashboard hit:
// a key minted while a second project was selected came back scoped to the
// first.
func TestHandleCreateAPIKey_BindsToTheNamedProject(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, "First", "first")
	second, err := store.CreateProject(ctx, "Second", "second")
	if err != nil {
		t.Fatalf("create second project: %v", err)
	}

	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"project_id": second.ID,
		"name":       "second-key",
		"scope":      "analytics:read",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp struct {
		APIKey struct {
			ProjectID string `json:"project_id"`
		} `json:"api_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.APIKey.ProjectID != second.ID {
		t.Errorf("bound project: want %s (Second), got %s", second.ID, resp.APIKey.ProjectID)
	}
}

// ---------------------------------------------------------------------------
// handleListAPIKeys — ?project_id= filter
// ---------------------------------------------------------------------------

func TestHandleListAPIKeys_ProjectFilter(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FilterSite", "filtersite")

	postJSON(t, srv, "/api/v1/apikeys", map[string]string{
		"name": "fk", "scope": "ingest", "project_id": p.ID,
	}, nil)

	w := getJSON(t, srv, "/api/v1/apikeys?project_id="+p.ID, nil)
	if w.Code != http.StatusOK {
		t.Errorf("list by project: want 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Funnel analysis with all segment types — exercises segmentClause + escapeSQLLiteral
// ---------------------------------------------------------------------------

func TestHandleFunnelAnalysis_AllSegments(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "AllSegs", "allsegs")
	f, _ := store.CreateFunnel(ctx, repositoryFunnel(p.ID, "Seg Funnel"))

	for _, seg := range []string{"logged_in", "not_logged_in", "mobile", "desktop", "tablet", "new_visitor", "returning"} {
		w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/analysis?segment="+seg, nil)
		if w.Code != http.StatusOK {
			t.Errorf("segment %q: want 200, got %d (body: %s)", seg, w.Code, w.Body.String())
		}
	}
}
