DROP TABLE IF EXISTS menu_addons;
DROP TABLE IF EXISTS menu_addon_groups;
DROP TABLE IF EXISTS menu_item_variants;
DROP TABLE IF EXISTS menu_item_variant_groups;
DROP TRIGGER IF EXISTS trg_menu_items_updated_at ON menu_items;
DROP TABLE IF EXISTS menu_items;
DROP TYPE IF EXISTS food_type;
DROP TRIGGER IF EXISTS trg_menu_categories_updated_at ON menu_categories;
DROP TABLE IF EXISTS menu_categories;
