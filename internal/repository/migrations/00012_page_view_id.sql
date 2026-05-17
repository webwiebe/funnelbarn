-- +goose Up
ALTER TABLE events ADD COLUMN page_view_id TEXT;
CREATE INDEX IF NOT EXISTS idx_events_page_view ON events (page_view_id) WHERE page_view_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_events_page_view;
