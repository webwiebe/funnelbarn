package api

import (
	"net/http"
	"testing"
)

func TestAgentRoutes_AbsentWithoutTheService(t *testing.T) {
	srv, _ := fullServer(t, nil)
	if w := getJSON(t, srv, "/api/v1/projects/p/agents/scoreboard", nil); w.Code != http.StatusNotFound {
		t.Errorf("want 404 without an agents service, got %d", w.Code)
	}
}
