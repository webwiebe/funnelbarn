package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Session represents an analytics session.
type Session struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	EventCount  int       `json:"event_count"`
	EntryURL    string    `json:"entry_url"`
	ExitURL     string    `json:"exit_url"`
	Referrer    string    `json:"referrer"`
	UTMSource   string    `json:"utm_source"`
	UTMMedium   string    `json:"utm_medium"`
	UTMCampaign string    `json:"utm_campaign"`
	DeviceType  string    `json:"device_type"`
	CountryCode string    `json:"country_code"`
}

// UpsertSession inserts or updates a session record.
func (s *Store) UpsertSession(ctx context.Context, sess Session) error {
	const q = `
		INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, event_count, entry_url, exit_url, referrer, utm_source, utm_medium, utm_campaign, device_type, country_code)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			event_count  = event_count + 1,
			exit_url     = excluded.exit_url`
	_, err := s.db.ExecContext(ctx, q,
		sess.ID, sess.ProjectID, sess.FirstSeenAt, sess.LastSeenAt,
		nullStr(sess.EntryURL), nullStr(sess.ExitURL),
		nullStr(sess.Referrer), nullStr(sess.UTMSource), nullStr(sess.UTMMedium), nullStr(sess.UTMCampaign),
		nullStr(sess.DeviceType), nullStr(sess.CountryCode),
	)
	return err
}

// SessionByID fetches a session by its ID.
func (s *Store) SessionByID(ctx context.Context, id string) (Session, error) {
	const q = `
		SELECT id, project_id, first_seen_at, last_seen_at, event_count,
			COALESCE(entry_url,''), COALESCE(exit_url,''), COALESCE(referrer,''),
			COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''),
			COALESCE(device_type,''), COALESCE(country_code,'')
		FROM sessions WHERE id = ?`
	return scanSession(s.db.QueryRowContext(ctx, q, id))
}

// ListSessions returns paginated sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID string, limit, offset int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT id, project_id, first_seen_at, last_seen_at, event_count,
			COALESCE(entry_url,''), COALESCE(exit_url,''), COALESCE(referrer,''),
			COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''),
			COALESCE(device_type,''), COALESCE(country_code,'')
		FROM sessions
		WHERE project_id = ?
		ORDER BY last_seen_at DESC
		LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, q, projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID, &sess.ProjectID, &sess.FirstSeenAt, &sess.LastSeenAt, &sess.EventCount,
			&sess.EntryURL, &sess.ExitURL, &sess.Referrer,
			&sess.UTMSource, &sess.UTMMedium, &sess.UTMCampaign,
			&sess.DeviceType, &sess.CountryCode,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// ActiveSessionCount returns the number of sessions with last_seen_at within the last withinMinutes minutes.
func (s *Store) ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error) {
	const q = `
		SELECT COUNT(*) FROM sessions
		WHERE project_id = ?
		AND last_seen_at >= datetime('now', ? || ' minutes')`
	var count int64
	err := s.db.QueryRowContext(ctx, q, projectID, fmt.Sprintf("-%d", withinMinutes)).Scan(&count)
	return count, err
}

func scanSession(row *sql.Row) (Session, error) {
	var sess Session
	err := row.Scan(
		&sess.ID, &sess.ProjectID, &sess.FirstSeenAt, &sess.LastSeenAt, &sess.EventCount,
		&sess.EntryURL, &sess.ExitURL, &sess.Referrer,
		&sess.UTMSource, &sess.UTMMedium, &sess.UTMCampaign,
		&sess.DeviceType, &sess.CountryCode,
	)
	return sess, err
}
