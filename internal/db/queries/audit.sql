-- internal/db/queries/audit.sql
-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    user_id,
    entity_type,
    entity_id,
    action,
    changes,
    old_values,
    new_values,
    ip_address,
    user_agent,
    prev_hash,
    hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: GetLatestAuditLogHash :one
SELECT hash FROM audit_logs
WHERE hash IS NOT NULL AND hash != ''
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListAuditLogsAsc :many
SELECT * FROM audit_logs
ORDER BY created_at ASC, id ASC;

-- name: ListAuditLogsByEntity :many
SELECT * FROM audit_logs
WHERE entity_type = $1 AND entity_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListRecentAuditLogs :many
SELECT * FROM audit_logs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
