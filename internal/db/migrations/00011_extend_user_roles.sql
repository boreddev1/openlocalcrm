-- +goose Up
-- Extend user_role ENUM with VERTRIEB and BACKOFFICE roles
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'VERTRIEB';
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'BACKOFFICE';

-- +goose Down
-- PostgreSQL does not support removing values from an ENUM directly.
