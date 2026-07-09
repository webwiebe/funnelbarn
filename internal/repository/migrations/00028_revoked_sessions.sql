-- +goose Up
-- Server-side session revocation (SESS-1). Session tokens are stateless HMAC
-- tokens; logging out records the token's hash here so it cannot be replayed
-- until it expires naturally. Rows are pruned once expires_at has passed.
CREATE TABLE revoked_sessions (
    token_hash TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL
);

CREATE INDEX idx_revoked_sessions_expires ON revoked_sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS revoked_sessions;
