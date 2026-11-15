package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ReadStore is the query side of the agents store. It holds only the read
// pool, and the slice has no write side.
type ReadStore struct {
	rdb *sql.DB
}

// NewStore returns the agents store over the read pool rdb. The write pool is
// unused because the slice only reads; it is accepted so every slice is
// constructed the same way.
func NewStore(_, rdb *sql.DB) *ReadStore {
	return &ReadStore{rdb: rdb}
}

// AgentEvents returns the project's events named in names that occurred in
// [from, to], oldest first. An event whose properties do not decode keeps an
// empty property map.
func (s *ReadStore) AgentEvents(ctx context.Context, projectID string, names []string, from, to time.Time) ([]AgentEvent, error) {
	if len(names) == 0 {
		return nil, nil
	}
	args := []any{projectID, from, to}
	for _, n := range names {
		args = append(args, n)
	}
	q := `SELECT name, occurred_at, session_id, COALESCE(user_id_hash, ''), COALESCE(properties, '')
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		  AND name IN (?` + strings.Repeat(",?", len(names)-1) + `)
		ORDER BY occurred_at, rowid`
	rows, err := s.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AgentEvent
	for rows.Next() {
		var e AgentEvent
		var props string
		if err := rows.Scan(&e.Name, &e.OccurredAt, &e.SessionID, &e.UserIDHash, &props); err != nil {
			return nil, err
		}
		e.Properties = map[string]any{}
		if props != "" {
			_ = json.Unmarshal([]byte(props), &e.Properties)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
