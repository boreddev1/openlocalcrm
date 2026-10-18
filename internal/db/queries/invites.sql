-- internal/db/queries/invites.sql

-- name: CreateUserInvite :one
INSERT INTO user_invites (user_id, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserInviteByTokenHash :one
SELECT * FROM user_invites
WHERE token_hash = $1
LIMIT 1;

-- name: CreateInvitedUserWithInvite :one
-- One statement creates the INVITED user and its invite so a half-created
-- account can never exist. The user gets a real but discarded Argon2id hash:
-- the account cannot log in until AcceptUserInvite replaces it.
-- Positional parameters keep role a plain string, like db.CreateUserParams.
WITH new_user AS (
    INSERT INTO users (email, password_hash, first_name, last_name, role, status)
    VALUES ($1, $2, $3, $4, $5, 'INVITED')
    RETURNING id
)
INSERT INTO user_invites (user_id, token_hash, created_by, expires_at)
SELECT new_user.id, $6, $7, $8
FROM new_user
RETURNING *;

-- name: AcceptUserInvite :one
-- One statement locks the active invite, consumes it and activates the
-- referenced user. The FOR UPDATE lock plus the active predicates make a
-- concurrent second acceptance match zero rows instead of reactivating the
-- account. No raw token is stored or returned: the caller supplies the hash.
WITH locked AS (
    SELECT id, user_id
    FROM user_invites
    WHERE user_invites.token_hash = sqlc.arg(token_hash)
      AND user_invites.consumed_at IS NULL
      AND user_invites.revoked_at IS NULL
      AND user_invites.expires_at > NOW()
    FOR UPDATE
), consumed AS (
    UPDATE user_invites i
    SET consumed_at = NOW()
    FROM locked l
    WHERE i.id = l.id
    RETURNING l.user_id
)
UPDATE users u
SET password_hash = $1,
    status = 'ACTIVE',
    updated_at = NOW()
FROM consumed c
WHERE u.id = c.user_id
RETURNING u.*;

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
