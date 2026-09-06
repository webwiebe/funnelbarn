-- +goose Up
-- sessions was keyed on id alone. Two projects that ever emit the same session
-- ID upserted the SAME row, and its project_id ended up owned by whichever
-- project wrote last. In production 28 session IDs appear under more than one
-- project (one spans 10), covering 45,869 of 84,764 events — 54% of the table —
-- and 29 rows sit under a project none of their own events belong to. Every
-- per-project session and unique-visitor count is decided by write order rather
-- than by the data: one project reports 141 sessions against 2 real ones.
--
-- The key becomes (project_id, id), which requires a table rebuild — SQLite
-- cannot alter a primary key in place.
--
-- REPAIR RULE: split per project. A colliding session becomes one row per
-- (project_id, session_id) pair present in events, with first_seen_at,
-- last_seen_at, event_count and entry/exit URL re-derived from that project's
-- own events. Most-events-wins was rejected: it keeps one row and silently
-- drops a real session from every other project's counts, which is the same bug
-- redistributed. events is the authority here — it has always carried the
-- correct project_id per row (its own FK enforces it), so the ownership being
-- re-derived is recorded fact, not a guess.
--
-- GEO AND DEVICE SIGNALS carry over only to the row whose project matches the
-- old row's project_id. Those columns exist only on the session, never on an
-- event, so a split has no per-project source to rebuild them from — but the
-- old row's project_id and its geo were written by the same visitor in the same
-- upsert, so that one pair is internally consistent. The other split rows start
-- with them empty and refill on their next event. Copying one visitor's city
-- and ASN onto another project's session would be fabricated data that looks
-- real.
--
-- Sessions whose ID has NO events anywhere are carried across untouched: their
-- events were removed by the retention purge, and there is nothing to re-derive
-- from. Sessions whose ID does have events are rebuilt from those events alone,
-- which is what discards the 29 mis-attributed rows.
--
-- Idempotent: the scratch table is dropped first, and re-running re-derives the
-- same rows from the same events.

DROP TABLE IF EXISTS sessions_rebuild;

CREATE TABLE sessions_rebuild (
    id            TEXT NOT NULL,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    first_seen_at DATETIME NOT NULL,
    last_seen_at  DATETIME NOT NULL,
    event_count   INTEGER NOT NULL DEFAULT 0,
    entry_url     TEXT,
    exit_url      TEXT,
    referrer      TEXT,
    utm_source    TEXT,
    utm_medium    TEXT,
    utm_campaign  TEXT,
    device_type   TEXT,
    country_code  TEXT,
    ip               TEXT,
    city             TEXT,
    region           TEXT,
    latitude         REAL,
    longitude        REAL,
    timezone         TEXT,
    asn_org          TEXT,
    connection_class TEXT,
    geo_anonymized   INTEGER NOT NULL DEFAULT 0,
    screen_width      INTEGER,
    screen_height     INTEGER,
    pixel_ratio       REAL,
    touch             INTEGER,
    dark_mode         INTEGER,
    reduced_motion    INTEGER,
    browser_timezone  TEXT,
    cpu_cores         INTEGER,
    signals_collected INTEGER NOT NULL DEFAULT 0,
    environment       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, id)
);

-- One row per (project, session) actually present in events. The non-aggregate
-- attributes come from that project's FIRST event for the session, which is
-- what UpsertSession recorded originally (it sets them on INSERT and leaves
-- them alone on every later event); exit_url comes from the last.
INSERT INTO sessions_rebuild (
    id, project_id, first_seen_at, last_seen_at, event_count,
    entry_url, exit_url, referrer, utm_source, utm_medium, utm_campaign,
    device_type, country_code, environment,
    ip, city, region, latitude, longitude, timezone, asn_org, connection_class,
    geo_anonymized,
    screen_width, screen_height, pixel_ratio, touch, dark_mode, reduced_motion,
    browser_timezone, cpu_cores, signals_collected
)
WITH ranked AS (
    SELECT
        e.project_id, e.session_id, e.url, e.referrer,
        e.utm_source, e.utm_medium, e.utm_campaign,
        e.device_type, e.country_code, e.environment,
        ROW_NUMBER() OVER w_first AS rn_first,
        ROW_NUMBER() OVER w_last  AS rn_last,
        COUNT(*)           OVER w_part AS event_count,
        MIN(e.occurred_at) OVER w_part AS first_seen_at,
        MAX(e.occurred_at) OVER w_part AS last_seen_at
    FROM events e
    WINDOW
        w_part  AS (PARTITION BY e.project_id, e.session_id),
        -- id breaks ties so the choice is deterministic rather than dependent
        -- on scan order when two events share a timestamp.
        w_first AS (PARTITION BY e.project_id, e.session_id ORDER BY e.occurred_at ASC,  e.id ASC),
        w_last  AS (PARTITION BY e.project_id, e.session_id ORDER BY e.occurred_at DESC, e.id DESC)
)
SELECT
    f.session_id, f.project_id, f.first_seen_at, f.last_seen_at, f.event_count,
    f.url, l.url, f.referrer, f.utm_source, f.utm_medium, f.utm_campaign,
    f.device_type, f.country_code, COALESCE(f.environment, ''),
    -- The join below matches only the project that already owned the old row,
    -- so every other split row gets NULL geo and NULL signals. That IS the
    -- rule; no CASE is needed to express it.
    s.ip, s.city, s.region, s.latitude, s.longitude, s.timezone, s.asn_org,
    s.connection_class, COALESCE(s.geo_anonymized, 0),
    s.screen_width, s.screen_height, s.pixel_ratio, s.touch, s.dark_mode,
    s.reduced_motion, s.browser_timezone, s.cpu_cores, COALESCE(s.signals_collected, 0)
FROM ranked f
JOIN ranked l
  ON l.project_id = f.project_id AND l.session_id = f.session_id AND l.rn_last = 1
LEFT JOIN sessions s
  ON s.id = f.session_id AND s.project_id = f.project_id
WHERE f.rn_first = 1;

-- Sessions whose ID has no events anywhere: retention removed the events and
-- left the row. Nothing to re-derive, so carry them across verbatim. A session
-- whose ID DOES have events is deliberately not matched here — the rebuild
-- above is authoritative for it, which is how the 29 mis-attributed rows are
-- dropped rather than preserved.
INSERT INTO sessions_rebuild (
    id, project_id, first_seen_at, last_seen_at, event_count,
    entry_url, exit_url, referrer, utm_source, utm_medium, utm_campaign,
    device_type, country_code, environment,
    ip, city, region, latitude, longitude, timezone, asn_org, connection_class,
    geo_anonymized,
    screen_width, screen_height, pixel_ratio, touch, dark_mode, reduced_motion,
    browser_timezone, cpu_cores, signals_collected
)
SELECT
    s.id, s.project_id, s.first_seen_at, s.last_seen_at, s.event_count,
    s.entry_url, s.exit_url, s.referrer, s.utm_source, s.utm_medium, s.utm_campaign,
    s.device_type, s.country_code, s.environment,
    s.ip, s.city, s.region, s.latitude, s.longitude, s.timezone, s.asn_org,
    s.connection_class, s.geo_anonymized,
    s.screen_width, s.screen_height, s.pixel_ratio, s.touch, s.dark_mode,
    s.reduced_motion, s.browser_timezone, s.cpu_cores, s.signals_collected
FROM sessions s
WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.session_id = s.id);

DROP TABLE sessions;
ALTER TABLE sessions_rebuild RENAME TO sessions;

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions (project_id, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_ip ON sessions (ip) WHERE ip IS NOT NULL;

-- The 00033 triggers were attached to the old table and went with it.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS sessions_project_id_not_empty_insert
BEFORE INSERT ON sessions FOR EACH ROW WHEN NEW.project_id = ''
BEGIN
    SELECT RAISE(ABORT, 'sessions.project_id must not be empty');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS sessions_project_id_not_empty_update
BEFORE UPDATE OF project_id ON sessions FOR EACH ROW WHEN NEW.project_id = ''
BEGIN
    SELECT RAISE(ABORT, 'sessions.project_id must not be empty');
END;
-- +goose StatementEnd

-- +goose Down
-- Not reversible. Going back to a single-column key would have to merge the
-- split rows again, and the merge has no correct answer — that ambiguity is the
-- bug this migration removes.
SELECT 1;
