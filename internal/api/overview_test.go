package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func seedEvent(t *testing.T, store *repository.Store, projectID, sessionID, name, url string) {
	t.Helper()
	e := repository.Event{
		ID:         name + "-" + sessionID + "-" + url,
		ProjectID:  projectID,
		SessionID:  sessionID,
		Name:       name,
		URL:        url,
		IngestID:   "ing-" + name + "-" + sessionID + "-" + url,
		OccurredAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := store.InsertEvent(context.Background(), e); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
}

func TestHandleOverview(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	a, _ := store.CreateProject(ctx, "A", "ov-a")
	b, _ := store.CreateProject(ctx, "B", "ov-b")
	seedEvent(t, store, a.ID, "a1", "pageview", "https://a/1")
	seedEvent(t, store, a.ID, "a2", "pageview", "https://a/2")
	seedEvent(t, store, b.ID, "b1", "pageview", "https://b/1")

	w := getJSON(t, srv, "/api/v1/overview?range=30d", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("overview: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		TotalEvents    int64 `json:"total_events"`
		UniqueSessions int64 `json:"unique_sessions"`
		Projects       []struct {
			ProjectID string `json:"project_id"`
			Events    int64  `json:"events"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalEvents != 3 {
		t.Errorf("total_events: want 3, got %d", resp.TotalEvents)
	}
	if len(resp.Projects) != 2 {
		t.Errorf("projects: want 2, got %d", len(resp.Projects))
	}
}

func TestHandleCanonicalEventsAndMappings(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "P", "cm-p")
	seedEvent(t, store, p.ID, "s1", "registration", "https://p/signup")

	// Catalog is seeded; sign_up should exist.
	w := getJSON(t, srv, "/api/v1/canonical-events", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list canonical: %d", w.Code)
	}

	// Suggestions should propose sign_up for "registration".
	w = getJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-mappings/suggestions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("suggestions: %d (%s)", w.Code, w.Body.String())
	}
	var sug struct {
		Suggestions []struct {
			RawName      string `json:"raw_name"`
			SuggestedKey string `json:"suggested_key"`
		} `json:"suggestions"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sug)
	found := false
	for _, s := range sug.Suggestions {
		if s.RawName == "registration" && s.SuggestedKey == "sign_up" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected registration->sign_up suggestion, got %+v", sug.Suggestions)
	}

	// Save the mapping via bulk PUT.
	body := map[string]any{"mappings": []map[string]string{{"raw_name": "registration", "canonical_key": "sign_up"}}}
	w = putJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-mappings", body, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("set mappings: %d (%s)", w.Code, w.Body.String())
	}

	// Unknown canonical key is rejected.
	bad := map[string]any{"mappings": []map[string]string{{"raw_name": "x", "canonical_key": "does_not_exist"}}}
	w = putJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-mappings", bad, nil)
	if w.Code < 400 {
		t.Errorf("expected error for unknown canonical key, got %d", w.Code)
	}
}

func TestHandleCanonicalFunnelCRUDAndAnalysis(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	a, _ := store.CreateProject(ctx, "A", "cf-a")
	if err := store.UpsertMapping(ctx, a.ID, "pageview", "page_view"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMapping(ctx, a.ID, "signup", "sign_up"); err != nil {
		t.Fatal(err)
	}
	seedEvent(t, store, a.ID, "a1", "pageview", "https://a/1")
	seedEvent(t, store, a.ID, "a1", "signup", "https://a/signup")
	seedEvent(t, store, a.ID, "a2", "pageview", "https://a/1")

	// Create funnel.
	create := map[string]any{
		"name":  "signup",
		"scope": "session",
		"steps": []map[string]any{
			{"canonical_key": "page_view"},
			{"canonical_key": "sign_up"},
		},
	}
	w := postJSON(t, srv, "/api/v1/overview/funnels", create, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create funnel: %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("no funnel id returned")
	}

	// Analyze.
	w = getJSON(t, srv, "/api/v1/overview/funnels/"+created.ID+"/analysis", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("analyze: %d (%s)", w.Code, w.Body.String())
	}
	var ana struct {
		Result struct {
			Steps []struct {
				Count int64 `json:"count"`
			} `json:"steps"`
		} `json:"result"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ana)
	if len(ana.Result.Steps) != 2 || ana.Result.Steps[0].Count != 2 || ana.Result.Steps[1].Count != 1 {
		t.Errorf("analysis steps unexpected: %+v", ana.Result.Steps)
	}
}
