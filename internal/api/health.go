package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if s.db != nil {
		if err := s.db.Ping(ctx); err != nil {
			slog.ErrorContext(ctx, "health check db ping failed", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unhealthy",
				"error":  "database unavailable",
				"time":   time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": s.version,
	})
}

func (s *Server) handleGetProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	health, err := s.projectHealth.GetProjectHealth(r.Context(), projectID)
	if err != nil {
		slog.ErrorContext(r.Context(), "get project health", "project_id", projectID, "err", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (s *Server) handleResetProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	if err := s.projectHealth.ResetProjectHealth(r.Context(), projectID); err != nil {
		slog.ErrorContext(r.Context(), "reset project health", "project_id", projectID, "err", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
