-- internal/db/queries/todos.sql
-- name: CreateTodo :one
INSERT INTO todos (
    title,
    description,
    due_date,
    status,
    priority,
    assigned_to,
    contact_id,
    deal_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetTodoByID :one
SELECT * FROM todos
WHERE id = $1 LIMIT 1;

-- name: ListTodos :many
SELECT * FROM todos
ORDER BY due_date ASC NULLS LAST, created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListTodosByAssignee :many
SELECT * FROM todos
WHERE assigned_to = $1
ORDER BY due_date ASC NULLS LAST;

-- name: UpdateTodoStatus :one
UPDATE todos
SET status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateTodo :one
UPDATE todos
SET title = $2,
    description = $3,
    due_date = $4,
    status = $5,
    priority = $6,
    assigned_to = $7,
    contact_id = $8,
    deal_id = $9,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteTodo :exec
DELETE FROM todos
WHERE id = $1;
