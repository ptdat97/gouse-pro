ALTER TABLE seller DROP CONSTRAINT IF EXISTS seller_verified_needs_account;
ALTER TABLE seller
    DROP COLUMN IF EXISTS bank_code,
    DROP COLUMN IF EXISTS bank_account_number_enc,
    DROP COLUMN IF EXISTS bank_account_last4,
    DROP COLUMN IF EXISTS bank_account_holder;
