package repository

import (
	"context"
	"fmt"
	"time"
)

// FeatureFlag represents an OpenFeature-compatible feature flag.
type FeatureFlag struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	FlagKey         string    `json:"flag_key"`
	Name            string    `json:"name"`
	FlagType        string    `json:"flag_type"`
	Variants        string    `json:"variants"`
	DefaultVariant  string    `json:"default_variant"`
	Split           string    `json:"split"`
	ConversionEvent string    `json:"conversion_event"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// FlagEvaluation records a single flag evaluation.
type FlagEvaluation struct {
	ID          string    `json:"id"`
	FlagID      string    `json:"flag_id"`
	ProjectID   string    `json:"project_id"`
	Variant     string    `json:"variant"`
	ContextHash string    `json:"context_hash"`
	SessionID   string    `json:"session_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// FlagAnalysisResult holds per-variant analysis.
type FlagAnalysisResult struct {
	Variant     string  `json:"variant"`
	Sample      int64   `json:"sample"`
	Conversions int64   `json:"conversions"`
	Rate        float64 `json:"rate"`
}

func (s *Store) CreateFlag(ctx context.Context, f FeatureFlag) (FeatureFlag, error) {
	var err error
	f.ID, err = generateUUID()
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("generate uuid: %w", err)
	}
	const q = `INSERT INTO feature_flags (id, project_id, flag_key, name, flag_type, variants, default_variant, split, conversion_event, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		f.ID, f.ProjectID, f.FlagKey, f.Name, f.FlagType,
		f.Variants, f.DefaultVariant, f.Split, f.ConversionEvent, f.Status,
	); err != nil {
		return FeatureFlag{}, fmt.Errorf("create flag: %w", err)
	}
	return s.FlagByID(ctx, f.ID)
}

func (s *Store) FlagByID(ctx context.Context, id string) (FeatureFlag, error) {
	const q = `SELECT id, project_id, flag_key, name, flag_type, variants, default_variant, split, COALESCE(conversion_event,''), status, created_at FROM feature_flags WHERE id = ?`
	var f FeatureFlag
	if err := s.db.QueryRowContext(ctx, q, id).Scan(
		&f.ID, &f.ProjectID, &f.FlagKey, &f.Name, &f.FlagType,
		&f.Variants, &f.DefaultVariant, &f.Split, &f.ConversionEvent,
		&f.Status, &f.CreatedAt,
	); err != nil {
		return FeatureFlag{}, err
	}
	return f, nil
}

func (s *Store) FlagByKey(ctx context.Context, projectID, flagKey string) (FeatureFlag, error) {
	const q = `SELECT id, project_id, flag_key, name, flag_type, variants, default_variant, split, COALESCE(conversion_event,''), status, created_at FROM feature_flags WHERE project_id = ? AND flag_key = ?`
	var f FeatureFlag
	if err := s.db.QueryRowContext(ctx, q, projectID, flagKey).Scan(
		&f.ID, &f.ProjectID, &f.FlagKey, &f.Name, &f.FlagType,
		&f.Variants, &f.DefaultVariant, &f.Split, &f.ConversionEvent,
		&f.Status, &f.CreatedAt,
	); err != nil {
		return FeatureFlag{}, err
	}
	return f, nil
}

func (s *Store) ListFlags(ctx context.Context, projectID string) ([]FeatureFlag, error) {
	const q = `SELECT id, project_id, flag_key, name, flag_type, variants, default_variant, split, COALESCE(conversion_event,''), status, created_at FROM feature_flags WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []FeatureFlag
	for rows.Next() {
		var f FeatureFlag
		if err := rows.Scan(
			&f.ID, &f.ProjectID, &f.FlagKey, &f.Name, &f.FlagType,
			&f.Variants, &f.DefaultVariant, &f.Split, &f.ConversionEvent,
			&f.Status, &f.CreatedAt,
		); err != nil {
			return nil, err
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

func (s *Store) UpdateFlag(ctx context.Context, f FeatureFlag) (FeatureFlag, error) {
	const q = `UPDATE feature_flags SET name=?, flag_type=?, variants=?, default_variant=?, split=?, conversion_event=?, status=? WHERE id=?`
	if _, err := s.db.ExecContext(ctx, q,
		f.Name, f.FlagType, f.Variants, f.DefaultVariant, f.Split,
		f.ConversionEvent, f.Status, f.ID,
	); err != nil {
		return FeatureFlag{}, fmt.Errorf("update flag: %w", err)
	}
	return s.FlagByID(ctx, f.ID)
}

func (s *Store) DeleteFlag(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM feature_flags WHERE id = ?`, id)
	return err
}

func (s *Store) RecordEvaluation(ctx context.Context, eval FlagEvaluation) error {
	var err error
	eval.ID, err = generateUUID()
	if err != nil {
		return fmt.Errorf("generate uuid: %w", err)
	}
	const q = `INSERT INTO flag_evaluations (id, flag_id, project_id, variant, context_hash, session_id) VALUES (?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, q, eval.ID, eval.FlagID, eval.ProjectID, eval.Variant, eval.ContextHash, nullStr(eval.SessionID))
	return err
}

func (s *Store) CountEvaluationsByVariant(ctx context.Context, flagID string, from, to time.Time) (map[string]int64, error) {
	const q = `SELECT variant, COUNT(DISTINCT context_hash) FROM flag_evaluations WHERE flag_id = ? AND created_at >= ? AND created_at <= ? GROUP BY variant`
	rows, err := s.db.QueryContext(ctx, q, flagID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var variant string
		var count int64
		if err := rows.Scan(&variant, &count); err != nil {
			return nil, err
		}
		result[variant] = count
	}
	return result, rows.Err()
}

func (s *Store) PurgeOldEvaluations(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM flag_evaluations WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) CountConversionsByVariant(ctx context.Context, flagID, conversionEvent, projectID string, from, to time.Time) (map[string]int64, error) {
	const q = `
		SELECT fe.variant, COUNT(DISTINCT fe.context_hash)
		FROM flag_evaluations fe
		JOIN events e ON fe.session_id = e.session_id
		WHERE fe.flag_id = ? AND e.name = ? AND e.project_id = ?
		  AND fe.created_at >= ? AND fe.created_at <= ?
		GROUP BY fe.variant`
	rows, err := s.db.QueryContext(ctx, q, flagID, conversionEvent, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var variant string
		var count int64
		if err := rows.Scan(&variant, &count); err != nil {
			return nil, err
		}
		result[variant] = count
	}
	return result, rows.Err()
}
