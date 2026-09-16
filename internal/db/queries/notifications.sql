-- internal/db/queries/notifications.sql
-- name: CreateNotification :one
INSERT INTO notifications (
    user_id,
    type,
    title,
    message,
    link,
    is_read
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: ListUnreadNotifications :many
SELECT * FROM notifications
WHERE (user_id = $1 OR user_id IS NULL)
  AND is_read = FALSE
ORDER BY created_at DESC
LIMIT $2;

-- name: ListAllNotifications :many
SELECT * FROM notifications
WHERE (user_id = $1 OR user_id IS NULL)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: MarkNotificationAsRead :exec
UPDATE notifications
SET is_read = TRUE
WHERE id = $1;

-- name: MarkAllNotificationsAsRead :exec
UPDATE notifications
SET is_read = TRUE
WHERE (user_id = $1 OR user_id IS NULL);
