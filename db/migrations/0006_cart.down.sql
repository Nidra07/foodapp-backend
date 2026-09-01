DROP TABLE IF EXISTS cart_item_addons;
DROP TRIGGER IF EXISTS trg_cart_items_updated_at ON cart_items;
DROP TABLE IF EXISTS cart_items;
DROP TRIGGER IF EXISTS trg_carts_updated_at ON carts;
DROP TABLE IF EXISTS carts;
