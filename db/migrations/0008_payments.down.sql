DROP TABLE IF EXISTS refunds;
DROP TYPE IF EXISTS refund_status;
DROP TABLE IF EXISTS saved_payment_methods;
DROP TRIGGER IF EXISTS trg_payment_transactions_updated_at ON payment_transactions;
DROP TABLE IF EXISTS payment_transactions;
DROP TYPE IF EXISTS payment_transaction_status;
