-- internal/db/queries/ai_research_jobs.sql

-- name: CreateAIResearchJob :one
INSERT INTO ai_research_jobs (
    company_name,
    domain,
    status
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: GetAIResearchJobByID :one
SELECT * FROM ai_research_jobs
WHERE id = $1 LIMIT 1;

-- name: ListAIResearchJobs :many
SELECT * FROM ai_research_jobs
ORDER BY created_at DESC;

-- name: UpdateAIResearchJobStatus :one
UPDATE ai_research_jobs
SET status = $2,
    result_json = $3,
    error_message = $4,
    completed_at = $5
WHERE id = $1
RETURNING *;
