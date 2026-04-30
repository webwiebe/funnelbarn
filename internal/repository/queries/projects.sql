-- name: GetProjectByID :one
SELECT id, name, slug, status, created_at FROM projects WHERE id = ?;

-- name: GetProjectBySlug :one
SELECT id, name, slug, status, created_at FROM projects WHERE slug = ?;

-- name: ListProjects :many
SELECT id, name, slug, status, created_at FROM projects ORDER BY name;

-- name: CreateProject :exec
INSERT INTO projects (id, name, slug) VALUES (?, ?, ?);

-- name: UpdateProjectName :exec
UPDATE projects SET name = ? WHERE id = ?;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = ?;

-- name: ApproveProject :exec
UPDATE projects SET status = 'active' WHERE id = ?;

-- name: CreateProjectPending :exec
INSERT INTO projects (id, name, slug, status) VALUES (?, ?, ?, 'pending');

-- name: HasProjects :one
SELECT COUNT(*) FROM projects;
