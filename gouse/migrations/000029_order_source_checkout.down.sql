DROP INDEX IF EXISTS order_one_per_checkout;
ALTER TABLE "order" DROP COLUMN IF EXISTS source_checkout_id;
