-- internal/db/queries/sessions.sql
-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (
    user_id,
    token_hash,
    user_agent,
    ip_address,
    expires_at,
    family_id
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1 AND expires_at > NOW()
LIMIT 1;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens
WHERE token_hash = $1;

-- name: DeleteUserRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE user_id = $1;

-- name: ConsumeRefreshToken :one
UPDATE refresh_tokens
SET used_at = NOW()
WHERE token_hash = $1
  AND used_at IS NULL
  AND expires_at > NOW()
RETURNING id, user_id, family_id, user_agent, ip_address, expires_at;

-- name: GetRefreshTokenByHash :one
-- Deliberately ignores expiry and used_at: the replay path needs to see a spent
-- token in order to detect the replay.
SELECT * FROM refresh_tokens
WHERE token_hash = $1
LIMIT 1;

-- name: DeleteRefreshTokenFamily :exec
DELETE FROM refresh_tokens
WHERE family_id = $1;

-- name: DeleteExpiredAndSpentRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW() OR (used_at IS NOT NULL AND used_at < NOW() - INTERVAL '30 days');
