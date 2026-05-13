package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// zTestTwoProportions — pure stats helper, covers a few representative cases.
// ---------------------------------------------------------------------------

func TestZTestTwoProportions(t *testing.T) {
	cases := []struct {
		name          string
		n1, x1, n2, x2 int64
		wantZero      bool
		wantSig       bool
	}{
		{"zero sample n1", 0, 0, 100, 50, true, false},
		{"zero sample n2", 100, 50, 0, 0, true, false},
		// Both arms 0% — pooled p is 0, can't compute.
		{"both zero conversions", 100, 0, 100, 0, true, false},
		// Both arms 100% — pooled p is 1, can't compute.
		{"both full conversions", 100, 100, 100, 100, true, false},
		// Equal rates → z = 0, not significant. Pure-zero is the *correct*
		// answer here (numerator of the z stat is p1 - p2 = 0), so allow it
		// alongside other non-significant cases.
		{"equal rates", 1000, 100, 1000, 100, true, false},
		// Strong difference, large samples → significant.
		{"large diff large sample", 1000, 500, 1000, 100, false, true},
		// Small difference, small sample → not significant.
		{"small diff small sample", 50, 25, 50, 22, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z, sig := zTestTwoProportions(c.n1, c.x1, c.n2, c.x2)
			if c.wantZero && z != 0 {
				t.Errorf("z: want 0, got %f", z)
			}
			if !c.wantZero && z == 0 {
				t.Error("z: got 0 but expected a non-zero value")
			}
			if sig != c.wantSig {
				t.Errorf("significant: want %v, got %v (z=%f)", c.wantSig, sig, z)
			}
		})
	}
}

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
