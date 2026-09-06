-- +goose Up
-- Production data repair, plus the storage-layer guard that stops it recurring.
--
-- Orphaned rows (#232): 344 events, 27 sessions and 1 funnel carry
-- project_id = '', which matches no row in projects. Every read path is
-- project-scoped, so these rows are stored, indexed and backed up while being
-- permanently unreachable. They were written between 2026-06-03 and 2026-06-25,
-- a window that predates both foreign-key enforcement (#195, 2026-07-09) and
-- the ErrNoProject guard in PersistEvent (#214, 2026-08-23).
--
-- They are deleted, not re-attributed. The obvious rescue — read the project
-- from the event's session — does not work here: those events' sessions are
-- themselves in the orphaned set, so the join resolves to '' again. Chasing it
-- further would mean attributing events by session_id alone, and session IDs
-- are currently shared across projects (#225 — 28 of them, covering 54% of the
-- events table), so a "rescue" could file one customer's events under another's
-- project. 344 unreachable rows are not worth that risk.
--
-- Idempotent throughout: every predicate is the condition that selected the
-- rows, so a second run matches nothing.

-- Funnels first: funnel_steps cascades from funnels, and deleting the parent
-- takes its steps with it.
DELETE FROM funnels  WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = funnels.project_id);
DELETE FROM sessions WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = sessions.project_id);
DELETE FROM events   WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = events.project_id);

-- Exact-duplicate funnels (#230). A pair only counts as duplicate when the
-- project, name, description, scope AND the full ordered step list all match —
-- so two funnels that merely share a name are left alone, and so is the
-- "Funnel Analysis Engagement" pair, whose members have 4 and 3 steps and are
-- therefore different funnels that happen to be named the same. The earliest
-- row of each identical group survives (ties broken by id, so the choice is
-- deterministic rather than dependent on scan order).
DELETE FROM funnels
WHERE EXISTS (
    SELECT 1 FROM funnels g
    WHERE g.id <> funnels.id
      AND g.project_id = funnels.project_id
      AND g.name = funnels.name
      AND COALESCE(g.description, '') = COALESCE(funnels.description, '')
      AND g.scope = funnels.scope
      AND (g.created_at < funnels.created_at
           OR (g.created_at = funnels.created_at AND g.id < funnels.id))
      AND (SELECT group_concat(step_order || ':' || event_name, '|')
             FROM (SELECT step_order, event_name FROM funnel_steps
                    WHERE funnel_id = g.id ORDER BY step_order, event_name))
        = (SELECT group_concat(step_order || ':' || event_name, '|')
             FROM (SELECT step_order, event_name FROM funnel_steps
                    WHERE funnel_id = funnels.id ORDER BY step_order, event_name))
);

-- Defence in depth (#232 item 3). SQLite cannot add a CHECK constraint to an
-- existing table — that needs a full table rebuild, and rebuilding the events
-- table (24 columns, 3 indexes, ~85k rows) inside a migration that runs
-- unattended at process start is a much larger risk than the one being guarded
-- against. BEFORE triggers give the same guarantee at the same layer: an empty
-- project_id is refused by the database whatever the caller does, and NOT NULL
-- (which accepts '') no longer stands alone.
--
-- This is not expected to change ingest behaviour. Two guards already stand in
-- front of it: the handler rejects a request that resolves no project slug with
-- a 400, and PersistEvent refuses ErrNoProject before InsertEvent. The trigger
-- is the backstop for a path that does not exist today.

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS events_project_id_not_empty_insert
BEFORE INSERT ON events FOR EACH ROW WHEN NEW.project_id = ''
BEGIN
    SELECT RAISE(ABORT, 'events.project_id must not be empty');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS events_project_id_not_empty_update
BEFORE UPDATE OF project_id ON events FOR EACH ROW WHEN NEW.project_id = ''
BEGIN
    SELECT RAISE(ABORT, 'events.project_id must not be empty');
END;
-- +goose StatementEnd

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
DROP TRIGGER IF EXISTS sessions_project_id_not_empty_update;
DROP TRIGGER IF EXISTS sessions_project_id_not_empty_insert;
DROP TRIGGER IF EXISTS events_project_id_not_empty_update;
DROP TRIGGER IF EXISTS events_project_id_not_empty_insert;
-- The deleted rows are not restored: they were unreachable by every query
-- before deletion, so there is nothing to restore them for.
