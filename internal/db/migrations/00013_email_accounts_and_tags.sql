-- migrations/00013_email_accounts_and_tags.sql
-- +goose Up

ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS account_type VARCHAR(20) NOT NULL DEFAULT 'personal';
ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ;
ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS last_uid BIGINT NOT NULL DEFAULT 0;
ALTER TABLE email_messages ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_email_accounts_owner ON email_accounts(owner_user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_email_accounts_owner;
ALTER TABLE email_messages DROP COLUMN IF EXISTS tags;
ALTER TABLE email_accounts DROP COLUMN IF EXISTS last_uid;
ALTER TABLE email_accounts DROP COLUMN IF EXISTS last_sync_at;
ALTER TABLE email_accounts DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE email_accounts DROP COLUMN IF EXISTS account_type;
