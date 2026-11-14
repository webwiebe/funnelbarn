package api

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// ---- Per-project mappings ----

func (s *Server) handleListMappings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.list",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	mappings, err := s.overview.ListMappings(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleListMappings")
		return
	}
	if mappings == nil {
		mappings = []domain.EventNameMapping{}
	}
	span.SetAttributes(attribute.Int("mappings.count", len(mappings)))
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

func (s *Server) handleSetMappings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Mappings []domain.EventNameMapping `json:"mappings"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.set",
		attribute.String("project.id", projectID),
		attribute.Int("mappings.count", len(body.Mappings)),
	)
	defer span.End()

	if err := s.overview.SetMappings(ctx, projectID, body.Mappings); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleSetMappings")
		return
	}
	// Return the fresh mapping set.
	mappings, err := s.overview.ListMappings(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleSetMappings.list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

func (s *Server) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	raw := r.PathValue("raw")
	if projectID == "" || raw == "" {
		jsonError(w, "project id and raw name required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.delete",
		attribute.String("project.id", projectID),
		attribute.String("mapping.raw", raw),
	)
	defer span.End()

	if err := s.overview.DeleteMapping(ctx, projectID, raw); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleDeleteMapping")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMappingSuggestions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.suggestions",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	suggestions, err := s.overview.MappingSuggestions(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleMappingSuggestions")
		return
	}
	if suggestions == nil {
		suggestions = []domain.MappingSuggestion{}
	}
	span.SetAttributes(attribute.Int("suggestions.count", len(suggestions)))
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}
