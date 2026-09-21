package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// addPaginationHeaders — emits a Link: rel="next" only when there *might* be more.
// ---------------------------------------------------------------------------

func TestAddPaginationHeaders_NoNextWhenUnderLimit(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/projects/p/events?limit=50", nil)
	w := httptest.NewRecorder()
	addPaginationHeaders(w, req, 50, 0, 10) // returned fewer than limit → no next page
	if got := w.Header().Get("Link"); got != "" {
		t.Errorf("no-next: expected empty Link, got %q", got)
	}
}

func TestAddPaginationHeaders_SetsNextWhenAtLimit(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/projects/p/events?limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	addPaginationHeaders(w, req, 50, 0, 50) // exactly limit → possibly more
	link := w.Header().Get("Link")
	if link == "" {
		t.Fatal("expected Link header with next page")
	}
	if !strings.Contains(link, `rel="next"`) {
		t.Errorf("Link: want rel=\"next\", got %q", link)
	}
	if !strings.Contains(link, "offset=50") {
		t.Errorf("Link: want offset=50, got %q", link)
	}
	if !strings.Contains(link, "limit=50") {
		t.Errorf("Link: want limit=50, got %q", link)
	}
}

func TestAddPaginationHeaders_PreservesOtherQueryParams(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/events?from=2026-01-01&to=2026-02-01&limit=20", nil)
	w := httptest.NewRecorder()
	addPaginationHeaders(w, req, 20, 0, 20)
	link := w.Header().Get("Link")
	if !strings.Contains(link, "from=2026-01-01") {
		t.Errorf("Link should preserve from param, got %q", link)
	}
	if !strings.Contains(link, "to=2026-02-01") {
		t.Errorf("Link should preserve to param, got %q", link)
	}
}
