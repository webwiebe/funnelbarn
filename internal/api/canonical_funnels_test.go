package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

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
