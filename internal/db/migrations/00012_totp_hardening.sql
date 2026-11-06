-- migrations/00012_totp_hardening.sql
-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_used_step BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS totp_last_used_step;
