package api

import "net/http"

// handleClientConfig returns public client-side configuration for the frontend.
// No auth required — values are safe to expose (ingest keys are public by design).
func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	type oidcOut struct {
		Enabled  bool   `json:"enabled"`
		LoginURL string `json:"loginURL,omitempty"`
	}
	type response struct {
		BugbarnEndpoint    string  `json:"bugbarn_endpoint"`
		BugbarnIngestKey   string  `json:"bugbarn_ingest_key"`
		BugbarnProject     string  `json:"bugbarn_project,omitempty"`
		FunnelbarnEndpoint string  `json:"funnelbarn_endpoint,omitempty"`
		FunnelbarnAPIKey   string  `json:"funnelbarn_api_key,omitempty"`
		FunnelbarnProject  string  `json:"funnelbarn_project,omitempty"`
		IAMBarnEnabled     bool    `json:"iambarn_enabled"`
		OIDC               oidcOut `json:"oidc"`
	}
	resp := response{
		BugbarnEndpoint:    s.bugbarnEndpoint,
		BugbarnIngestKey:   s.bugbarnIngestKey,
		BugbarnProject:     s.bugbarnProject,
		FunnelbarnEndpoint: s.publicURL,
		FunnelbarnAPIKey:   s.dogfoodAPIKey,
		FunnelbarnProject:  s.dogfoodProject,
		IAMBarnEnabled:     s.iambarnFlagEnabled(r.Context(), map[string]any{"user_agent": r.Header.Get("User-Agent")}),
	}
	if s.oidc != nil {
		resp.OIDC = oidcOut{Enabled: true, LoginURL: "/api/v1/oidc/login"}
	}
	writeJSON(w, http.StatusOK, resp)
}
