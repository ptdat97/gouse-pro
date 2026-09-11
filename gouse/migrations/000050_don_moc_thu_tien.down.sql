DROP INDEX IF EXISTS order_paid_at_idx;
ALTER TABLE "order" DROP COLUMN IF EXISTS paid_at;
