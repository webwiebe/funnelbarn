package api

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handleActiveSessionCount returns the number of sessions active in the last 5 minutes.
func (s *Server) handleActiveSessionCount(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	env, ok := s.environmentParam(w, r, projectID, "handleActiveSessionCount")
	if !ok {
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "sessions.active_count",
		attribute.String("project.id", projectID),
		attribute.Int("window.minutes", 5),
	)
	defer span.End()

	count, err := s.sessions.ActiveSessionCount(ctx, projectID, env, 5)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleActiveSessionCount")
		return
	}
	span.SetAttributes(attribute.Int64("sessions.active_count", count))
	writeJSON(w, http.StatusOK, map[string]any{"active_sessions": count, "window_minutes": 5})
}

// handleSessionDistributions returns project-wide value distributions for segment fields.
func (s *Server) handleSessionDistributions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	env, ok := s.environmentParam(w, r, projectID, "handleSessionDistributions")
	if !ok {
		return
	}
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, map[string]any{"distributions": map[string]any{}})
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "sessions.distributions",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	dist, err := s.sessions.SessionDistributions(ctx, projectID, env)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleSessionDistributions")
		return
	}
	span.SetAttributes(attribute.Int("distributions.fields.count", len(dist)))
	writeJSON(w, http.StatusOK, map[string]any{"distributions": dist})
}

// handleListSessions returns paginated sessions for a project.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	env, ok := s.environmentParam(w, r, projectID, "handleListSessions")
	if !ok {
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	ctx, span := tracing.StartSpan(r.Context(), "sessions.list",
		attribute.String("project.id", projectID),
		attribute.Int("limit", limit),
		attribute.Int("offset", offset),
	)
	defer span.End()

	sessions, err := s.sessions.ListSessions(ctx, projectID, env, limit, offset)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleListSessions")
		return
	}
	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))

	addPaginationHeaders(w, r, limit, offset, len(sessions))
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"limit":    limit,
		"offset":   offset,
	})
}
