package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// OrphanCounts reports rows whose project_id matches no project. Every read
// path is project-scoped, so such rows are unreachable: stored, indexed and
// backed up, but invisible to the product. 372 of them accumulated over three
// weeks in June 2026 and went unnoticed until an audit two months later.
type OrphanCounts struct {
	Events   int64
	Sessions int64
	Funnels  int64
	// APIKeys counts keys whose project is gone. Unlike the others these are
	// not merely unreachable — until #257 they still authenticated, so every
	// event they admitted was accepted with a 202 and then dead-lettered.
	APIKeys int64
}

// Total returns the number of orphaned rows across all tables.
func (o OrphanCounts) Total() int64 {
	return o.Events + o.Sessions + o.Funnels + o.APIKeys
}

// CountOrphanedRows counts rows whose project_id matches no row in projects.
// Foreign keys and a BEFORE-INSERT trigger both refuse these now, so a non-zero
// result means a guard has been bypassed and is worth an alert, not a metric.
func (s *ReadStore) CountOrphanedRows(ctx context.Context) (OrphanCounts, error) {
	const q = `
		SELECT
			(SELECT COUNT(*) FROM events   e WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = e.project_id)),
			(SELECT COUNT(*) FROM sessions s WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = s.project_id)),
			(SELECT COUNT(*) FROM funnels  f WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = f.project_id)),
			(SELECT COUNT(*) FROM api_keys k WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = k.project_id))`
	var out OrphanCounts
	if err := s.rdb.QueryRowContext(ctx, q).Scan(&out.Events, &out.Sessions, &out.Funnels, &out.APIKeys); err != nil {
		return OrphanCounts{}, err
	}
	return out, nil
}

// ListEvents returns a paginated list of events for a project.
func (s *ReadStore) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]domain.Event, error) {
	return s.ListEventsFiltered(ctx, projectID, domain.EventListFilter{}, limit, offset)
}

// CountEvents returns the total event count for a project in a time range.
func (s *ReadStore) CountEvents(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error) {
	const q = `SELECT COUNT(*) FROM events WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)`
	var n int64
	err := s.rdb.QueryRowContext(ctx, q, projectID, from, to, env, env).Scan(&n)
	return n, err
}

// TopPages returns the most visited pages for a project in a time range.
func (s *ReadStore) TopPages(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.PageStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(url, '(unknown)'), COUNT(*) as views
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY url
		ORDER BY views DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.PageStat
	for rows.Next() {
		var ps domain.PageStat
		if err := rows.Scan(&ps.URL, &ps.Views); err != nil {
			return nil, err
		}
		stats = append(stats, ps)
	}
	return stats, rows.Err()
}

// TopReferrers returns the most common referrer domains.
func (s *ReadStore) TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.ReferrerStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(referrer_domain, '(direct)'), COUNT(*) as visits
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY referrer_domain
		ORDER BY visits DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.ReferrerStat
	for rows.Next() {
		var rs domain.ReferrerStat
		if err := rows.Scan(&rs.Domain, &rs.Visits); err != nil {
			return nil, err
		}
		stats = append(stats, rs)
	}
	return stats, rows.Err()
}

// EventTimeSeries returns hourly event counts for a project over a time range.
func (s *ReadStore) EventTimeSeries(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error) {
	const q = `
		SELECT substr(occurred_at, 1, 13) || ':00:00Z' as hour, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY hour
		ORDER BY hour`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []domain.TimeSeriesPoint
	for rows.Next() {
		var p domain.TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

// topUTMColumn executes the common UTM breakdown query for the given column.
// column is an internal constant (utm_source, utm_medium, utm_campaign) — never user input.
func topUTMColumn(ctx context.Context, db *sql.DB, projectID, column, env string, from, to time.Time, limit int) ([]domain.UTMStat, error) {
	if limit <= 0 {
		limit = 10
	}
	q := fmt.Sprintf(`
		SELECT COALESCE(%s, '(none)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND %s != '' AND (? = '' OR environment = ?)
		GROUP BY %s
		ORDER BY count DESC
		LIMIT ?`, column, column, column)
	rows, err := db.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.UTMStat
	for rows.Next() {
		var us domain.UTMStat
		if err := rows.Scan(&us.Value, &us.Count); err != nil {
			return nil, err
		}
		stats = append(stats, us)
	}
	return stats, rows.Err()
}

// TopUTMSources returns the most common UTM sources.
func (s *ReadStore) TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.UTMStat, error) {
	return topUTMColumn(ctx, s.rdb, projectID, "utm_source", env, from, to, limit)
}

// UniqueSessionCount returns distinct session IDs in a time range.
func (s *ReadStore) UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error) {
	const q = `
		SELECT COUNT(DISTINCT session_id)
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)`
	var n int64
	err := s.rdb.QueryRowContext(ctx, q, projectID, from, to, env, env).Scan(&n)
	return n, err
}

// CountNewEvents returns new event count since a given time.
func (s *ReadStore) CountNewEvents(ctx context.Context, projectID string, since time.Time, env string) (int64, error) {
	const q = `SELECT COUNT(*) FROM events WHERE project_id = ? AND occurred_at >= ? AND (? = '' OR environment = ?)`
	var n int64
	err := s.rdb.QueryRowContext(ctx, q, projectID, since, env, env).Scan(&n)
	return n, err
}

// GetEventByIngestID fetches an event by its ingest ID (idempotency check).
func (s *ReadStore) GetEventByIngestID(ctx context.Context, ingestID string) (*domain.Event, error) {
	const q = `
		SELECT id, project_id, session_id, COALESCE(user_id_hash,''), name,
			COALESCE(url,''), COALESCE(referrer,''), COALESCE(referrer_domain,''),
			COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''), COALESCE(utm_term,''), COALESCE(utm_content,''),
			COALESCE(properties,''), COALESCE(user_agent,''), COALESCE(browser,''), COALESCE(os,''), COALESCE(device_type,''), COALESCE(country_code,''),
			ingest_id, occurred_at, created_at, COALESCE(environment,'')
		FROM events WHERE ingest_id = ? LIMIT 1`
	rows, err := s.rdb.QueryContext(ctx, q, ingestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil || len(events) == 0 {
		return nil, err
	}
	return &events[0], nil
}

// DailyEventCounts returns daily event counts for a project over a time range.
func (s *ReadStore) DailyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error) {
	const q = `
		SELECT substr(occurred_at, 1, 10) as day, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY day
		ORDER BY day`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []domain.TimeSeriesPoint
	for rows.Next() {
		var p domain.TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

// HourlyEventCounts returns hourly event counts for a project over a time range.
func (s *ReadStore) HourlyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error) {
	const q = `
		SELECT substr(occurred_at, 1, 13) || ':00:00' as hour, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY hour
		ORDER BY hour`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []domain.TimeSeriesPoint
	for rows.Next() {
		var p domain.TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

// BounceRate computes the fraction of sessions with only 1 event.
func (s *ReadStore) BounceRate(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error) {
	const q = `
		WITH session_counts AS (
			SELECT session_id, COUNT(*) as cnt
			FROM events
			WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
			GROUP BY session_id
		)
		SELECT
			CAST(SUM(CASE WHEN cnt = 1 THEN 1 ELSE 0 END) AS REAL) /
			CAST(COUNT(*) AS REAL)
		FROM session_counts`
	var rate sql.NullFloat64
	err := s.rdb.QueryRowContext(ctx, q, projectID, from, to, env, env).Scan(&rate)
	if err != nil {
		return 0, err
	}
	if !rate.Valid {
		return 0, nil
	}
	return rate.Float64, nil
}

// AvgEventsPerSession computes average events per session.
func (s *ReadStore) AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error) {
	const q = `
		WITH session_counts AS (
			SELECT session_id, COUNT(*) as cnt
			FROM events
			WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
			GROUP BY session_id
		)
		SELECT AVG(cnt) FROM session_counts`
	var avg sql.NullFloat64
	err := s.rdb.QueryRowContext(ctx, q, projectID, from, to, env, env).Scan(&avg)
	if err != nil {
		return 0, err
	}
	if !avg.Valid {
		return 0, nil
	}
	return avg.Float64, nil
}

// DailyUniqueSessions returns daily unique session counts.
func (s *ReadStore) DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error) {
	const q = `
		SELECT substr(occurred_at, 1, 10) as day, COUNT(DISTINCT session_id) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY day
		ORDER BY day`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []domain.TimeSeriesPoint
	for rows.Next() {
		var p domain.TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

func scanEvents(rows *sql.Rows) ([]domain.Event, error) {
	var events []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.SessionID, &e.UserIDHash, &e.Name,
			&e.URL, &e.Referrer, &e.ReferrerDomain,
			&e.UTMSource, &e.UTMMedium, &e.UTMCampaign, &e.UTMTerm, &e.UTMContent,
			&e.Properties, &e.UserAgent, &e.Browser, &e.OS, &e.DeviceType, &e.CountryCode,
			&e.IngestID, &e.OccurredAt, &e.CreatedAt, &e.Environment,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// SessionsForPage returns distinct session IDs that have at least one event
// with the given URL within the time range. Used to link flow nodes to recordings.
func (s *ReadStore) SessionsForPage(ctx context.Context, projectID, page string, from, to time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `
		SELECT DISTINCT session_id
		FROM events
		WHERE project_id = ? AND url = ? AND occurred_at >= ? AND occurred_at <= ?
		ORDER BY session_id LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, page, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
