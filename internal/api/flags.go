package api

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
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

	flag, err := s.flags.CreateFlag(r.Context(), repository.FeatureFlag{
		ProjectID:       projectID,
		FlagKey:         body.FlagKey,
		Name:            body.Name,
		FlagType:        body.FlagType,
		Variants:        body.Variants,
		DefaultVariant:  body.DefaultVariant,
		Split:           body.Split,
		ConversionEvent: body.ConversionEvent,
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
		Status          string `json:"status"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	flag, err := s.flags.UpdateFlag(r.Context(), repository.FeatureFlag{
		ID:              flagID,
		Name:            body.Name,
		FlagType:        body.FlagType,
		Variants:        body.Variants,
		DefaultVariant:  body.DefaultVariant,
		Split:           body.Split,
		ConversionEvent: body.ConversionEvent,
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

	var body struct {
		FlagKey      string            `json:"flag_key"`
		DefaultValue any               `json:"default_value"`
		Context      map[string]string `json:"context"`
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
		body.Context = map[string]string{}
	}

	ctx, span := tracing.StartSpan(r.Context(), "flags.evaluate.handler",
		attribute.String("flag.key", body.FlagKey),
		attribute.String("project.id", projectID),
	)
	defer span.End()

	result, err := s.flags.EvaluateFlag(ctx, projectID, body.FlagKey, body.Context)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"value":    body.DefaultValue,
			"variant":  "",
			"reason":   "ERROR",
			"flag_key": body.FlagKey,
			"error":    err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
