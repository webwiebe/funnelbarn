-- name: UpsertUser :exec
INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)
ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash;

-- name: GetUserByUsername :one
SELECT id, username, password_hash, created_at FROM users WHERE username = ?;
