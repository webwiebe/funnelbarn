-- name: GetProjectHealth :one
SELECT project_id, setup_called, events_received, flags_evaluated, recordings_received, updated_at
FROM project_health
WHERE project_id = ?;

-- name: MarkProjectHealthSetupCalled :exec
INSERT INTO project_health (project_id, setup_called, updated_at)
VALUES (?, 1, CURRENT_TIMESTAMP)
ON CONFLICT(project_id) DO UPDATE SET
    setup_called = 1,
    updated_at = CURRENT_TIMESTAMP;

-- name: MarkProjectHealthEventsReceived :exec
INSERT INTO project_health (project_id, events_received, updated_at)
VALUES (?, 1, CURRENT_TIMESTAMP)
ON CONFLICT(project_id) DO UPDATE SET
    events_received = 1,
    updated_at = CURRENT_TIMESTAMP;

-- name: MarkProjectHealthFlagsEvaluated :exec
INSERT INTO project_health (project_id, flags_evaluated, updated_at)
VALUES (?, 1, CURRENT_TIMESTAMP)
ON CONFLICT(project_id) DO UPDATE SET
    flags_evaluated = 1,
    updated_at = CURRENT_TIMESTAMP;

-- name: MarkProjectHealthRecordingsReceived :exec
INSERT INTO project_health (project_id, recordings_received, updated_at)
VALUES (?, 1, CURRENT_TIMESTAMP)
ON CONFLICT(project_id) DO UPDATE SET
    recordings_received = 1,
    updated_at = CURRENT_TIMESTAMP;

-- name: ResetProjectHealth :exec
UPDATE project_health
SET setup_called = 0,
    events_received = 0,
    flags_evaluated = 0,
    recordings_received = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = ?;
