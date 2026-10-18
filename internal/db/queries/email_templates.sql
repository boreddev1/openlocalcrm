-- internal/db/queries/email_templates.sql

-- name: CreateEmailTemplate :one
INSERT INTO email_templates (key, name, subject, body, category, locale)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetEmailTemplateByKey :one
SELECT * FROM email_templates
WHERE key = $1
LIMIT 1;

-- name: ListEmailTemplates :many
SELECT * FROM email_templates
ORDER BY category, key;

-- name: UpdateEmailTemplate :one
UPDATE email_templates
SET name = $2,
    subject = $3,
    body = $4,
    category = $5,
    locale = $6,
    updated_at = NOW()
WHERE key = $1
RETURNING *;

-- name: DeleteEmailTemplate :exec
DELETE FROM email_templates
WHERE key = $1;

-- name: ExportEmailTemplatesRows :many
SELECT row_to_json(t) FROM email_templates t ORDER BY t.id;

-- name: UpsertEmailTemplatesRow :exec
INSERT INTO email_templates (id, key, name, subject, body, category, locale, created_at, updated_at)
SELECT r.id::uuid, r.key, r.name, r.subject, r.body,
       r.category, r.locale,
       r.created_at::timestamptz, r.updated_at::timestamptz
FROM jsonb_to_record($1::jsonb) AS r(
    id text, key text, name text, subject text, body text,
    category text, locale text, created_at text, updated_at text)
ON CONFLICT (key) DO UPDATE SET
    name = EXCLUDED.name, subject = EXCLUDED.subject, body = EXCLUDED.body,
    category = EXCLUDED.category, locale = EXCLUDED.locale,
    updated_at = EXCLUDED.updated_at;
