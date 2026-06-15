package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// flagColumns is the shared SELECT column list for feature_flags, kept in one
// place so every read path scans an identical row shape via scanFlag.
const flagColumns = `id, project_id, flag_key, name, flag_type, variants, default_variant, split, COALESCE(conversion_event,''), COALESCE(targeting_rules,'[]'), status, created_at, COALESCE(origin,'manual'), last_evaluated_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanFlag(sc rowScanner) (FeatureFlag, error) {
	var f FeatureFlag
	var lastEval sql.NullTime
	if err := sc.Scan(
		&f.ID, &f.ProjectID, &f.FlagKey, &f.Name, &f.FlagType,
		&f.Variants, &f.DefaultVariant, &f.Split, &f.ConversionEvent,
		&f.TargetingRules, &f.Status, &f.CreatedAt, &f.Origin, &lastEval,
	); err != nil {
		return FeatureFlag{}, err
	}
	if lastEval.Valid {
		f.LastEvaluatedAt = &lastEval.Time
	}
	return f, nil
}

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
	TargetingRules  string    `json:"targeting_rules"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	// Origin is "manual" for human-created flags and "auto" for flags created on
	// first evaluation. LastEvaluatedAt tracks the most recent evaluation of an
	// auto flag so stale, never-configured ones can be pruned.
	Origin          string     `json:"origin"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
}

// FlagEvaluation records a single flag evaluation.
type FlagEvaluation struct {
	ID          string    `json:"id"`
	FlagID      string    `json:"flag_id"`
	ProjectID   string    `json:"project_id"`
	Variant     string    `json:"variant"`
	ContextHash string    `json:"context_hash"`
	SessionID   string    `json:"session_id"`
	ContextKeys []string  `json:"context_keys"` // key names present in the eval context
	CreatedAt   time.Time `json:"created_at"`
}

// ContextKeySuggestion is a context key seen in recent evaluations with its frequency.
type ContextKeySuggestion struct {
	ContextKey string `json:"context_key"`
	SeenCount  int64  `json:"seen_count"`
	Pct        int    `json:"pct"` // percentage of evaluations in the last 30 days that included this key
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
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	if f.Origin == "" {
		f.Origin = "manual"
	}
	const q = `INSERT INTO feature_flags (id, project_id, flag_key, name, flag_type, variants, default_variant, split, conversion_event, targeting_rules, status, origin) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		f.ID, f.ProjectID, f.FlagKey, f.Name, f.FlagType,
		f.Variants, f.DefaultVariant, f.Split, f.ConversionEvent, f.TargetingRules, f.Status, f.Origin,
	); err != nil {
		return FeatureFlag{}, fmt.Errorf("create flag: %w", err)
	}
	return s.FlagByID(ctx, f.ID)
}

// EnsureAutoFlag inserts an auto-created flag if no flag with the same
// (project_id, flag_key) exists yet, then returns the current row. It is
// idempotent and race-safe: concurrent first-evaluations of the same key
// converge on a single row via the UNIQUE(project_id, flag_key) constraint.
func (s *Store) EnsureAutoFlag(ctx context.Context, f FeatureFlag) (FeatureFlag, error) {
	id, err := generateUUID()
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("generate uuid: %w", err)
	}
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	if f.Origin == "" {
		f.Origin = "auto"
	}
	const q = `INSERT INTO feature_flags (id, project_id, flag_key, name, flag_type, variants, default_variant, split, conversion_event, targeting_rules, status, origin)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, flag_key) DO NOTHING`
	if _, err := s.db.ExecContext(ctx, q,
		id, f.ProjectID, f.FlagKey, f.Name, f.FlagType,
		f.Variants, f.DefaultVariant, f.Split, f.ConversionEvent, f.TargetingRules, f.Status, f.Origin,
	); err != nil {
		return FeatureFlag{}, fmt.Errorf("ensure auto flag: %w", err)
	}
	return s.FlagByKey(ctx, f.ProjectID, f.FlagKey)
}

// CountAutoFlags returns how many auto-created flags a project has. Manual flags
// are not counted — only the auto-registration spam vector is bounded.
func (s *Store) CountAutoFlags(ctx context.Context, projectID string) (int, error) {
	const q = `SELECT COUNT(*) FROM feature_flags WHERE project_id = ? AND origin = 'auto'`
	var n int
	if err := s.db.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// TouchFlagEvaluated records that an auto flag was just evaluated, so the
// retention sweep can tell live unconfigured flags from abandoned ones.
func (s *Store) TouchFlagEvaluated(ctx context.Context, flagID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE feature_flags SET last_evaluated_at = CURRENT_TIMESTAMP WHERE id = ?`, flagID)
	return err
}

// PurgeStaleAutoFlags deletes auto-created, never-configured (status='inactive')
// flags whose most recent evaluation — or creation, if never evaluated — predates
// the cutoff. Human-claimed flags (origin='manual') and activated/paused flags are
// never touched.
func (s *Store) PurgeStaleAutoFlags(ctx context.Context, cutoff time.Time) (int64, error) {
	const q = `DELETE FROM feature_flags
		WHERE origin = 'auto' AND status = 'inactive'
		  AND COALESCE(last_evaluated_at, created_at) < ?`
	result, err := s.db.ExecContext(ctx, q, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) FlagByID(ctx context.Context, id string) (FeatureFlag, error) {
	q := `SELECT ` + flagColumns + ` FROM feature_flags WHERE id = ?`
	return scanFlag(s.db.QueryRowContext(ctx, q, id))
}

func (s *Store) FlagByKey(ctx context.Context, projectID, flagKey string) (FeatureFlag, error) {
	q := `SELECT ` + flagColumns + ` FROM feature_flags WHERE project_id = ? AND flag_key = ?`
	return scanFlag(s.db.QueryRowContext(ctx, q, projectID, flagKey))
}

func (s *Store) ListFlags(ctx context.Context, projectID string) ([]FeatureFlag, error) {
	q := `SELECT ` + flagColumns + ` FROM feature_flags WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []FeatureFlag
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

func (s *Store) UpdateFlag(ctx context.Context, f FeatureFlag) (FeatureFlag, error) {
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	// A human editing a flag claims it (origin='manual'): it now appears as a
	// normal flag and is exempt from the stale-auto-flag sweep.
	const q = `UPDATE feature_flags SET name=?, flag_type=?, variants=?, default_variant=?, split=?, conversion_event=?, targeting_rules=?, status=?, origin='manual' WHERE id=?`
	if _, err := s.db.ExecContext(ctx, q,
		f.Name, f.FlagType, f.Variants, f.DefaultVariant, f.Split,
		f.ConversionEvent, f.TargetingRules, f.Status, f.ID,
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
	keysJSON := "[]"
	if len(eval.ContextKeys) > 0 {
		if b, jerr := json.Marshal(eval.ContextKeys); jerr == nil {
			keysJSON = string(b)
		}
	}
	const q = `INSERT INTO flag_evaluations (id, flag_id, project_id, variant, context_hash, session_id, context_keys) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, q, eval.ID, eval.FlagID, eval.ProjectID, eval.Variant, eval.ContextHash, nullStr(eval.SessionID), keysJSON)
	return err
}

// FlagContextKeySuggestions returns context keys seen in recent evaluations for a project,
// ordered by frequency, with the percentage of evaluations that included each key.
func (s *Store) FlagContextKeySuggestions(ctx context.Context, projectID string) ([]ContextKeySuggestion, error) {
	const q = `
		WITH total AS (
			SELECT COUNT(*) AS n
			FROM flag_evaluations
			WHERE project_id = ? AND created_at > datetime('now', '-30 days')
		),
		key_counts AS (
			SELECT value AS context_key, COUNT(*) AS seen_count
			FROM flag_evaluations, json_each(context_keys)
			WHERE project_id = ? AND created_at > datetime('now', '-30 days')
			GROUP BY value
		)
		SELECT kc.context_key, kc.seen_count,
		       CAST(ROUND(kc.seen_count * 100.0 / NULLIF(t.n, 0)) AS INTEGER) AS pct
		FROM key_counts kc, total t
		ORDER BY kc.seen_count DESC
		LIMIT 20`
	rows, err := s.db.QueryContext(ctx, q, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContextKeySuggestion
	for rows.Next() {
		var s ContextKeySuggestion
		if err := rows.Scan(&s.ContextKey, &s.SeenCount, &s.Pct); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
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
