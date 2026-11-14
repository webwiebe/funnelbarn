package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func seedEvent(t *testing.T, store *repository.Store, projectID, sessionID, name, url string) {
	t.Helper()
	e := domain.Event{
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
