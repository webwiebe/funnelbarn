package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
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
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "funnel not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to load funnel", http.StatusInternalServerError)
		return
	}
	if existing.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
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

	funnel := repository.Funnel{
		ID:          funnelID,
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Steps:       body.Steps,
	}

	updated, err := s.funnels.UpdateFunnel(r.Context(), funnel)
	if err != nil {
		jsonError(w, "failed to update funnel", http.StatusInternalServerError)
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
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "funnel not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to load funnel", http.StatusInternalServerError)
		return
	}
	if existing.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	if err := s.funnels.DeleteFunnel(r.Context(), funnelID); err != nil {
		jsonError(w, "failed to delete funnel", http.StatusInternalServerError)
		return
	}

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
		jsonError(w, "failed to list funnels", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"funnels": funnels})
}

// handleCreateFunnel creates a new funnel with steps.
func (s *Server) handleCreateFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
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

	funnel := repository.Funnel{
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Steps:       body.Steps,
	}

	created, err := s.funnels.CreateFunnel(r.Context(), funnel)
	if err != nil {
		jsonError(w, "failed to create funnel", http.StatusInternalServerError)
		return
	}

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
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "funnel not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to load funnel", http.StatusInternalServerError)
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

	// Parse optional segment filter.
	seg := parseSegment(r.URL.Query().Get("segment"))

	results, err := s.funnels.AnalyzeFunnel(r.Context(), funnel, from, to, seg)
	if err != nil {
		jsonError(w, "failed to analyze funnel", http.StatusInternalServerError)
		return
	}

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
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "funnel not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to load funnel", http.StatusInternalServerError)
		return
	}
	if funnel.ProjectID != projectID {
		jsonError(w, "funnel not found", http.StatusNotFound)
		return
	}

	segs, err := s.funnels.FunnelSegmentData(r.Context(), projectID)
	if err != nil {
		jsonError(w, "failed to load segment data", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, segs)
}
