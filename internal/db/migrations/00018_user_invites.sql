-- migrations/00018_user_invites.sql
-- +goose Up
CREATE TABLE user_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_invites_user_id ON user_invites(user_id);
CREATE INDEX idx_user_invites_active ON user_invites(expires_at)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS user_invites;
