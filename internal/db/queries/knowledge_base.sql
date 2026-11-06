-- internal/db/queries/knowledge_base.sql

-- name: CreateKBArticle :one
INSERT INTO knowledge_base_articles (
    title,
    category,
    content,
    tags,
    author,
    embedding,
    embedding_model
) VALUES (
    $1, $2, $3, $4, $5, sqlc.arg(embedding)::vector, sqlc.arg(embedding_model)
)
RETURNING id, title, category, content, tags, author, embedding_model,
    (embedding IS NOT NULL)::boolean AS indexed, created_at, updated_at;

-- name: GetKBArticleByID :one
SELECT id, title, category, content, tags, author, embedding_model,
    (embedding IS NOT NULL)::boolean AS indexed, created_at, updated_at
FROM knowledge_base_articles
WHERE id = $1 LIMIT 1;

-- name: ListKBArticles :many
SELECT id, title, category, content, tags, author, embedding_model,
    (embedding IS NOT NULL)::boolean AS indexed, created_at, updated_at
FROM knowledge_base_articles
ORDER BY created_at DESC;

-- name: UpdateKBArticleEmbedding :exec
UPDATE knowledge_base_articles
SET embedding = sqlc.arg(embedding)::vector, embedding_model = sqlc.arg(embedding_model)
WHERE id = sqlc.arg(id);

-- name: SearchKBArticlesByEmbedding :many
SELECT id, title, category, content, tags, author, embedding_model,
    (embedding IS NOT NULL)::boolean AS indexed, created_at, updated_at,
    (embedding <=> sqlc.arg(embedding)::vector)::float8 AS distance
FROM knowledge_base_articles
WHERE embedding IS NOT NULL
ORDER BY embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(limit_count);

-- name: DeleteKBArticle :exec
DELETE FROM knowledge_base_articles
WHERE id = $1;
