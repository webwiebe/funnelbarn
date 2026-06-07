-- +goose Up
CREATE TABLE project_recording_settings (
    project_id   TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    enabled      INTEGER,  -- NULL=inherit, 0=off, 1=on
    sample_rate  REAL,     -- NULL=inherit from instance
    rules        TEXT NOT NULL DEFAULT '[]',
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS project_recording_settings;
