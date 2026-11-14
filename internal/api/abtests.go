package api

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

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
		tests = []domain.ABTest{}
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

	ctx, span := tracing.StartSpan(r.Context(), "abtests.create",
		attribute.String("project.id", projectID),
		attribute.String("abtest.conversion_event", body.ConversionEvent),
	)
	defer span.End()

	test, err := s.abtests.CreateABTest(ctx, domain.ABTest{
		ProjectID:       projectID,
		Name:            body.Name,
		Status:          "running",
		ControlFilter:   body.ControlFilter,
		VariantFilter:   body.VariantFilter,
		ConversionEvent: body.ConversionEvent,
	})
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleCreateABTest")
		return
	}
	span.SetAttributes(attribute.String("abtest.id", test.ID))
	slog.InfoContext(ctx, "ab test created", "test_id", test.ID, "project_id", projectID, "request_id", RequestIDFromContext(ctx))
	writeJSON(w, http.StatusCreated, test)
}

// abTestRange reads the analysis window: the last 30 days unless range
// (24h, 7d, 30d) or explicit RFC 3339 from and to say otherwise.
func abTestRange(r *http.Request) (from, to time.Time) {
	to = time.Now().UTC()
	from = to.AddDate(0, 0, -30)
	switch r.URL.Query().Get("range") {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	}
	return overrideWindow(r, from, to)
}

// splitABResults maps the per-variant results into the flat control and
// variant totals the UI expects.
func splitABResults(results []domain.ABTestResult) (controlSample, controlConversions, variantSample, variantConversions int64) {
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
	return
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

	from, to := abTestRange(r)

	ctx, span := tracing.StartSpan(r.Context(), "abtests.analyze",
		attribute.String("project.id", test.ProjectID),
		attribute.String("abtest.id", test.ID),
		attribute.String("abtest.conversion_event", test.ConversionEvent),
	)
	defer span.End()
	results, err := s.abtests.AnalyzeABTest(ctx, test, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleABTestAnalysis")
		return
	}

	controlSample, controlConversions, variantSample, variantConversions := splitABResults(results)

	// Two-proportion z-test for statistical significance.
	zScore, significant := service.ZTestTwoProportions(controlSample, controlConversions, variantSample, variantConversions)

	span.SetAttributes(
		attribute.Int64("abtest.control_sample", controlSample),
		attribute.Int64("abtest.variant_sample", variantSample),
		attribute.Bool("abtest.significant", significant),
	)

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
