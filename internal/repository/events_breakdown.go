package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// TopEventNamesFiltered is TopEventNames with the extra EventCountFilter conditions.
func (s *ReadStore) TopEventNamesFiltered(ctx context.Context, projectID string, from, to time.Time, limit int, f domain.EventCountFilter) ([]domain.EventNameStat, error) {
	if limit <= 0 {
		limit = 10
	}
	cond, extra := eventCountWhere(f)
	q := `SELECT name, COUNT(*) as count FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?` + cond + `
		GROUP BY name ORDER BY count DESC LIMIT ?`
	args := append([]any{projectID, from, to}, extra...)
	rows, err := s.rdb.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var stats []domain.EventNameStat
	for rows.Next() {
		var es domain.EventNameStat
		if err := rows.Scan(&es.Name, &es.Count); err != nil {
			return nil, err
		}
		stats = append(stats, es)
	}
	return stats, rows.Err()
}

// EventBreakdown counts one event by the values of dimension (a key of
// BreakdownDimensions): events and distinct sessions per value, largest first.
// Events with an empty value are reported under "".
func (s *ReadStore) EventBreakdown(ctx context.Context, projectID, eventName, dimension string, from, to time.Time, limit int, f domain.EventCountFilter) ([]domain.BreakdownRow, error) {
	col, ok := domain.BreakdownDimensions[dimension]
	if !ok {
		return nil, fmt.Errorf("unsupported breakdown dimension %q", dimension)
	}
	if limit <= 0 {
		limit = 50
	}
	cond, extra := eventCountWhere(f)
	q := `SELECT COALESCE(` + col + `,'') AS v, COUNT(*) AS c, COUNT(DISTINCT session_id) FROM events
		WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?` + cond + `
		GROUP BY v ORDER BY c DESC, v LIMIT ?`
	args := append([]any{projectID, eventName, from, to}, extra...)
	rows, err := s.rdb.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.BreakdownRow
	for rows.Next() {
		var r domain.BreakdownRow
		if err := rows.Scan(&r.Value, &r.Count, &r.Sessions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// eventCountWhere returns the conditions (each starting with " AND") and their args.
func eventCountWhere(f domain.EventCountFilter) (string, []any) {
	var sql string
	var args []any
	if f.Environment != "" {
		sql += " AND environment = ?"
		args = append(args, f.Environment)
	}
	if f.ExcludeBots {
		sql += " AND COALESCE(device_type,'') != 'bot'"
	}
	if f.PropKey != "" {
		// json_extract returns 1 or 0 for JSON booleans and a number for JSON
		// numbers, so compare booleans by json_type and everything else as text.
		sql += ` AND (json_type(properties, '$.' || ?) IN ('true','false') AND json_type(properties, '$.' || ?) = ?
			OR (json_type(properties, '$.' || ?) NOT IN ('true','false') AND CAST(json_extract(properties, '$.' || ?) AS TEXT) = ?))`
		args = append(args, f.PropKey, f.PropKey, f.PropValue, f.PropKey, f.PropKey, f.PropValue)
	}
	return sql, args
}
