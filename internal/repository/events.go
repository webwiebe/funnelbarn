package repository

import (
	"context"
	"database/sql"
	"time"
)

// Event represents a single analytics event.
type Event struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	SessionID      string    `json:"session_id"`
	UserIDHash     string    `json:"user_id_hash"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Referrer       string    `json:"referrer"`
	ReferrerDomain string    `json:"referrer_domain"`
	UTMSource      string    `json:"utm_source"`
	UTMMedium      string    `json:"utm_medium"`
	UTMCampaign    string    `json:"utm_campaign"`
	UTMTerm        string    `json:"utm_term"`
	UTMContent     string    `json:"utm_content"`
	Properties     string    `json:"properties"`
	UserAgent      string    `json:"user_agent"`
	Browser        string    `json:"browser"`
	OS             string    `json:"os"`
	DeviceType     string    `json:"device_type"`
	CountryCode    string    `json:"country_code"`
	IngestID       string    `json:"ingest_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// InsertEvent writes a new event to the database.
func (s *Store) InsertEvent(ctx context.Context, e Event) error {
	const q = `
		INSERT INTO events (
			id, project_id, session_id, user_id_hash, name,
			url, referrer, referrer_domain,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content,
			properties, user_agent, browser, os, device_type, country_code,
			ingest_id, occurred_at
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?
		)`
	_, err := s.db.ExecContext(ctx, q,
		e.ID, e.ProjectID, e.SessionID, nullStr(e.UserIDHash), e.Name,
		nullStr(e.URL), nullStr(e.Referrer), nullStr(e.ReferrerDomain),
		nullStr(e.UTMSource), nullStr(e.UTMMedium), nullStr(e.UTMCampaign), nullStr(e.UTMTerm), nullStr(e.UTMContent),
		nullStr(e.Properties), nullStr(e.UserAgent), nullStr(e.Browser), nullStr(e.OS), nullStr(e.DeviceType), nullStr(e.CountryCode),
		e.IngestID, e.OccurredAt,
	)
	return err
}

// ListEvents returns a paginated list of events for a project.
func (s *Store) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT id, project_id, session_id, COALESCE(user_id_hash,''), name,
			COALESCE(url,''), COALESCE(referrer,''), COALESCE(referrer_domain,''),
			COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''), COALESCE(utm_term,''), COALESCE(utm_content,''),
			COALESCE(properties,''), COALESCE(user_agent,''), COALESCE(browser,''), COALESCE(os,''), COALESCE(device_type,''), COALESCE(country_code,''),
			ingest_id, occurred_at, created_at
		FROM events
		WHERE project_id = ?
		ORDER BY occurred_at DESC
		LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// CountEvents returns the total event count for a project in a time range.
func (s *Store) CountEvents(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	const q = `SELECT COUNT(*) FROM events WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?`
	var n int64
	err := s.db.QueryRowContext(ctx, q, projectID, from, to).Scan(&n)
	return n, err
}

// TopPages returns the most visited pages for a project in a time range.
func (s *Store) TopPages(ctx context.Context, projectID string, from, to time.Time, limit int) ([]PageStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(url, '(unknown)'), COUNT(*) as views
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY url
		ORDER BY views DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PageStat
	for rows.Next() {
		var ps PageStat
		if err := rows.Scan(&ps.URL, &ps.Views); err != nil {
			return nil, err
		}
		stats = append(stats, ps)
	}
	return stats, rows.Err()
}

// PageStat is a page URL with view count.
type PageStat struct {
	URL   string `json:"url"`
	Views int64  `json:"views"`
}

// TopReferrers returns the most common referrer domains.
func (s *Store) TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]ReferrerStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(referrer_domain, '(direct)'), COUNT(*) as visits
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY referrer_domain
		ORDER BY visits DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ReferrerStat
	for rows.Next() {
		var rs ReferrerStat
		if err := rows.Scan(&rs.Domain, &rs.Visits); err != nil {
			return nil, err
		}
		stats = append(stats, rs)
	}
	return stats, rows.Err()
}

// ReferrerStat is a referrer domain with visit count.
type ReferrerStat struct {
	Domain string `json:"domain"`
	Visits int64  `json:"visits"`
}

// EventTimeSeries returns hourly event counts for a project over a time range.
func (s *Store) EventTimeSeries(ctx context.Context, projectID string, from, to time.Time) ([]TimeSeriesPoint, error) {
	const q = `
		SELECT strftime('%Y-%m-%dT%H:00:00Z', occurred_at) as hour, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY hour
		ORDER BY hour`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

// TimeSeriesPoint is a timestamp+count pair.
type TimeSeriesPoint struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

// TopUTMSources returns the most common UTM sources.
func (s *Store) TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int) ([]UTMStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(utm_source, '(none)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND utm_source != ''
		GROUP BY utm_source
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UTMStat
	for rows.Next() {
		var us UTMStat
		if err := rows.Scan(&us.Value, &us.Count); err != nil {
			return nil, err
		}
		stats = append(stats, us)
	}
	return stats, rows.Err()
}

// UTMStat is a UTM value with count.
type UTMStat struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// UniqueSessionCount returns distinct session IDs in a time range.
func (s *Store) UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	const q = `
		SELECT COUNT(DISTINCT session_id)
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?`
	var n int64
	err := s.db.QueryRowContext(ctx, q, projectID, from, to).Scan(&n)
	return n, err
}

// CountNewEvents returns new event count since a given time.
func (s *Store) CountNewEvents(ctx context.Context, projectID string, since time.Time) (int64, error) {
	const q = `SELECT COUNT(*) FROM events WHERE project_id = ? AND occurred_at >= ?`
	var n int64
	err := s.db.QueryRowContext(ctx, q, projectID, since).Scan(&n)
	return n, err
}

// GetEventByIngestID fetches an event by its ingest ID (idempotency check).
func (s *Store) GetEventByIngestID(ctx context.Context, ingestID string) (*Event, error) {
	const q = `
		SELECT id, project_id, session_id, COALESCE(user_id_hash,''), name,
			COALESCE(url,''), COALESCE(referrer,''), COALESCE(referrer_domain,''),
			COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''), COALESCE(utm_term,''), COALESCE(utm_content,''),
			COALESCE(properties,''), COALESCE(user_agent,''), COALESCE(browser,''), COALESCE(os,''), COALESCE(device_type,''), COALESCE(country_code,''),
			ingest_id, occurred_at, created_at
		FROM events WHERE ingest_id = ? LIMIT 1`
	rows, err := s.db.QueryContext(ctx, q, ingestID)
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

// TopBrowsers returns the most common browsers.
func (s *Store) TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]BrowserStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(browser, '(unknown)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY browser
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []BrowserStat
	for rows.Next() {
		var bs BrowserStat
		if err := rows.Scan(&bs.Browser, &bs.Count); err != nil {
			return nil, err
		}
		stats = append(stats, bs)
	}
	return stats, rows.Err()
}

// BrowserStat is a browser name with count.
type BrowserStat struct {
	Browser string `json:"browser"`
	Count   int64  `json:"count"`
}

// TopDeviceTypes returns breakdown by device type.
func (s *Store) TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time) ([]DeviceStat, error) {
	const q = `
		SELECT COALESCE(device_type, 'unknown'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY device_type
		ORDER BY count DESC`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DeviceStat
	for rows.Next() {
		var ds DeviceStat
		if err := rows.Scan(&ds.DeviceType, &ds.Count); err != nil {
			return nil, err
		}
		stats = append(stats, ds)
	}
	return stats, rows.Err()
}

// DeviceStat is a device type with count.
type DeviceStat struct {
	DeviceType string `json:"device_type"`
	Count      int64  `json:"count"`
}

// TopEventNames returns the most frequent event names.
func (s *Store) TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int) ([]EventNameStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT name, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY name
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// EventNameStat is an event name with count.
type EventNameStat struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// TopCountries returns breakdown by country code.
func (s *Store) TopCountries(ctx context.Context, projectID string, from, to time.Time, limit int) ([]CountryStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(country_code, 'unknown'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND country_code != ''
		GROUP BY country_code
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []CountryStat
	for rows.Next() {
		var cs CountryStat
		if err := rows.Scan(&cs.CountryCode, &cs.Count); err != nil {
			return nil, err
		}
		stats = append(stats, cs)
	}
	return stats, rows.Err()
}

// CountryStat is a country code with count.
type CountryStat struct {
	CountryCode string `json:"country_code"`
	Count       int64  `json:"count"`
}

// TopOSSystems returns breakdown by operating system.
func (s *Store) TopOSSystems(ctx context.Context, projectID string, from, to time.Time, limit int) ([]OSStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(os, '(unknown)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY os
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []OSStat
	for rows.Next() {
		var os OSStat
		if err := rows.Scan(&os.OS, &os.Count); err != nil {
			return nil, err
		}
		stats = append(stats, os)
	}
	return stats, rows.Err()
}

// OSStat is an OS name with count.
type OSStat struct {
	OS    string `json:"os"`
	Count int64  `json:"count"`
}

// TopUTMCampaigns returns breakdown by UTM campaign.
func (s *Store) TopUTMCampaigns(ctx context.Context, projectID string, from, to time.Time, limit int) ([]UTMStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(utm_campaign, '(none)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND utm_campaign != ''
		GROUP BY utm_campaign
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UTMStat
	for rows.Next() {
		var us UTMStat
		if err := rows.Scan(&us.Value, &us.Count); err != nil {
			return nil, err
		}
		stats = append(stats, us)
	}
	return stats, rows.Err()
}

// TopUTMMediums returns breakdown by UTM medium.
func (s *Store) TopUTMMediums(ctx context.Context, projectID string, from, to time.Time, limit int) ([]UTMStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(utm_medium, '(none)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND utm_medium != ''
		GROUP BY utm_medium
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UTMStat
	for rows.Next() {
		var us UTMStat
		if err := rows.Scan(&us.Value, &us.Count); err != nil {
			return nil, err
		}
		stats = append(stats, us)
	}
	return stats, rows.Err()
}

// DailyEventCounts returns daily event counts for a project over a time range.
func (s *Store) DailyEventCounts(ctx context.Context, projectID string, from, to time.Time) ([]TimeSeriesPoint, error) {
	const q = `
		SELECT strftime('%Y-%m-%d', occurred_at) as day, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY day
		ORDER BY day`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

// BounceRate computes the fraction of sessions with only 1 event.
func (s *Store) BounceRate(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	const q = `
		WITH session_counts AS (
			SELECT session_id, COUNT(*) as cnt
			FROM events
			WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
			GROUP BY session_id
		)
		SELECT
			CAST(SUM(CASE WHEN cnt = 1 THEN 1 ELSE 0 END) AS REAL) /
			CAST(COUNT(*) AS REAL)
		FROM session_counts`
	var rate sql.NullFloat64
	err := s.db.QueryRowContext(ctx, q, projectID, from, to).Scan(&rate)
	if err != nil {
		return 0, err
	}
	if !rate.Valid {
		return 0, nil
	}
	return rate.Float64, nil
}

// AvgEventsPerSession computes average events per session.
func (s *Store) AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	const q = `
		WITH session_counts AS (
			SELECT session_id, COUNT(*) as cnt
			FROM events
			WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
			GROUP BY session_id
		)
		SELECT AVG(cnt) FROM session_counts`
	var avg sql.NullFloat64
	err := s.db.QueryRowContext(ctx, q, projectID, from, to).Scan(&avg)
	if err != nil {
		return 0, err
	}
	if !avg.Valid {
		return 0, nil
	}
	return avg.Float64, nil
}

// DailyUniqueSessions returns daily unique session counts.
func (s *Store) DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time) ([]TimeSeriesPoint, error) {
	const q = `
		SELECT strftime('%Y-%m-%d', occurred_at) as day, COUNT(DISTINCT session_id) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ?
		GROUP BY day
		ORDER BY day`
	rows, err := s.db.QueryContext(ctx, q, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.Count); err != nil {
			return nil, err
		}
		series = append(series, p)
	}
	return series, rows.Err()
}

// TopOS is an alias for TopOSSystems kept for backward compatibility in tests.
func (s *Store) TopOS(ctx context.Context, projectID string, from, to time.Time, limit int) ([]OSStat, error) {
	return s.TopOSSystems(ctx, projectID, from, to, limit)
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.SessionID, &e.UserIDHash, &e.Name,
			&e.URL, &e.Referrer, &e.ReferrerDomain,
			&e.UTMSource, &e.UTMMedium, &e.UTMCampaign, &e.UTMTerm, &e.UTMContent,
			&e.Properties, &e.UserAgent, &e.Browser, &e.OS, &e.DeviceType, &e.CountryCode,
			&e.IngestID, &e.OccurredAt, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
