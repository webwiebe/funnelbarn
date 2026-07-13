package api

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

func (s *Server) handleGetInstanceSettings(w http.ResponseWriter, r *http.Request) {
	if s.instanceSettings == nil {
		writeJSON(w, http.StatusOK, map[string]any{"settings": map[string]string{}})
		return
	}
	settings, err := s.instanceSettings.GetAllInstanceSettings(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleGetInstanceSettings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handlePutInstanceSettings(w http.ResponseWriter, r *http.Request) {
	if s.instanceSettings == nil {
		jsonError(w, "instance settings unavailable", http.StatusServiceUnavailable)
		return
	}
	var body map[string]string
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "instance_settings.update",
		attribute.Int("instance_settings.key_count", len(body)),
	)
	defer span.End()

	for key, value := range body {
		if err := s.instanceSettings.SetInstanceSetting(ctx, key, value); err != nil {
			tracing.RecordError(span, err)
			mapServiceError(w, err, "handlePutInstanceSettings")
			return
		}
	}
	settings, err := s.instanceSettings.GetAllInstanceSettings(ctx)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handlePutInstanceSettings.get")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}
