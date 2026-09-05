package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// seedEvents writes n events with the given name into a project, spread one
// minute apart ending now, so a range query has something to count.
func seedEvents(t *testing.T, store *repository.Store, projectID, name string, n int) {
	t.Helper()
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		err := store.InsertEvent(context.Background(), repository.Event{
			ID:         fmt.Sprintf("ev-%s-%s-%d", projectID, name, i),
			ProjectID:  projectID,
			SessionID:  fmt.Sprintf("sess-%s-%d", name, i),
			Name:       name,
			URL:        "https://example.test/" + name,
			OccurredAt: now.Add(-time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("InsertEvent(%s): %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// What the scope is for: a scripted readout, no browser session anywhere.
// ---------------------------------------------------------------------------

func TestAnalyticsToken_ReadsTheRoutesAReadoutNeeds(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "Professionals", "professionals")
	seedEvents(t, store, p.ID, "first_run_started", 5)
	seedEvents(t, store, p.ID, "first_run_completed", 2)
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "First run",
		Steps: []repository.FunnelStep{
			{EventName: "first_run_started"},
			{EventName: "first_run_completed"},
		},
	})
	key := scopedKey(t, store, p.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	for _, path := range []string{
		"/api/v1/projects",
		"/api/v1/projects/" + p.ID + "/dashboard",
		"/api/v1/projects/" + p.ID + "/events",
		"/api/v1/projects/" + p.ID + "/event-names",
		"/api/v1/projects/" + p.ID + "/event-counts",
		"/api/v1/projects/" + p.ID + "/funnels",
		"/api/v1/projects/" + p.ID + "/funnels/" + f.ID + "/analysis",
	} {
		if w := withKey(t, srv, http.MethodGet, path, key, nil); w.Code != http.StatusOK {
			t.Errorf("GET %s: want 200, got %d (body: %s)", path, w.Code, w.Body.String())
		}
	}
}

// The list has no {id} for the middleware to check, so the handler has to
// narrow it itself — otherwise one customer's readout token enumerates every
// project on the instance.
func TestAnalyticsToken_ProjectListIsNarrowedToTheToken(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	mine, _ := store.CreateProject(ctx, "Mine", "mine")
	other, _ := store.CreateProject(ctx, "Someone Else", "someone-else")
	key := scopedKey(t, store, mine.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Projects []repository.Project `json:"projects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Projects) != 1 || resp.Projects[0].ID != mine.ID {
		t.Fatalf("a token must see only its own project, got %+v", resp.Projects)
	}
	if resp.Projects[0].ID == other.ID {
		t.Fatal("token leaked another project")
	}

	// A dashboard session still sees everything.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.AddCookie(sessionCookieFor(t, srv, "admin"))
	sw := httptest.NewRecorder()
	srv.ServeHTTP(sw, req)
	var all struct {
		Projects []repository.Project `json:"projects"`
	}
	_ = json.Unmarshal(sw.Body.Bytes(), &all)
	if len(all.Projects) != 2 {
		t.Errorf("a session should still list every project, got %d", len(all.Projects))
	}
}

// ---------------------------------------------------------------------------
// The scope is read-only, and it is not a skeleton key.
// ---------------------------------------------------------------------------

func TestAnalyticsToken_CannotWriteOrTouchFlags(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "P", "p")
	flag := seedFlag(t, store, p.ID, "outbound_email")
	key := scopedKey(t, store, p.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	// Flags are a different scope entirely — reading analytics grants nothing there.
	if w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", key, nil); w.Code != http.StatusForbidden {
		t.Errorf("flags read: want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
	w := withKey(t, srv, http.MethodPut, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, key,
		map[string]any{"status": "paused"})
	if w.Code != http.StatusForbidden {
		t.Errorf("flag write: want 403, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Nor can it create a funnel on the project it can read.
	w = withKey(t, srv, http.MethodPost, "/api/v1/projects/"+p.ID+"/funnels", key, map[string]any{
		"name": "Sneaky", "steps": []map[string]any{{"event_name": "a"}},
	})
	if w.Code == http.StatusOK || w.Code == http.StatusCreated {
		t.Errorf("a read-only token must not create a funnel, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestAnalyticsToken_CannotReadAnotherProject(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	mine, _ := store.CreateProject(ctx, "Mine", "mine")
	other, _ := store.CreateProject(ctx, "Other", "other")
	seedEvents(t, store, other.ID, "secret_event", 3)
	key := scopedKey(t, store, mine.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+other.ID+"/event-counts", key, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// An ingest key is the one every project already has in production; it must not
// have quietly become a read credential.
func TestAnalyticsToken_IngestScopeCannotRead(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	key := scopedKey(t, store, p.ID, "ingest-key", repository.APIKeyScopeIngest)

	if w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/event-counts", key, nil); w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// The instance-wide FUNNELBARN_API_KEY resolves with no project, which would
// make it a master read key for every project on the instance.
func TestAnalyticsToken_GlobalKeyIsRefused(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/event-counts", "global-instance-key", nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d (body: %s)", w.Code, w.Body.String())
	}
	if w := withKey(t, srv, http.MethodGet, "/api/v1/projects", "global-instance-key", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("project list with the global key: want 401, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/projects/{id}/event-counts
// ---------------------------------------------------------------------------

type eventCountsResponse struct {
	ProjectID   string                     `json:"project_id"`
	From        string                     `json:"from"`
	To          string                     `json:"to"`
	Limit       int                        `json:"limit"`
	Events      []repository.EventNameStat `json:"events"`
	TotalEvents int64                      `json:"total_events"`
}

func TestEventCounts_CountsPerNameOverRange(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedEvents(t, store, p.ID, "first_run_started", 5)
	seedEvents(t, store, p.ID, "first_run_completed", 2)
	key := scopedKey(t, store, p.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/event-counts?range=24h", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp eventCountsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]int64{}
	for _, e := range resp.Events {
		got[e.Name] = e.Count
	}
	if got["first_run_started"] != 5 || got["first_run_completed"] != 2 {
		t.Fatalf("counts: %+v", got)
	}
	if resp.TotalEvents != 7 {
		t.Errorf("total_events: want 7, got %d", resp.TotalEvents)
	}
	if resp.ProjectID != p.ID || resp.From == "" || resp.To == "" {
		t.Errorf("the response must echo the window it answered for: %+v", resp)
	}
}

// A window with nothing in it is an empty list, not null — a shell script
// pipes this straight into jq.
func TestEventCounts_EmptyWindowIsAnEmptyList(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedEvents(t, store, p.ID, "signup", 3)
	key := scopedKey(t, store, p.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	old := time.Now().UTC().AddDate(-1, 0, 0)
	path := fmt.Sprintf("/api/v1/projects/%s/event-counts?from=%s&to=%s",
		p.ID, old.Format(time.RFC3339), old.AddDate(0, 0, 1).Format(time.RFC3339))
	w := withKey(t, srv, http.MethodGet, path, key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !jsonHasEmptyEvents(t, body) {
		t.Errorf(`want "events":[], got %s`, body)
	}
}

func jsonHasEmptyEvents(t *testing.T, body string) bool {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return string(raw["events"]) == "[]"
}

// A readout that asked for last week and quietly got last month reports the
// wrong number with no way to notice — so bad input is a 400, not a default.
func TestEventCounts_RejectsUnparseableRange(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	key := scopedKey(t, store, p.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)

	for _, q := range []string{
		"?range=90d",
		"?from=last-tuesday",
		"?to=nope",
		"?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z",
		"?limit=0",
		"?limit=9000",
		"?limit=lots",
	} {
		w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/event-counts"+q, key, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d (body: %s)", q, w.Code, w.Body.String())
		}
	}
}

func TestEventCounts_SessionStillWorks(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedEvents(t, store, p.ID, "signup", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+p.ID+"/event-counts", nil)
	req.AddCookie(sessionCookieFor(t, srv, "admin"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Issuance
// ---------------------------------------------------------------------------

func TestCreateAPIKey_AcceptsAnalyticsReadScope(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postJSONWithCSRF(t, srv, "/api/v1/apikeys", map[string]any{
		"project_id": p.ID, "name": "weekly readout", "scope": repository.APIKeyScopeAnalyticsRead,
	}, cookie, csrf)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	apiKey, _ := body["api_key"].(map[string]any)
	if apiKey["scope"] != repository.APIKeyScopeAnalyticsRead {
		t.Errorf("scope: want %s, got %v", repository.APIKeyScopeAnalyticsRead, apiKey["scope"])
	}
}
