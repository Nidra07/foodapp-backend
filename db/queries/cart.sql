-- name: GetCartByCustomer :one
SELECT * FROM carts WHERE customer_id = $1;

-- name: GetCartByID :one
SELECT * FROM carts WHERE id = $1;

-- name: CreateCart :one
INSERT INTO carts (customer_id, restaurant_id) VALUES ($1, $2) RETURNING *;

-- name: DeleteCart :exec
DELETE FROM carts WHERE id = $1;

-- name: TouchCart :exec
UPDATE carts SET updated_at = now() WHERE id = $1;

-- name: AddCartItem :one
INSERT INTO cart_items (cart_id, menu_item_id, variant_id, quantity, special_instructions)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetCartItemByID :one
SELECT * FROM cart_items WHERE id = $1;

-- name: ListCartItems :many
SELECT * FROM cart_items WHERE cart_id = $1 ORDER BY created_at;

-- name: UpdateCartItemQuantity :one
UPDATE cart_items SET quantity = $2 WHERE id = $1 RETURNING *;

-- name: DeleteCartItem :exec
DELETE FROM cart_items WHERE id = $1;

-- name: ClearCartItems :exec
DELETE FROM cart_items WHERE cart_id = $1;

-- name: AddCartItemAddon :one
INSERT INTO cart_item_addons (cart_item_id, addon_id) VALUES ($1, $2) RETURNING *;

-- name: ListCartItemAddons :many
SELECT * FROM cart_item_addons WHERE cart_item_id = $1;

-- name: ClearCartItemAddons :exec
DELETE FROM cart_item_addons WHERE cart_item_id = $1;

-- name: ListCartItemAddonsByCart :many
SELECT cia.* FROM cart_item_addons cia
JOIN cart_items ci ON ci.id = cia.cart_item_id
WHERE ci.cart_id = $1;

-- Pricing/detail joins used to compute cart totals without N+1 lookups
-- for the common case (list all items with their live menu price info).

-- name: ListCartItemsWithPricing :many
SELECT
  ci.id, ci.cart_id, ci.menu_item_id, ci.variant_id, ci.quantity, ci.special_instructions,
  mi.name AS item_name, mi.base_price AS item_base_price, mi.is_available AS item_is_available,
  v.name AS variant_name, v.price AS variant_price, v.is_available AS variant_is_available
FROM cart_items ci
JOIN menu_items mi ON mi.id = ci.menu_item_id
LEFT JOIN menu_item_variants v ON v.id = ci.variant_id
WHERE ci.cart_id = $1
ORDER BY ci.created_at;

-- name: ListAddonsForCartItem :many
SELECT cia.cart_item_id, a.id AS addon_id, a.name AS addon_name, a.price AS addon_price, a.is_available
FROM cart_item_addons cia
JOIN menu_addons a ON a.id = cia.addon_id
WHERE cia.cart_item_id = $1;
