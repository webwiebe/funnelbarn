-- +goose Up
CREATE TABLE project_health (
    project_id          TEXT    PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    setup_called        INTEGER NOT NULL DEFAULT 0,
    events_received     INTEGER NOT NULL DEFAULT 0,
    flags_evaluated     INTEGER NOT NULL DEFAULT 0,
    recordings_received INTEGER NOT NULL DEFAULT 0,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS project_health;
