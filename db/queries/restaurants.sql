-- name: CreateRestaurant :one
INSERT INTO restaurants (
  owner_user_id, name, slug, description, cuisine_tags, is_veg_only,
  address_line1, address_line2, city, state, postal_code, country, location
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
  ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography
)
RETURNING *;

-- name: GetRestaurantByID :one
SELECT * FROM restaurants WHERE id = $1 AND deleted_at IS NULL;

-- name: GetRestaurantBySlug :one
SELECT * FROM restaurants WHERE slug = $1 AND deleted_at IS NULL;

-- name: ListRestaurantsByOwner :many
SELECT * FROM restaurants WHERE owner_user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC;

-- name: ListNearbyRestaurants :many
-- Distance search using PostGIS. Only returns restaurants that are
-- approved, accepting orders, and within their own service radius of the
-- customer's point.
SELECT r.*,
       ST_Distance(
           r.location,
           ST_SetSRID(
               ST_MakePoint(
                   CAST(sqlc.arg(lng) AS DOUBLE PRECISION),
                   CAST(sqlc.arg(lat) AS DOUBLE PRECISION)
               ),
               4326
           )::geography
       ) / 1000.0 AS distance_km
FROM restaurants r
JOIN restaurant_service_areas sa
  ON sa.restaurant_id = r.id
 AND sa.is_active = true
WHERE r.deleted_at IS NULL
  AND r.status = 'approved'
  AND r.is_accepting_orders = true
  AND ST_DWithin(
      r.location,
      ST_SetSRID(
          ST_MakePoint(
              CAST(sqlc.arg(lng) AS DOUBLE PRECISION),
              CAST(sqlc.arg(lat) AS DOUBLE PRECISION)
          ),
          4326
      )::geography,
      LEAST(
          CAST(sqlc.arg(search_radius_m) AS DOUBLE PRECISION),
          sa.radius_km * 1000
      )
  )
ORDER BY distance_km ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: ListRestaurantsForAdmin :many
SELECT * FROM restaurants
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::restaurant_status IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountRestaurantsForAdmin :one
SELECT COUNT(*) FROM restaurants
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::restaurant_status IS NULL OR status = sqlc.narg('status'));

-- name: UpdateRestaurantProfile :one
UPDATE restaurants SET
  name = COALESCE(sqlc.narg('name'), name),
  description = COALESCE(sqlc.narg('description'), description),
  cuisine_tags = COALESCE(sqlc.narg('cuisine_tags'), cuisine_tags),
  logo_url = COALESCE(sqlc.narg('logo_url'), logo_url),
  banner_url = COALESCE(sqlc.narg('banner_url'), banner_url),
  min_order_amount = COALESCE(sqlc.narg('min_order_amount'), min_order_amount),
  avg_prep_time_mins = COALESCE(sqlc.narg('avg_prep_time_mins'), avg_prep_time_mins)
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SetRestaurantStatus :exec
UPDATE restaurants SET status = $2, approved_at = CASE WHEN $2 = 'approved' THEN now() ELSE approved_at END, approved_by = CASE WHEN $2 = 'approved' THEN $3 ELSE approved_by END
WHERE id = $1;

-- name: SetRestaurantAcceptingOrders :exec
UPDATE restaurants SET is_accepting_orders = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: SetRestaurantKYCStatus :exec
UPDATE restaurants SET kyc_status = $2 WHERE id = $1;

-- name: SoftDeleteRestaurant :exec
UPDATE restaurants SET deleted_at = now(), status = 'closed' WHERE id = $1;

-- name: UpsertOperatingHours :one
INSERT INTO restaurant_operating_hours (restaurant_id, day_of_week, open_time, close_time, is_closed)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteOperatingHoursForDay :exec
DELETE FROM restaurant_operating_hours WHERE restaurant_id = $1 AND day_of_week = $2;

-- name: ListOperatingHours :many
SELECT * FROM restaurant_operating_hours WHERE restaurant_id = $1 ORDER BY day_of_week;

-- name: UpsertServiceArea :one
INSERT INTO restaurant_service_areas (restaurant_id, radius_km, is_active)
VALUES ($1, $2, $3)
ON CONFLICT (restaurant_id) DO UPDATE SET radius_km = EXCLUDED.radius_km, is_active = EXCLUDED.is_active
RETURNING *;

-- name: UpsertRestaurantDocument :one
INSERT INTO restaurant_documents (restaurant_id, document_type, file_url, document_number, expires_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (restaurant_id, document_type)
DO UPDATE SET file_url = EXCLUDED.file_url, document_number = EXCLUDED.document_number,
              expires_at = EXCLUDED.expires_at, status = 'pending', reviewed_by = NULL, reviewed_at = NULL, rejection_reason = NULL
RETURNING *;

-- name: ListRestaurantDocuments :many
SELECT * FROM restaurant_documents WHERE restaurant_id = $1 ORDER BY document_type;

-- name: ReviewRestaurantDocument :one
UPDATE restaurant_documents SET
  status = $2, rejection_reason = $3, reviewed_by = $4, reviewed_at = now()
WHERE id = $1
RETURNING *;

-- name: AddRestaurantStaff :one
INSERT INTO restaurant_staff (restaurant_id, user_id, permissions, invited_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListRestaurantStaff :many
SELECT * FROM restaurant_staff WHERE restaurant_id = $1 AND status = 'active';

-- name: RevokeRestaurantStaff :exec
UPDATE restaurant_staff SET status = 'revoked' WHERE id = $1;
