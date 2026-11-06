-- migrations/00017_audit_logs_diff.sql
-- +goose Up
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS old_values JSONB,
    ADD COLUMN IF NOT EXISTS new_values JSONB;

-- +goose Down
ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS new_values,
    DROP COLUMN IF EXISTS old_values;
