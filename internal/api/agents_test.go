package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func agentsServer(t *testing.T) (*Server, *repository.Store, string) {
	t.Helper()
	srv, store := fullServer(t, func(cfg *ServerConfig) {
		cfg.Agents = service.NewAgentService(cfg.DB.(*repository.Store))
	})
	p, err := store.CreateProject(context.Background(), "Agents", "agents")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return srv, store, p.ID
}

func TestAgentLeadTimes_ReportsMergedPRs(t *testing.T) {
	srv, store, projectID := agentsServer(t)
	ctx := context.Background()
	opened := time.Date(2026, time.October, 6, 9, 0, 0, 0, time.UTC)
	for i, e := range []struct {
		name string
		at   time.Time
	}{{"pr_opened", opened}, {"pr_merged", opened.Add(2 * time.Hour)}} {
		err := store.InsertEvent(ctx, repository.Event{
			ID: e.name, ProjectID: projectID, Name: e.name, IngestID: e.name, OccurredAt: e.at,
			SessionID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Properties: `{"source":"github","repo":"o/r","pr":7,"branch":"issue-1/x","merge_sha":"abc"}`,
		})
		if err != nil {
			t.Fatalf("InsertEvent %d: %v", i, err)
		}
	}

	w := getJSON(t, srv, "/api/v1/projects/"+projectID+"/agents/lead-times?from=2026-10-05T00:00:00Z&to=2026-10-12T00:00:00Z", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var out service.AgentLeadTimes
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.PRToMerge.Count != 1 || out.PRToMerge.MedianSeconds != 7200 {
		t.Errorf("pr_to_merge = %+v, want one PR merged after 7200s", out.PRToMerge)
	}
}

func TestAgentEndpoints_RejectBadInput(t *testing.T) {
	srv, _, projectID := agentsServer(t)
	base := "/api/v1/projects/" + projectID + "/agents/"
	for path, want := range map[string]int{
		base + "scoreboard?week=2026-W99":     http.StatusUnprocessableEntity,
		base + "scoreboard?week=last-week":    http.StatusUnprocessableEntity,
		base + "scoreboard?range=1y":          http.StatusBadRequest,
		base + "lead-times?from=yesterday":    http.StatusBadRequest,
		base + "scoreboard":                   http.StatusOK,
		base + "scoreboard?week=2026-W41":     http.StatusOK,
		base + "lead-times?repo=o/r&range=7d": http.StatusOK,
	} {
		if w := getJSON(t, srv, path, nil); w.Code != want {
			t.Errorf("GET %s: want %d, got %d (%s)", path, want, w.Code, w.Body.String())
		}
	}
}

func TestAgentRoutes_AbsentWithoutTheService(t *testing.T) {
	srv, _ := fullServer(t, nil)
	if w := getJSON(t, srv, "/api/v1/projects/p/agents/scoreboard", nil); w.Code != http.StatusNotFound {
		t.Errorf("want 404 without an agents service, got %d", w.Code)
	}
}
