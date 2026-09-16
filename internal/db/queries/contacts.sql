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
    custom_fields
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetContactByID :one
SELECT * FROM contacts
WHERE id = $1 LIMIT 1;

-- name: ListContacts :many
SELECT * FROM contacts
ORDER BY last_name ASC, first_name ASC
LIMIT $1 OFFSET $2;

-- name: ListContactsByCompany :many
SELECT * FROM contacts
WHERE company_id = $1
ORDER BY last_name ASC, first_name ASC;

-- name: SearchContacts :many
SELECT * FROM contacts
WHERE first_name ILIKE '%' || $1 || '%'
   OR last_name ILIKE '%' || $1 || '%'
   OR email ILIKE '%' || $1 || '%'
   OR phone ILIKE '%' || $1 || '%'
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
    custom_fields = $15,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteContact :exec
DELETE FROM contacts
WHERE id = $1;
