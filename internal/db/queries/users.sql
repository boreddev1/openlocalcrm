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

-- name: EnableTOTPWithSecret :exec
UPDATE users
SET totp_secret_encrypted = $2, totp_enabled = TRUE, totp_last_used_step = 0, updated_at = NOW()
WHERE id = $1;

-- name: CurrentTokenVersion :one
SELECT token_version FROM users
WHERE id = $1;

-- name: IncrementUserTokenVersion :one
-- Invalidates every access token already issued to this user.
UPDATE users
SET token_version = token_version + 1, updated_at = NOW()
WHERE id = $1
RETURNING token_version;

-- name: SetUserMustChangePassword :exec
UPDATE users
SET must_change_password = $2, updated_at = NOW()
WHERE id = $1;

-- name: ListUnownedContacts :many
SELECT * FROM contacts
WHERE owner_user_id IS NULL
ORDER BY created_at, id;

-- name: ListUnownedCompanies :many
SELECT * FROM companies
WHERE owner_user_id IS NULL
ORDER BY created_at, id;

-- name: SetContactOwner :one
UPDATE contacts
SET owner_user_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCompanyOwner :one
UPDATE companies
SET owner_user_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
