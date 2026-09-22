package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// These tests cover handleUpdateFunnel's move to FunnelService.UpdateFunnel
// for validation (see internal/service/funnels.go's validateFunnel): REST
// must reject the same bad input CreateFunnel does, with the same 422
// status and service message, and preserve an existing funnel's scope when
// the request omits it.

func TestHandleUpdateFunnel_MissingName_Returns422(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FUpdVal2", "fupdval2")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})

	w := putJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID, map[string]any{
		"steps": []map[string]string{{"event_name": "ev"}},
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for missing name, got %d (body: %s)", w.Code, w.Body.String())
	}
	var errResp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["error"] == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestHandleUpdateFunnel_NoSteps_Returns422(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FUpdVal3", "fupdval3")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})

	w := putJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID, map[string]any{
		"name":  "Funnel",
		"steps": []map[string]string{},
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for no steps, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFunnel_EmptyStepEventName_Returns422(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FUpdVal4", "fupdval4")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})

	w := putJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID, map[string]any{
		"name":  "Funnel",
		"steps": []map[string]string{{"event_name": ""}},
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for empty step event_name, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestHandleUpdateFunnel_PreservesScopeWhenOmitted mirrors the service-layer
// test but through the REST handler: a PUT that leaves scope unset must not
// reset it to the "session" default.
func TestHandleUpdateFunnel_PreservesScopeWhenOmitted(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FUpdScope", "fupdscope")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel",
		Scope:     "page_view",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})
	if f.Scope != "page_view" {
		t.Fatalf("setup: want scope page_view, got %q", f.Scope)
	}

	w := putJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID, map[string]any{
		"name":  "Funnel Renamed",
		"steps": []map[string]string{{"event_name": "ev"}},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var updated repository.Funnel
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Scope != "page_view" {
		t.Errorf("scope: want page_view to be preserved, got %q", updated.Scope)
	}
}
