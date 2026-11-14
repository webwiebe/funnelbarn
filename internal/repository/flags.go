package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// flagColumns is the shared SELECT column list for feature_flags, kept in one
// place so every read path scans an identical row shape via scanFlag.
const flagColumns = `id, project_id, flag_key, name, flag_type, variants, default_variant, split, COALESCE(conversion_event,''), COALESCE(targeting_rules,'[]'), status, created_at, COALESCE(origin,'manual'), last_evaluated_at, COALESCE(flag_kind,'experiment')`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanFlag(sc rowScanner) (domain.FeatureFlag, error) {
	var f domain.FeatureFlag
	var lastEval sql.NullTime
	if err := sc.Scan(
		&f.ID, &f.ProjectID, &f.FlagKey, &f.Name, &f.FlagType,
		&f.Variants, &f.DefaultVariant, &f.Split, &f.ConversionEvent,
		&f.TargetingRules, &f.Status, &f.CreatedAt, &f.Origin, &lastEval, &f.Kind,
	); err != nil {
		return domain.FeatureFlag{}, err
	}
	if lastEval.Valid {
		f.LastEvaluatedAt = &lastEval.Time
	}
	return f, nil
}

func (s *Store) CreateFlag(ctx context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error) {
	var err error
	f.ID, err = generateUUID()
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("generate uuid: %w", err)
	}
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	if f.Origin == "" {
		f.Origin = "manual"
	}
	if f.Kind == "" {
		f.Kind = domain.FlagKindExperiment
	}
	const q = `INSERT INTO feature_flags (id, project_id, flag_key, name, flag_type, variants, default_variant, split, conversion_event, targeting_rules, status, origin, flag_kind) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		f.ID, f.ProjectID, f.FlagKey, f.Name, f.FlagType,
		f.Variants, f.DefaultVariant, f.Split, f.ConversionEvent, f.TargetingRules, f.Status, f.Origin, f.Kind,
	); err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("create flag: %w", err)
	}
	return s.flagByID(ctx, s.db, f.ID)
}

// EnsureAutoFlag inserts an auto-created flag if no flag with the same
// (project_id, flag_key) exists yet, then returns the current row. It is
// idempotent and race-safe: concurrent first-evaluations of the same key
// converge on a single row via the UNIQUE(project_id, flag_key) constraint.
func (s *Store) EnsureAutoFlag(ctx context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error) {
	id, err := generateUUID()
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("generate uuid: %w", err)
	}
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	if f.Origin == "" {
		f.Origin = "auto"
	}
	if f.Kind == "" {
		f.Kind = domain.FlagKindExperiment
	}
	const q = `INSERT INTO feature_flags (id, project_id, flag_key, name, flag_type, variants, default_variant, split, conversion_event, targeting_rules, status, origin, flag_kind)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, flag_key) DO NOTHING`
	if _, err := s.db.ExecContext(ctx, q,
		id, f.ProjectID, f.FlagKey, f.Name, f.FlagType,
		f.Variants, f.DefaultVariant, f.Split, f.ConversionEvent, f.TargetingRules, f.Status, f.Origin, f.Kind,
	); err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("ensure auto flag: %w", err)
	}
	return s.FlagByKey(ctx, f.ProjectID, f.FlagKey)
}

// CountAutoFlags returns how many auto-created flags a project has. Manual flags
// are not counted — only the auto-registration spam vector is bounded.
func (r *ReadStore) CountAutoFlags(ctx context.Context, projectID string) (int, error) {
	const q = `SELECT COUNT(*) FROM feature_flags WHERE project_id = ? AND origin = 'auto'`
	var n int
	if err := r.rdb.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// TouchFlagEvaluated records that a flag was just evaluated, so the retention
// sweep can tell live flags from abandoned ones and the dashboard can show when
// a flag was last in service. Called for every origin: it used to be reached
// only for origin='auto', which left the busiest flags reading as
// never-evaluated.
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

func (r *ReadStore) FlagByID(ctx context.Context, id string) (domain.FeatureFlag, error) {
	return r.flagByID(ctx, r.rdb, id)
}

// flagByID reads through db, so write paths can read back on the write pool.
func (r *ReadStore) flagByID(ctx context.Context, db querier, id string) (domain.FeatureFlag, error) {
	q := `SELECT ` + flagColumns + ` FROM feature_flags WHERE id = ?`
	return scanFlag(db.QueryRowContext(ctx, q, id))
}

func (r *ReadStore) FlagByKey(ctx context.Context, projectID, flagKey string) (domain.FeatureFlag, error) {
	q := `SELECT ` + flagColumns + ` FROM feature_flags WHERE project_id = ? AND flag_key = ?`
	return scanFlag(r.rdb.QueryRowContext(ctx, q, projectID, flagKey))
}

func (r *ReadStore) ListFlags(ctx context.Context, projectID string) ([]domain.FeatureFlag, error) {
	q := `SELECT ` + flagColumns + ` FROM feature_flags WHERE project_id = ? ORDER BY created_at DESC`
	return r.queryFlags(ctx, q, projectID)
}

// ListAllFlags returns every project's flags. The in-memory flag snapshot
// loads it every few seconds; production holds a few dozen rows.
func (r *ReadStore) ListAllFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	return r.queryFlags(ctx, `SELECT `+flagColumns+` FROM feature_flags`)
}

func (r *ReadStore) queryFlags(ctx context.Context, q string, args ...any) ([]domain.FeatureFlag, error) {
	rows, err := r.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []domain.FeatureFlag
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

func (s *Store) UpdateFlag(ctx context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error) {
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	// A human editing a flag claims it (origin='manual'): it now appears as a
	// normal flag and is exempt from the stale-auto-flag sweep.
	if f.Kind == "" {
		f.Kind = domain.FlagKindExperiment
	}
	const q = `UPDATE feature_flags SET name=?, flag_type=?, variants=?, default_variant=?, split=?, conversion_event=?, targeting_rules=?, status=?, flag_kind=?, origin='manual' WHERE id=?`
	if _, err := s.db.ExecContext(ctx, q,
		f.Name, f.FlagType, f.Variants, f.DefaultVariant, f.Split,
		f.ConversionEvent, f.TargetingRules, f.Status, f.Kind, f.ID,
	); err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("update flag: %w", err)
	}
	return s.flagByID(ctx, s.db, f.ID)
}

func (s *Store) DeleteFlag(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM feature_flags WHERE id = ?`, id)
	return err
}

func (s *Store) RecordEvaluation(ctx context.Context, eval domain.FlagEvaluation) error {
	if eval.ID == "" {
		id, err := generateUUID()
		if err != nil {
			return fmt.Errorf("generate uuid: %w", err)
		}
		eval.ID = id
	}
	keysJSON := "[]"
	if len(eval.ContextKeys) > 0 {
		if b, jerr := json.Marshal(eval.ContextKeys); jerr == nil {
			keysJSON = string(b)
		}
	}
	// OR IGNORE: a queued command redelivered after a crash carries the same
	// id and inserts nothing new.
	const q = `INSERT OR IGNORE INTO flag_evaluations (id, flag_id, project_id, variant, context_hash, session_id, context_keys) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q, eval.ID, eval.FlagID, eval.ProjectID, eval.Variant, eval.ContextHash, nullStr(eval.SessionID), keysJSON)
	return err
}

// FlagContextKeySuggestions returns context keys seen in recent evaluations for a project,
// ordered by frequency, with the percentage of evaluations that included each key.
func (r *ReadStore) FlagContextKeySuggestions(ctx context.Context, projectID string) ([]domain.ContextKeySuggestion, error) {
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
	rows, err := r.rdb.QueryContext(ctx, q, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ContextKeySuggestion
	for rows.Next() {
		var s domain.ContextKeySuggestion
		if err := rows.Scan(&s.ContextKey, &s.SeenCount, &s.Pct); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ReadStore) CountEvaluationsByVariant(ctx context.Context, flagID string, from, to time.Time) (map[string]int64, error) {
	const q = `SELECT variant, COUNT(DISTINCT context_hash) FROM flag_evaluations WHERE flag_id = ? AND created_at >= ? AND created_at <= ? GROUP BY variant`
	rows, err := r.rdb.QueryContext(ctx, q, flagID, from, to)
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

func (r *ReadStore) CountConversionsByVariant(ctx context.Context, flagID, conversionEvent, projectID string, from, to time.Time) (map[string]int64, error) {
	const q = `
		SELECT fe.variant, COUNT(DISTINCT fe.context_hash)
		FROM flag_evaluations fe
		JOIN events e ON fe.session_id = e.session_id
		WHERE fe.flag_id = ? AND e.name = ? AND e.project_id = ?
		  AND fe.created_at >= ? AND fe.created_at <= ?
		GROUP BY fe.variant`
	rows, err := r.rdb.QueryContext(ctx, q, flagID, conversionEvent, projectID, from, to)
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
