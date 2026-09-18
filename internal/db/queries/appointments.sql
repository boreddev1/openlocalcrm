-- internal/db/queries/appointments.sql

-- name: CreateAppointment :one
INSERT INTO appointments (
    title,
    contact_id,
    company_id,
    deal_id,
    start_time,
    end_time,
    location,
    notes,
    ics_uid,
    assigned_to,
    type,
    is_external,
    provider,
    is_private,
    is_pushed
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: GetAppointmentByID :one
SELECT * FROM appointments
WHERE id = $1 LIMIT 1;

-- name: ListAppointments :many
SELECT * FROM appointments
ORDER BY start_time ASC;

-- name: UpdateAppointment :one
UPDATE appointments
SET title = $2,
    contact_id = $3,
    start_time = $4,
    end_time = $5,
    location = $6,
    notes = $7,
    assigned_to = $8,
    type = $9,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateAppointmentPushStatus :one
UPDATE appointments
SET is_pushed = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteAppointment :exec
DELETE FROM appointments
WHERE id = $1;
