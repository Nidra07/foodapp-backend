-- name: CreateOrder :one
INSERT INTO orders (
  order_number, customer_id, restaurant_id, subtotal, tax_amount, delivery_fee, discount_amount, total_amount,
  payment_method, delivery_address_line1, delivery_address_line2, delivery_city, delivery_state,
  delivery_postal_code, delivery_lat, delivery_lng, contact_phone, special_instructions
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: GetOrderByNumber :one
SELECT * FROM orders WHERE order_number = $1;

-- name: ListOrdersByCustomer :many
SELECT * FROM orders WHERE customer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountOrdersByCustomer :one
SELECT COUNT(*) FROM orders WHERE customer_id = $1;

-- name: ListOrdersByRestaurant :many
SELECT * FROM orders
WHERE restaurant_id = $1
  AND (sqlc.narg('status')::order_status IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountOrdersByRestaurant :one
SELECT COUNT(*) FROM orders
WHERE restaurant_id = $1
  AND (sqlc.narg('status')::order_status IS NULL OR status = sqlc.narg('status'));

-- name: ListActiveOrdersByRestaurant :many
-- "Active" = not yet delivered/cancelled; used for the restaurant app's
-- live order queue.
SELECT * FROM orders
WHERE restaurant_id = $1 AND status NOT IN ('delivered', 'cancelled')
ORDER BY created_at ASC;

-- name: UpdateOrderStatus :one
UPDATE orders SET
  status = $2,
  confirmed_at = CASE WHEN $2 = 'confirmed' THEN now() ELSE confirmed_at END,
  ready_at = CASE WHEN $2 = 'ready_for_pickup' THEN now() ELSE ready_at END,
  picked_up_at = CASE WHEN $2 = 'out_for_delivery' THEN now() ELSE picked_up_at END,
  delivered_at = CASE WHEN $2 = 'delivered' THEN now() ELSE delivered_at END
WHERE id = $1
RETURNING *;

-- name: CancelOrder :one
UPDATE orders SET status = 'cancelled', cancelled_at = now(), cancellation_reason = $2, cancelled_by = $3
WHERE id = $1
RETURNING *;

-- name: SetOrderPaymentStatus :exec
UPDATE orders SET payment_status = $2 WHERE id = $1;

-- name: CreateOrderItem :one
INSERT INTO order_items (order_id, menu_item_id, item_name, variant_name, unit_price, quantity, line_total, special_instructions)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListOrderItems :many
SELECT * FROM order_items WHERE order_id = $1;

-- name: CreateOrderItemAddon :one
INSERT INTO order_item_addons (order_item_id, addon_name, addon_price)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListOrderItemAddons :many
SELECT * FROM order_item_addons WHERE order_item_id = $1;

-- name: ListOrderItemAddonsByOrder :many
SELECT oia.* FROM order_item_addons oia
JOIN order_items oi ON oi.id = oia.order_item_id
WHERE oi.order_id = $1;

-- name: CreateOrderStatusHistory :one
INSERT INTO order_status_history (order_id, status, changed_by, notes)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListOrderStatusHistory :many
SELECT * FROM order_status_history WHERE order_id = $1 ORDER BY created_at;

-- name: SumSettlementDataForRestaurant :one
-- Used by the Settlements module (via a small consumer-defined interface,
-- not a direct cross-module repository dependency) to compute what a
-- restaurant is owed for a cycle: every delivered + paid order counts,
-- regardless of when it was placed vs delivered, keyed on delivered_at
-- falling inside the cycle window — a restaurant is paid when the order
-- is DONE, not when it was placed, since a placed-but-not-yet-delivered
-- order isn't a completed transaction yet.
SELECT
  COUNT(*)::bigint AS order_count,
  COALESCE(SUM(subtotal), 0)::numeric AS gross_subtotal
FROM orders
WHERE restaurant_id = $1
  AND status = 'delivered'
  AND payment_status = 'paid'
  AND delivered_at >= sqlc.arg('from_ts')::timestamptz
  AND delivered_at < sqlc.arg('to_ts')::timestamptz;
