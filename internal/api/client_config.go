package api

import "net/http"

// handleClientConfig returns public client-side configuration for the frontend.
// No auth required — values are safe to expose (ingest keys are public by design).
func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	type response struct {
		BugbarnEndpoint  string `json:"bugbarn_endpoint"`
		BugbarnIngestKey string `json:"bugbarn_ingest_key"`
	}
	writeJSON(w, http.StatusOK, response{
		BugbarnEndpoint:  s.bugbarnEndpoint,
		BugbarnIngestKey: s.bugbarnIngestKey,
	})
}
