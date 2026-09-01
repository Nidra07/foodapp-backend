-- name: CreatePaymentTransaction :one
INSERT INTO payment_transactions (order_id, customer_id, amount, currency, method, gateway, gateway_order_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetPaymentTransactionByID :one
SELECT * FROM payment_transactions WHERE id = $1;

-- name: GetPaymentTransactionByGatewayOrderID :one
SELECT * FROM payment_transactions WHERE gateway = $1 AND gateway_order_id = $2;

-- name: ListPaymentTransactionsByOrder :many
SELECT * FROM payment_transactions WHERE order_id = $1 ORDER BY created_at DESC;

-- name: GetLatestPaymentTransactionForOrder :one
SELECT * FROM payment_transactions WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: MarkPaymentCaptured :one
UPDATE payment_transactions SET
  status = 'captured', gateway_payment_id = $2, gateway_signature = $3, completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkPaymentFailed :one
UPDATE payment_transactions SET status = 'failed', failure_reason = $2, completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkPaymentAuthorized :one
UPDATE payment_transactions SET status = 'authorized', gateway_payment_id = $2
WHERE id = $1
RETURNING *;

-- name: RecordWebhookPayload :exec
UPDATE payment_transactions SET raw_webhook_payload = $2 WHERE id = $1;

-- name: SetPaymentRefundStatus :exec
UPDATE payment_transactions SET status = $2 WHERE id = $1;

-- name: CreateSavedPaymentMethod :one
INSERT INTO saved_payment_methods (customer_id, method, gateway, gateway_token, display_label, is_default)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListSavedPaymentMethods :many
SELECT * FROM saved_payment_methods WHERE customer_id = $1 ORDER BY is_default DESC, created_at DESC;

-- name: UnsetDefaultPaymentMethods :exec
UPDATE saved_payment_methods SET is_default = false WHERE customer_id = $1;

-- name: DeleteSavedPaymentMethod :exec
DELETE FROM saved_payment_methods WHERE id = $1 AND customer_id = $2;

-- name: CreateRefund :one
INSERT INTO refunds (payment_transaction_id, order_id, amount, reason, initiated_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRefundByID :one
SELECT * FROM refunds WHERE id = $1;

-- name: ListRefundsByOrder :many
SELECT * FROM refunds WHERE order_id = $1 ORDER BY initiated_at DESC;

-- name: MarkRefundCompleted :one
UPDATE refunds SET status = 'completed', gateway_refund_id = $2, completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkRefundFailed :one
UPDATE refunds SET status = 'failed', failure_reason = $2, completed_at = now()
WHERE id = $1
RETURNING *;

-- name: SumCompletedRefundsForOrder :one
SELECT COALESCE(SUM(amount), 0)::numeric AS total_refunded FROM refunds WHERE order_id = $1 AND status = 'completed';
