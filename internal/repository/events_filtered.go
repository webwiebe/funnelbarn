package repository

import (
	"context"
	"time"
)

// EventListFilter narrows ListEventsFiltered. An empty field matches everything and
// a zero From or To leaves that side of the window open.
type EventListFilter struct {
	Name        string
	Environment string
	From        time.Time
	To          time.Time
}

// ListEventsFiltered returns a paginated list of events for a project that
// match f, newest first.
func (s *ReadStore) ListEventsFiltered(ctx context.Context, projectID string, f EventListFilter, limit, offset int) ([]Event, error) {
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
		WHERE project_id = ?`
	args := []any{projectID}
	if f.Name != "" {
		q += " AND name = ?"
		args = append(args, f.Name)
	}
	if f.Environment != "" {
		q += " AND environment = ?"
		args = append(args, f.Environment)
	}
	if !f.From.IsZero() {
		q += " AND occurred_at >= ?"
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		q += " AND occurred_at <= ?"
		args = append(args, f.To)
	}
	q += " ORDER BY occurred_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := s.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}
