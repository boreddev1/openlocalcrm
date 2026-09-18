-- internal/db/queries/workflows.sql

-- name: CreateWorkflow :one
INSERT INTO workflows (
    id,
    name,
    description,
    trigger_type,
    target_type,
    is_active,
    steps_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: ListWorkflows :many
SELECT * FROM workflows
ORDER BY created_at DESC;

-- name: GetWorkflowByID :one
SELECT * FROM workflows
WHERE id = $1 LIMIT 1;

-- name: CreateWorkflowRun :one
INSERT INTO workflow_runs (
    id,
    workflow_id,
    target_id,
    target_type,
    target_name,
    status,
    current_step,
    snapshot_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: ListWorkflowRuns :many
SELECT * FROM workflow_runs
ORDER BY started_at DESC;

-- name: UpdateWorkflowRunStatus :one
UPDATE workflow_runs
SET status = $2,
    current_step = $3,
    completed_at = $4
WHERE id = $1
RETURNING *;
