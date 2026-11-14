package api

import (
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ---- Canonical event catalog ----

func (s *Server) handleListCanonicalEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.overview.ListCanonicalEvents(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleListCanonicalEvents")
		return
	}
	if events == nil {
		events = []repository.CanonicalEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"canonical_events": events})
}

func (s *Server) handleCreateCanonicalEvent(w http.ResponseWriter, r *http.Request) {
	var body repository.CanonicalEvent
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := s.overview.CreateCanonicalEvent(r.Context(), body)
	if err != nil {
		mapServiceError(w, err, "handleCreateCanonicalEvent")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateCanonicalEvent(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, "key required", http.StatusBadRequest)
		return
	}
	var body repository.CanonicalEvent
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	body.Key = key
	updated, err := s.overview.UpdateCanonicalEvent(r.Context(), body)
	if err != nil {
		mapServiceError(w, err, "handleUpdateCanonicalEvent")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteCanonicalEvent(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, "key required", http.StatusBadRequest)
		return
	}
	if err := s.overview.DeleteCanonicalEvent(r.Context(), key); err != nil {
		mapServiceError(w, err, "handleDeleteCanonicalEvent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
