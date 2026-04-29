package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

// handleListFunnels returns all funnels for a project.
func (s *Server) handleListFunnels(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	funnels, err := s.store.ListFunnels(r.Context(), projectID)
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
		Name        string               `json:"name"`
		Description string               `json:"description"`
		Steps       []storage.FunnelStep `json:"steps"`
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

	funnel := storage.Funnel{
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Steps:       body.Steps,
	}

	created, err := s.store.CreateFunnel(r.Context(), funnel)
	if err != nil {
		jsonError(w, "failed to create funnel", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// parseSegment maps a preset segment name to a SegmentFilter.
// Returns nil when name is "all" or empty (no filter).
func parseSegment(name string) *storage.SegmentFilter {
	switch name {
	case "logged_in":
		return &storage.SegmentFilter{Field: "user_id_hash", Op: "is_not_null"}
	case "not_logged_in":
		return &storage.SegmentFilter{Field: "user_id_hash", Op: "is_null"}
	case "mobile":
		return &storage.SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}
	case "desktop":
		return &storage.SegmentFilter{Field: "device_type", Op: "eq", Value: "desktop"}
	case "tablet":
		return &storage.SegmentFilter{Field: "device_type", Op: "eq", Value: "tablet"}
	case "new_visitor":
		return &storage.SegmentFilter{Field: "session_returning", Op: "eq", Value: "false"}
	case "returning":
		return &storage.SegmentFilter{Field: "session_returning", Op: "eq", Value: "true"}
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

	funnel, err := s.store.FunnelByID(r.Context(), funnelID)
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

	results, err := s.store.AnalyzeFunnel(r.Context(), funnel, from, to, seg)
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
	funnel, err := s.store.FunnelByID(r.Context(), funnelID)
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

	segs, err := s.store.FunnelSegmentData(r.Context(), projectID)
	if err != nil {
		jsonError(w, "failed to load segment data", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, segs)
}
