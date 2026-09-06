-- +goose Up
-- events.country_code was never written: the geo lookup only ever landed on the
-- session row. #246 fixed the ingest path, but every event already stored still
-- has an empty country, and TopCountries / OverviewTopCountries group events,
-- not sessions.
--
-- The value can be recovered from sessions, which did receive it — but only now.
-- This backfill joins events to sessions on (project_id, session_id), and that
-- join was ambiguous until 00034: session IDs were shared across projects
-- (28 of them, covering 54% of the events table), so before the composite key
-- an event could have picked up the country of a different visitor in a
-- different project. Held back from #246 for exactly that reason.
--
-- Only fills rows that have no country of their own, and only from sessions
-- that have one. Idempotent: the predicate is the condition that selects the
-- rows, so a re-run matches nothing new, and no event that already carries a
-- country is touched.
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
