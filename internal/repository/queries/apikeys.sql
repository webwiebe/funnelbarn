-- name: CreateAPIKey :exec
INSERT INTO api_keys (id, project_id, name, key_hash, scope)
VALUES (?, ?, ?, ?, ?);

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
SELECT project_id, scope FROM api_keys
WHERE key_hash = ? LIMIT 1;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE key_hash = ?;

-- name: EnsureSetupAPIKey :exec
INSERT INTO api_keys (id, project_id, name, key_hash, scope)
VALUES (?, ?, 'setup', ?, 'ingest')
ON CONFLICT(key_hash) DO NOTHING;
