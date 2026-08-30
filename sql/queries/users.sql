-- name: CreateUser :one
INSERT INTO users (
    id,
    tenant_id,
    email,
    name,
    password_hash,
    role,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListUsersByTenant :many
SELECT * FROM users
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1 AND tenant_id = $2;
