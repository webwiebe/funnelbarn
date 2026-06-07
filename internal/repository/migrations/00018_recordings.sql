-- +goose Up
CREATE TABLE recordings (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id  TEXT NOT NULL,
    environment TEXT NOT NULL DEFAULT '',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    started_at  DATETIME NOT NULL,
    ended_at    DATETIME,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_recordings_project     ON recordings(project_id, started_at DESC);
CREATE INDEX idx_recordings_session     ON recordings(session_id);
CREATE INDEX idx_recordings_project_env ON recordings(project_id, environment, started_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_recordings_project_env;
DROP INDEX IF EXISTS idx_recordings_session;
DROP INDEX IF EXISTS idx_recordings_project;
DROP TABLE IF EXISTS recordings;
