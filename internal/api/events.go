package api

import (
	"net/http"
	"strconv"

	"github.com/wiebe-xyz/funnelbarn/internal/apierr"
)

// handleListEvents returns a paginated list of events for a project.
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id required"))
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

	events, err := s.events.ListEvents(r.Context(), projectID, limit, offset)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"limit":  limit,
		"offset": offset,
	})
}
