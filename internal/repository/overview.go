package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// This file holds cross-project ("instance-wide") analytics queries. Unlike the
// per-project methods in events.go these do NOT filter by project_id; several
// carry project_id in the result so the UI can attribute each row and drill down.
// The shared WHERE clause matches events.go: occurred_at range + optional env.

// ProjectRollups returns one summary row per project that has events in range.
func (s *ReadStore) ProjectRollups(ctx context.Context, from, to time.Time, env string) ([]domain.ProjectRollup, error) {
	const q = `
		SELECT project_id, COUNT(*) AS events, COUNT(DISTINCT session_id) AS sessions
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY project_id
		ORDER BY events DESC`
	rows, err := s.rdb.QueryContext(ctx, q, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ProjectRollup
	for rows.Next() {
		var r domain.ProjectRollup
		if err := rows.Scan(&r.ProjectID, &r.Events, &r.UniqueSessions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// OverviewTotals returns instance-wide event and unique-session counts in range.
func (s *ReadStore) OverviewTotals(ctx context.Context, from, to time.Time, env string) (events, sessions int64, err error) {
	const q = `
		SELECT COUNT(*), COUNT(DISTINCT session_id)
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)`
	err = s.rdb.QueryRowContext(ctx, q, from, to, env, env).Scan(&events, &sessions)
	return events, sessions, err
}

// OverviewVisitorsByProject returns unique sessions grouped by project and time
// bucket (hourly or daily), feeding the "visitors per site" multi-line chart.
func (s *ReadStore) OverviewVisitorsByProject(ctx context.Context, from, to time.Time, env string, hourly bool) ([]domain.ProjectDayCount, error) {
	bucket := `substr(occurred_at, 1, 10)`
	if hourly {
		bucket = `replace(substr(occurred_at, 1, 13), ' ', 'T') || ':00:00Z'`
	}
	q := fmt.Sprintf(`
		SELECT %s AS day, project_id, COUNT(DISTINCT session_id) AS count
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY day, project_id
		ORDER BY day`, bucket)
	rows, err := s.rdb.QueryContext(ctx, q, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ProjectDayCount
	for rows.Next() {
		var p domain.ProjectDayCount
		if err := rows.Scan(&p.Day, &p.ProjectID, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// OverviewTopPages returns the most-viewed pages across all projects, each row
// attributed to its project for drill-down.
func (s *ReadStore) OverviewTopPages(ctx context.Context, from, to time.Time, limit int, env string) ([]domain.OverviewPageStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT project_id, COALESCE(url, '(unknown)'), COUNT(*) AS views
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY project_id, url
		ORDER BY views DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.OverviewPageStat
	for rows.Next() {
		var p domain.OverviewPageStat
		if err := rows.Scan(&p.ProjectID, &p.URL, &p.Views); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// OverviewTopReferrers returns the most common referrer domains across all
// projects, attributed to project for drill-down.
func (s *ReadStore) OverviewTopReferrers(ctx context.Context, from, to time.Time, limit int, env string) ([]domain.OverviewReferrerStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT project_id, COALESCE(referrer_domain, '(direct)'), COUNT(*) AS visits
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY project_id, referrer_domain
		ORDER BY visits DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.OverviewReferrerStat
	for rows.Next() {
		var r domain.OverviewReferrerStat
		if err := rows.Scan(&r.ProjectID, &r.Domain, &r.Visits); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// OverviewTopCountries returns visitor counts by country across all projects.
func (s *ReadStore) OverviewTopCountries(ctx context.Context, from, to time.Time, limit int, env string) ([]domain.OverviewCountryStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT project_id, COALESCE(country_code, 'unknown'), COUNT(*) AS count
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND country_code != '' AND (? = '' OR environment = ?)
		GROUP BY project_id, country_code
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.OverviewCountryStat
	for rows.Next() {
		var c domain.OverviewCountryStat
		if err := rows.Scan(&c.ProjectID, &c.CountryCode, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// overviewDimensionColumns whitelists the columns allowed for a dimension
// breakdown, guarding against SQL injection via the dimension name.
var overviewDimensionColumns = map[string]string{
	"device_type":     "device_type",
	"country_code":    "country_code",
	"browser":         "browser",
	"os":              "os",
	"referrer_domain": "referrer_domain",
	"utm_source":      "utm_source",
}

// OverviewDimensionBreakdown returns instance-wide counts grouped by one
// whitelisted dimension (device/country/browser/os/referrer/utm_source),
// powering the "visitors per type" view. Unknown dimensions return nil.
func (s *ReadStore) OverviewDimensionBreakdown(ctx context.Context, dimension string, from, to time.Time, limit int, env string) ([]domain.DimensionStat, error) {
	col, ok := overviewDimensionColumns[dimension]
	if !ok {
		return nil, fmt.Errorf("unsupported dimension %q", dimension)
	}
	if limit <= 0 {
		limit = 10
	}
	q := fmt.Sprintf(`
		SELECT COALESCE(%s, '(unknown)'), COUNT(*) AS count
		FROM events
		WHERE occurred_at >= ? AND occurred_at <= ? AND %s IS NOT NULL AND %s != '' AND (? = '' OR environment = ?)
		GROUP BY %s
		ORDER BY count DESC
		LIMIT ?`, col, col, col, col)
	rows, err := s.rdb.QueryContext(ctx, q, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.DimensionStat
	for rows.Next() {
		var d domain.DimensionStat
		if err := rows.Scan(&d.Value, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListAllEvents returns events across all projects, newest first, using keyset
// pagination on (occurred_at, id) so deep pages don't degrade into full scans.
func (s *ReadStore) ListAllEvents(ctx context.Context, f domain.EventFilter, limit int) ([]domain.Event, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT id, project_id, session_id, COALESCE(user_id_hash,''), name,
			COALESCE(url,''), COALESCE(referrer,''), COALESCE(referrer_domain,''),
			COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''), COALESCE(utm_term,''), COALESCE(utm_content,''),
			COALESCE(properties,''), COALESCE(user_agent,''), COALESCE(browser,''), COALESCE(os,''), COALESCE(device_type,''), COALESCE(country_code,''),
			ingest_id, occurred_at, created_at, COALESCE(environment,'')
		FROM events
		WHERE 1=1`
	var args []any
	if f.ProjectID != "" {
		q += ` AND project_id = ?`
		args = append(args, f.ProjectID)
	}
	if f.Name != "" {
		q += ` AND name = ?`
		args = append(args, f.Name)
	}
	if f.Environment != "" {
		q += ` AND environment = ?`
		args = append(args, f.Environment)
	}
	if !f.CursorOccurredAt.IsZero() {
		// Strict "older than" comparison on the composite key.
		q += ` AND (occurred_at < ? OR (occurred_at = ? AND id < ?))`
		args = append(args, f.CursorOccurredAt, f.CursorOccurredAt, f.CursorID)
	}
	q += ` ORDER BY occurred_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}
