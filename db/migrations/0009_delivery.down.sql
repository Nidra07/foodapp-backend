DROP INDEX IF EXISTS idx_delivery_assignments_one_active_per_order;
DROP TABLE IF EXISTS delivery_assignments;
DROP TYPE IF EXISTS assignment_status;
DROP TRIGGER IF EXISTS trg_delivery_partners_updated_at ON delivery_partners;
DROP TABLE IF EXISTS delivery_partners;
DROP TYPE IF EXISTS vehicle_type;
