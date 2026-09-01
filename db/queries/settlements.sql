-- name: CreatePayoutAccount :one
INSERT INTO payout_accounts (owner_type, owner_id, account_holder_name, account_number_last4, account_token, ifsc_code, bank_name)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (owner_type, owner_id) DO UPDATE SET
  account_holder_name = EXCLUDED.account_holder_name, account_number_last4 = EXCLUDED.account_number_last4,
  account_token = EXCLUDED.account_token, ifsc_code = EXCLUDED.ifsc_code, bank_name = EXCLUDED.bank_name, is_verified = false
RETURNING *;

-- name: GetPayoutAccount :one
SELECT * FROM payout_accounts WHERE owner_type = $1 AND owner_id = $2;

-- name: SetPayoutAccountVerified :exec
UPDATE payout_accounts SET is_verified = $2 WHERE id = $1;

-- name: CreateSettlementCycle :one
INSERT INTO settlement_cycles (cycle_start, cycle_end)
VALUES ($1, $2)
RETURNING *;

-- name: GetSettlementCycle :one
SELECT * FROM settlement_cycles WHERE id = $1;

-- name: ListSettlementCycles :many
SELECT * FROM settlement_cycles ORDER BY cycle_start DESC LIMIT $1 OFFSET $2;

-- name: SetCycleStatus :exec
UPDATE settlement_cycles SET status = $2, processed_at = CASE WHEN $2 = 'completed' THEN now() ELSE processed_at END WHERE id = $1;

-- name: UpsertRestaurantSettlement :one
INSERT INTO restaurant_settlements (cycle_id, restaurant_id, order_count, gross_subtotal, commission_amount, net_payable)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (cycle_id, restaurant_id) DO UPDATE SET
  order_count = EXCLUDED.order_count, gross_subtotal = EXCLUDED.gross_subtotal,
  commission_amount = EXCLUDED.commission_amount, net_payable = EXCLUDED.net_payable
RETURNING *;

-- name: GetRestaurantSettlement :one
SELECT * FROM restaurant_settlements WHERE id = $1;

-- name: ListRestaurantSettlementsForRestaurant :many
SELECT * FROM restaurant_settlements WHERE restaurant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListRestaurantSettlementsForCycle :many
SELECT * FROM restaurant_settlements WHERE cycle_id = $1 ORDER BY net_payable DESC;

-- name: ListPendingRestaurantSettlements :many
SELECT * FROM restaurant_settlements WHERE status = 'pending' ORDER BY created_at LIMIT $1 OFFSET $2;

-- name: MarkRestaurantSettlementPaid :one
UPDATE restaurant_settlements SET status = 'paid', payout_reference = $2, payout_account_id = $3, paid_at = now() WHERE id = $1 RETURNING *;

-- name: MarkRestaurantSettlementFailed :one
UPDATE restaurant_settlements SET status = 'failed', failure_reason = $2 WHERE id = $1 RETURNING *;

-- name: UpsertDeliverySettlement :one
INSERT INTO delivery_settlements (cycle_id, delivery_partner_id, delivery_count, gross_earnings, incentive_amount, net_payable)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (cycle_id, delivery_partner_id) DO UPDATE SET
  delivery_count = EXCLUDED.delivery_count, gross_earnings = EXCLUDED.gross_earnings,
  incentive_amount = EXCLUDED.incentive_amount, net_payable = EXCLUDED.net_payable
RETURNING *;

-- name: GetDeliverySettlement :one
SELECT * FROM delivery_settlements WHERE id = $1;

-- name: ListDeliverySettlementsForPartner :many
SELECT * FROM delivery_settlements WHERE delivery_partner_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListDeliverySettlementsForCycle :many
SELECT * FROM delivery_settlements WHERE cycle_id = $1 ORDER BY net_payable DESC;

-- name: ListPendingDeliverySettlements :many
SELECT * FROM delivery_settlements WHERE status = 'pending' ORDER BY created_at LIMIT $1 OFFSET $2;

-- name: MarkDeliverySettlementPaid :one
UPDATE delivery_settlements SET status = 'paid', payout_reference = $2, payout_account_id = $3, paid_at = now() WHERE id = $1 RETURNING *;

-- name: MarkDeliverySettlementFailed :one
UPDATE delivery_settlements SET status = 'failed', failure_reason = $2 WHERE id = $1 RETURNING *;

-- name: ListDistinctRestaurantsWithDeliveredOrders :many
-- Drives ProcessCycle: which restaurants need a settlement row computed
-- for this cycle at all (rather than iterating every restaurant in the
-- system, most of which had zero activity in a given window).
SELECT DISTINCT restaurant_id FROM orders
WHERE status = 'delivered' AND payment_status = 'paid'
  AND delivered_at >= sqlc.arg('from_ts')::timestamptz AND delivered_at < sqlc.arg('to_ts')::timestamptz;

-- name: ListDistinctPartnersWithDeliveries :many
SELECT DISTINCT delivery_partner_id FROM delivery_assignments
WHERE status = 'delivered'
  AND delivered_at >= sqlc.arg('from_ts')::timestamptz AND delivered_at < sqlc.arg('to_ts')::timestamptz;
