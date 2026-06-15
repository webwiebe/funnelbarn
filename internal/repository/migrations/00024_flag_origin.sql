-- +goose Up
-- origin distinguishes flags created by a human ('manual') from those auto-created
-- on first evaluation ('auto'). last_evaluated_at tracks when an auto flag was last
-- seen so the retention sweep can prune stale, never-configured ones.
ALTER TABLE feature_flags ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE feature_flags ADD COLUMN last_evaluated_at DATETIME;

-- +goose Down
ALTER TABLE feature_flags DROP COLUMN last_evaluated_at;
ALTER TABLE feature_flags DROP COLUMN origin;
