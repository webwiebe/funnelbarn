package widgets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// newTestMux serves the widget routes the way the API server registers them,
// minus the session middleware, over a migrated database.
func newTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	store := repotest.Open(t)
	p, err := store.CreateProject(context.Background(), "WidgetSite", "widget-site")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(New(store.DB(), store.ReadDB()))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects/{id}/widgets", h.List)
	mux.HandleFunc("POST /api/v1/projects/{id}/widgets", h.Create)
	mux.HandleFunc("PUT /api/v1/projects/{id}/widgets/{wid}", h.Update)
	mux.HandleFunc("DELETE /api/v1/projects/{id}/widgets/{wid}", h.Delete)
	mux.HandleFunc("GET /api/v1/projects/{id}/widgets/{wid}/breakdown", h.Breakdown)
	mux.HandleFunc("GET /api/v1/projects/{id}/widgets/breakdowns", h.BatchBreakdowns)
	return mux, p.ID
}

func serve(t *testing.T, mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestHandleWidgets_CRUD(t *testing.T) {
	mux, pid := newTestMux(t)
	base := "/api/v1/projects/" + pid + "/widgets"

	if w := serve(t, mux, http.MethodGet, base, ""); w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	wc := serve(t, mux, http.MethodPost, base, `{"event_name":"signup","property":"plan","title":"Signups by plan","position":1,"size":2}`)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (body: %s)", wc.Code, wc.Body.String())
	}
	var widget Widget
	_ = json.Unmarshal(wc.Body.Bytes(), &widget)
	if widget.ID == "" {
		t.Fatal("expected widget id in response")
	}

	wu := serve(t, mux, http.MethodPut, base+"/"+widget.ID, `{"event_name":"signup","property":"source","title":"Signups by source"}`)
	if wu.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (body: %s)", wu.Code, wu.Body.String())
	}

	if w := serve(t, mux, http.MethodGet, base+"/"+widget.ID+"/breakdown?window=50&limit=5", ""); w.Code != http.StatusOK {
		t.Fatalf("breakdown: expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if w := serve(t, mux, http.MethodGet, base+"/breakdowns", ""); w.Code != http.StatusOK {
		t.Fatalf("batch: expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if w := serve(t, mux, http.MethodDelete, base+"/"+widget.ID, ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}
}

func TestHandleCreateWidget_MissingEventName(t *testing.T) {
	mux, pid := newTestMux(t)
	w := serve(t, mux, http.MethodPost, "/api/v1/projects/"+pid+"/widgets", `{"event_name":""}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing event_name, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleCreateWidget_BadJSON(t *testing.T) {
	mux, pid := newTestMux(t)
	w := serve(t, mux, http.MethodPost, "/api/v1/projects/"+pid+"/widgets", "{bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleWidgetBreakdown_NotFound(t *testing.T) {
	mux, pid := newTestMux(t)
	// Unknown widget id -> GetWidget errors -> mapped error status (not 200).
	w := serve(t, mux, http.MethodGet, "/api/v1/projects/"+pid+"/widgets/nonexistent/breakdown", "")
	if w.Code == http.StatusOK {
		t.Fatalf("expected error status for unknown widget, got 200 (body: %s)", w.Body.String())
	}
}

func TestPositiveQueryInt(t *testing.T) {
	cases := map[string]int{
		"":        100,
		"?n=1":    1,
		"?n=1000": 1000,
		"?n=0":    100,
		"?n=-1":   100,
		"?n=1001": 100,
		"?n=abc":  100,
	}
	for query, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/"+query, nil)
		if got := positiveQueryInt(r, "n", 100, 1000); got != want {
			t.Errorf("positiveQueryInt(%q) = %d, want %d", query, got, want)
		}
	}
}
