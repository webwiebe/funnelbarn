package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// GetProjectRecordingSettings returns per-project recording settings.
// Returns a zero-value struct (all nil) if no settings have been saved yet.
func (s *ReadStore) GetProjectRecordingSettings(ctx context.Context, projectID string) (*domain.ProjectRecordingSettings, error) {
	const q = `SELECT enabled, sample_rate, rules, updated_at
               FROM project_recording_settings WHERE project_id = ?`
	row := s.rdb.QueryRowContext(ctx, q, projectID)

	var enabledInt sql.NullInt64
	var sampleRate sql.NullFloat64
	var rulesJSON string
	var updatedAt time.Time

	err := row.Scan(&enabledInt, &sampleRate, &rulesJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &domain.ProjectRecordingSettings{
			ProjectID: projectID,
			Rules:     []domain.RecordingRule{},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	out := &domain.ProjectRecordingSettings{
		ProjectID: projectID,
		UpdatedAt: updatedAt,
	}
	if enabledInt.Valid {
		b := enabledInt.Int64 != 0
		out.Enabled = &b
	}
	if sampleRate.Valid {
		out.SampleRate = &sampleRate.Float64
	}

	if rulesJSON == "" || rulesJSON == "null" {
		out.Rules = []domain.RecordingRule{}
	} else if err := json.Unmarshal([]byte(rulesJSON), &out.Rules); err != nil {
		out.Rules = []domain.RecordingRule{}
	}

	return out, nil
}

// UpsertProjectRecordingSettings saves per-project recording overrides.
func (s *Store) UpsertProjectRecordingSettings(ctx context.Context, settings *domain.ProjectRecordingSettings) error {
	rulesJSON, err := json.Marshal(settings.Rules)
	if err != nil {
		return err
	}

	var enabledVal sql.NullInt64
	if settings.Enabled != nil {
		if *settings.Enabled {
			enabledVal = sql.NullInt64{Int64: 1, Valid: true}
		} else {
			enabledVal = sql.NullInt64{Int64: 0, Valid: true}
		}
	}
	var sampleRateVal sql.NullFloat64
	if settings.SampleRate != nil {
		sampleRateVal = sql.NullFloat64{Float64: *settings.SampleRate, Valid: true}
	}

	const q = `INSERT INTO project_recording_settings (project_id, enabled, sample_rate, rules, updated_at)
               VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
               ON CONFLICT(project_id) DO UPDATE SET
                   enabled = excluded.enabled,
                   sample_rate = excluded.sample_rate,
                   rules = excluded.rules,
                   updated_at = CURRENT_TIMESTAMP`
	_, err = s.db.ExecContext(ctx, q, settings.ProjectID, enabledVal, sampleRateVal, string(rulesJSON))
	return err
}
