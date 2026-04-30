package api

import (
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/apierr"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
)

// handleUpdateFunnel updates a funnel's name and steps.
func (s *Server) handleUpdateFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id and funnel id required"))
		return
	}

	existing, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.MapDB(err, "funnel not found"))
		return
	}
	if existing.ProjectID != projectID {
		apierr.WriteHTTP(w, apierr.NotFound("funnel not found"))
		return
	}

	var body struct {
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Steps       []repository.FunnelStep `json:"steps"`
	}
	if err := readJSON(r, &body); err != nil {
		apierr.WriteHTTP(w, apierr.BadRequest("invalid request body"))
		return
	}
	if body.Name == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("name is required"))
		return
	}
	if len(body.Steps) == 0 {
		apierr.WriteHTTP(w, apierr.BadRequest("at least one step is required"))
		return
	}

	updated, err := s.funnels.UpdateFunnel(r.Context(), repository.Funnel{
		ID:          funnelID,
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Steps:       body.Steps,
	})
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteFunnel deletes a funnel and its steps.
func (s *Server) handleDeleteFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id and funnel id required"))
		return
	}

	existing, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.MapDB(err, "funnel not found"))
		return
	}
	if existing.ProjectID != projectID {
		apierr.WriteHTTP(w, apierr.NotFound("funnel not found"))
		return
	}

	if err := s.funnels.DeleteFunnel(r.Context(), funnelID); err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListFunnels returns all funnels for a project.
func (s *Server) handleListFunnels(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id required"))
		return
	}

	funnels, err := s.funnels.ListFunnels(r.Context(), projectID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"funnels": funnels})
}

// handleCreateFunnel creates a new funnel with steps.
func (s *Server) handleCreateFunnel(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id required"))
		return
	}

	var body struct {
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Steps       []repository.FunnelStep `json:"steps"`
	}
	if err := readJSON(r, &body); err != nil {
		apierr.WriteHTTP(w, apierr.BadRequest("invalid request body"))
		return
	}
	if body.Name == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("name is required"))
		return
	}
	if len(body.Steps) == 0 {
		apierr.WriteHTTP(w, apierr.BadRequest("at least one step is required"))
		return
	}

	created, err := s.funnels.CreateFunnel(r.Context(), repository.Funnel{
		ProjectID:   projectID,
		Name:        body.Name,
		Description: body.Description,
		Steps:       body.Steps,
	})
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleFunnelAnalysis runs funnel analysis and returns step conversion rates.
func (s *Server) handleFunnelAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id and funnel id required"))
		return
	}

	funnel, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.MapDB(err, "funnel not found"))
		return
	}
	if funnel.ProjectID != projectID {
		apierr.WriteHTTP(w, apierr.NotFound("funnel not found"))
		return
	}

	tr := timerange.Parse(r.URL.Query())
	seg := service.ParseSegment(r.URL.Query().Get("segment"))

	results, err := s.funnels.AnalyzeFunnel(r.Context(), funnel, tr.From, tr.To, seg)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"funnel":  funnel,
		"results": results,
		"from":    tr.From.Format("2006-01-02T15:04:05Z07:00"),
		"to":      tr.To.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// handleFunnelSegments returns available segment values for a funnel's project data.
func (s *Server) handleFunnelSegments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	if projectID == "" || funnelID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id and funnel id required"))
		return
	}

	funnel, err := s.funnels.GetFunnel(r.Context(), funnelID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.MapDB(err, "funnel not found"))
		return
	}
	if funnel.ProjectID != projectID {
		apierr.WriteHTTP(w, apierr.NotFound("funnel not found"))
		return
	}

	segs, err := s.funnels.FunnelSegmentData(r.Context(), projectID)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusOK, segs)
}
