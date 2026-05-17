package api

import (
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func (s *Server) handleListSegments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	segs, err := s.segments.ListSegments(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleListSegments")
		return
	}
	if segs == nil {
		segs = []repository.Segment{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"segments": segs})
}

func (s *Server) handleCreateSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Name  string                   `json:"name"`
		Rules []repository.SegmentRule `json:"rules"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusUnprocessableEntity)
		return
	}
	if body.Rules == nil {
		body.Rules = []repository.SegmentRule{}
	}
	seg, err := s.segments.CreateSegment(r.Context(), projectID, body.Name, body.Rules)
	if err != nil {
		mapServiceError(w, err, "handleCreateSegment")
		return
	}
	writeJSON(w, http.StatusCreated, seg)
}

func (s *Server) handleUpdateSegment(w http.ResponseWriter, r *http.Request) {
	segID := r.PathValue("sid")
	if segID == "" {
		jsonError(w, "segment id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Name  string                   `json:"name"`
		Rules []repository.SegmentRule `json:"rules"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Rules == nil {
		body.Rules = []repository.SegmentRule{}
	}
	seg, err := s.segments.UpdateSegment(r.Context(), segID, body.Name, body.Rules)
	if err != nil {
		mapServiceError(w, err, "handleUpdateSegment")
		return
	}
	writeJSON(w, http.StatusOK, seg)
}

func (s *Server) handleDeleteSegment(w http.ResponseWriter, r *http.Request) {
	segID := r.PathValue("sid")
	if segID == "" {
		jsonError(w, "segment id required", http.StatusBadRequest)
		return
	}
	if err := s.segments.DeleteSegment(r.Context(), segID); err != nil {
		mapServiceError(w, err, "handleDeleteSegment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
