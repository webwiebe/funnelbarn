package api

import (
	"context"
	"net/http"
	"testing"
)

func TestHandlePageFlows_HappyPath(t *testing.T) {
	srv, store := newTestServer(t)
	p, _ := store.CreateProject(context.Background(), "FlowSite", "flow-site")

	// Exercise the range/page/depth query-param parsing plus a valid from/to.
	path := "/api/v1/projects/" + p.ID + "/flows?range=7d&page=%2Fhome&depth=3" +
		"&from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&environment=production"
	w := getJSON(t, srv, path, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandlePageFlows_DefaultRange(t *testing.T) {
	srv, store := newTestServer(t)
	p, _ := store.CreateProject(context.Background(), "FlowSite2", "flow-site-2")

	// 24h range + out-of-bounds depth (ignored -> default 5) + junk from/to (ignored).
	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flows?range=24h&depth=99&from=bad&to=bad", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}
