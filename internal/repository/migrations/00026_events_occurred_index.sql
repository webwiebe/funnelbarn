-- +goose Up
-- Cross-project analytics filters events by occurred_at alone (no project_id),
-- so the existing (project_id, occurred_at) indexes don't apply. This time-only
-- index keeps instance-wide time-range scans and keyset pagination off a full scan.
CREATE INDEX IF NOT EXISTS idx_events_occurred ON events (occurred_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_events_occurred;
