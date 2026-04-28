package storage

// Schema is the complete SQLite DDL for Trailpost.
// All tables use TEXT primary keys (UUID v4) except where noted.
const Schema = `
CREATE TABLE IF NOT EXISTS projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT UNIQUE NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL UNIQUE,
    scope       TEXT NOT NULL DEFAULT 'full',
    last_used_at DATETIME,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions_http (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id      TEXT NOT NULL,
    user_id_hash    TEXT,
    name            TEXT NOT NULL,
    url             TEXT,
    referrer        TEXT,
    referrer_domain TEXT,
    utm_source      TEXT,
    utm_medium      TEXT,
    utm_campaign    TEXT,
    utm_term        TEXT,
    utm_content     TEXT,
    properties      TEXT,
    user_agent      TEXT,
    browser         TEXT,
    os              TEXT,
    device_type     TEXT,
    country_code    TEXT,
    ingest_id       TEXT NOT NULL,
    occurred_at     DATETIME NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_events_project_occurred ON events (project_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_session ON events (session_id);
CREATE INDEX IF NOT EXISTS idx_events_name ON events (project_id, name);

CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    first_seen_at DATETIME NOT NULL,
    last_seen_at  DATETIME NOT NULL,
    event_count  INTEGER NOT NULL DEFAULT 0,
    entry_url    TEXT,
    exit_url     TEXT,
    referrer     TEXT,
    utm_source   TEXT,
    utm_medium   TEXT,
    utm_campaign TEXT,
    device_type  TEXT,
    country_code TEXT
);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions (project_id, last_seen_at DESC);

CREATE TABLE IF NOT EXISTS funnels (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS funnel_steps (
    id          TEXT PRIMARY KEY,
    funnel_id   TEXT NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    step_order  INTEGER NOT NULL,
    event_name  TEXT NOT NULL,
    filters     TEXT
);

CREATE INDEX IF NOT EXISTS idx_funnel_steps_funnel ON funnel_steps (funnel_id, step_order);
`

// APIKeyScopeFull allows full API access.
const APIKeyScopeFull = "full"

// APIKeyScopeIngest allows only event ingest.
const APIKeyScopeIngest = "ingest"
