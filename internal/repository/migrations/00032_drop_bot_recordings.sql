-- +goose Up
-- Bot recordings (is_bot = 1) are 56% of the production recordings table, 91%
-- of it our own crawler, every one with a full DOM snapshot. Ingest now refuses
-- them, but the existing rows still have to go.
--
-- The ROWS are not deleted here. Each recording's rrweb chunks live in R2 under
-- a key derived from (project_id, recording_id, chunk_index) — no SQL statement
-- can reach them, and deleting the row destroys the only record of what those
-- keys were, stranding ~6,000 gzipped DOM snapshots in object storage forever.
-- RecordingService.PurgeBotRecordings does the delete instead: chunks from R2
-- first, then the row. It runs on the maintenance cycle and is idempotent.
--
-- What this migration does is make that sweep cheap and clean up after the
-- deletes that already happened without one.

-- The sweep's lookup is `WHERE is_bot = 1`, run every maintenance cycle
-- forever. Partial, so it indexes only the shrinking bot set and costs nothing
-- once the backlog is drained.
CREATE INDEX IF NOT EXISTS idx_recordings_bot ON recordings(is_bot) WHERE is_bot = 1;

-- recording_traces has no foreign key to recordings, so every recording deleted
-- by the retention purge (and by project deletes) left its trace rows behind.
-- DeleteRecording now removes them; this clears the ones already stranded.
-- Idempotent: the predicate is "parent row is gone", which a re-run re-checks.
DELETE FROM recording_traces
WHERE recording_id NOT IN (SELECT id FROM recordings);

-- +goose Down
DROP INDEX IF EXISTS idx_recordings_bot;
