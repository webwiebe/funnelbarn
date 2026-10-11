-- +goose Up
-- conversion_window_seconds bounds a `user`-scoped funnel (#331): step N
-- counts only when it happens within this many seconds of step 1. NULL means
-- the default window (30 days) for a `user` funnel. The `session` and
-- `page_view` scopes ignore the column, so existing funnels are unchanged.
ALTER TABLE funnels ADD COLUMN conversion_window_seconds INTEGER;

-- +goose Down
ALTER TABLE funnels DROP COLUMN conversion_window_seconds;
