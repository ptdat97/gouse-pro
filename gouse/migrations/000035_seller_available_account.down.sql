ALTER TABLE ledger_entry DROP CONSTRAINT IF EXISTS ledger_entry_entry_type_check;
ALTER TABLE ledger_entry
    ADD CONSTRAINT ledger_entry_entry_type_check CHECK (entry_type IN (
        'ORDER_REVENUE', 'COGS', 'REFUND', 'PAYOUT', 'ADJUSTMENT', 'FEE'
    ));

ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_payable_needs_owner;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_payable_needs_owner CHECK (
        account_type NOT IN (
            'SELLER_PAYABLE', 'CREATOR_PAYABLE',
            'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE'
        )
        OR length(trim(account_owner_id)) > 0
    );

ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_account_type_check;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_account_type_check CHECK (account_type IN (
        'PLATFORM_CASH', 'PLATFORM_REVENUE', 'SELLER_PAYABLE',
        'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE',
        'COGS', 'FEE_EXPENSE', 'INVENTORY_ASSET'
    ));
