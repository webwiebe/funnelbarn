package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func (s *Server) handleListFlags(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	flags, err := s.flags.ListFlags(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleListFlags")
		return
	}
	if flags == nil {
		flags = []repository.FeatureFlag{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"flags": flags})
}

func (s *Server) handleCreateFlag(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		FlagKey         string `json:"flag_key"`
		Name            string `json:"name"`
		FlagType        string `json:"flag_type"`
		Variants        string `json:"variants"`
		DefaultVariant  string `json:"default_variant"`
		Split           string `json:"split"`
		ConversionEvent string `json:"conversion_event"`
		TargetingRules  string `json:"targeting_rules"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.FlagKey == "" || body.Name == "" {
		jsonError(w, "flag_key and name are required", http.StatusUnprocessableEntity)
		return
	}
	if body.FlagType == "" {
		body.FlagType = "boolean"
	}
	if body.TargetingRules == "" {
		body.TargetingRules = "[]"
	}
	if err := service.ValidateTargetingRules(body.TargetingRules); err != nil {
		jsonError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	flag, err := s.flags.CreateFlag(r.Context(), repository.FeatureFlag{
		ProjectID:       projectID,
		FlagKey:         body.FlagKey,
		Name:            body.Name,
		FlagType:        body.FlagType,
		Variants:        body.Variants,
		DefaultVariant:  body.DefaultVariant,
		Split:           body.Split,
		ConversionEvent: body.ConversionEvent,
		TargetingRules:  body.TargetingRules,
		Status:          "active",
	})
	if err != nil {
		mapServiceError(w, err, "handleCreateFlag")
		return
	}
	slog.InfoContext(r.Context(), "flag created", "flag_id", flag.ID, "project_id", projectID)
	writeJSON(w, http.StatusCreated, flag)
}

func (s *Server) handleGetFlag(w http.ResponseWriter, r *http.Request) {
	flagID := r.PathValue("fid")
	if flagID == "" {
		jsonError(w, "flag id required", http.StatusBadRequest)
		return
	}
	flag, err := s.flags.GetFlag(r.Context(), flagID)
	if err != nil {
		mapServiceError(w, err, "handleGetFlag")
		return
	}
	writeJSON(w, http.StatusOK, flag)
}

func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	flagID := r.PathValue("fid")
	if flagID == "" {
		jsonError(w, "flag id required", http.StatusBadRequest)
		return
	}

	var body struct {
		Name            string `json:"name"`
		FlagType        string `json:"flag_type"`
		Variants        string `json:"variants"`
		DefaultVariant  string `json:"default_variant"`
		Split           string `json:"split"`
		ConversionEvent string `json:"conversion_event"`
		TargetingRules  string `json:"targeting_rules"`
		Status          string `json:"status"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.TargetingRules != "" {
		if err := service.ValidateTargetingRules(body.TargetingRules); err != nil {
			jsonError(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
	}

	// Preserve flag_type from the existing record when the caller omits it.
	if body.FlagType == "" {
		existing, err := s.flags.GetFlag(r.Context(), flagID)
		if err != nil {
			mapServiceError(w, err, "handleUpdateFlag.get")
			return
		}
		body.FlagType = existing.FlagType
	}

	flag, err := s.flags.UpdateFlag(r.Context(), repository.FeatureFlag{
		ID:              flagID,
		Name:            body.Name,
		FlagType:        body.FlagType,
		Variants:        body.Variants,
		DefaultVariant:  body.DefaultVariant,
		Split:           body.Split,
		ConversionEvent: body.ConversionEvent,
		TargetingRules:  body.TargetingRules,
		Status:          body.Status,
	})
	if err != nil {
		mapServiceError(w, err, "handleUpdateFlag")
		return
	}
	writeJSON(w, http.StatusOK, flag)
}

func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	flagID := r.PathValue("fid")
	if flagID == "" {
		jsonError(w, "flag id required", http.StatusBadRequest)
		return
	}
	if err := s.flags.DeleteFlag(r.Context(), flagID); err != nil {
		mapServiceError(w, err, "handleDeleteFlag")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFlagAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	flagID := r.PathValue("fid")
	if projectID == "" || flagID == "" {
		jsonError(w, "project id and flag id required", http.StatusBadRequest)
		return
	}

	flag, err := s.flags.GetFlag(r.Context(), flagID)
	if err != nil {
		mapServiceError(w, err, "handleFlagAnalysis.getFlag")
		return
	}
	if flag.ProjectID != projectID {
		jsonError(w, "flag not found", http.StatusNotFound)
		return
	}

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

	results, err := s.flags.AnalyzeFlag(r.Context(), flag, from, to)
	if err != nil {
		mapServiceError(w, err, "handleFlagAnalysis")
		return
	}
	if results == nil {
		results = []repository.FlagAnalysisResult{}
	}

	// Run z-test if we have exactly 2 variants.
	var zScore float64
	var significant bool
	if len(results) == 2 {
		zScore, significant = zTestTwoProportions(
			results[0].Sample, results[0].Conversions,
			results[1].Sample, results[1].Conversions,
		)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"flag":        flag,
		"results":     results,
		"significant": significant,
		"z_score":     zScore,
		"from":        from.Format(time.RFC3339),
		"to":          to.Format(time.RFC3339),
	})
}

func (s *Server) handleEvaluateFlag(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.projectHealth != nil {
		pid := projectID
		go func() {
			if err := s.projectHealth.MarkFlagsEvaluated(context.Background(), pid); err != nil {
				slog.Warn("evaluate flag: mark health", "project_id", pid, "err", err)
			}
		}()
	}
	s.evaluateFlagInProject(w, r, projectID)
}

// handlePlaygroundEvaluateFlag is the session-authed dashboard counterpart of
// handleEvaluateFlag. The project ID comes from the URL instead of the API key.
func (s *Server) handlePlaygroundEvaluateFlag(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	s.evaluateFlagInProject(w, r, projectID)
}

func (s *Server) evaluateFlagInProject(w http.ResponseWriter, r *http.Request, projectID string) {
	var body struct {
		FlagKey      string         `json:"flag_key"`
		DefaultValue any            `json:"default_value"`
		Context      map[string]any `json:"context"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.FlagKey == "" {
		jsonError(w, "flag_key is required", http.StatusUnprocessableEntity)
		return
	}
	if body.Context == nil {
		body.Context = map[string]any{}
	}

	result, err := s.flags.EvaluateFlag(r.Context(), projectID, body.FlagKey, body.Context)
	if err != nil {
		errorCode := "GENERAL"
		if strings.Contains(err.Error(), "flag not found") {
			errorCode = "FLAG_NOT_FOUND"
		}
		// FLAG_NOT_FOUND is normal client behaviour; everything else (DB
		// failure, JSON corruption) is a real server problem that was
		// previously hidden behind a 200 OK + reason=ERROR. Surface those.
		if errorCode == "FLAG_NOT_FOUND" {
			slog.DebugContext(r.Context(), "flag evaluate: not found",
				"project_id", projectID, "flag_key", body.FlagKey)
		} else {
			slog.ErrorContext(r.Context(), "flag evaluate failed",
				"err", err, "handled", false,
				"project_id", projectID,
				"flag_key", body.FlagKey,
				"error_code", errorCode,
				"request_id", RequestIDFromContext(r.Context()),
			)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"value":      body.DefaultValue,
			"variant":    "",
			"reason":     "ERROR",
			"flag_key":   body.FlagKey,
			"error_code": errorCode,
			"error":      err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFlagContextKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	suggestions, err := s.flags.ContextKeySuggestions(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleFlagContextKeys")
		return
	}
	if suggestions == nil {
		suggestions = []repository.ContextKeySuggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}
