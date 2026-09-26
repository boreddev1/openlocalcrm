-- internal/db/queries/invites.sql

-- name: CreateUserInvite :one
INSERT INTO user_invites (user_id, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserInviteByTokenHash :one
SELECT * FROM user_invites
WHERE token_hash = $1
LIMIT 1;

-- name: RevokeUserInvite :exec
UPDATE user_invites
SET revoked_at = NOW()
WHERE id = $1 AND consumed_at IS NULL;

-- name: ListUserInvites :many
SELECT * FROM user_invites ORDER BY created_at DESC;

-- name: ExportUserInvitesRows :many
SELECT row_to_json(t) FROM user_invites t ORDER BY t.id;

-- name: UpsertUserInvitesRow :exec
INSERT INTO user_invites (id, user_id, token_hash, created_by, expires_at, consumed_at, revoked_at, created_at)
SELECT r.id::uuid, r.user_id::uuid, r.token_hash,
       r.created_by::uuid, r.expires_at::timestamptz,
       r.consumed_at::timestamptz, r.revoked_at::timestamptz,
       r.created_at::timestamptz
FROM jsonb_to_record($1::jsonb) AS r(
    id text, user_id text, token_hash text, created_by text,
    expires_at text, consumed_at text, revoked_at text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id, token_hash = EXCLUDED.token_hash,
    created_by = EXCLUDED.created_by, expires_at = EXCLUDED.expires_at,
    consumed_at = EXCLUDED.consumed_at, revoked_at = EXCLUDED.revoked_at;
