package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

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
