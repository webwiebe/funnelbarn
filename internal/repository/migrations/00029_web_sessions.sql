-- +goose Up
-- Token-bound server-side sessions. The browser cookie holds only an opaque
-- random handle; id_hash is its SHA-256 hex and the row key. OIDC sessions
-- carry the iambarn token set (renewed via the refresh_token grant); local
-- password/admin sessions leave the token columns NULL. Deleting a row IS
-- revocation — it is effective on the next request, which replaces the old
-- stateless-HMAC + revocation-list scheme.
CREATE TABLE web_sessions (
    id_hash               TEXT PRIMARY KEY,
    username              TEXT NOT NULL,
    auth_method           TEXT NOT NULL CHECK (auth_method IN ('oidc', 'local')),
    idp_sub               TEXT,
    idp_sid               TEXT,
    id_token              TEXT,
    access_token          TEXT,
    refresh_token         TEXT,
    access_expires_at     INTEGER,
    claims_json           TEXT,
    created_at            INTEGER NOT NULL,
    absolute_expires_at   INTEGER NOT NULL,
    last_refresh_at       INTEGER,
    refresh_failing_since INTEGER
);

CREATE INDEX idx_web_sessions_idp_sid ON web_sessions (idp_sid);
CREATE INDEX idx_web_sessions_idp_sub ON web_sessions (idp_sub);
CREATE INDEX idx_web_sessions_absolute_expires ON web_sessions (absolute_expires_at);

-- The stateless-HMAC revocation list is obsolete: sessions are now rows, and
-- deleting the row revokes the session.
DROP TABLE revoked_sessions;

-- +goose Down
CREATE TABLE revoked_sessions (
    token_hash TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL
);
CREATE INDEX idx_revoked_sessions_expires ON revoked_sessions (expires_at);

DROP TABLE IF EXISTS web_sessions;
