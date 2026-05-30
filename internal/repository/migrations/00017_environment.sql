-- +goose Up
ALTER TABLE events ADD COLUMN environment TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN environment TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_events_env ON events(project_id, environment);

-- +goose Down
DROP INDEX IF EXISTS idx_events_env;
