-- name: GetFunnelByID :one
SELECT id, project_id, name, COALESCE(description, '') AS description, created_at
FROM funnels WHERE id = ?;

-- name: ListFunnels :many
SELECT id, project_id, name, COALESCE(description, '') AS description, created_at
FROM funnels WHERE project_id = ? ORDER BY created_at;

-- name: InsertFunnel :exec
INSERT INTO funnels (id, project_id, name, description) VALUES (?, ?, ?, ?);

-- name: UpdateFunnelMeta :exec
UPDATE funnels SET name = ?, description = ? WHERE id = ?;

-- name: DeleteFunnel :exec
DELETE FROM funnels WHERE id = ?;

-- name: InsertFunnelStep :exec
INSERT INTO funnel_steps (id, funnel_id, step_order, event_name, filters)
VALUES (?, ?, ?, ?, ?);

-- name: ListFunnelSteps :many
SELECT id, funnel_id, step_order, event_name, COALESCE(filters, '[]') AS filters
FROM funnel_steps WHERE funnel_id = ? ORDER BY step_order;

-- name: DeleteFunnelSteps :exec
DELETE FROM funnel_steps WHERE funnel_id = ?;
