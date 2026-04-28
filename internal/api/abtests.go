package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

// handleListABTests returns all A/B tests for a project.
func (s *Server) handleListABTests(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	tests, err := s.store.ListABTests(r.Context(), projectID)
	if err != nil {
		jsonError(w, "failed to list a/b tests", http.StatusInternalServerError)
		return
	}
	if tests == nil {
		tests = []storage.ABTest{}
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
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if body.ConversionEvent == "" {
		jsonError(w, "conversion_event is required", http.StatusBadRequest)
		return
	}

	test, err := s.store.CreateABTest(r.Context(), storage.ABTest{
		ProjectID:       projectID,
		Name:            body.Name,
		Status:          "running",
		ControlFilter:   body.ControlFilter,
		VariantFilter:   body.VariantFilter,
		ConversionEvent: body.ConversionEvent,
	})
	if err != nil {
		jsonError(w, "failed to create a/b test", http.StatusInternalServerError)
		return
	}
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

	test, err := s.store.ABTestByID(r.Context(), testID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "a/b test not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to load a/b test", http.StatusInternalServerError)
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

	results, err := s.store.AnalyzeABTest(r.Context(), test, from, to)
	if err != nil {
		jsonError(w, "failed to analyze a/b test", http.StatusInternalServerError)
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

	// Simple significance check: require at least 100 samples per arm and
	// a detectable difference (> 5% relative lift or a minimum absolute diff).
	significant := false
	confidence := 0.0
	if controlSample >= 100 && variantSample >= 100 {
		controlRate := float64(controlConversions) / float64(controlSample)
		variantRate := float64(variantConversions) / float64(variantSample)
		if controlRate > 0 {
			relativeLift := (variantRate - controlRate) / controlRate
			if relativeLift > 0.05 || relativeLift < -0.05 {
				significant = true
				// Heuristic confidence score: clamp relative lift to [0,1].
				diff := relativeLift
				if diff < 0 {
					diff = -diff
				}
				confidence = diff / (diff + 0.1)
				if confidence > 0.99 {
					confidence = 0.99
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"test":                 test,
		"control_sample":       controlSample,
		"control_conversions":  controlConversions,
		"variant_sample":       variantSample,
		"variant_conversions":  variantConversions,
		"significant":          significant,
		"confidence":           confidence,
		"from":                 from.Format(time.RFC3339),
		"to":                   to.Format(time.RFC3339),
	})
}
