package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// RecordingRule is a URL pattern rule used to decide whether to capture or ignore a page.
type RecordingRule struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"` // "capture" or "ignore"
}

// ProjectRecordingSettings holds per-project recording overrides.
// Nil pointer fields mean "inherit from instance settings".
type ProjectRecordingSettings struct {
	ProjectID  string
	Enabled    *bool    // nil = inherit from instance
	SampleRate *float64 // nil = inherit from instance
	Rules      []RecordingRule
	UpdatedAt  time.Time
}

// GetProjectRecordingSettings returns per-project recording settings.
// Returns a zero-value struct (all nil) if no settings have been saved yet.
func (s *Store) GetProjectRecordingSettings(ctx context.Context, projectID string) (*ProjectRecordingSettings, error) {
	const q = `SELECT enabled, sample_rate, rules, updated_at
               FROM project_recording_settings WHERE project_id = ?`
	row := s.db.QueryRowContext(ctx, q, projectID)

	var enabledInt sql.NullInt64
	var sampleRate sql.NullFloat64
	var rulesJSON string
	var updatedAt time.Time

	err := row.Scan(&enabledInt, &sampleRate, &rulesJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &ProjectRecordingSettings{
			ProjectID: projectID,
			Rules:     []RecordingRule{},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	out := &ProjectRecordingSettings{
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
		out.Rules = []RecordingRule{}
	} else if err := json.Unmarshal([]byte(rulesJSON), &out.Rules); err != nil {
		out.Rules = []RecordingRule{}
	}

	return out, nil
}

// UpsertProjectRecordingSettings saves per-project recording overrides.
func (s *Store) UpsertProjectRecordingSettings(ctx context.Context, settings *ProjectRecordingSettings) error {
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
