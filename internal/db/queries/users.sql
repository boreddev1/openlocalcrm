-- internal/db/queries/users.sql
-- name: CreateUser :one
INSERT INTO users (
    email,
    password_hash,
    first_name,
    last_name,
    role,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 LIMIT 1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at ASC;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserRole :exec
UPDATE users
SET role = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserStatus :exec
UPDATE users
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserTOTP :exec
UPDATE users
SET totp_secret_encrypted = $2, totp_enabled = $3, totp_last_used_step = $4, updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserTOTPLastUsedStep :exec
UPDATE users
SET totp_last_used_step = $2, updated_at = NOW()
WHERE id = $1;

