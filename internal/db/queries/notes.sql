-- internal/db/queries/notes.sql

-- name: CreateNote :one
INSERT INTO notes (
    entity_type,
    entity_id,
    type,
    author,
    content
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetNoteByID :one
SELECT * FROM notes
WHERE id = $1 LIMIT 1;

-- name: ListNotes :many
SELECT * FROM notes
ORDER BY created_at DESC;

-- name: ListNotesByEntity :many
SELECT * FROM notes
WHERE entity_type = $1 AND entity_id = $2
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
