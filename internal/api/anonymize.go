package api

import (
	"net"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
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

	ctx, span := tracing.StartSpan(r.Context(), "anonymize.geo",
		attribute.Bool("anonymize.by_session", body.SessionID != ""),
		attribute.Bool("anonymize.by_ip", body.IP != ""),
	)
	defer span.End()

	var anonymized int64

	if body.SessionID != "" {
		if err := s.geoAnonymizer.AnonymizeSessionGeo(ctx, body.SessionID); err != nil {
			tracing.RecordError(span, err)
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
		n, err := s.geoAnonymizer.AnonymizeSessionsByIP(ctx, body.IP)
		if err != nil {
			tracing.RecordError(span, err)
			mapServiceError(w, err, "handleAnonymizeGeo.ip")
			return
		}
		anonymized += n
	}

	span.SetAttributes(attribute.Int64("anonymize.session_count", anonymized))
	writeJSON(w, http.StatusOK, map[string]any{"anonymized": anonymized})
}
