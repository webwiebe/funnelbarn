-- name: InsertEvent :exec
INSERT INTO events (
    id, project_id, session_id, user_id_hash, name, url, referrer, referrer_domain,
    utm_source, utm_medium, utm_campaign, utm_term, utm_content,
    properties, user_agent, browser, os, device_type, country_code,
    ingest_id, occurred_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetEventByIngestID :one
SELECT id FROM events WHERE ingest_id = ? LIMIT 1;

-- name: ListEvents :many
SELECT id, project_id, session_id, user_id_hash, name, url, referrer, referrer_domain,
    utm_source, utm_medium, utm_campaign, utm_term, utm_content,
    properties, user_agent, browser, os, device_type, country_code,
    ingest_id, occurred_at, created_at
FROM events WHERE project_id = ? ORDER BY occurred_at DESC LIMIT ? OFFSET ?;
