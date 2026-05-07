-- +goose Up
CREATE TABLE feature_flags (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    flag_key         TEXT NOT NULL,
    name             TEXT NOT NULL,
    flag_type        TEXT NOT NULL DEFAULT 'boolean',
    variants         TEXT NOT NULL DEFAULT '{}',
    default_variant  TEXT NOT NULL DEFAULT 'off',
    split            TEXT NOT NULL DEFAULT '{}',
    conversion_event TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'active',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, flag_key)
);

CREATE INDEX idx_flags_project ON feature_flags(project_id);

CREATE TABLE flag_evaluations (
    id           TEXT PRIMARY KEY,
    flag_id      TEXT NOT NULL REFERENCES feature_flags(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    variant      TEXT NOT NULL,
    context_hash TEXT NOT NULL,
    session_id   TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_flag_evals_flag ON flag_evaluations(flag_id);
CREATE INDEX idx_flag_evals_context ON flag_evaluations(flag_id, context_hash);

-- +goose Down
DROP TABLE IF EXISTS flag_evaluations;
DROP TABLE IF EXISTS feature_flags;
