package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestHandleFunnelAnalysis_Environment(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FAnalyEnv", "fanalyenv")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Env Funnel",
		Steps:     []repository.FunnelStep{{EventName: "visit"}},
	})
	now := time.Now().UTC().Add(-time.Hour)
	for i, env := range []string{"production", "staging", "staging"} {
		if err := store.InsertEvent(ctx, repository.Event{
			ID: randomID() + string(rune('a'+i)), ProjectID: p.ID, SessionID: "s" + string(rune('a'+i)),
			Name: "visit", Properties: "{}", IngestID: "i" + randomID() + string(rune('a'+i)),
			OccurredAt: now, Environment: env,
		}); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}

	count := func(query string) int64 {
		t.Helper()
		w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/analysis"+query, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d (body: %s)", query, w.Code, w.Body.String())
		}
		var resp struct {
			Results []repository.FunnelStepResult `json:"results"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || len(resp.Results) != 1 {
			t.Fatalf("%s: bad body %s", query, w.Body.String())
		}
		return resp.Results[0].Count
	}

	for query, want := range map[string]int64{
		"":                         3,
		"?environment=":            3,
		"?environment=production":  1,
		"?environment=prod":        1,
		"?environment=staging":     2,
		"?environment=stg":         2,
		"?environment=development": 0,
	} {
		if got := count(query); got != want {
			t.Errorf("analysis%s: count = %d, want %d", query, got, want)
		}
	}
}

func TestHandleFunnelStepSessions_Environment(t *testing.T) {
	srv, store := newRecordingServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FStepEnv", "fstepenv")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Env Step Funnel",
		Steps:     []repository.FunnelStep{{EventName: "visit"}, {EventName: "book"}},
	})
	now := time.Now().UTC().Add(-time.Hour)
	for i, ev := range []struct{ session, env string }{
		{"prod-a", "production"}, {"stg-a", "staging"}, {"stg-b", "staging"},
	} {
		if err := store.InsertEvent(ctx, repository.Event{
			ID: randomID() + string(rune('a'+i)), ProjectID: p.ID, SessionID: ev.session,
			Name: "visit", Properties: "{}", IngestID: "i" + randomID() + string(rune('a'+i)),
			OccurredAt: now, Environment: ev.env,
		}); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}

	sessions := func(query string) []string {
		t.Helper()
		w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/steps/1/sessions"+query, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d (body: %s)", query, w.Code, w.Body.String())
		}
		var resp struct {
			SessionIDs []string `json:"session_ids"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: bad body %s", query, w.Body.String())
		}
		return resp.SessionIDs
	}

	for query, want := range map[string]int{
		"":                         3,
		"?environment=production":  1,
		"?environment=prod":        1,
		"?environment=staging":     2,
		"?environment=development": 0,
	} {
		if got := sessions(query); len(got) != want {
			t.Errorf("step sessions%s: got %v, want %d sessions", query, got, want)
		}
	}
	if got := sessions("?environment=production"); len(got) != 1 || got[0] != "prod-a" {
		t.Errorf("production drill-down = %v, want [prod-a]", got)
	}
}
