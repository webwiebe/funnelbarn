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
