package api

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

func (s *Server) handleListSegments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "segments.list",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	segs, err := s.segments.ListSegments(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleListSegments")
		return
	}
	if segs == nil {
		segs = []repository.Segment{}
	}
	span.SetAttributes(attribute.Int("segments.count", len(segs)))
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

	ctx, span := tracing.StartSpan(r.Context(), "segments.create",
		attribute.String("project.id", projectID),
		attribute.String("segment.name", body.Name),
		attribute.Int("segment.rules.count", len(body.Rules)),
	)
	defer span.End()

	seg, err := s.segments.CreateSegment(ctx, projectID, body.Name, body.Rules)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleCreateSegment")
		return
	}
	span.SetAttributes(attribute.String("segment.id", seg.ID))
	writeJSON(w, http.StatusCreated, seg)
}

func (s *Server) handleUpdateSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	segID := r.PathValue("sid")
	if projectID == "" || segID == "" {
		jsonError(w, "project id and segment id required", http.StatusBadRequest)
		return
	}

	// Verify the segment belongs to this project.
	existing, err := s.segments.GetSegment(r.Context(), segID)
	if err != nil {
		mapServiceError(w, err, "handleUpdateSegment.getSegment")
		return
	}
	if existing.ProjectID != projectID {
		jsonError(w, "segment not found", http.StatusNotFound)
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

	ctx, span := tracing.StartSpan(r.Context(), "segments.update",
		attribute.String("project.id", projectID),
		attribute.String("segment.id", segID),
		attribute.Int("segment.rules.count", len(body.Rules)),
	)
	defer span.End()

	seg, err := s.segments.UpdateSegment(ctx, segID, body.Name, body.Rules)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleUpdateSegment")
		return
	}
	writeJSON(w, http.StatusOK, seg)
}

func (s *Server) handleDeleteSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	segID := r.PathValue("sid")
	if projectID == "" || segID == "" {
		jsonError(w, "project id and segment id required", http.StatusBadRequest)
		return
	}

	// Verify the segment belongs to this project.
	existing, err := s.segments.GetSegment(r.Context(), segID)
	if err != nil {
		mapServiceError(w, err, "handleDeleteSegment.getSegment")
		return
	}
	if existing.ProjectID != projectID {
		jsonError(w, "segment not found", http.StatusNotFound)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "segments.delete",
		attribute.String("project.id", projectID),
		attribute.String("segment.id", segID),
	)
	defer span.End()

	if err := s.segments.DeleteSegment(ctx, segID); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleDeleteSegment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
