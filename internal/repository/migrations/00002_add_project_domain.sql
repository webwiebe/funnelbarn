-- +goose Up
ALTER TABLE projects ADD COLUMN domain TEXT;

-- +goose Down
-- SQLite does not support DROP COLUMN in older versions; this is a best-effort rollback.
-- For SQLite 3.35.0+ the below works:
ALTER TABLE projects DROP COLUMN domain;
