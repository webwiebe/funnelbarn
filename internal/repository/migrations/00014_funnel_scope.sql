-- +goose Up
ALTER TABLE funnels ADD COLUMN scope TEXT NOT NULL DEFAULT 'session';

-- +goose Down
