-- internal/db/migrations/00021_audit_hash_chain.sql
-- +goose Up
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS prev_hash TEXT,
    ADD COLUMN IF NOT EXISTS hash TEXT;

ALTER TABLE ai_audit_logs
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_ai_audit_logs_user_id ON ai_audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_hash ON audit_logs(hash);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_logs_hash;
DROP INDEX IF EXISTS idx_ai_audit_logs_user_id;

ALTER TABLE ai_audit_logs
    DROP COLUMN IF EXISTS user_id;

ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS hash,
    DROP COLUMN IF EXISTS prev_hash;
