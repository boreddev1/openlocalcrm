-- internal/db/queries/emails.sql
-- name: CreateEmailAccount :one
INSERT INTO email_accounts (
    name,
    email_address,
    provider,
    imap_host,
    imap_port,
    smtp_host,
    smtp_port,
    username,
    password_encrypted,
    is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: ListEmailAccounts :many
SELECT * FROM email_accounts
WHERE is_active = TRUE
ORDER BY name ASC;

-- name: GetEmailAccountByID :one
SELECT * FROM email_accounts
WHERE id = $1 LIMIT 1;

-- name: UpdateEmailAccountLastSynced :exec
UPDATE email_accounts
SET last_synced_at = $2, updated_at = NOW()
WHERE id = $1;

-- name: CreateEmailMessage :one
INSERT INTO email_messages (
    account_id,
    thread_id,
    message_id,
    in_reply_to,
    direction,
    sender_email,
    sender_name,
    recipient_emails,
    subject,
    body_text,
    body_html,
    received_at,
    is_read,
    contact_id,
    deal_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: ListEmailMessages :many
SELECT * FROM email_messages
ORDER BY received_at DESC
LIMIT $1 OFFSET $2;

-- name: ListEmailMessagesByThread :many
SELECT * FROM email_messages
WHERE thread_id = $1
ORDER BY received_at ASC;

-- name: GetEmailMessageByID :one
SELECT * FROM email_messages
WHERE id = $1 LIMIT 1;

-- name: MarkEmailMessageRead :exec
UPDATE email_messages
SET is_read = TRUE
WHERE id = $1;

-- name: CreateEmailAttachment :one
INSERT INTO email_attachments (
    message_id,
    filename,
    content_type,
    size_bytes,
    storage_path
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: ListEmailAttachmentsByMessage :many
SELECT * FROM email_attachments
WHERE message_id = $1
ORDER BY created_at ASC;
