DROP INDEX IF EXISTS ledger_entry_reverses_unique;
ALTER TABLE ledger_entry DROP COLUMN IF EXISTS reverses_entry_id;

ALTER TABLE ledger_entry DROP CONSTRAINT IF EXISTS ledger_entry_entry_type_check;
ALTER TABLE ledger_entry
    ADD CONSTRAINT ledger_entry_entry_type_check CHECK (entry_type IN (
        'ORDER_REVENUE', 'COGS', 'REFUND', 'PAYOUT', 'ADJUSTMENT', 'FEE',
        'SELLER_RELEASE'
    ));

ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_account_type_check;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_account_type_check CHECK (account_type IN (
        'PLATFORM_CASH', 'PLATFORM_REVENUE',
        'SELLER_PAYABLE', 'SELLER_AVAILABLE',
        'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE',
        'COGS', 'FEE_EXPENSE', 'INVENTORY_ASSET'
    ));
