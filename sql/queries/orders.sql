-- name: CreateOrder :one
INSERT INTO orders (
    id,
    tenant_id,
    user_id,
    total_cents,
    status,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: CreateOrderItem :one
INSERT INTO order_items (
    id,
    order_id,
    tenant_id,
    product_name,
    quantity,
    unit_price_cents,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListOrderItems :many
SELECT * FROM order_items
WHERE order_id = $1 AND tenant_id = $2
ORDER BY created_at ASC;

-- name: UpdateOrderStatus :one
UPDATE orders
SET status = $3, updated_at = $4
WHERE id = $1 AND tenant_id = $2
RETURNING *;
