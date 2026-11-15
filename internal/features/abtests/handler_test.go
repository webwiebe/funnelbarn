package abtests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/features/abtests"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// newTestMux serves the A/B test routes the way the API server registers
// them, minus the session middleware, over a migrated database. It returns
// the mux and the ids of two projects.
func newTestMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	store := repotest.Open(t)
	ctx := context.Background()
	p, err := store.CreateProject(ctx, "ABSite", "absite")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateProject(ctx, "Other", "other-site")
	if err != nil {
		t.Fatal(err)
	}
	h := abtests.NewHandler(abtests.New(store.DB(), store.ReadDB()), func(context.Context) string { return "req-1" })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects/{id}/abtests", h.List)
	mux.HandleFunc("POST /api/v1/projects/{id}/abtests", h.Create)
	mux.HandleFunc("GET /api/v1/projects/{id}/abtests/{abid}/analysis", h.Analysis)
	return mux, p.ID, other.ID
}

func serve(t *testing.T, mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestHandler_ListEmpty(t *testing.T) {
	mux, pid, _ := newTestMux(t)
	w := serve(t, mux, http.MethodGet, "/api/v1/projects/"+pid+"/abtests", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string][]abtests.ABTest
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["tests"] == nil || len(resp["tests"]) != 0 {
		t.Errorf("want an empty tests array, got %s", w.Body.String())
	}
}

func TestHandler_CreateListAnalyze(t *testing.T) {
	mux, pid, _ := newTestMux(t)
	base := "/api/v1/projects/" + pid + "/abtests"

	w := serve(t, mux, http.MethodPost, base, `{"name":"Headline Test","conversion_event":"signup"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var test abtests.ABTest
	_ = json.Unmarshal(w.Body.Bytes(), &test)
	if test.ID == "" || test.Name != "Headline Test" || test.Status != "running" {
		t.Errorf("unexpected created test: %+v", test)
	}

	var list map[string][]abtests.ABTest
	w = serve(t, mux, http.MethodGet, base, "")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list["tests"]) != 1 {
		t.Errorf("want one test in list, got %s", w.Body.String())
	}

	for _, q := range []string{"", "?range=24h", "?range=7d", "?from=2020-01-01T00:00:00Z&to=2020-02-01T00:00:00Z"} {
		w = serve(t, mux, http.MethodGet, base+"/"+test.ID+"/analysis"+q, "")
		if w.Code != http.StatusOK {
			t.Fatalf("analysis%s: want 200, got %d (body: %s)", q, w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		for _, k := range []string{"test", "control_sample", "control_conversions", "variant_sample", "variant_conversions", "significant", "z_score", "from", "to"} {
			if _, ok := resp[k]; !ok {
				t.Errorf("analysis%s: response missing %q", q, k)
			}
		}
	}
}

func TestHandler_CreateValidation(t *testing.T) {
	mux, pid, _ := newTestMux(t)
	base := "/api/v1/projects/" + pid + "/abtests"
	cases := map[string]struct {
		body string
		want int
	}{
		"missing name":             {`{"conversion_event":"click"}`, http.StatusUnprocessableEntity},
		"missing conversion event": {`{"name":"Test"}`, http.StatusUnprocessableEntity},
		"invalid json":             {`{`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if w := serve(t, mux, http.MethodPost, base, c.body); w.Code != c.want {
				t.Errorf("want %d, got %d (body: %s)", c.want, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_AnalysisNotFound(t *testing.T) {
	mux, pid, _ := newTestMux(t)
	w := serve(t, mux, http.MethodGet, "/api/v1/projects/"+pid+"/abtests/nonexistent/analysis", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestHandler_AnalysisOtherProject(t *testing.T) {
	mux, pid, otherID := newTestMux(t)
	w := serve(t, mux, http.MethodPost, "/api/v1/projects/"+pid+"/abtests", `{"name":"T","conversion_event":"signup"}`)
	var test abtests.ABTest
	_ = json.Unmarshal(w.Body.Bytes(), &test)

	w = serve(t, mux, http.MethodGet, "/api/v1/projects/"+otherID+"/abtests/"+test.ID+"/analysis", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("cross-project analysis: want 404, got %d", w.Code)
	}
}
