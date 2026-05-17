-- +goose Up
CREATE TABLE IF NOT EXISTS instance_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO instance_settings (key, value) VALUES ('geo_enabled', 'true');

-- +goose Down
DROP TABLE IF EXISTS instance_settings;
