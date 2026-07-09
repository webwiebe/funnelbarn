package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func widgetsTestSetup(t *testing.T) (*Server, string) {
	t.Helper()
	srv, store := newTestServer(t) // newTestServer wires Widgets
	p, _ := store.CreateProject(context.Background(), "WidgetSite", "widget-site")
	return srv, p.ID
}

func TestHandleWidgets_CRUD(t *testing.T) {
	srv, pid := widgetsTestSetup(t)

	// List (empty).
	wl := getJSON(t, srv, "/api/v1/projects/"+pid+"/widgets", nil)
	if wl.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d (body: %s)", wl.Code, wl.Body.String())
	}

	// Create.
	wc := postJSON(t, srv, "/api/v1/projects/"+pid+"/widgets", map[string]any{
		"event_name": "signup",
		"property":   "plan",
		"title":      "Signups by plan",
		"position":   1,
		"size":       2,
	}, nil)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (body: %s)", wc.Code, wc.Body.String())
	}
	var widget repository.DashboardWidget
	_ = json.Unmarshal(wc.Body.Bytes(), &widget)
	if widget.ID == "" {
		t.Fatal("expected widget id in response")
	}

	// Update.
	wu := putJSON(t, srv, "/api/v1/projects/"+pid+"/widgets/"+widget.ID, map[string]any{
		"event_name": "signup",
		"property":   "source",
		"title":      "Signups by source",
	}, nil)
	if wu.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (body: %s)", wu.Code, wu.Body.String())
	}

	// Breakdown (single widget).
	wb := getJSON(t, srv, "/api/v1/projects/"+pid+"/widgets/"+widget.ID+"/breakdown?window=50&limit=5", nil)
	if wb.Code != http.StatusOK {
		t.Fatalf("breakdown: expected 200, got %d (body: %s)", wb.Code, wb.Body.String())
	}

	// Batch breakdowns.
	wbatch := getJSON(t, srv, "/api/v1/projects/"+pid+"/widgets/breakdowns", nil)
	if wbatch.Code != http.StatusOK {
		t.Fatalf("batch: expected 200, got %d (body: %s)", wbatch.Code, wbatch.Body.String())
	}

	// Delete.
	wd := deleteReq(t, srv, "/api/v1/projects/"+pid+"/widgets/"+widget.ID, nil)
	if wd.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", wd.Code)
	}
}

func TestHandleCreateWidget_MissingEventName(t *testing.T) {
	srv, pid := widgetsTestSetup(t)
	w := postJSON(t, srv, "/api/v1/projects/"+pid+"/widgets", map[string]any{"event_name": ""}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing event_name, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleCreateWidget_BadJSON(t *testing.T) {
	srv, pid := widgetsTestSetup(t)
	w := postRaw(t, srv, "/api/v1/projects/"+pid+"/widgets", "{bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleWidgetBreakdown_NotFound(t *testing.T) {
	srv, pid := widgetsTestSetup(t)
	// Unknown widget id -> GetWidget errors -> mapped error status (not 200).
	w := getJSON(t, srv, "/api/v1/projects/"+pid+"/widgets/nonexistent/breakdown", nil)
	if w.Code == http.StatusOK {
		t.Fatalf("expected error status for unknown widget, got 200 (body: %s)", w.Body.String())
	}
}
