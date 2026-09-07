-- +goose Up
-- canonical_events was seeded by 00027 with 6 keys; event_name_mappings has
-- never had a row. Nothing has ever been mapped, so page.view and seo.page_view
-- (2,885 events between them) remain invisible to anything built on page_view.
--
-- The machinery for this all exists and is wired end to end: the repository
-- layer, MappingSuggestions, the REST endpoints, the Event Mapping page, and
-- the read path in AnalyzeCanonicalFunnel that joins through the table. Mapping
-- is applied at READ time, not at ingest — the raw name is preserved on the
-- event and resolved through this table when a canonical funnel is analysed.
-- That is reversible and rewrites no history, and is the design 00027 already
-- committed to.
--
-- What was missing is rows: the table starts empty and stays empty until
-- someone opens the page and saves. This seeds the mappings that are true by
-- SPELLING alone, so the feature is useful on first load and the suggestions a
-- human then reviews are the genuinely ambiguous ones.
--
-- Strictly syntactic. A raw name qualifies only when it is the same word as a
-- catalog key with different separators or casing. Names that differ in MEANING
-- are never seeded, however similar they look: login_started is not login, and
-- signup_started is not signup_completed — collapsing a start into a completion
-- would silently inflate every funnel built on it. Those stay on the Event
-- Mapping page for a human to decide.
--
-- Idempotent: NOT EXISTS plus ON CONFLICT DO NOTHING, and the join is over
-- names that are already present. A mapping a human has already made is never
-- overwritten.

-- 1. Exact match after stripping separators and case: page.view, page-view,
--    PageView -> page_view.
INSERT INTO event_name_mappings (project_id, raw_name, canonical_key)
SELECT DISTINCT e.project_id, e.name, c.key
FROM events e
JOIN canonical_events c
  ON replace(replace(replace(lower(e.name), '_', ''), '.', ''), '-', '')
   = replace(replace(replace(lower(c.key),  '_', ''), '.', ''), '-', '')
WHERE e.name <> ''
  AND NOT EXISTS (
    SELECT 1 FROM event_name_mappings m
    WHERE m.project_id = e.project_id AND m.raw_name = e.name
  )
ON CONFLICT(project_id, raw_name) DO NOTHING;

-- 2. The same match after dropping ONE leading namespace segment:
--    seo.page_view -> page_view. Only names carrying a dot are considered, the
--    remainder is matched exactly as above, and a name that already matched in
--    step 1 is excluded by the NOT EXISTS — so a namespace can never
--    manufacture a match the bare name would not have had.
INSERT INTO event_name_mappings (project_id, raw_name, canonical_key)
SELECT DISTINCT e.project_id, e.name, c.key
FROM events e
JOIN canonical_events c
  ON replace(replace(replace(lower(substr(e.name, instr(e.name, '.') + 1)), '_', ''), '.', ''), '-', '')
   = replace(replace(replace(lower(c.key), '_', ''), '.', ''), '-', '')
WHERE instr(e.name, '.') > 1
  AND instr(e.name, '.') < length(e.name)
  AND NOT EXISTS (
    SELECT 1 FROM event_name_mappings m
    WHERE m.project_id = e.project_id AND m.raw_name = e.name
  )
ON CONFLICT(project_id, raw_name) DO NOTHING;

-- +goose Down
-- Only the rows this migration could have created, and only where they still
-- say what it would have said — a mapping a human has since re-pointed is left
-- alone.
DELETE FROM event_name_mappings
WHERE (project_id, raw_name, canonical_key) IN (
    SELECT DISTINCT e.project_id, e.name, c.key
    FROM events e
    JOIN canonical_events c
      ON replace(replace(replace(lower(e.name), '_', ''), '.', ''), '-', '')
       = replace(replace(replace(lower(c.key), '_', ''), '.', ''), '-', '')
);
