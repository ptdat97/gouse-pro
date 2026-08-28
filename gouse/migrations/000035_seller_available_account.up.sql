-- Tài khoản SELLER_AVAILABLE — tiền nhà bán ĐÃ RÚT ĐƯỢC.
--
-- VÌ SAO TÁCH KHỎI SELLER_PAYABLE: hai trạng thái tiền khác nhau về QUYỀN.
--
--   SELLER_PAYABLE    đơn đã giao, CÒN trong hạn đổi trả
--   SELLER_AVAILABLE  hết hạn đổi trả, sẵn sàng chi trả
--
-- Chi trả từ SELLER_PAYABLE là chi trả một khoản khách vẫn có thể đòi lại
-- — và khi nhà bán đã rút rồi thì đòi bằng gì.
--
-- # Vì sao là TÀI KHOẢN chứ không phải một cột trạng thái
--
-- Số dư là KẾT QUẢ TÍNH từ sổ cái (ADR-0008 quyết định 3). "Chuyển trạng
-- thái tiền" vì thế phải là một BÚT TOÁN:
--
--   DEBIT   SELLER_PAYABLE
--   CREDIT  SELLER_AVAILABLE
--
-- Tổng nợ phải trả không đổi, tiền chỉ đổi chỗ — và mọi lần đổi chỗ đều
-- để lại dấu vết đối chiếu được. Sửa thẳng một cột số dư là phá bỏ chính
-- thứ khiến sổ cái có nghĩa.
--
-- BA ràng buộc phải sửa cùng lúc, không phải một. Thiếu bất kỳ cái nào
-- thì bút toán bị database từ chối — và ba tập hợp giá trị hợp lệ này là
-- ba chỗ khác nhau nhớ cùng một điều.

-- 1. Loại tài khoản
ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_account_type_check;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_account_type_check CHECK (account_type IN (
        'PLATFORM_CASH', 'PLATFORM_REVENUE',
        'SELLER_PAYABLE', 'SELLER_AVAILABLE',
        'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE',
        'COGS', 'FEE_EXPENSE', 'INVENTORY_ASSET'
    ));

-- 2. Tài khoản phải trả BẮT BUỘC có chủ sở hữu.
--
-- SELLER_AVAILABLE mà không biết của nhà bán nào thì không chi trả được —
-- đúng lý do SELLER_PAYABLE đã nằm trong danh sách này từ đầu.
ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_payable_needs_owner;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_payable_needs_owner CHECK (
        account_type NOT IN (
            'SELLER_PAYABLE', 'SELLER_AVAILABLE',
            'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE'
        )
        OR length(trim(account_owner_id)) > 0
    );

-- 3. Loại bút toán
ALTER TABLE ledger_entry DROP CONSTRAINT IF EXISTS ledger_entry_entry_type_check;
ALTER TABLE ledger_entry
    ADD CONSTRAINT ledger_entry_entry_type_check CHECK (entry_type IN (
        'ORDER_REVENUE', 'COGS', 'REFUND', 'PAYOUT', 'ADJUSTMENT', 'FEE',
        'SELLER_RELEASE'
    ));
