-- +goose Up
CREATE INDEX IF NOT EXISTS idx_events_ingest_id ON events (ingest_id);

-- +goose Down
DROP INDEX IF EXISTS idx_events_ingest_id;
