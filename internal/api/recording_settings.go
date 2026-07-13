package api

import (
	"context"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// ProjectRecordingSettingsRepo is the narrow repo interface for per-project recording settings.
type ProjectRecordingSettingsRepo interface {
	GetProjectRecordingSettings(ctx context.Context, projectID string) (*repository.ProjectRecordingSettings, error)
	UpsertProjectRecordingSettings(ctx context.Context, settings *repository.ProjectRecordingSettings) error
}

// handleGetProjectRecordingSettings returns the per-project recording settings.
// GET /api/v1/projects/{id}/recording-settings
func (s *Server) handleGetProjectRecordingSettings(w http.ResponseWriter, r *http.Request) {
	if s.recordingSettings == nil {
		jsonError(w, "recording settings not available", http.StatusServiceUnavailable)
		return
	}
	projectID := r.PathValue("id")

	ctx, span := tracing.StartSpan(r.Context(), "recording_settings.get",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	settings, err := s.recordingSettings.GetProjectRecordingSettings(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleGetProjectRecordingSettings")
		return
	}

	effective := s.resolveEffectiveRecordingConfig(r.WithContext(ctx), settings)
	span.SetAttributes(
		attribute.Bool("recording.effective_enabled", effective.Enabled),
		attribute.Float64("recording.effective_rate", effective.SampleRate),
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":           settings.Enabled,
		"sample_rate":       settings.SampleRate,
		"rules":             settings.Rules,
		"effective_enabled": effective.Enabled,
		"effective_rate":    effective.SampleRate,
	})
}

// handleUpdateProjectRecordingSettings saves per-project recording settings.
// PUT /api/v1/projects/{id}/recording-settings
func (s *Server) handleUpdateProjectRecordingSettings(w http.ResponseWriter, r *http.Request) {
	if s.recordingSettings == nil {
		jsonError(w, "recording settings not available", http.StatusServiceUnavailable)
		return
	}
	projectID := r.PathValue("id")

	var body struct {
		Enabled    *bool                      `json:"enabled"`
		SampleRate *float64                   `json:"sample_rate"`
		Rules      []repository.RecordingRule `json:"rules"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	if body.SampleRate != nil && (*body.SampleRate < 0 || *body.SampleRate > 1) {
		jsonError(w, "sample_rate must be between 0 and 1", http.StatusBadRequest)
		return
	}
	for _, rule := range body.Rules {
		if rule.Action != "capture" && rule.Action != "ignore" {
			jsonError(w, "rule action must be 'capture' or 'ignore'", http.StatusBadRequest)
			return
		}
		if rule.Pattern == "" {
			jsonError(w, "rule pattern must not be empty", http.StatusBadRequest)
			return
		}
	}
	if body.Rules == nil {
		body.Rules = []repository.RecordingRule{}
	}

	ctx, span := tracing.StartSpan(r.Context(), "recording_settings.update",
		attribute.String("project.id", projectID),
		attribute.Int("recording.rules.count", len(body.Rules)),
	)
	defer span.End()
	if body.Enabled != nil {
		span.SetAttributes(attribute.Bool("recording.enabled", *body.Enabled))
	}
	if body.SampleRate != nil {
		span.SetAttributes(attribute.Float64("recording.sample_rate", *body.SampleRate))
	}

	settings := &repository.ProjectRecordingSettings{
		ProjectID:  projectID,
		Enabled:    body.Enabled,
		SampleRate: body.SampleRate,
		Rules:      body.Rules,
	}
	if err := s.recordingSettings.UpsertProjectRecordingSettings(ctx, settings); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleUpdateProjectRecordingSettings")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// effectiveRecordingConfig is the resolved recording config for a project.
type effectiveRecordingConfig struct {
	Enabled    bool
	SampleRate float64
	Rules      []repository.RecordingRule
}

// resolveEffectiveRecordingConfig merges per-project settings with instance defaults.
func (s *Server) resolveEffectiveRecordingConfig(r *http.Request, settings *repository.ProjectRecordingSettings) effectiveRecordingConfig {
	cfg := effectiveRecordingConfig{
		Enabled:    false,
		SampleRate: 1.0,
	}

	// Apply instance-level defaults first.
	if s.instanceSettings != nil {
		if all, err := s.instanceSettings.GetAllInstanceSettings(r.Context()); err == nil {
			cfg.Enabled = all["recording_enabled"] == "true"
			if rv := all["recording_sample_rate"]; rv != "" {
				if rate, err := strconv.ParseFloat(rv, 64); err == nil && rate >= 0 && rate <= 1 {
					cfg.SampleRate = rate
				}
			}
		}
	}

	// Per-project overrides win over instance defaults.
	if settings != nil {
		if settings.Enabled != nil {
			cfg.Enabled = *settings.Enabled
		}
		if settings.SampleRate != nil {
			cfg.SampleRate = *settings.SampleRate
		}
		cfg.Rules = settings.Rules
	}

	if cfg.Rules == nil {
		cfg.Rules = []repository.RecordingRule{}
	}
	return cfg
}

// handleGetRecordingConfig returns the effective recording config for the project
// identified by the API key. Used by the JS SDK on init.
// GET /api/v1/recording-config
func (s *Server) handleGetRecordingConfig(w http.ResponseWriter, r *http.Request) {
	// Identify project via API key (same mechanism as event ingest).
	projectID, _, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		ctx, span := tracing.StartSpan(r.Context(), "recording_settings.get_config",
			attribute.Bool("api_key.valid", false),
		)
		defer span.End()
		// No valid API key — return the instance-level defaults (unauthenticated context).
		// The SDK will use these as a fallback.
		cfg := s.resolveEffectiveRecordingConfig(r.WithContext(ctx), nil)
		span.SetAttributes(attribute.Bool("recording.effective_enabled", cfg.Enabled))
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":     cfg.Enabled,
			"sample_rate": cfg.SampleRate,
			"rules":       cfg.Rules,
		})
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "recording_settings.get_config",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	var settings *repository.ProjectRecordingSettings
	if s.recordingSettings != nil {
		if ps, err := s.recordingSettings.GetProjectRecordingSettings(ctx, projectID); err == nil {
			settings = ps
		}
	}

	cfg := s.resolveEffectiveRecordingConfig(r.WithContext(ctx), settings)
	span.SetAttributes(
		attribute.Bool("recording.effective_enabled", cfg.Enabled),
		attribute.Float64("recording.effective_rate", cfg.SampleRate),
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":     cfg.Enabled,
		"sample_rate": cfg.SampleRate,
		"rules":       cfg.Rules,
	})
}
