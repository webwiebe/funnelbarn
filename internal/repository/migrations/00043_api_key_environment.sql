-- +goose Up
-- environment maps a key to one deploy environment (#545). When a per-environment
-- project is merged into the product project, its old ingest keys keep working
-- and file every event they carry under the environment the old project stood
-- for. Empty means the key does not choose one: the event's own environment
-- attribute (or production when it has none) decides, as before.
ALTER TABLE api_keys ADD COLUMN environment TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN environment;
