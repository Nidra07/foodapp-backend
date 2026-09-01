-- name: SearchRestaurants :many
-- Combines full-text relevance (ts_rank_cd against the generated
-- search_vector) with the same "within the restaurant's own service
-- radius" geo-filter used by ListNearbyRestaurants in
-- db/queries/restaurants.sql — a text match on a restaurant the
-- customer can't actually order from isn't a useful search result.
SELECT
  r.id, r.name, r.slug, r.cuisine_tags, r.rating_avg, r.rating_count, r.logo_url, r.min_order_amount,
  ST_Distance(r.location, ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography) / 1000.0 AS distance_km,
  ts_rank_cd(r.search_vector, websearch_to_tsquery('english', sqlc.arg(query))) AS rank
FROM restaurants r
JOIN restaurant_service_areas sa ON sa.restaurant_id = r.id AND sa.is_active = true
WHERE r.deleted_at IS NULL
  AND r.status = 'approved'
  AND r.is_accepting_orders = true
  AND r.search_vector @@ websearch_to_tsquery('english', sqlc.arg(query))
  AND ST_DWithin(
        r.location,
        ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography,
        LEAST(sqlc.arg(search_radius_m)::float8, sa.radius_km * 1000)
      )
ORDER BY rank DESC, distance_km ASC
LIMIT $1 OFFSET $2;

-- name: SearchMenuItems :many
-- "Search for biryani" — finds dishes across every live restaurant's
-- menu, joined back to the restaurant for display + the same
-- service-radius geo-filter as SearchRestaurants, so a matching dish at
-- a restaurant that can't deliver here doesn't show up.
SELECT
  mi.id AS item_id, mi.name AS item_name, mi.base_price, mi.food_type, mi.image_url, mi.is_bestseller,
  r.id AS restaurant_id, r.name AS restaurant_name, r.slug AS restaurant_slug, r.rating_avg,
  ST_Distance(r.location, ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography) / 1000.0 AS distance_km,
  ts_rank_cd(mi.search_vector, websearch_to_tsquery('english', sqlc.arg(query))) AS rank
FROM menu_items mi
JOIN restaurants r ON r.id = mi.restaurant_id
JOIN restaurant_service_areas sa ON sa.restaurant_id = r.id AND sa.is_active = true
WHERE mi.deleted_at IS NULL AND mi.is_active = true AND mi.is_available = true
  AND r.deleted_at IS NULL AND r.status = 'approved' AND r.is_accepting_orders = true
  AND mi.search_vector @@ websearch_to_tsquery('english', sqlc.arg(query))
  AND ST_DWithin(
        r.location,
        ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography,
        LEAST(sqlc.arg(search_radius_m)::float8, sa.radius_km * 1000)
      )
ORDER BY rank DESC, distance_km ASC
LIMIT $1 OFFSET $2;

-- name: LogSearchQuery :one
INSERT INTO search_logs (user_id, query, search_type, result_count)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListTrendingSearches :many
-- Powers a simple "trending near you" / suggestions list: the most
-- frequently searched queries in a recent window, case-folded so
-- "Biryani" and "biryani" count as the same trend.
SELECT lower(query) AS query, COUNT(*)::bigint AS search_count
FROM search_logs
WHERE created_at > now() - sqlc.arg('window')::interval
  AND result_count > 0 -- zero-result queries are a gap signal, not a suggestion to repeat
GROUP BY lower(query)
ORDER BY search_count DESC
LIMIT $1;

-- name: ListZeroResultSearches :many
-- Operational visibility: what are customers searching for that the
-- platform can't currently satisfy (missing restaurant/cuisine/dish).
-- Not exposed to customers — this is an admin/analytics query.
SELECT lower(query) AS query, COUNT(*)::bigint AS search_count
FROM search_logs
WHERE created_at > now() - sqlc.arg('window')::interval
  AND result_count = 0
GROUP BY lower(query)
ORDER BY search_count DESC
LIMIT $1;
