-- name: CreateDeliveryPartner :one
INSERT INTO delivery_partners (user_id, vehicle_type, vehicle_number, license_number)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetDeliveryPartnerByID :one
SELECT * FROM delivery_partners WHERE id = $1;

-- name: GetDeliveryPartnerByUserID :one
SELECT * FROM delivery_partners WHERE user_id = $1;

-- name: SetDeliveryPartnerKYCStatus :exec
UPDATE delivery_partners SET kyc_status = $2 WHERE id = $1;

-- name: SetDeliveryPartnerOnline :exec
UPDATE delivery_partners SET is_online = $2 WHERE id = $1;

-- name: UpdateDeliveryPartnerLocation :exec
UPDATE delivery_partners SET
  current_location = ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography,
  last_location_update_at = now()
WHERE id = sqlc.arg('id');

-- name: IncrementActiveAssignmentCount :exec
UPDATE delivery_partners SET active_assignment_count = active_assignment_count + 1 WHERE id = $1;

-- name: DecrementActiveAssignmentCount :exec
UPDATE delivery_partners SET active_assignment_count = GREATEST(active_assignment_count - 1, 0) WHERE id = $1;

-- name: ListNearbyAvailablePartners :many
-- Candidates for assignment: verified, currently online, under the
-- concurrent-delivery cap, ordered by distance. The cap (max_active)
-- is passed in rather than hardcoded so the application layer owns
-- that policy.
SELECT id, user_id,
       ST_Distance(current_location, ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography) / 1000.0 AS distance_km
FROM delivery_partners
WHERE kyc_status = 'verified'
  AND is_online = true
  AND active_assignment_count < sqlc.arg('max_active')::smallint
  AND current_location IS NOT NULL
  AND ST_DWithin(current_location, ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326)::geography, sqlc.arg('radius_m')::float8)
ORDER BY distance_km ASC
LIMIT $1;

-- name: CreateDeliveryAssignment :one
INSERT INTO delivery_assignments (order_id, restaurant_id, delivery_partner_id, delivery_otp)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetDeliveryAssignmentByID :one
SELECT * FROM delivery_assignments WHERE id = $1;

-- name: GetActiveAssignmentForOrder :one
SELECT * FROM delivery_assignments WHERE order_id = $1 AND status NOT IN ('rejected', 'cancelled') LIMIT 1;

-- name: ListAssignmentsForOrder :many
SELECT * FROM delivery_assignments WHERE order_id = $1 ORDER BY offered_at DESC;

-- name: ListAssignmentsForPartner :many
SELECT * FROM delivery_assignments
WHERE delivery_partner_id = $1
  AND (sqlc.narg('status')::assignment_status IS NULL OR status = sqlc.narg('status'))
ORDER BY offered_at DESC
LIMIT $2 OFFSET $3;

-- name: ListActiveAssignmentsForPartner :many
SELECT * FROM delivery_assignments
WHERE delivery_partner_id = $1 AND status IN ('offered', 'accepted', 'picked_up')
ORDER BY offered_at ASC;

-- name: AcceptAssignment :one
UPDATE delivery_assignments SET status = 'accepted', accepted_at = now() WHERE id = $1 AND status = 'offered' RETURNING *;

-- name: RejectAssignment :one
UPDATE delivery_assignments SET status = 'rejected', rejected_at = now() WHERE id = $1 AND status = 'offered' RETURNING *;

-- name: MarkPickedUp :one
UPDATE delivery_assignments SET status = 'picked_up', picked_up_at = now() WHERE id = $1 AND status = 'accepted' RETURNING *;

-- name: MarkDelivered :one
UPDATE delivery_assignments SET status = 'delivered', delivered_at = now() WHERE id = $1 AND status = 'picked_up' RETURNING *;

-- name: CancelAssignment :one
UPDATE delivery_assignments SET status = 'cancelled', cancelled_at = now(), cancellation_reason = $2 WHERE id = $1 RETURNING *;

-- name: CountDeliveredForPartner :one
-- Used by the Settlements module (same consumer-interface pattern as
-- Orders' SumSettlementDataForRestaurant) to compute a delivery
-- partner's payout for a cycle: a flat count of completed deliveries in
-- the window, keyed on delivered_at.
SELECT COUNT(*)::bigint AS delivery_count
FROM delivery_assignments
WHERE delivery_partner_id = $1
  AND status = 'delivered'
  AND delivered_at >= sqlc.arg('from_ts')::timestamptz
  AND delivered_at < sqlc.arg('to_ts')::timestamptz;
