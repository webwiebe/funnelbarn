package repository

import (
	"context"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// DistinctEventNames returns all unique event names for a project (for autocomplete).
func (s *ReadStore) DistinctEventNames(ctx context.Context, projectID string) ([]string, error) {
	const q = `SELECT DISTINCT name FROM events WHERE project_id = ? ORDER BY name`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// DistinctEventProperties returns all unique JSON property keys used by events
// with the given name for a project.
func (s *ReadStore) DistinctEventProperties(ctx context.Context, projectID, eventName string) ([]string, error) {
	const q = `
		SELECT DISTINCT j.key
		FROM events e, json_each(e.properties) j
		WHERE e.project_id = ? AND e.name = ? AND e.properties != '' AND e.properties IS NOT NULL
		ORDER BY j.key`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, eventName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DistinctPropertyValues returns up to `limit` unique values for a JSON
// property key on events with the given name for a project.
func (s *ReadStore) DistinctPropertyValues(ctx context.Context, projectID, eventName, property string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT DISTINCT json_extract(e.properties, '$.' || ?) AS val
		FROM events e
		WHERE e.project_id = ? AND e.name = ?
		  AND e.properties != '' AND e.properties IS NOT NULL
		  AND val IS NOT NULL AND val != ''
		ORDER BY val
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, property, projectID, eventName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vals []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return vals, rows.Err()
}

// PopulatedMetadataColumns returns only metadata columns that have at least
// one non-empty value among the newest 1000 events of the given
// project and event name. Reading a sample keeps the call cheap on a project
// with millions of events, and the newest events show what the SDK sends now.
func (s *ReadStore) PopulatedMetadataColumns(ctx context.Context, projectID, eventName string) ([]string, error) {
	cols, q := domain.MetadataColumns(), `SELECT `
	for i, col := range cols {
		if i > 0 {
			q += `, `
		}
		// An aggregate over no rows is NULL, not an empty result.
		q += fmt.Sprintf(`COALESCE(MAX(CASE WHEN %s IS NOT NULL AND %s != '' THEN 1 ELSE 0 END), 0)`, col, col)
	}
	q += ` FROM (SELECT * FROM events WHERE project_id = ? AND name = ? ORDER BY occurred_at DESC LIMIT 1000)`

	flags := make([]int, len(cols))
	ptrs := make([]any, len(cols))
	for i := range flags {
		ptrs[i] = &flags[i]
	}
	if err := s.rdb.QueryRowContext(ctx, q, projectID, eventName).Scan(ptrs...); err != nil {
		return nil, err
	}

	var populated []string
	for i, f := range flags {
		if f == 1 {
			populated = append(populated, cols[i])
		}
	}
	return populated, nil
}

// DistinctEnvironments returns the canonical environment values recorded for a project.
func (s *ReadStore) DistinctEnvironments(ctx context.Context, projectID string) ([]string, error) {
	const q = `SELECT DISTINCT environment FROM events WHERE project_id = ? AND environment != '' ORDER BY environment`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}
