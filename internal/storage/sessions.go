package storage

import (
	"context"
	"database/sql"
	"time"
)

// Session represents an analytics session.
type Session struct {
	ID          string
	ProjectID   string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	EventCount  int
	EntryURL    string
	ExitURL     string
	Referrer    string
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	DeviceType  string
	CountryCode string
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
