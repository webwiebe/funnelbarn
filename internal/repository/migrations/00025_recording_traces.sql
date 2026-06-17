-- +goose Up
-- recording_traces links a recording's timeline to the W3C trace_ids observed in
-- the browser during that recording. It is the join key across the observability
-- stack: SpanBarn (traces) and BugBarn (errors) carry a trace_id, and this table
-- resolves that trace_id back to the FunnelBarn session + recording so the session
-- can be replayed at the exact moment the trace fired.
--
-- A single recording can hold many trace_ids (one per instrumented request), each
-- timestamped so the replay can seek. occurred_at is the wall-clock time the trace
-- was observed in the browser; the seek offset is occurred_at - recordings.started_at.
CREATE TABLE recording_traces (
    project_id   TEXT NOT NULL,
    session_id   TEXT NOT NULL,
    recording_id TEXT NOT NULL,
    trace_id     TEXT NOT NULL,
    span_id      TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL DEFAULT '',
    occurred_at  DATETIME NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (recording_id, trace_id, occurred_at)
);

-- Reverse lookup (SpanBarn/BugBarn trace_id -> recording). Scoped by project so a
-- leaked/guessed trace_id can only resolve within the owning project's key scope.
CREATE INDEX idx_recording_traces_lookup ON recording_traces (project_id, trace_id);
-- Forward lookup (recording -> ordered trace timeline) for the replay UI/CLI.
CREATE INDEX idx_recording_traces_recording ON recording_traces (recording_id, occurred_at);
-- Session-level rollup.
CREATE INDEX idx_recording_traces_session ON recording_traces (session_id);

-- +goose Down
DROP TABLE recording_traces;
