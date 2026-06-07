package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Recording holds metadata for a session recording.
type Recording struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	SessionID   string     `json:"session_id"`
	Environment string     `json:"environment"`
	ChunkCount  int        `json:"chunk_count"`
	DurationMs  int64      `json:"duration_ms"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeviceType  string     `json:"device_type"`
	UserAgent   string     `json:"user_agent,omitempty"`
	IsBot       bool       `json:"is_bot"`
	PageURL     string     `json:"page_url,omitempty"`
}

// FlagEvaluationEntry is a flag evaluation linked to a session.
type FlagEvaluationEntry struct {
	FlagName    string    `json:"flag_name"`
	Variant     string    `json:"variant"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// RecordingListOpts holds optional filter/pagination parameters for ListRecordings.
type RecordingListOpts struct {
	Environment string
	SessionIDs  []string
	DeviceType  string // "mobile" | "desktop" | "tablet" | ""
	HumanOnly   bool
	PageURL     string
	Limit       int
	Offset      int
}

// UpsertRecording inserts a new recording or updates an existing one's
// chunk count, duration, and ended_at timestamp. Metadata fields
// (device_type, user_agent, is_bot, page_url) are set only on insert.
func (s *Store) UpsertRecording(ctx context.Context, r Recording) error {
	const q = `
		INSERT INTO recordings
			(id, project_id, session_id, environment, chunk_count, duration_ms, started_at, ended_at,
			 device_type, user_agent, is_bot, page_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			chunk_count = excluded.chunk_count,
			duration_ms = excluded.duration_ms,
			ended_at    = excluded.ended_at`
	var endedAt interface{}
	if r.EndedAt != nil {
		endedAt = *r.EndedAt
	}
	isBot := 0
	if r.IsBot {
		isBot = 1
	}
	_, err := s.db.ExecContext(ctx, q,
		r.ID, r.ProjectID, r.SessionID, r.Environment,
		r.ChunkCount, r.DurationMs, r.StartedAt, endedAt,
		r.DeviceType, r.UserAgent, isBot, r.PageURL,
	)
	return err
}

// GetRecording fetches a single recording by ID.
func (s *Store) GetRecording(ctx context.Context, id string) (Recording, error) {
	const q = `
		SELECT id, project_id, session_id, environment, chunk_count, duration_ms, started_at, ended_at, created_at,
		       device_type, user_agent, is_bot, page_url
		FROM recordings WHERE id = ?`
	return scanRecording(s.db.QueryRowContext(ctx, q, id))
}

// ListRecordings returns recordings for a project with optional filters.
func (s *Store) ListRecordings(ctx context.Context, projectID string, opts RecordingListOpts) ([]Recording, error) {
	where := []string{"project_id = ?"}
	args := []any{projectID}

	if opts.Environment != "" {
		where = append(where, "environment = ?")
		args = append(args, opts.Environment)
	}
	if len(opts.SessionIDs) > 0 {
		placeholders := make([]string, len(opts.SessionIDs))
		for i, id := range opts.SessionIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, "session_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if opts.DeviceType != "" {
		where = append(where, "device_type = ?")
		args = append(args, opts.DeviceType)
	}
	if opts.HumanOnly {
		where = append(where, "is_bot = 0")
	}
	if opts.PageURL != "" {
		where = append(where, "page_url = ?")
		args = append(args, opts.PageURL)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := opts.Offset

	q := fmt.Sprintf(`
		SELECT id, project_id, session_id, environment, chunk_count, duration_ms, started_at, ended_at, created_at,
		       device_type, user_agent, is_bot, page_url
		FROM recordings
		WHERE %s
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?`, strings.Join(where, " AND "))
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recording
	for rows.Next() {
		r, err := scanRecordingRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListOldRecordings returns recordings older than the given threshold (by created_at).
func (s *Store) ListOldRecordings(ctx context.Context, before time.Time) ([]Recording, error) {
	const q = `
		SELECT id, project_id, chunk_count
		FROM recordings
		WHERE started_at < ?
		LIMIT 500`
	rows, err := s.db.QueryContext(ctx, q, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recording
	for rows.Next() {
		var r Recording
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.ChunkCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRecording deletes a recording row by ID.
func (s *Store) DeleteRecording(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, id)
	return err
}

// FlagEvaluationsForSession returns all flag evaluations that occurred
// during the session associated with a recording.
func (s *Store) FlagEvaluationsForSession(ctx context.Context, sessionID, projectID string) ([]FlagEvaluationEntry, error) {
	const q = `
		SELECT f.name, fe.variant, fe.created_at
		FROM flag_evaluations fe
		JOIN feature_flags f ON f.id = fe.flag_id
		WHERE fe.session_id = ? AND fe.project_id = ?
		ORDER BY fe.created_at ASC`
	rows, err := s.db.QueryContext(ctx, q, sessionID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FlagEvaluationEntry
	for rows.Next() {
		var e FlagEvaluationEntry
		if err := rows.Scan(&e.FlagName, &e.Variant, &e.EvaluatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanRecording(row *sql.Row) (Recording, error) {
	var r Recording
	var endedAt sql.NullTime
	var isBot int
	err := row.Scan(
		&r.ID, &r.ProjectID, &r.SessionID, &r.Environment,
		&r.ChunkCount, &r.DurationMs, &r.StartedAt, &endedAt, &r.CreatedAt,
		&r.DeviceType, &r.UserAgent, &isBot, &r.PageURL,
	)
	if endedAt.Valid {
		r.EndedAt = &endedAt.Time
	}
	r.IsBot = isBot != 0
	return r, err
}

func scanRecordingRow(scan func(...any) error) (Recording, error) {
	var r Recording
	var endedAt sql.NullTime
	var isBot int
	err := scan(
		&r.ID, &r.ProjectID, &r.SessionID, &r.Environment,
		&r.ChunkCount, &r.DurationMs, &r.StartedAt, &endedAt, &r.CreatedAt,
		&r.DeviceType, &r.UserAgent, &isBot, &r.PageURL,
	)
	if endedAt.Valid {
		r.EndedAt = &endedAt.Time
	}
	r.IsBot = isBot != 0
	return r, err
}
