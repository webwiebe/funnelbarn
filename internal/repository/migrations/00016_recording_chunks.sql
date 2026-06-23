-- +goose Up
CREATE TABLE IF NOT EXISTS recording_chunks (
    id          TEXT     PRIMARY KEY,
    project_id  TEXT     NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id  TEXT     NOT NULL,
    chunk_index INTEGER  NOT NULL DEFAULT 0,
    events_json TEXT     NOT NULL,
    received_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recording_chunks_session
    ON recording_chunks(session_id);

CREATE INDEX IF NOT EXISTS idx_recording_chunks_project_received
    ON recording_chunks(project_id, received_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_recording_chunks_project_received;
DROP INDEX IF EXISTS idx_recording_chunks_session;
DROP TABLE IF EXISTS recording_chunks;
