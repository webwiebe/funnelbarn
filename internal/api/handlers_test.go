package api

// Additional handler tests: abtests, funnel analysis/segments, dashboard, sessions.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

func randomID() string { return fmt.Sprintf("%x", time.Now().UnixNano()) }

// ---------------------------------------------------------------------------
// A/B test handlers
// ---------------------------------------------------------------------------

func TestHandleListABTests_Empty(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ABSite", "absite")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/abtests", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["tests"]; !ok {
		t.Error("response missing 'tests' key")
	}
}

func TestHandleCreateABTest_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ABCreate", "abcreate")

	w := postJSON(t, srv, "/api/v1/projects/"+p.ID+"/abtests", map[string]string{
		"name":             "Headline Test",
		"conversion_event": "signup",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var test repository.ABTest
	_ = json.Unmarshal(w.Body.Bytes(), &test)
	if test.ID == "" {
		t.Error("expected non-empty ID in response")
	}
	if test.Name != "Headline Test" {
		t.Errorf("Name: want Headline Test, got %q", test.Name)
	}
}

func TestHandleCreateABTest_MissingName(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ABVal", "abval")

	w := postJSON(t, srv, "/api/v1/projects/"+p.ID+"/abtests", map[string]string{
		"conversion_event": "click",
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422 for missing name, got %d", w.Code)
	}
}

func TestHandleCreateABTest_MissingConversionEvent(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ABVal2", "abval2")

	w := postJSON(t, srv, "/api/v1/projects/"+p.ID+"/abtests", map[string]string{
		"name": "Test",
	}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422 for missing conversion_event, got %d", w.Code)
	}
}

func TestHandleABTestAnalysis_NotFound(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ABAnaly", "abanaly")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/abtests/nonexistent/analysis", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestHandleABTestAnalysis_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ABAnalyOK", "abanalyok")
	test, _ := store.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "CTA colour",
		Status:          "running",
		ConversionEvent: "purchase",
	})

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/abtests/"+test.ID+"/analysis", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["significant"]; !ok {
		t.Error("response missing 'significant' field")
	}
}

// ---------------------------------------------------------------------------
// Funnel analysis, update, and segments
// ---------------------------------------------------------------------------

func TestHandleFunnelAnalysis_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FAnaly", "fanaly")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Checkout",
		Steps: []repository.FunnelStep{
			{EventName: "cart_view"},
			{EventName: "checkout"},
		},
	})

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/analysis", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["funnel"]; !ok {
		t.Error("response missing 'funnel' key")
	}
	if _, ok := resp["results"]; !ok {
		t.Error("response missing 'results' key")
	}
}

func TestHandleFunnelAnalysis_WithSegment(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FAnalySeg", "fanalyseg")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Mobile Funnel",
		Steps:     []repository.FunnelStep{{EventName: "visit"}},
	})

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/analysis?segment=mobile", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with mobile segment, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleFunnelAnalysis_TimeRange(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FAnalyRange", "fanalyrange")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Range Funnel",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/analysis?range=7d", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with range param, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleFunnelSegments_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FSeg", "fseg")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Seg Funnel",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/segments", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateFunnel_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FUpd", "fupd")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Old Name",
		Steps:     []repository.FunnelStep{{EventName: "step1"}},
	})

	body, _ := json.Marshal(map[string]any{
		"name": "New Name",
		"steps": []map[string]string{
			{"event_name": "step1"},
			{"event_name": "step2"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var updated repository.Funnel
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Name != "New Name" {
		t.Errorf("Name: want New Name, got %q", updated.Name)
	}
	if len(updated.Steps) != 2 {
		t.Errorf("Steps: want 2, got %d", len(updated.Steps))
	}
}

func TestHandleUpdateFunnel_MissingName(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FUpdVal", "fupdval")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel",
		Steps:     []repository.FunnelStep{{EventName: "ev"}},
	})

	body, _ := json.Marshal(map[string]any{
		"steps": []map[string]string{{"event_name": "ev"}},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for missing name, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func TestHandleDashboard_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "DashSite", "dashsite")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/dashboard", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	for _, key := range []string{"total_events", "unique_sessions", "bounce_rate", "top_pages", "events_time_series"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("dashboard response missing key %q", key)
		}
	}
}

func TestHandleDashboard_TimeRanges(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "DashRange", "dashrange")

	for _, rangeParam := range []string{"24h", "7d", "30d"} {
		w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/dashboard?range="+rangeParam, nil)
		if w.Code != http.StatusOK {
			t.Errorf("range=%s: want 200, got %d", rangeParam, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func TestHandleActiveSessionCount_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ActSess", "actsess")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/sessions/active", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["active_sessions"]; !ok {
		t.Error("response missing 'active_sessions'")
	}
}

func TestHandleListSessions_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "SessList", "sesslist")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/sessions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

func TestHandleListEvents_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "EvtList2", "evtlist2")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/events", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["events"]; !ok {
		t.Error("response missing 'events'")
	}
}

func TestHandleListEvents_PaginationClamped(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "EvtPage", "evtpage")

	// limit=9999 should be clamped to 500
	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/events?limit=9999", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// limit in response should be clamped to ≤ 500
	if lim, ok := resp["limit"].(float64); ok && lim > 500 {
		t.Errorf("limit clamped: want ≤ 500, got %.0f", lim)
	}
}

// ---------------------------------------------------------------------------
// Event properties / property values handlers
// ---------------------------------------------------------------------------

func TestHandleEventProperties_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "PropsSite", "propssite")

	_ = store.InsertEvent(ctx, repository.Event{
		ID: "ep1", ProjectID: p.ID, SessionID: "s1", Name: "signup",
		Properties: `{"plan":"pro","source":"ads"}`,
		URL:        "https://example.com",
		Browser:    "Chrome",
		IngestID:   "i-ep1", OccurredAt: now(),
	})

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-properties?event_name=signup", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Properties []string `json:"properties"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// 2 populated metadata columns (url, browser) + 2 custom JSON properties (plan, source)
	expectedCount := 4
	if len(resp.Properties) != expectedCount {
		t.Fatalf("want %d properties (2 metadata + 2 custom), got %d: %v", expectedCount, len(resp.Properties), resp.Properties)
	}
}

func TestHandleEventProperties_MissingEventName(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "PropsNoName", "propsnoname")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-properties", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleEventPropertyValues_Success(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ValsSite", "valssite")

	for _, plan := range []string{"pro", "free", "pro"} {
		_ = store.InsertEvent(ctx, repository.Event{
			ID: randomID(), ProjectID: p.ID, SessionID: "s1", Name: "signup",
			Properties: `{"plan":"` + plan + `"}`,
			IngestID:   randomID(), OccurredAt: now(),
		})
	}

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-property-values?event_name=signup&property=plan", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Values []string `json:"values"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Values) != 2 {
		t.Fatalf("want 2 distinct values, got %d: %v", len(resp.Values), resp.Values)
	}
}

func TestHandleEventPropertyValues_MissingParams(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ValsNoParam", "valsnoparam")

	// Missing event_name
	w1 := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-property-values?property=plan", nil)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing event_name, got %d", w1.Code)
	}

	// Missing property
	w2 := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/event-property-values?event_name=signup", nil)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing property, got %d", w2.Code)
	}
}
