-- migrations/00014_pgvector_768.sql
-- +goose Up

ALTER TABLE knowledge_base_articles ADD COLUMN IF NOT EXISTS embedding vector(768);
ALTER TABLE knowledge_base_articles ADD COLUMN IF NOT EXISTS embedding_model TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_embedding_hnsw
    ON knowledge_base_articles USING hnsw (embedding vector_cosine_ops);

-- +goose Down

DROP INDEX IF EXISTS idx_kb_embedding_hnsw;
ALTER TABLE knowledge_base_articles DROP COLUMN IF EXISTS embedding_model;
ALTER TABLE knowledge_base_articles DROP COLUMN IF EXISTS embedding;
