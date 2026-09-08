-- +goose Up
-- Delete the api_keys rows whose project no longer exists (#257 item 1).
--
-- 17 such rows are in production, one per deleted project, created between
-- 2026-05-13 and 2026-05-31 — before foreign keys were enforced (#195), so
-- deleting a project did not cascade to its keys even though the column has
-- always been declared REFERENCES projects(id) ON DELETE CASCADE.
--
-- #257 held this deletion back until the affected callers were rejected rather
-- than silently dropped, so that a broken integration would show up as a 401
-- instead of disappearing. #265 did that: the key lookup now inner-joins
-- projects, so none of these 17 rows has authenticated anything since it
-- deployed. They are inert, and the only thing they still do is make
-- CountOrphanedRows report a non-zero total on every maintenance pass, which is
-- logged at Error and files a BugBarn issue that no longer describes a fault.
--
-- Deleting them leaves the check reporting what it was built to report: with
-- foreign keys enforced, a non-zero count means a guard was bypassed.
--
-- Idempotent: the predicate is the condition that selects the rows, so a second
-- run matches nothing. Environments with no orphans (testing, staging, and any
-- fresh database) delete nothing.
DELETE FROM api_keys
WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = api_keys.project_id);

-- +goose Down
-- The deleted rows are not restored. A key whose project is gone cannot
-- authenticate anything (#265), so there is nothing to restore it for, and the
-- key material itself is a hash — recreating the row would not give any caller
-- a working key back.
SELECT 1;
