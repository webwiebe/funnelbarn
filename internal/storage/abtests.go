package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ABTest represents an A/B test experiment.
type ABTest struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	ControlFilter   string    `json:"control_filter"`
	VariantFilter   string    `json:"variant_filter"`
	ConversionEvent string    `json:"conversion_event"`
	CreatedAt       time.Time `json:"created_at"`
}

// ABTestFilter is a JSON-serialisable event property filter.
type ABTestFilter struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// ABTestResult holds per-variant analysis results.
type ABTestResult struct {
	Variant     string  `json:"variant"`
	Total       int64   `json:"total"`
	Conversions int64   `json:"conversions"`
	Rate        float64 `json:"rate"`
}

// CreateABTest inserts a new A/B test.
func (s *Store) CreateABTest(ctx context.Context, t ABTest) (ABTest, error) {
	t.ID = generateUUID()
	const q = `
		INSERT INTO ab_tests (id, project_id, name, status, control_filter, variant_filter, conversion_event)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		t.ID, t.ProjectID, t.Name, t.Status,
		nullStr(t.ControlFilter), nullStr(t.VariantFilter), t.ConversionEvent,
	); err != nil {
		return ABTest{}, fmt.Errorf("create ab_test: %w", err)
	}
	return s.ABTestByID(ctx, t.ID)
}

// ABTestByID fetches a single A/B test.
func (s *Store) ABTestByID(ctx context.Context, id string) (ABTest, error) {
	const q = `
		SELECT id, project_id, name, status,
		       COALESCE(control_filter,''), COALESCE(variant_filter,''),
		       conversion_event, created_at
		FROM ab_tests WHERE id = ?`
	var t ABTest
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&t.ID, &t.ProjectID, &t.Name, &t.Status,
		&t.ControlFilter, &t.VariantFilter,
		&t.ConversionEvent, &t.CreatedAt,
	)
	if err != nil {
		return ABTest{}, err
	}
	return t, nil
}

// ListABTests returns all A/B tests for a project.
func (s *Store) ListABTests(ctx context.Context, projectID string) ([]ABTest, error) {
	const q = `
		SELECT id, project_id, name, status,
		       COALESCE(control_filter,''), COALESCE(variant_filter,''),
		       conversion_event, created_at
		FROM ab_tests WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tests []ABTest
	for rows.Next() {
		var t ABTest
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.Name, &t.Status,
			&t.ControlFilter, &t.VariantFilter,
			&t.ConversionEvent, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		tests = append(tests, t)
	}
	return tests, rows.Err()
}

// AnalyzeABTest counts control vs variant totals and conversions over a time window.
// The filter JSON encodes an ABTestFilter {property, value} matched against
// event properties (JSON blob) or top-level columns.
func (s *Store) AnalyzeABTest(ctx context.Context, t ABTest, from, to time.Time) ([]ABTestResult, error) {
	type arm struct {
		name   string
		filter string
	}
	arms := []arm{
		{name: "control", filter: t.ControlFilter},
		{name: "variant", filter: t.VariantFilter},
	}

	var results []ABTestResult
	for _, a := range arms {
		var f ABTestFilter
		if a.filter != "" {
			if err := json.Unmarshal([]byte(a.filter), &f); err != nil {
				return nil, fmt.Errorf("parse filter for %s: %w", a.name, err)
			}
		}

		// Count sessions that fired any event matching the filter.
		total, err := s.countSessionsWithFilter(ctx, t.ProjectID, f, from, to)
		if err != nil {
			return nil, fmt.Errorf("count %s total: %w", a.name, err)
		}

		// Count sessions matching the filter AND that also fired the conversion event.
		conversions, err := s.countConversionsWithFilter(ctx, t.ProjectID, f, t.ConversionEvent, from, to)
		if err != nil {
			return nil, fmt.Errorf("count %s conversions: %w", a.name, err)
		}

		rate := 0.0
		if total > 0 {
			rate = float64(conversions) / float64(total)
		}

		results = append(results, ABTestResult{
			Variant:     a.name,
			Total:       total,
			Conversions: conversions,
			Rate:        rate,
		})
	}
	return results, nil
}

// countSessionsWithFilter counts distinct sessions where any event matches the filter.
// When the filter is empty (zero-value), all sessions in the window are counted.
func (s *Store) countSessionsWithFilter(ctx context.Context, projectID string, f ABTestFilter, from, to time.Time) (int64, error) {
	if f.Property == "" {
		const q = `SELECT COUNT(DISTINCT session_id) FROM events WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?`
		var n int64
		return n, s.db.QueryRowContext(ctx, q, projectID, from, to).Scan(&n)
	}
	// Match against properties JSON blob using json_extract.
	const q = `
		SELECT COUNT(DISTINCT session_id) FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		  AND json_extract(properties, '$.' || ?) = ?`
	var n int64
	return n, s.db.QueryRowContext(ctx, q, projectID, from, to, f.Property, f.Value).Scan(&n)
}

// countConversionsWithFilter counts sessions matching the filter that also fired the conversion event.
func (s *Store) countConversionsWithFilter(ctx context.Context, projectID string, f ABTestFilter, conversionEvent string, from, to time.Time) (int64, error) {
	if f.Property == "" {
		const q = `
			SELECT COUNT(DISTINCT session_id) FROM events
			WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?`
		var n int64
		return n, s.db.QueryRowContext(ctx, q, projectID, conversionEvent, from, to).Scan(&n)
	}
	// Sessions that have a filter-matching event AND a conversion event.
	const q = `
		SELECT COUNT(DISTINCT e1.session_id)
		FROM events e1
		JOIN events e2 ON e1.session_id = e2.session_id
		WHERE e1.project_id = ?
		  AND e1.occurred_at >= ? AND e1.occurred_at <= ?
		  AND json_extract(e1.properties, '$.' || ?) = ?
		  AND e2.name = ?
		  AND e2.project_id = ?`
	var n int64
	err := s.db.QueryRowContext(ctx, q,
		projectID, from, to,
		f.Property, f.Value,
		conversionEvent, projectID,
	).Scan(&n)
	return n, err
}

// HasProjects returns true if at least one project exists in the database.
func (s *Store) HasProjects(ctx context.Context) (bool, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&n)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return n > 0, nil
}
