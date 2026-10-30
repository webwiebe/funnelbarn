package repository

import (
	"context"
	"time"
)

// ProjectHourCount is one project's event count in one hour of created_at. Hour
// is the first 13 characters of the stored timestamp, such as "2026-10-30 18".
type ProjectHourCount struct {
	Project string
	Hour    string
	Count   int64
}

// EnvironmentEventsByHour returns the events of one environment stored at or after
// since per project and hour. It reads created_at because a client sets occurred_at:
// some send a date with no time, which puts a row up to a day before it arrived.
// created_at has no index, so this scans the table. It suits an hourly check.
func (s *ReadStore) EnvironmentEventsByHour(ctx context.Context, env string, since time.Time) ([]ProjectHourCount, error) {
	rows, err := s.rdb.QueryContext(ctx, `
		SELECT p.slug, substr(e.created_at, 1, 13) AS hour, COUNT(*)
		FROM events e
		JOIN projects p ON p.id = e.project_id
		WHERE e.created_at >= ? AND e.environment = ?
		GROUP BY p.slug, hour`, since.UTC().Format(time.DateTime), env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectHourCount
	for rows.Next() {
		var c ProjectHourCount
		if err := rows.Scan(&c.Project, &c.Hour, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
