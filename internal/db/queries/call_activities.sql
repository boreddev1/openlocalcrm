-- internal/db/queries/call_activities.sql

-- name: CreateCallActivity :one
INSERT INTO call_activities (
    contact_id,
    duration_seconds,
    disposition,
    notes
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: ListCallActivitiesByContact :many
SELECT * FROM call_activities
WHERE contact_id = $1
ORDER BY created_at DESC;

-- name: ListRecentCallActivities :many
SELECT * FROM call_activities
ORDER BY created_at DESC
LIMIT $1;
