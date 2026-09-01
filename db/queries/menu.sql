-- name: CreateMenuCategory :one
INSERT INTO menu_categories (restaurant_id, name, description, display_order)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMenuCategoryByID :one
SELECT * FROM menu_categories WHERE id = $1;

-- name: ListMenuCategories :many
SELECT * FROM menu_categories WHERE restaurant_id = $1 AND is_active = true ORDER BY display_order, name;

-- name: UpdateMenuCategory :one
UPDATE menu_categories SET
  name = COALESCE(sqlc.narg('name'), name),
  description = COALESCE(sqlc.narg('description'), description),
  display_order = COALESCE(sqlc.narg('display_order'), display_order)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetMenuCategoryActive :exec
UPDATE menu_categories SET is_active = $2 WHERE id = $1;

-- name: DeleteMenuCategory :exec
DELETE FROM menu_categories WHERE id = $1;

-- name: CreateMenuItem :one
INSERT INTO menu_items (
  restaurant_id, category_id, name, description, food_type, base_price,
  image_url, calories, spice_level, prep_time_mins, display_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetMenuItemByID :one
SELECT * FROM menu_items WHERE id = $1 AND deleted_at IS NULL;

-- name: ListMenuItemsByRestaurant :many
SELECT * FROM menu_items
WHERE restaurant_id = $1 AND deleted_at IS NULL AND is_active = true
ORDER BY category_id, display_order, name;

-- name: ListMenuItemsByCategory :many
SELECT * FROM menu_items
WHERE category_id = $1 AND deleted_at IS NULL AND is_active = true
ORDER BY display_order, name;

-- name: UpdateMenuItem :one
UPDATE menu_items SET
  name = COALESCE(sqlc.narg('name'), name),
  description = COALESCE(sqlc.narg('description'), description),
  food_type = COALESCE(sqlc.narg('food_type'), food_type),
  base_price = COALESCE(sqlc.narg('base_price'), base_price),
  image_url = COALESCE(sqlc.narg('image_url'), image_url),
  calories = COALESCE(sqlc.narg('calories'), calories),
  spice_level = COALESCE(sqlc.narg('spice_level'), spice_level),
  prep_time_mins = COALESCE(sqlc.narg('prep_time_mins'), prep_time_mins),
  category_id = COALESCE(sqlc.narg('category_id'), category_id)
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SetMenuItemAvailability :exec
UPDATE menu_items SET is_available = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: SetMenuItemActive :exec
UPDATE menu_items SET is_active = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteMenuItem :exec
UPDATE menu_items SET deleted_at = now() WHERE id = $1;

-- name: CreateVariantGroup :one
INSERT INTO menu_item_variant_groups (menu_item_id, name, is_required, min_select, max_select, display_order)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListVariantGroupsByItem :many
SELECT * FROM menu_item_variant_groups WHERE menu_item_id = $1 ORDER BY display_order;

-- name: DeleteVariantGroup :exec
DELETE FROM menu_item_variant_groups WHERE id = $1;

-- name: CreateMenuItemVariant :one
INSERT INTO menu_item_variants (variant_group_id, name, price, display_order)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListVariantsByGroup :many
SELECT * FROM menu_item_variants WHERE variant_group_id = $1 ORDER BY display_order;

-- name: ListVariantsByItem :many
SELECT v.* FROM menu_item_variants v
JOIN menu_item_variant_groups g ON g.id = v.variant_group_id
WHERE g.menu_item_id = $1
ORDER BY g.display_order, v.display_order;

-- name: SetVariantAvailability :exec
UPDATE menu_item_variants SET is_available = $2 WHERE id = $1;

-- name: DeleteMenuItemVariant :exec
DELETE FROM menu_item_variants WHERE id = $1;

-- name: CreateAddonGroup :one
INSERT INTO menu_addon_groups (menu_item_id, name, is_required, min_select, max_select, display_order)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAddonGroupsByItem :many
SELECT * FROM menu_addon_groups WHERE menu_item_id = $1 ORDER BY display_order;

-- name: DeleteAddonGroup :exec
DELETE FROM menu_addon_groups WHERE id = $1;

-- name: CreateMenuAddon :one
INSERT INTO menu_addons (addon_group_id, name, price, display_order)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAddonsByGroup :many
SELECT * FROM menu_addons WHERE addon_group_id = $1 ORDER BY display_order;

-- name: ListAddonsByItem :many
SELECT a.* FROM menu_addons a
JOIN menu_addon_groups g ON g.id = a.addon_group_id
WHERE g.menu_item_id = $1
ORDER BY g.display_order, a.display_order;

-- name: SetAddonAvailability :exec
UPDATE menu_addons SET is_available = $2 WHERE id = $1;

-- name: DeleteMenuAddon :exec
DELETE FROM menu_addons WHERE id = $1;
