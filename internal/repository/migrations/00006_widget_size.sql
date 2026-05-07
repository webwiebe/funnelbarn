-- +goose Up
ALTER TABLE dashboard_widgets ADD COLUMN size INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE dashboard_widgets DROP COLUMN size;
