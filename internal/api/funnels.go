package api

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handleUpdateFunnel updates a funnel's name and steps.
func (s *Server) handleUpdateFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		jsonError(w, "project id and funnel id required", http.StatusBadRequest)
		return
	}

	// Verify funnel belongs to project.
	existing, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		mapServiceError(w, err, "handleUpdateFunnel.getFunnel")
		return
	}
	if existing.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Scope       string                  `json:"scope"`
		Steps       []repository.FunnelStep `json:"steps"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(body.Steps) == 0 {
		jsonError(w, "at least one step is required", http.StatusBadRequest)
		return
	}
	if body.Scope == "" {
		body.Scope = existing.Scope
	}

	funnel := repository.Funnel{
		ID:          funnelID,
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Scope:       body.Scope,
		Steps:       body.Steps,
	}

	ctx, span := tracing.StartSpan(r.Context(), "funnels.update",
		attribute.String("project.id", projectID),
		attribute.String("funnel.id", funnelID),
		attribute.Int("funnel.step_count", len(body.Steps)),
	)
	defer span.End()

	updated, err := s.funnels.UpdateFunnel(ctx, funnel)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleUpdateFunnel")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteFunnel deletes a funnel and its steps.
func (s *Server) handleDeleteFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		jsonError(w, "project id and funnel id required", http.StatusBadRequest)
		return
	}

	// Verify funnel belongs to project.
	existing, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		mapServiceError(w, err, "handleDeleteFunnel.getFunnel")
		return
	}
	if existing.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	if err := s.funnels.DeleteFunnel(r.Context(), funnelID); err != nil {
		mapServiceError(w, err, "handleDeleteFunnel")
		return
	}

	slog.InfoContext(r.Context(), "funnel deleted", "funnel_id", funnelID, "project_id", projectID, "request_id", RequestIDFromContext(r.Context()))
	w.WriteHeader(http.StatusNoContent)
}

// handleListFunnels returns all funnels for a project.
func (s *Server) handleListFunnels(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	funnels, err := s.funnels.ListFunnels(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleListFunnels")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"funnels":     funnels,
		"total_count": len(funnels),
	})
}

// handleCreateFunnel creates a new funnel with steps.
func (s *Server) handleCreateFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Scope       string                  `json:"scope"`
		Steps       []repository.FunnelStep `json:"steps"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Scope == "" {
		body.Scope = "session"
	}

	funnel := repository.Funnel{
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Scope:       body.Scope,
		Steps:       body.Steps,
	}

	ctx, span := tracing.StartSpan(r.Context(), "funnels.create",
		attribute.String("project.id", projectID),
		attribute.Int("funnel.step_count", len(body.Steps)),
	)
	defer span.End()

	created, err := s.funnels.CreateFunnel(ctx, funnel)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleCreateFunnel")
		return
	}

	span.SetAttributes(attribute.String("funnel.id", created.ID))
	slog.InfoContext(ctx, "funnel created", "funnel_id", created.ID, "project_id", projectID, "request_id", RequestIDFromContext(ctx))
	writeJSON(w, http.StatusCreated, created)
}

// parseSegment maps a preset segment name to a SegmentFilter.
// Returns nil when name is "all" or empty (no filter).
func parseSegment(name string) *repository.SegmentFilter {
	switch name {
	case "logged_in":
		return &repository.SegmentFilter{Field: "user_id_hash", Op: "is_not_null"}
	case "not_logged_in":
		return &repository.SegmentFilter{Field: "user_id_hash", Op: "is_null"}
	case "mobile":
		return &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}
	case "desktop":
		return &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "desktop"}
	case "tablet":
		return &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "tablet"}
	case "new_visitor":
		return &repository.SegmentFilter{Field: "session_returning", Op: "eq", Value: "false"}
	case "returning":
		return &repository.SegmentFilter{Field: "session_returning", Op: "eq", Value: "true"}
	default:
		return nil
	}
}

// handleFunnelAnalysis runs funnel analysis and returns step conversion rates.
func (s *Server) handleFunnelAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		jsonError(w, "project id and funnel id required", http.StatusBadRequest)
		return
	}

	funnel, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		mapServiceError(w, err, "handleFunnelAnalysis.getFunnel")
		return
	}
	if funnel.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	// Parse time range from query params (default: last 30 days).
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
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

	// Parse optional preset segment filter.
	seg := parseSegment(r.URL.Query().Get("segment"))

	// Look up stored segment rules if segment_id provided.
	var segRules []repository.SegmentRule
	if segID := r.URL.Query().Get("segment_id"); segID != "" && s.segments != nil {
		stored, err := s.segments.GetSegment(r.Context(), segID)
		if err != nil {
			// Non-fatal: analyse without the segment filter. Warn so a
			// broken UI dropdown (e.g. stale segment_id) is visible.
			slog.WarnContext(r.Context(), "funnel: segment lookup failed",
				"err", err, "handled", true,
				"segment_id", segID, "project_id", projectID)
		} else if stored.ProjectID == projectID {
			segRules = stored.Rules
		}
	}

	ctx, span := tracing.StartSpan(r.Context(), "funnels.analyze",
		attribute.String("project.id", projectID),
		attribute.String("funnel.id", funnelID),
		attribute.Int("funnel.step_count", len(funnel.Steps)),
		attribute.String("funnel.segment", r.URL.Query().Get("segment")),
	)
	defer span.End()

	results, err := s.funnels.AnalyzeFunnel(ctx, funnel, from, to, seg, segRules...)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleFunnelAnalysis")
		return
	}

	span.SetAttributes(attribute.Int("funnel.result_count", len(results)))

	writeJSON(w, http.StatusOK, map[string]any{
		"funnel":  funnel,
		"results": results,
		"from":    from.Format(time.RFC3339),
		"to":      to.Format(time.RFC3339),
	})
}

// handleFunnelSegments returns available segment values for a funnel's project data.
func (s *Server) handleFunnelSegments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		jsonError(w, "project id and funnel id required", http.StatusBadRequest)
		return
	}

	// Verify funnel belongs to project.
	funnel, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		mapServiceError(w, err, "handleFunnelSegments.getFunnel")
		return
	}
	if funnel.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "funnels.segmentData",
		attribute.String("project.id", projectID),
		attribute.String("funnel.id", funnelID),
	)
	defer span.End()

	segs, err := s.funnels.FunnelSegmentData(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleFunnelSegments")
		return
	}

	writeJSON(w, http.StatusOK, segs)
}
