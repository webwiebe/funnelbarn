package repository

import (
	"context"
	"fmt"
	"time"
)

// EventCountFilter narrows the event-counts and event-breakdown queries. An
// empty field matches everything.
type EventCountFilter struct {
	Environment string
	// ExcludeBots drops events whose device_type is "bot".
	ExcludeBots bool
	// PropKey and PropValue keep events whose JSON property PropKey equals
	// PropValue. PropKey must be a plain identifier (see ValidPropertyKey).
	PropKey   string
	PropValue string // "true" and "false" also match JSON booleans
}

// BreakdownDimensions are the columns EventBreakdown can group by.
var BreakdownDimensions = map[string]string{
	"device_type":     "device_type",
	"referrer_domain": "referrer_domain",
	"utm_source":      "utm_source",
}

// ValidPropertyKey reports whether key can be used as a JSON property filter:
// letters, digits and underscores only, so it cannot alter the JSON path.
func ValidPropertyKey(key string) bool {
	if key == "" || len(key) > 64 {
		return false
	}
	for _, r := range key {
		if r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// where returns the conditions (each starting with " AND") and their args.
func (f EventCountFilter) where() (string, []any) {
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

// TopEventNamesFiltered is TopEventNames with the extra EventCountFilter conditions.
func (s *ReadStore) TopEventNamesFiltered(ctx context.Context, projectID string, from, to time.Time, limit int, f EventCountFilter) ([]EventNameStat, error) {
	if limit <= 0 {
		limit = 10
	}
	cond, extra := f.where()
	q := `SELECT name, COUNT(*) as count FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?` + cond + `
		GROUP BY name ORDER BY count DESC LIMIT ?`
	args := append([]any{projectID, from, to}, extra...)
	rows, err := s.rdb.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var stats []EventNameStat
	for rows.Next() {
		var es EventNameStat
		if err := rows.Scan(&es.Name, &es.Count); err != nil {
			return nil, err
		}
		stats = append(stats, es)
	}
	return stats, rows.Err()
}

// BreakdownRow is one value of a breakdown dimension.
type BreakdownRow struct {
	Value    string `json:"value"`
	Count    int64  `json:"count"`
	Sessions int64  `json:"sessions"`
}

// EventBreakdown counts one event by the values of dimension (a key of
// BreakdownDimensions): events and distinct sessions per value, largest first.
// Events with an empty value are reported under "".
func (s *ReadStore) EventBreakdown(ctx context.Context, projectID, eventName, dimension string, from, to time.Time, limit int, f EventCountFilter) ([]BreakdownRow, error) {
	col, ok := BreakdownDimensions[dimension]
	if !ok {
		return nil, fmt.Errorf("unsupported breakdown dimension %q", dimension)
	}
	if limit <= 0 {
		limit = 50
	}
	cond, extra := f.where()
	q := `SELECT COALESCE(` + col + `,'') AS v, COUNT(*) AS c, COUNT(DISTINCT session_id) FROM events
		WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?` + cond + `
		GROUP BY v ORDER BY c DESC, v LIMIT ?`
	args := append([]any{projectID, eventName, from, to}, extra...)
	rows, err := s.rdb.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []BreakdownRow
	for rows.Next() {
		var r BreakdownRow
		if err := rows.Scan(&r.Value, &r.Count, &r.Sessions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
