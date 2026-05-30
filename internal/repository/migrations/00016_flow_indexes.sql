-- +goose Up
-- Partial index covering page_view events with all columns the flow queries need.
-- The WHERE clause keeps this index compact: it only indexes page_view rows.
-- Supports the page_seq CTE (project + time range) and the window function over session_id.
CREATE INDEX IF NOT EXISTS idx_events_pageview_session_url
ON events (project_id, occurred_at, session_id, url, referrer_domain)
WHERE name = 'page_view';

-- Supports flowTotalSessions which looks up a specific URL within a time range.
CREATE INDEX IF NOT EXISTS idx_events_pageview_url
ON events (project_id, url, occurred_at, session_id)
WHERE name = 'page_view';

-- +goose Down
DROP INDEX IF EXISTS idx_events_pageview_session_url;
DROP INDEX IF EXISTS idx_events_pageview_url;
