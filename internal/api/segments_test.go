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

func TestHandleCreateSegment_InvalidField(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	w := postJSON(t, srv, "/api/v1/projects/"+pid+"/segments", map[string]any{
		"name": "Bad field",
		"rules": []repository.SegmentRule{
			{Field: "not_a_real_field", Operator: "eq", Value: "x"},
		},
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid rule field, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleCreateSegment_InvalidOperator(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	w := postJSON(t, srv, "/api/v1/projects/"+pid+"/segments", map[string]any{
		"name": "Bad operator",
		"rules": []repository.SegmentRule{
			{Field: "country_code", Operator: "like", Value: "NL"},
		},
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid rule operator, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateSegment_MissingName(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	wc := postJSON(t, srv, "/api/v1/projects/"+pid+"/segments", map[string]any{
		"name": "Original", "rules": []repository.SegmentRule{},
	}, nil)
	var seg repository.Segment
	_ = json.Unmarshal(wc.Body.Bytes(), &seg)

	w := putJSON(t, srv, "/api/v1/projects/"+pid+"/segments/"+seg.ID, map[string]any{
		"name": "", "rules": []repository.SegmentRule{},
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing name on update, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestHandleSegment_CrossProject verifies that update and delete return 404
// for a segment that belongs to a different project than the one in the URL.
func TestHandleSegment_CrossProject(t *testing.T) {
	srv, store := fullServer(t, nil)
	ctx := context.Background()
	p1, _ := store.CreateProject(ctx, "SegProj1", "seg-proj1")
	p2, _ := store.CreateProject(ctx, "SegProj2", "seg-proj2")

	wc := postJSON(t, srv, "/api/v1/projects/"+p1.ID+"/segments", map[string]any{
		"name": "P1 Segment", "rules": []repository.SegmentRule{},
	}, nil)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (body: %s)", wc.Code, wc.Body.String())
	}
	var seg repository.Segment
	_ = json.Unmarshal(wc.Body.Bytes(), &seg)

	// Update via the other project's URL: 404.
	wu := putJSON(t, srv, "/api/v1/projects/"+p2.ID+"/segments/"+seg.ID, map[string]any{
		"name": "Hijacked", "rules": []repository.SegmentRule{},
	}, nil)
	if wu.Code != http.StatusNotFound {
		t.Fatalf("update via wrong project: expected 404, got %d (body: %s)", wu.Code, wu.Body.String())
	}

	// Delete via the other project's URL: 404, and the segment must survive.
	wd := deleteReq(t, srv, "/api/v1/projects/"+p2.ID+"/segments/"+seg.ID, nil)
	if wd.Code != http.StatusNotFound {
		t.Fatalf("delete via wrong project: expected 404, got %d (body: %s)", wd.Code, wd.Body.String())
	}

	wg := getJSON(t, srv, "/api/v1/projects/"+p1.ID+"/segments", nil)
	var listResp map[string]any
	_ = json.Unmarshal(wg.Body.Bytes(), &listResp)
	segs, _ := listResp["segments"].([]any)
	if len(segs) != 1 {
		t.Fatalf("segment should survive a cross-project delete attempt, got %d segments", len(segs))
	}
}

func TestHandleUpdateSegment_NotFound(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	w := putJSON(t, srv, "/api/v1/projects/"+pid+"/segments/does-not-exist", map[string]any{
		"name": "X", "rules": []repository.SegmentRule{},
	}, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent segment, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSegment_NotFound(t *testing.T) {
	srv, pid := segmentsTestSetup(t)
	w := deleteReq(t, srv, "/api/v1/projects/"+pid+"/segments/does-not-exist", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent segment, got %d (body: %s)", w.Code, w.Body.String())
	}
}
