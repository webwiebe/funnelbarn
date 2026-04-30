package api

import (
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/apierr"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
)

// handleListABTests returns all A/B tests for a project.
func (s *Server) handleListABTests(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id required"))
		return
	}

	tests, err := s.abtests.ListABTests(r.Context(), projectID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.MapDB(err, "failed to list a/b tests"))
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
		apierr.WriteHTTP(w, apierr.BadRequest("project id required"))
		return
	}

	var body struct {
		Name            string `json:"name"`
		ControlFilter   string `json:"control_filter"`
		VariantFilter   string `json:"variant_filter"`
		ConversionEvent string `json:"conversion_event"`
	}
	if err := readJSON(r, &body); err != nil {
		apierr.WriteHTTP(w, apierr.BadRequest("invalid request body"))
		return
	}
	if body.Name == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("name is required"))
		return
	}
	if body.ConversionEvent == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("conversion_event is required"))
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
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}
	writeJSON(w, http.StatusCreated, test)
}

// handleABTestAnalysis computes variant conversion rates and statistical significance.
func (s *Server) handleABTestAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	testID := r.PathValue("abid")
	if projectID == "" || testID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id and ab test id required"))
		return
	}

	test, err := s.abtests.GetABTest(r.Context(), testID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.MapDB(err, "a/b test not found"))
		return
	}
	if test.ProjectID != projectID {
		apierr.WriteHTTP(w, apierr.NotFound("a/b test not found"))
		return
	}

	tr := timerange.Parse(r.URL.Query())

	results, err := s.abtests.AnalyzeABTest(r.Context(), test, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	var controlSample, controlConversions, variantSample, variantConversions int64
	for _, res := range results {
		switch res.Variant {
		case "control":
			controlSample = res.Total
			controlConversions = res.Conversions
		case "variant":
			variantSample = res.Total
			variantConversions = res.Conversions
		}
	}

	// Statistical significance lives in the service layer.
	zScore, significant := service.ZTest(controlSample, controlConversions, variantSample, variantConversions)

	writeJSON(w, http.StatusOK, map[string]any{
		"test":                test,
		"control_sample":      controlSample,
		"control_conversions": controlConversions,
		"variant_sample":      variantSample,
		"variant_conversions": variantConversions,
		"significant":         significant,
		"z_score":             zScore,
		"from":                tr.From.Format("2006-01-02T15:04:05Z07:00"),
		"to":                  tr.To.Format("2006-01-02T15:04:05Z07:00"),
	})
}
