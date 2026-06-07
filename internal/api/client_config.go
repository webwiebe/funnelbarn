package api

import (
	"net/http"
	"strconv"
	"strings"
)

// handleClientConfig returns public client-side configuration for the frontend.
// No auth required — values are safe to expose (ingest keys are public by design).
func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	type oidcOut struct {
		Enabled  bool   `json:"enabled"`
		LoginURL string `json:"loginURL,omitempty"`
	}
	type iambarnOut struct {
		ProfileURL string `json:"profile_url,omitempty"`
	}
	type response struct {
		BugbarnEndpoint         string     `json:"bugbarn_endpoint"`
		BugbarnIngestKey        string     `json:"bugbarn_ingest_key"`
		BugbarnProject          string     `json:"bugbarn_project,omitempty"`
		FunnelbarnEndpoint      string     `json:"funnelbarn_endpoint,omitempty"`
		FunnelbarnAPIKey        string     `json:"funnelbarn_api_key,omitempty"`
		FunnelbarnProject       string     `json:"funnelbarn_project,omitempty"`
		FunnelbarnRecording     bool       `json:"funnelbarn_recording,omitempty"`
		FunnelbarnRecordingRate float64    `json:"funnelbarn_recording_rate,omitempty"`
		IAMBarnEnabled          bool       `json:"iambarn_enabled"`
		IAMBarn                 iambarnOut `json:"iambarn,omitempty"`
		OIDC                    oidcOut    `json:"oidc"`
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

	// Expose recording config when recording is enabled and settings are available.
	if s.recordings != nil && s.instanceSettings != nil {
		if settings, err := s.instanceSettings.GetAllInstanceSettings(r.Context()); err == nil {
			if settings["recording_enabled"] == "true" {
				resp.FunnelbarnRecording = true
				resp.FunnelbarnRecordingRate = 1.0
				if rv := settings["recording_sample_rate"]; rv != "" {
					if rate, err := strconv.ParseFloat(rv, 64); err == nil && rate >= 0 && rate <= 1 {
						resp.FunnelbarnRecordingRate = rate
					}
				}
			}
		}
	}

	if s.oidc != nil {
		resp.OIDC = oidcOut{Enabled: true, LoginURL: "/api/v1/oidc/login"}
	}
	if issuer := s.iambarnIssuer(); issuer != "" {
		resp.IAMBarn.ProfileURL = strings.TrimRight(issuer, "/") + "/admin#profile"
	}
	writeJSON(w, http.StatusOK, resp)
}

// iambarnIssuer returns the configured iambarn issuer URL from either the
// new confidential-client OIDC config or the legacy IAMBarn PKCE provider.
// Returns "" when neither path is configured.
func (s *Server) iambarnIssuer() string {
	if s.oidc != nil {
		if iss := s.oidc.Config().Issuer; iss != "" {
			return iss
		}
	}
	if s.iambarnProvider != nil {
		return s.iambarnProvider.Issuer()
	}
	return ""
}
