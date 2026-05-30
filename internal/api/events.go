package api

import (
	"fmt"
	"net/http"
	"strconv"
)

// addPaginationHeaders sets a Link header for the next page when more results
// may be available (i.e. the returned count equals the page limit).
func addPaginationHeaders(w http.ResponseWriter, r *http.Request, limit, offset, count int) {
	if count < limit {
		return // no next page
	}
	nextOffset := offset + limit
	q := r.URL.Query()
	q.Set("offset", strconv.Itoa(nextOffset))
	q.Set("limit", strconv.Itoa(limit))
	next := r.URL.Path + "?" + q.Encode()
	w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
}

// handleEventNames returns distinct event names for a project (autocomplete).
func (s *Server) handleEventNames(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	names, err := s.events.DistinctEventNames(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleEventNames")
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"event_names": names})
}

// handleEventProperties returns distinct JSON property keys for a given event name.
func (s *Server) handleEventProperties(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	eventName := r.URL.Query().Get("event_name")
	if eventName == "" {
		jsonError(w, "event_name query parameter required", http.StatusBadRequest)
		return
	}

	props, err := s.events.DistinctEventProperties(r.Context(), projectID, eventName)
	if err != nil {
		mapServiceError(w, err, "handleEventProperties")
		return
	}
	if props == nil {
		props = []string{}
	}
	populated, err := s.events.PopulatedMetadataColumns(r.Context(), projectID, eventName)
	if err != nil {
		mapServiceError(w, err, "handleEventProperties.metadata")
		return
	}
	all := append(populated, props...)
	writeJSON(w, http.StatusOK, map[string]any{"properties": all})
}

// handleEventPropertyValues returns distinct values for a property key on a given event name.
func (s *Server) handleEventPropertyValues(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	eventName := r.URL.Query().Get("event_name")
	if eventName == "" {
		jsonError(w, "event_name query parameter required", http.StatusBadRequest)
		return
	}
	property := r.URL.Query().Get("property")
	if property == "" {
		jsonError(w, "property query parameter required", http.StatusBadRequest)
		return
	}

	vals, err := s.events.DistinctPropertyValues(r.Context(), projectID, eventName, property, 50)
	if err != nil {
		mapServiceError(w, err, "handleEventPropertyValues")
		return
	}
	if vals == nil {
		vals = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"values": vals})
}

// handleEnvironments returns the distinct canonical environment values recorded for a project.
func (s *Server) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	envs, err := s.events.DistinctEnvironments(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleEnvironments")
		return
	}
	if envs == nil {
		envs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": envs})
}

// handleListEvents returns a paginated list of events for a project.
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
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

	events, err := s.events.ListEvents(r.Context(), projectID, limit, offset)
	if err != nil {
		mapServiceError(w, err, "handleListEvents")
		return
	}

	addPaginationHeaders(w, r, limit, offset, len(events))
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"limit":  limit,
		"offset": offset,
	})
}
