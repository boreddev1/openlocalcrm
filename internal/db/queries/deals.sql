-- internal/db/queries/deals.sql
-- name: CreateDeal :one
INSERT INTO deals (
    title,
    company_id,
    contact_id,
    value,
    currency,
    stage,
    probability,
    assigned_to,
    closed_at,
    custom_fields
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetDealByID :one
SELECT * FROM deals
WHERE id = $1 LIMIT 1;

-- name: ListDeals :many
SELECT * FROM deals
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListDealsByStage :many
SELECT * FROM deals
WHERE stage = $1
ORDER BY created_at DESC;

-- name: UpdateDealStage :one
UPDATE deals
SET stage = $2,
    probability = $3,
    closed_at = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateDeal :one
UPDATE deals
SET title = $2,
    company_id = $3,
    contact_id = $4,
    value = $5,
    currency = $6,
    stage = $7,
    probability = $8,
    assigned_to = $9,
    closed_at = $10,
    custom_fields = $11,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteDeal :exec
DELETE FROM deals
WHERE id = $1;
