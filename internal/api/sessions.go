package api

import (
	"net/http"
	"strconv"
)

// handleActiveSessionCount returns the number of sessions active in the last 5 minutes.
func (s *Server) handleActiveSessionCount(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	count, err := s.store.ActiveSessionCount(r.Context(), projectID, 5)
	if err != nil {
		jsonError(w, "failed to count sessions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active_sessions": count, "window_minutes": 5})
}

// handleListSessions returns paginated sessions for a project.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
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

	sessions, err := s.store.ListSessions(r.Context(), projectID, limit, offset)
	if err != nil {
		jsonError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"limit":    limit,
		"offset":   offset,
	})
}
