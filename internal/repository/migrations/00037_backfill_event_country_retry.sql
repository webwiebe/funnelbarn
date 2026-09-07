-- +goose Up
-- 00035 was a no-op on production and it should not have been.
--
-- 00034 rebuilt the sessions table and took country_code from the EVENT rather
-- than from the session, so the rebuild overwrote all 5,249 session countries
-- with the event's empty string. 00035 ran seconds later, found no session with
-- a country to copy from, and filled nothing. Both sides ended up empty. The
-- session values were restored from the litestream replica; this re-runs the
-- events half, which 00035 cannot do again now that goose has recorded it.
--
-- 00034 is corrected in the same change, so a database migrating from scratch
-- never loses the value and reaches this migration with nothing left to do.
--
-- Identical predicate and effect to 00035: fills only events with no country of
-- their own, only from sessions that have one, joined on (project_id,
-- session_id). Idempotent, and a no-op on any database where 00035 already did
-- its job.
UPDATE events
SET country_code = (
    SELECT s.country_code FROM sessions s
    WHERE s.project_id = events.project_id
      AND s.id         = events.session_id
)
WHERE COALESCE(country_code, '') = ''
  AND EXISTS (
    SELECT 1 FROM sessions s
    WHERE s.project_id = events.project_id
      AND s.id         = events.session_id
      AND COALESCE(s.country_code, '') <> ''
  );

-- +goose Down
-- Not reversible: a backfilled country is indistinguishable from one resolved
-- at ingest, so undoing this would clear both.
SELECT 1;
