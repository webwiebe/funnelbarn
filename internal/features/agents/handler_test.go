package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// eventInserter seeds events into the test database.
type eventInserter interface {
	InsertEvent(ctx context.Context, e domain.Event) error
}

// newTestMux serves the agents routes the way the API server registers them,
// minus the session and token middleware, over a migrated database.
func newTestMux(t *testing.T) (*http.ServeMux, eventInserter, string) {
	t.Helper()
	store := repotest.Open(t)
	p, err := store.CreateProject(context.Background(), "Agents", "agents")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	h := NewHandler(New(store.DB(), store.ReadDB()))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects/{id}/agents/lead-times", h.LeadTimes)
	mux.HandleFunc("GET /api/v1/projects/{id}/agents/scoreboard", h.Scoreboard)
	return mux, store, p.ID
}

func getJSON(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestAgentLeadTimes_ReportsMergedPRs(t *testing.T) {
	mux, store, projectID := newTestMux(t)
	ctx := context.Background()
	opened := time.Date(2026, time.October, 6, 9, 0, 0, 0, time.UTC)
	for i, e := range []struct {
		name string
		at   time.Time
	}{{"pr_opened", opened}, {"pr_merged", opened.Add(2 * time.Hour)}} {
		err := store.InsertEvent(ctx, domain.Event{
			ID: e.name, ProjectID: projectID, Name: e.name, IngestID: e.name, OccurredAt: e.at,
			SessionID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Properties: `{"source":"github","repo":"o/r","pr":7,"branch":"issue-1/x","merge_sha":"abc"}`,
		})
		if err != nil {
			t.Fatalf("InsertEvent %d: %v", i, err)
		}
	}

	w := getJSON(t, mux, "/api/v1/projects/"+projectID+"/agents/lead-times?from=2026-10-05T00:00:00Z&to=2026-10-12T00:00:00Z")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var out AgentLeadTimes
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.PRToMerge.Count != 1 || out.PRToMerge.MedianSeconds != 7200 {
		t.Errorf("pr_to_merge = %+v, want one PR merged after 7200s", out.PRToMerge)
	}
}

func TestAgentEndpoints_RejectBadInput(t *testing.T) {
	mux, _, projectID := newTestMux(t)
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
		if w := getJSON(t, mux, path); w.Code != want {
			t.Errorf("GET %s: want %d, got %d (%s)", path, want, w.Code, w.Body.String())
		}
	}
}
