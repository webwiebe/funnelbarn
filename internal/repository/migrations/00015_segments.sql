-- +goose Up
CREATE TABLE IF NOT EXISTS segments (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    rules      TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_segments_project ON segments (project_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS segments;
