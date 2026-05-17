-- +goose Up
ALTER TABLE sessions ADD COLUMN ip TEXT;
ALTER TABLE sessions ADD COLUMN city TEXT;
ALTER TABLE sessions ADD COLUMN region TEXT;
ALTER TABLE sessions ADD COLUMN latitude REAL;
ALTER TABLE sessions ADD COLUMN longitude REAL;
ALTER TABLE sessions ADD COLUMN timezone TEXT;
ALTER TABLE sessions ADD COLUMN asn_org TEXT;
ALTER TABLE sessions ADD COLUMN connection_class TEXT;
ALTER TABLE sessions ADD COLUMN geo_anonymized INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_sessions_ip ON sessions (ip) WHERE ip IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_sessions_ip;
