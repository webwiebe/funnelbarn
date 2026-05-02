package api

import (
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// zTestTwoProportions performs a two-proportion z-test.
// Returns the z-score and whether the result is significant at the 95% CI (|z| > 1.96).
func zTestTwoProportions(n1, x1, n2, x2 int64) (zScore float64, significant bool) {
	if n1 == 0 || n2 == 0 {
		return 0, false
	}
	p1 := float64(x1) / float64(n1)
	p2 := float64(x2) / float64(n2)
	pPool := float64(x1+x2) / float64(n1+n2)
	if pPool == 0 || pPool == 1 {
		return 0, false
	}
	se := math.Sqrt(pPool * (1 - pPool) * (1/float64(n1) + 1/float64(n2)))
	if se == 0 {
		return 0, false
	}
	z := math.Abs((p1 - p2) / se)
	// 95% CI: z > 1.96
	return z, z > 1.96
}

// handleListABTests returns all A/B tests for a project.
func (s *Server) handleListABTests(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	tests, err := s.abtests.ListABTests(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleListABTests")
		return
	}
	if tests == nil {
		tests = []repository.ABTest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tests": tests})
}

// handleCreateABTest creates a new A/B test for a project.
func (s *Server) handleCreateABTest(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		Name            string `json:"name"`
		ControlFilter   string `json:"control_filter"`
		VariantFilter   string `json:"variant_filter"`
		ConversionEvent string `json:"conversion_event"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	test, err := s.abtests.CreateABTest(r.Context(), repository.ABTest{
		ProjectID:       projectID,
		Name:            body.Name,
		Status:          "running",
		ControlFilter:   body.ControlFilter,
		VariantFilter:   body.VariantFilter,
		ConversionEvent: body.ConversionEvent,
	})
	if err != nil {
		mapServiceError(w, err, "handleCreateABTest")
		return
	}
	slog.InfoContext(r.Context(), "ab test created", "test_id", test.ID, "project_id", projectID, "request_id", RequestIDFromContext(r.Context()))
	writeJSON(w, http.StatusCreated, test)
}

// handleABTestAnalysis computes variant conversion rates for an A/B test.
func (s *Server) handleABTestAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	testID := r.PathValue("abid")
	if projectID == "" || testID == "" {
		jsonError(w, "project id and ab test id required", http.StatusBadRequest)
		return
	}

	test, err := s.abtests.GetABTest(r.Context(), testID)
	if err != nil {
		mapServiceError(w, err, "handleABTestAnalysis.getABTest")
		return
	}
	if test.ProjectID != projectID {
		jsonError(w, "a/b test not found", http.StatusNotFound)
		return
	}

	// Parse time range (default: last 30 days).
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	switch r.URL.Query().Get("range") {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	case "30d":
		from = to.AddDate(0, 0, -30)
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	results, err := s.abtests.AnalyzeABTest(r.Context(), test, from, to)
	if err != nil {
		mapServiceError(w, err, "handleABTestAnalysis")
		return
	}

	// Map results into the flat shape the UI expects.
	var controlSample, controlConversions, variantSample, variantConversions int64
	for _, r := range results {
		switch r.Variant {
		case "control":
			controlSample = r.Total
			controlConversions = r.Conversions
		case "variant":
			variantSample = r.Total
			variantConversions = r.Conversions
		}
	}

	// Two-proportion z-test for statistical significance.
	zScore, significant := zTestTwoProportions(controlSample, controlConversions, variantSample, variantConversions)

	writeJSON(w, http.StatusOK, map[string]any{
		"test":                test,
		"control_sample":      controlSample,
		"control_conversions": controlConversions,
		"variant_sample":      variantSample,
		"variant_conversions": variantConversions,
		"significant":         significant,
		"z_score":             zScore,
		"from":                from.Format(time.RFC3339),
		"to":                  to.Format(time.RFC3339),
	})
}
