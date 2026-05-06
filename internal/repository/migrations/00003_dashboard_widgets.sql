-- +goose Up
CREATE TABLE dashboard_widgets (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_name TEXT NOT NULL,
    property   TEXT NOT NULL,
    title      TEXT NOT NULL DEFAULT '',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_widgets_project ON dashboard_widgets(project_id);

-- +goose Down
DROP TABLE IF EXISTS dashboard_widgets;
