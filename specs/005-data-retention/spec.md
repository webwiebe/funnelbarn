# Spec 005: Event Data Retention + Schema Versioning

## Goal
Events grow indefinitely. Add configurable TTL-based purging. Also add schema version tracking so future migrations have a clear mechanism.

## Files to modify
- `internal/storage/schema.go` — add `schema_migrations` table, bump schema if needed
- `internal/storage/db.go` — add `PurgeOldEvents(ctx, cutoff time.Time) (int64, error)`
- `internal/config/config.go` — add `EventRetentionDays int` field (default 90, 0 = disabled)
- `cmd/funnelbarn/main.go` — call purge in `runBackgroundWorker()` daily

## Schema changes (internal/storage/schema.go)

Add to the schema SQL string:
```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);
```

## Config (internal/config/config.go)
Add field to `Config` struct:
```go
EventRetentionDays int // 0 = disabled; default 90
```

Load from env var `FUNNELBARN_EVENT_RETENTION_DAYS`. Default 90.

## Storage method (internal/storage/db.go)
```go
func (s *Store) PurgeOldEvents(ctx context.Context, before time.Time) (int64, error) {
    res, err := s.db.ExecContext(ctx,
        `DELETE FROM events WHERE occurred_at < ?`,
        before.UTC().Format(time.RFC3339),
    )
    if err != nil {
        return 0, err
    }
    return res.RowsAffected()
}
```

## Background worker (cmd/funnelbarn/main.go)
In `runBackgroundWorker()`, add a daily purge ticker alongside the existing spool ticker:
```go
purgeTicker := time.NewTicker(24 * time.Hour)
defer purgeTicker.Stop()
```

In the select loop:
```go
case <-purgeTicker.C:
    if cfg.EventRetentionDays > 0 {
        cutoff := time.Now().AddDate(0, 0, -cfg.EventRetentionDays)
        n, err := store.PurgeOldEvents(ctx, cutoff)
        if err != nil {
            slog.Error("purge old events", "err", err)
        } else if n > 0 {
            slog.Info("purged old events", "count", n, "before", cutoff.Format(time.DateOnly))
        }
    }
```

## Acceptance criteria
- `schema_migrations` table exists after fresh schema init
- `FUNNELBARN_EVENT_RETENTION_DAYS=90` config works
- `PurgeOldEvents` deletes rows older than cutoff and returns count
- Daily purge runs in background worker when retention is enabled (>0)
- `go build ./...` and `go test ./...` pass
