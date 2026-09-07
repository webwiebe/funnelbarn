-- name: InsertAPIKey :exec
INSERT INTO api_keys (id, project_id, name, key_hash, scope) VALUES (?, ?, ?, ?, ?);

-- name: GetAPIKeyByID :one
SELECT id, project_id, name, key_hash, scope, last_used_at, created_at
FROM api_keys WHERE id = ?;

-- name: ListAPIKeysByProject :many
SELECT id, project_id, name, key_hash, scope, last_used_at, created_at
FROM api_keys WHERE project_id = ? ORDER BY created_at;

-- name: ListAllAPIKeys :many
SELECT id, project_id, name, key_hash, scope, last_used_at, created_at
FROM api_keys ORDER BY created_at;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = ?;

-- name: LookupAPIKeyBySHA256 :one
-- Authenticating a key REQUIRES its project to still exist. Rows written before
-- foreign keys were enforced (#195) outlived their project's deletion, so keys
-- for deleted projects still authenticated: the event was accepted with a 202,
-- spooled, refused by EnsureProject as an unresolvable UUID slug, and
-- dead-lettered. The inner join makes such a key not found, so the caller gets
-- a 401 it can act on instead of silence.
SELECT k.project_id, k.scope FROM api_keys k JOIN projects p ON p.id = k.project_id WHERE k.key_hash = ? LIMIT 1;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE key_hash = ?;

-- name: EnsureSetupAPIKey :exec
INSERT INTO api_keys (id, project_id, name, key_hash, scope)
VALUES (?, ?, 'setup', ?, 'ingest')
ON CONFLICT(key_hash) DO NOTHING;
