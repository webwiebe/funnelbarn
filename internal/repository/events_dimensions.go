package repository

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// TopBrowsers returns the most common browsers.
func (s *ReadStore) TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.BrowserStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(browser, '(unknown)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY browser
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.BrowserStat
	for rows.Next() {
		var bs domain.BrowserStat
		if err := rows.Scan(&bs.Browser, &bs.Count); err != nil {
			return nil, err
		}
		stats = append(stats, bs)
	}
	return stats, rows.Err()
}

// TopDeviceTypes returns breakdown by device type.
func (s *ReadStore) TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.DeviceStat, error) {
	const q = `
		SELECT COALESCE(device_type, 'unknown'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY device_type
		ORDER BY count DESC`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.DeviceStat
	for rows.Next() {
		var ds domain.DeviceStat
		if err := rows.Scan(&ds.DeviceType, &ds.Count); err != nil {
			return nil, err
		}
		stats = append(stats, ds)
	}
	return stats, rows.Err()
}

// TopEventNames returns the most frequent event names.
func (s *ReadStore) TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.EventNameStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT name, COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY name
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// TopCountries returns breakdown by country code.
func (s *ReadStore) TopCountries(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.CountryStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(country_code, 'unknown'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND country_code != '' AND (? = '' OR environment = ?)
		GROUP BY country_code
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.CountryStat
	for rows.Next() {
		var cs domain.CountryStat
		if err := rows.Scan(&cs.CountryCode, &cs.Count); err != nil {
			return nil, err
		}
		stats = append(stats, cs)
	}
	return stats, rows.Err()
}

// TopOSSystems returns breakdown by operating system.
func (s *ReadStore) TopOSSystems(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.OSStat, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
		SELECT COALESCE(os, '(unknown)'), COUNT(*) as count
		FROM events
		WHERE project_id = ? AND occurred_at >= ? AND occurred_at <= ? AND (? = '' OR environment = ?)
		GROUP BY os
		ORDER BY count DESC
		LIMIT ?`
	rows, err := s.rdb.QueryContext(ctx, q, projectID, from, to, env, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.OSStat
	for rows.Next() {
		var os domain.OSStat
		if err := rows.Scan(&os.OS, &os.Count); err != nil {
			return nil, err
		}
		stats = append(stats, os)
	}
	return stats, rows.Err()
}

// TopUTMCampaigns returns breakdown by UTM campaign.
func (s *ReadStore) TopUTMCampaigns(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.UTMStat, error) {
	return topUTMColumn(ctx, s.rdb, projectID, "utm_campaign", env, from, to, limit)
}

// TopUTMMediums returns breakdown by UTM medium.
func (s *ReadStore) TopUTMMediums(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.UTMStat, error) {
	return topUTMColumn(ctx, s.rdb, projectID, "utm_medium", env, from, to, limit)
}

// TopOS is an alias for TopOSSystems kept for backward compatibility in tests.
func (s *ReadStore) TopOS(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.OSStat, error) {
	return s.TopOSSystems(ctx, projectID, from, to, limit, env)
}
