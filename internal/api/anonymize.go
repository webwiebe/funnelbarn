package api

import (
	"net"
	"net/http"
)

func (s *Server) handleAnonymizeGeo(w http.ResponseWriter, r *http.Request) {
	if s.geoAnonymizer == nil {
		jsonError(w, "geo anonymization unavailable", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
		IP        string `json:"ip"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.SessionID == "" && body.IP == "" {
		jsonError(w, "session_id or ip is required", http.StatusUnprocessableEntity)
		return
	}

	var anonymized int64

	if body.SessionID != "" {
		if err := s.geoAnonymizer.AnonymizeSessionGeo(r.Context(), body.SessionID); err != nil {
			mapServiceError(w, err, "handleAnonymizeGeo.session")
			return
		}
		anonymized++
	}

	if body.IP != "" {
		if net.ParseIP(body.IP) == nil {
			jsonError(w, "invalid ip address", http.StatusUnprocessableEntity)
			return
		}
		n, err := s.geoAnonymizer.AnonymizeSessionsByIP(r.Context(), body.IP)
		if err != nil {
			mapServiceError(w, err, "handleAnonymizeGeo.ip")
			return
		}
		anonymized += n
	}

	writeJSON(w, http.StatusOK, map[string]any{"anonymized": anonymized})
}
