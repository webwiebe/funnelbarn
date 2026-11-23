package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSessionReadsRejectUnknownEnvironment(t *testing.T) {
	srv, store := newTestServer(t)
	p, _ := store.CreateProject(context.Background(), "SessEnvSite", "sess-env-site")

	for _, path := range []string{"sessions", "sessions/active", "session-distributions"} {
		w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/"+path+"?environment=nope", nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400 for an unknown environment, got %d", path, w.Code)
		}
		if !strings.Contains(w.Body.String(), "unknown environment") {
			t.Errorf("%s: body should name the problem, got %s", path, w.Body.String())
		}
		ok := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/"+path+"?environment=staging", nil)
		if ok.Code != http.StatusOK {
			t.Errorf("%s: expected 200 for a canonical environment, got %d", path, ok.Code)
		}
	}
}
