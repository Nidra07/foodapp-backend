DROP TABLE IF EXISTS search_logs;
DROP INDEX IF EXISTS idx_menu_items_search_vector;
ALTER TABLE menu_items DROP COLUMN IF EXISTS search_vector;
DROP INDEX IF EXISTS idx_restaurants_search_vector;
ALTER TABLE restaurants DROP COLUMN IF EXISTS search_vector;
