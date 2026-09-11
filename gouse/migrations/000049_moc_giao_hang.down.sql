DROP INDEX IF EXISTS order_delivered_at_idx;
ALTER TABLE "order" DROP COLUMN IF EXISTS delivered_at;
