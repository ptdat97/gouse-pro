ALTER TABLE fulfillment_order
    DROP COLUMN IF EXISTS notify_phone,
    DROP COLUMN IF EXISTS notify_email,
    DROP COLUMN IF EXISTS customer_id;
