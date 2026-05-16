package api

import "net/http"

// handleClientConfig returns public client-side configuration for the frontend.
// No auth required — values are safe to expose (ingest keys are public by design).
func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	type response struct {
		BugbarnEndpoint    string `json:"bugbarn_endpoint"`
		BugbarnIngestKey   string `json:"bugbarn_ingest_key"`
		BugbarnProject     string `json:"bugbarn_project,omitempty"`
		FunnelbarnEndpoint string `json:"funnelbarn_endpoint,omitempty"`
		FunnelbarnAPIKey   string `json:"funnelbarn_api_key,omitempty"`
		FunnelbarnProject  string `json:"funnelbarn_project,omitempty"`
		IAMBarnEnabled     bool   `json:"iambarn_enabled"`
	}
	writeJSON(w, http.StatusOK, response{
		BugbarnEndpoint:    s.bugbarnEndpoint,
		BugbarnIngestKey:   s.bugbarnIngestKey,
		BugbarnProject:     s.bugbarnProject,
		FunnelbarnEndpoint: s.publicURL,
		FunnelbarnAPIKey:   s.dogfoodAPIKey,
		FunnelbarnProject:  s.dogfoodProject,
		IAMBarnEnabled:     s.iambarnFlagEnabled(r.Context(), map[string]any{"user_agent": r.Header.Get("User-Agent")}),
	})
}
