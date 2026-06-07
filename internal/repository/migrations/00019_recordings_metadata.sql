-- +goose Up
ALTER TABLE recordings ADD COLUMN device_type TEXT NOT NULL DEFAULT '';
ALTER TABLE recordings ADD COLUMN user_agent  TEXT NOT NULL DEFAULT '';
ALTER TABLE recordings ADD COLUMN is_bot      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recordings ADD COLUMN page_url    TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_recordings_device ON recordings(project_id, device_type, started_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_recordings_device;
-- SQLite does not support DROP COLUMN; schema is rolled back by recreating DB in dev.
