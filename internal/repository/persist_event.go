package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// execer is the write surface shared by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertEventSQL inserts the event unless a row with the same ingest_id
// exists. The ingest_id index is not unique (rows duplicated before this guard
// are left alone), so the guard lives in the statement: with one write
// connection the check and the insert cannot interleave with another insert.
// An empty ingest_id is never deduplicated; ingest always sets one, and only
// callers outside the ingest path (tests, the CLI) leave it empty.
const insertEventSQL = `
	INSERT INTO events (
		id, project_id, session_id, user_id_hash, name,
		url, referrer, referrer_domain,
		utm_source, utm_medium, utm_campaign, utm_term, utm_content,
		properties, user_agent, browser, os, device_type, country_code,
		page_view_id, ingest_id, occurred_at, environment
	)
	SELECT
		?, ?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?
	WHERE ? = '' OR NOT EXISTS (SELECT 1 FROM events WHERE ingest_id = ?)`

// insertEvent writes e unless its ingest_id is already stored, and reports
// whether a row was inserted.
func insertEvent(ctx context.Context, ex execer, e Event) (bool, error) {
	res, err := ex.ExecContext(ctx, insertEventSQL,
		e.ID, e.ProjectID, e.SessionID, nullStr(e.UserIDHash), e.Name,
		nullStr(e.URL), nullStr(e.Referrer), nullStr(e.ReferrerDomain),
		nullStr(e.UTMSource), nullStr(e.UTMMedium), nullStr(e.UTMCampaign), nullStr(e.UTMTerm), nullStr(e.UTMContent),
		nullStr(e.Properties), nullStr(e.UserAgent), nullStr(e.Browser), nullStr(e.OS), nullStr(e.DeviceType), nullStr(e.CountryCode),
		nullStr(e.PageViewID), e.IngestID, e.OccurredAt, e.Environment,
		e.IngestID, e.IngestID,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// InsertEvent writes a new event to the database. An event whose ingest_id is
// already stored is skipped.
func (s *Store) InsertEvent(ctx context.Context, e Event) error {
	_, err := insertEvent(ctx, s.db, e)
	return err
}

// PersistEvent stores an ingested event and its session effects in one
// transaction, and reports whether the event was new. A redelivered event
// (same ingest_id) changes nothing: the session's event_count only moves
// together with the insert, so a replay cannot count twice, and a failure
// part-way leaves neither the event nor the session change behind.
// signals may be nil.
func (s *Store) PersistEvent(ctx context.Context, e Event, sess Session, signals *SessionSignals) (inserted bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	inserted, err = insertEvent(ctx, tx, e)
	if err != nil {
		return false, fmt.Errorf("insert event: %w", err)
	}
	if !inserted {
		return false, tx.Commit()
	}
	if err = upsertSession(ctx, tx, sess); err != nil {
		return false, fmt.Errorf("upsert session: %w", err)
	}
	if signals != nil {
		if err = updateSessionSignals(ctx, tx, sess.ProjectID, sess.ID, *signals); err != nil {
			return false, fmt.Errorf("session signals: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// CountDuplicateIngestIDs counts ingest_ids stored more than once. Events
// written before PersistEvent's guard could be duplicated by a replay; a zero
// count in every environment is what would allow a unique index later.
func (s *ReadStore) CountDuplicateIngestIDs(ctx context.Context) (int64, error) {
	const q = `SELECT COUNT(*) FROM (SELECT ingest_id FROM events GROUP BY ingest_id HAVING COUNT(*) > 1)`
	var n int64
	err := s.rdb.QueryRowContext(ctx, q).Scan(&n)
	return n, err
}
