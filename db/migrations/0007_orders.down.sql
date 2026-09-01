DROP TABLE IF EXISTS order_status_history;
DROP TABLE IF EXISTS order_item_addons;
DROP TABLE IF EXISTS order_items;
DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders;
DROP TABLE IF EXISTS orders;
DROP TYPE IF EXISTS payment_method;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS order_status;
