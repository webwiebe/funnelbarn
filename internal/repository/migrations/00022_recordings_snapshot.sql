-- +goose Up
ALTER TABLE recordings ADD COLUMN last_chunk_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recordings ADD COLUMN has_snapshot     INTEGER NOT NULL DEFAULT 0;

-- Backfill existing rows. Before this change a recording's playable span was
-- inferred from first_chunk_index + chunk_count, and the rrweb full snapshot
-- always lands in chunk 0, so a surviving snapshot implies first_chunk_index = 0.
UPDATE recordings SET has_snapshot = 1 WHERE first_chunk_index = 0;
UPDATE recordings SET last_chunk_index = first_chunk_index + chunk_count - 1 WHERE chunk_count > 0;

-- +goose Down
-- SQLite does not support DROP COLUMN on older versions; migration is a no-op on rollback.
