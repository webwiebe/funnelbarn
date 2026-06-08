-- +goose Up
ALTER TABLE recordings ADD COLUMN first_chunk_index INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN on older versions; migration is a no-op on rollback.
