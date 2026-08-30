-- name: CreateTenant :one
INSERT INTO tenants (
    id,
    name,
    status,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants
WHERE id = $1
LIMIT 1;

-- name: UpdateTenantStatus :one
UPDATE tenants
SET status = $2, updated_at = $3
WHERE id = $1
RETURNING *;
