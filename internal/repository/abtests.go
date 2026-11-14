package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// CreateABTest inserts a new A/B test.
func (s *Store) CreateABTest(ctx context.Context, t domain.ABTest) (domain.ABTest, error) {
	var err error
	t.ID, err = generateUUID()
	if err != nil {
		return domain.ABTest{}, fmt.Errorf("generate uuid: %w", err)
	}
	const q = `
		INSERT INTO ab_tests (id, project_id, name, status, control_filter, variant_filter, conversion_event)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		t.ID, t.ProjectID, t.Name, t.Status,
		nullStr(t.ControlFilter), nullStr(t.VariantFilter), t.ConversionEvent,
	); err != nil {
		return domain.ABTest{}, fmt.Errorf("create ab_test: %w", err)
	}
	return s.abTestByID(ctx, s.db, t.ID)
}

// ABTestByID fetches a single A/B test.
func (s *ReadStore) ABTestByID(ctx context.Context, id string) (domain.ABTest, error) {
	return s.abTestByID(ctx, s.rdb, id)
}

// abTestByID reads through db, so write paths can read back on the write pool.
func (s *ReadStore) abTestByID(ctx context.Context, db querier, id string) (domain.ABTest, error) {
	const q = `
		SELECT id, project_id, name, status,
		       COALESCE(control_filter,''), COALESCE(variant_filter,''),
		       conversion_event, created_at
		FROM ab_tests WHERE id = ?`
	var t domain.ABTest
	err := db.QueryRowContext(ctx, q, id).Scan(
		&t.ID, &t.ProjectID, &t.Name, &t.Status,
		&t.ControlFilter, &t.VariantFilter,
		&t.ConversionEvent, &t.CreatedAt,
	)
	if err != nil {
		return domain.ABTest{}, err // sql.ErrNoRows propagated to caller; service wraps as domain.ErrNotFound
	}
	return t, nil
}

// ListABTests returns all A/B tests for a project.
func (s *ReadStore) ListABTests(ctx context.Context, projectID string) ([]domain.ABTest, error) {
	const q = `
		SELECT id, project_id, name, status,
		       COALESCE(control_filter,''), COALESCE(variant_filter,''),
		       conversion_event, created_at
		FROM ab_tests WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tests []domain.ABTest
	for rows.Next() {
		var t domain.ABTest
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
func (s *ReadStore) AnalyzeABTest(ctx context.Context, t domain.ABTest, from, to time.Time) ([]domain.ABTestResult, error) {
	type arm struct {
		name   string
		filter string
	}
	arms := []arm{
		{name: "control", filter: t.ControlFilter},
		{name: "variant", filter: t.VariantFilter},
	}

	var results []domain.ABTestResult
	for _, a := range arms {
		var f domain.ABTestFilter
		if a.filter != "" {
			if err := json.Unmarshal([]byte(a.filter), &f); err != nil {
				return nil, fmt.Errorf("parse filter for %s: %w", a.name, err)
			}
		}

		total, err := s.countSessionsWithFilter(ctx, t.ProjectID, f, from, to)
		if err != nil {
			return nil, fmt.Errorf("count %s total: %w", a.name, err)
		}

		conversions, err := s.countConversionsWithFilter(ctx, t.ProjectID, f, t.ConversionEvent, from, to)
		if err != nil {
			return nil, fmt.Errorf("count %s conversions: %w", a.name, err)
		}

		rate := 0.0
		if total > 0 {
			rate = float64(conversions) / float64(total)
		}

		results = append(results, domain.ABTestResult{
			Variant:     a.name,
			Total:       total,
			Conversions: conversions,
			Rate:        rate,
		})
	}
	return results, nil
}

// countSessionsWithFilter counts distinct sessions where any event matches the filter.
func (s *ReadStore) countSessionsWithFilter(ctx context.Context, projectID string, f domain.ABTestFilter, from, to time.Time) (int64, error) {
	if f.Property == "" {
		const q = `SELECT COUNT(DISTINCT session_id) FROM events WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?`
		var n int64
		return n, s.rdb.QueryRowContext(ctx, q, projectID, from, to).Scan(&n)
	}
	const q = `
		SELECT COUNT(DISTINCT session_id) FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		  AND json_extract(properties, '$.' || ?) = ?`
	var n int64
	return n, s.rdb.QueryRowContext(ctx, q, projectID, from, to, f.Property, f.Value).Scan(&n)
}

// countConversionsWithFilter counts sessions matching the filter that also fired the conversion event.
func (s *ReadStore) countConversionsWithFilter(ctx context.Context, projectID string, f domain.ABTestFilter, conversionEvent string, from, to time.Time) (int64, error) {
	if f.Property == "" {
		const q = `
			SELECT COUNT(DISTINCT session_id) FROM events
			WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?`
		var n int64
		return n, s.rdb.QueryRowContext(ctx, q, projectID, conversionEvent, from, to).Scan(&n)
	}
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
	err := s.rdb.QueryRowContext(ctx, q,
		projectID, from, to,
		f.Property, f.Value,
		conversionEvent, projectID,
	).Scan(&n)
	return n, err
}
