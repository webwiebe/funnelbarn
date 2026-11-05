package repository

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// ProjectHourCount is one project's event count in one hour of created_at. Hour
// is the first 13 characters of the stored timestamp, such as "2026-10-30 18".
type ProjectHourCount struct {
	Project string
	Hour    string
	Count   int64
}

// EnvironmentEventsByHour returns the events of one environment stored at or after
// since per project and hour. It reads created_at because a client sets occurred_at:
// some send a date with no time, which puts a row up to a day before it arrived.
//
// created_at has no index, and a filter on it alone scanned the whole table. Over
// 2026-10-31 to 2026-11-05, with about 204,000 rows, 189 of 201 volume drop reads
// ran past the check's 30s deadline (FUN-65). The read now starts at
// the first rowid stored at or after since, so it reads only the window.
//
// The same scan took under 100ms on a local copy of that size, so the read is
// split into child spans: the wait for a read connection, the rowid bound and the
// count. They show which part a slow read spends its time in.
func (s *ReadStore) EnvironmentEventsByHour(ctx context.Context, env string, since time.Time) ([]ProjectHourCount, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.EnvironmentEventsByHour")
	defer span.End()
	out, err := s.environmentEventsByHour(ctx, env, since.UTC().Format(time.DateTime))
	if err != nil {
		tracing.RecordError(span, err)
	}
	return out, err
}

func (s *ReadStore) environmentEventsByHour(ctx context.Context, env, at string) ([]ProjectHourCount, error) {
	acquireCtx, acquire := tracing.StartSpan(ctx, "repository.acquire_read_conn")
	conn, err := s.rdb.Conn(acquireCtx)
	acquire.End()
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	boundCtx, bound := tracing.StartSpan(ctx, "repository.events_rowid_bound")
	from, lookups, err := firstRowidSince(boundCtx, conn, at)
	bound.SetAttributes(attribute.Int64("rowid.from", from), attribute.Int("lookups", lookups))
	bound.End()
	if err != nil {
		return nil, err
	}

	countCtx, count := tracing.StartSpan(ctx, "repository.events_hourly_count")
	defer count.End()
	out, err := hourlyCounts(countCtx, conn, from, at, env)
	count.SetAttributes(attribute.Int("groups", len(out)))
	return out, err
}

func hourlyCounts(ctx context.Context, conn *sql.Conn, from int64, at, env string) ([]ProjectHourCount, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT p.slug, x.hour, x.n
		FROM (
			SELECT project_id, substr(created_at, 1, 13) AS hour, COUNT(*) AS n
			FROM events
			WHERE rowid >= ? AND created_at >= ? AND environment = ?
			GROUP BY project_id, hour
		) x
		JOIN projects p ON p.id = x.project_id`, from, at, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectHourCount
	for rows.Next() {
		var c ProjectHourCount
		if err := rows.Scan(&c.Project, &c.Hour, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// firstRowidSince returns the lowest rowid from which every event has created_at
// at or after since, and the number of lookups it took. The single writer gives
// rows their rowid in insert order and created_at at insert, so created_at rises
// with rowid, and a binary search over rowid finds the boundary in about 20
// lookups on the rowid tree. A clock that stepped back would put a row a few
// seconds off the boundary, 8 days before the hour the volume drop check evaluates.
func firstRowidSince(ctx context.Context, conn *sql.Conn, since string) (int64, int, error) {
	var lo, hi sql.NullInt64
	// Two subqueries: SQLite answers a lone min() or max() of rowid from the end
	// of the tree, and both in one SELECT would scan.
	if err := conn.QueryRowContext(ctx,
		`SELECT (SELECT min(rowid) FROM events), (SELECT max(rowid) FROM events)`).Scan(&lo, &hi); err != nil {
		return 0, 0, err
	}
	if !lo.Valid {
		return 0, 0, nil
	}
	// The answer is the lowest x whose first row at or after x is in the window,
	// with max+1, past every row, as the answer when none is.
	l, h := lo.Int64, hi.Int64+1
	lookups := 0
	for l < h {
		mid := l + (h-l)/2
		lookups++
		var inWindow bool
		err := conn.QueryRowContext(ctx,
			`SELECT created_at >= ? FROM events WHERE rowid >= ? ORDER BY rowid LIMIT 1`, since, mid).Scan(&inWindow)
		if err != nil {
			return 0, lookups, err
		}
		if inWindow {
			h = mid
		} else {
			l = mid + 1
		}
	}
	return l, lookups, nil
}
