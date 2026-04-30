-- name: GetSessionByID :one
SELECT id, project_id, first_seen_at, last_seen_at, event_count,
       entry_url, exit_url, referrer, utm_source, utm_medium, utm_campaign,
       device_type, country_code
FROM sessions WHERE id = ?;

-- name: InsertSession :exec
INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, event_count,
    entry_url, exit_url, referrer, utm_source, utm_medium, utm_campaign,
    device_type, country_code)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateSession :exec
UPDATE sessions SET last_seen_at = ?, event_count = ?, exit_url = ? WHERE id = ?;

-- name: ListSessions :many
SELECT id, project_id, first_seen_at, last_seen_at, event_count,
       entry_url, exit_url, referrer, utm_source, utm_medium, utm_campaign,
       device_type, country_code
FROM sessions WHERE project_id = ? ORDER BY last_seen_at DESC LIMIT ? OFFSET ?;
