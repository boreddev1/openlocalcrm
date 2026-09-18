-- internal/db/queries/knowledge_base.sql

-- name: CreateKBArticle :one
INSERT INTO knowledge_base_articles (
    title,
    category,
    content,
    tags,
    author
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetKBArticleByID :one
SELECT * FROM knowledge_base_articles
WHERE id = $1 LIMIT 1;

-- name: ListKBArticles :many
SELECT * FROM knowledge_base_articles
ORDER BY created_at DESC;

-- name: DeleteKBArticle :exec
DELETE FROM knowledge_base_articles
WHERE id = $1;
