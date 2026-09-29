-- internal/db/queries/notes.sql

-- name: CreateNote :one
INSERT INTO notes (
    entity_type,
    entity_id,
    type,
    author,
    content,
    user_id
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetNoteByID :one
SELECT * FROM notes
WHERE id = $1 LIMIT 1;

-- name: ListNotes :many
SELECT * FROM notes
WHERE (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
ORDER BY created_at DESC;

-- name: ListNotesByEntity :many
SELECT * FROM notes
WHERE entity_type = $1 AND entity_id = $2
  AND (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
ORDER BY created_at DESC;

-- name: UpdateNote :one
UPDATE notes
SET content = $2,
    type = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteNote :exec
DELETE FROM notes
WHERE id = $1;
