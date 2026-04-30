-- name: InsertABTest :exec
INSERT INTO ab_tests (id, project_id, name, status, control_filter, variant_filter, conversion_event)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetABTestByID :one
SELECT id, project_id, name, status, control_filter, variant_filter, conversion_event, created_at
FROM ab_tests WHERE id = ?;

-- name: ListABTests :many
SELECT id, project_id, name, status, control_filter, variant_filter, conversion_event, created_at
FROM ab_tests WHERE project_id = ? ORDER BY created_at DESC;
