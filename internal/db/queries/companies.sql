-- internal/db/queries/companies.sql
-- name: CreateCompany :one
INSERT INTO companies (
    name,
    domain,
    phone,
    email,
    address_street,
    address_zip,
    address_city,
    address_country,
    custom_fields
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: GetCompanyByID :one
SELECT * FROM companies
WHERE id = $1 LIMIT 1;

-- name: ListCompanies :many
SELECT * FROM companies
ORDER BY name ASC
LIMIT $1 OFFSET $2;

-- name: SearchCompanies :many
SELECT * FROM companies
WHERE name ILIKE '%' || $1 || '%' OR domain ILIKE '%' || $1 || '%'
ORDER BY name ASC
LIMIT $2;

-- name: UpdateCompany :one
UPDATE companies
SET name = $2,
    domain = $3,
    phone = $4,
    email = $5,
    address_street = $6,
    address_zip = $7,
    address_city = $8,
    address_country = $9,
    custom_fields = $10,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCompany :exec
DELETE FROM companies
WHERE id = $1;
