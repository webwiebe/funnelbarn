package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func segmentsTestSetup(t *testing.T) (*Server, string) {
	t.Helper()
	srv, store := fullServer(t, nil) // fullServer already wires Segments
	p, _ := store.CreateProject(context.Background(), "SegSite", "seg-site")
	return srv, p.ID
}

func TestHandleSegments_CRUD(t *testing.T) {
	srv, pid := segmentsTestSetup(t)

	// List (empty).
	wl := getJSON(t, srv, "/api/v1/projects/"+pid+"/segments", nil)
	if wl.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d (body: %s)", wl.Code, wl.Body.String())
	}

	// Create.
	wc := postJSON(t, srv, "/api/v1/projects/"+pid+"/segments", map[string]any{
		"name": "Dutch",
		"rules": []repository.SegmentRule{
			{Field: "country_code", Operator: "eq", Value: "NL"},
		},
	}, nil)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (body: %s)", wc.Code, wc.Body.String())
	}
	var seg repository.Segment
	_ = json.Unmarshal(wc.Body.Bytes(), &seg)
	if seg.ID == "" {
		t.Fatal("expected segment id in response")
	}

	// Update.
	wu := putJSON(t, srv, "/api/v1/projects/"+pid+"/segments/"+seg.ID, map[string]any{
		"name":  "Dutch Visitors",
		"rules": []repository.SegmentRule{},
	}, nil)
	if wu.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (body: %s)", wu.Code, wu.Body.String())
	}

	// Delete.
	wd := deleteReq(t, srv, "/api/v1/projects/"+pid+"/segments/"+seg.ID, nil)
	if wd.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", wd.Code)
	}
}

func TestHandleCreateSegment_MissingName(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	w := postJSON(t, srv, "/api/v1/projects/"+pid+"/segments", map[string]any{"name": ""}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing name, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleCreateSegment_BadJSON(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	w := postRaw(t, srv, "/api/v1/projects/"+pid+"/segments", "{bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d (body: %s)", w.Code, w.Body.String())
	}
}
