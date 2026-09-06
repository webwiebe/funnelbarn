-- +goose Up
-- Events and sessions written before the ingest pipeline defaulted an absent
-- environment carry environment = ''. Every analytics query filters with
-- `(? = '' OR environment = ?)`, so those rows are invisible under the
-- dashboard's default "production" filter and only reappear under "all".
-- File them as production: the environment field postdates the SDKs that wrote
-- them, and those SDKs only ever ran against production.
--
-- Idempotent: the predicate is the same one that selects the rows, so a re-run
-- matches nothing. Scoped to environment = '' — no row with a real tag is
-- touched.
UPDATE events   SET environment = 'production' WHERE environment = '';
UPDATE sessions SET environment = 'production' WHERE environment = '';

-- +goose Down
-- Not reversible: the original rows are indistinguishable from events that
-- were genuinely tagged 'production' at ingest, so undoing the backfill would
-- untag both. Left as a no-op rather than losing correct data.
SELECT 1;
