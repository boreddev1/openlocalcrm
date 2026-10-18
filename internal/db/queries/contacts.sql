-- internal/db/queries/contacts.sql
-- name: CreateContact :one
INSERT INTO contacts (
    company_id,
    first_name,
    last_name,
    email,
    phone,
    mobile,
    position,
    lead_source,
    address_street,
    address_zip,
    address_city,
    latitude,
    longitude,
    consent_phone,
    consent_email,
    consent_updated_at,
    stromverbrauch_kwh,
    gasverbrauch_kwh,
    zaehlernummer,
    eigentuemer_status,
    custom_fields
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20, $21
)
RETURNING *;

-- name: GetContactByID :one
SELECT * FROM contacts
WHERE id = $1 LIMIT 1;

-- name: ListContacts :many
SELECT * FROM contacts
WHERE (sqlc.narg('owner_user_id')::uuid IS NULL OR owner_user_id = sqlc.narg('owner_user_id')::uuid)
ORDER BY last_name ASC, first_name ASC
LIMIT $1 OFFSET $2;

-- name: ListContactsByCompany :many
SELECT * FROM contacts
WHERE company_id = $1
ORDER BY last_name ASC, first_name ASC;

-- name: SearchContacts :many
SELECT * FROM contacts
WHERE (first_name ILIKE '%' || $1 || '%'
   OR last_name ILIKE '%' || $1 || '%'
   OR email ILIKE '%' || $1 || '%'
   OR phone ILIKE '%' || $1 || '%')
  AND (sqlc.narg('owner_user_id')::uuid IS NULL OR owner_user_id = sqlc.narg('owner_user_id')::uuid)
ORDER BY last_name ASC
LIMIT $2;

-- name: UpdateContact :one
UPDATE contacts
SET company_id = $2,
    first_name = $3,
    last_name = $4,
    email = $5,
    phone = $6,
    mobile = $7,
    position = $8,
    lead_source = $9,
    address_street = $10,
    address_zip = $11,
    address_city = $12,
    latitude = $13,
    longitude = $14,
    consent_phone = $15,
    consent_email = $16,
    consent_updated_at = $17,
    stromverbrauch_kwh = $18,
    gasverbrauch_kwh = $19,
    zaehlernummer = $20,
    eigentuemer_status = $21,
    custom_fields = $22,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteContact :exec
DELETE FROM contacts
WHERE id = $1;

-- name: UpdateContactTags :one
UPDATE contacts
SET tags = CASE WHEN tags @> sqlc.arg(tags)::jsonb THEN tags ELSE tags || sqlc.arg(tags)::jsonb END,
    updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListInactiveContacts :many
SELECT * FROM contacts
WHERE updated_at < $1
ORDER BY updated_at ASC
LIMIT $2;

