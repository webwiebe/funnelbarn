package api

import "net/http"

// handleKeyCheck answers whether the x-funnelbarn-api-key header would be accepted by
// POST /api/v1/events: 204 when it is, 401 when it is missing, unknown or
// revoked. It stores nothing, so a deploy check can call it with a site's
// public ingest key and fail when FunnelBarn rejects that key.
func (s *Server) handleKeyCheck(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.ingest.APIKeyProjectScope(r); !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
