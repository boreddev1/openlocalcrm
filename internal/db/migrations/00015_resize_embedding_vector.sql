-- migrations/00015_resize_embedding_vector.sql
-- +goose Up

-- The embedding column dimension changes 768 -> 1024 (qwen3-embedding:0.6b).
-- A vector(n) typmod change cannot hold the old values: stored vectors are
-- invalidated and must be re-embedded. NULL them out before the ALTER so the
-- typmod change succeeds.
UPDATE knowledge_base_articles SET embedding = NULL WHERE embedding IS NOT NULL;

ALTER TABLE knowledge_base_articles ALTER COLUMN embedding TYPE vector(1024);

-- ALTER COLUMN TYPE rebuilds dependent indexes; re-create explicitly so the
-- HNSW index is guaranteed to exist after the resize.
CREATE INDEX IF NOT EXISTS idx_kb_embedding_hnsw
    ON knowledge_base_articles USING hnsw (embedding vector_cosine_ops);

-- +goose Down

DROP INDEX IF EXISTS idx_kb_embedding_hnsw;
ALTER TABLE knowledge_base_articles ALTER COLUMN embedding TYPE vector(768);
