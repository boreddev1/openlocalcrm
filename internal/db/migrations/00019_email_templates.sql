-- migrations/00019_email_templates.sql
-- +goose Up
CREATE TABLE email_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    subject VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(64) NOT NULL DEFAULT '',
    locale VARCHAR(16) NOT NULL DEFAULT 'de-DE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_email_templates_category ON email_templates(category);

-- +goose Down
DROP TABLE IF EXISTS email_templates;
