-- +goose Up
-- recording_chunks records which chunk indexes of a recording have been
-- applied, so a chunk delivered twice is counted once (#302).
--
-- recordings.chunk_count used to grow by one on every upload of a chunk. The
-- SDK retries an upload whose response it never saw, and a queue consumer
-- redelivers a message it could not acknowledge, so one chunk could count
-- several times. ApplyChunk checks this table and the recording row in one
-- transaction and skips a chunk it has already applied.
--
-- No backfill: the indexes of chunks applied before this table existed are not
-- recorded anywhere (a recording row keeps only first, last and a count). A
-- redelivery of one of those chunks is counted once more, as it was before.
CREATE TABLE recording_chunks (
    recording_id TEXT NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    chunk_index  INTEGER NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (recording_id, chunk_index)
);

-- +goose Down
DROP TABLE recording_chunks;
