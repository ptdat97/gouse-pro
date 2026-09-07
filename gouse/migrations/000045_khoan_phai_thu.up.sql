-- KHOẢN PHẢI THU — ADR-0018 phần B1.
--
-- # Vì sao cần
--
-- Bút toán doanh thu ghi lúc `checkout.completed`, tức là lúc khách bấm
-- xong phiên thanh toán chứ KHÔNG phải lúc tiền về. Nó ghi NỢ
-- `PLATFORM_CASH` — nghĩa là khẳng định nền tảng đang cầm số tiền đó.
--
-- Đo được lúc thêm migration này:
--
--   PENDING_PAYMENT  3110 bút toán  PLATFORM_CASH ghi NỢ 1.132.273.000 đ
--   PAID                1 bút toán                             390.000 đ
--
-- Ghi doanh thu sớm còn là tranh luận kế toán. Nói mình đang cầm 1,13 tỷ
-- tiền mặt không có là một câu SAI SỰ THẬT về tài sản, và nó đi thẳng vào
-- mọi báo cáo số dư.
--
-- ACCOUNTS_RECEIVABLE nói đúng thứ đang có: một khoản KHÁCH NỢ. Khi thu
-- được thì bút toán thứ hai chuyển nó thành tiền mặt.
--
-- # Vì sao KHÔNG dời luôn thời điểm ghi doanh thu
--
-- Đó là phương án B2, và nó động tới cả COD (tiền về lúc GIAO hàng) lẫn
-- mọi báo cáo doanh thu. Chủ dự án chọn B1: bỏ đúng câu nói sai, không mở
-- lại tranh luận lớn hơn. B2 vẫn nên bàn, chỉ là không cùng lúc.
ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_account_type_check;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_account_type_check CHECK (account_type IN (
        'PLATFORM_CASH', 'PLATFORM_REVENUE', 'ACCOUNTS_RECEIVABLE',
        'SELLER_PAYABLE', 'SELLER_AVAILABLE',
        'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE',
        'COGS', 'FEE_EXPENSE', 'INVENTORY_ASSET'
    ));

-- ACCOUNTS_RECEIVABLE KHÔNG cần chủ sở hữu.
--
-- Nó là khoản phải thu của NỀN TẢNG với khách, gộp chung — không phải một
-- sổ nợ theo từng khách. Bắt nó có `account_owner_id` sẽ tạo hàng nghìn
-- tài khoản con mà không ai đối chiếu theo từng cái.

-- Hai loại bút toán mới.
--
--   PAYMENT_RECEIVED  thu được tiền: phải thu → tiền mặt
--   REVERSAL          ĐẢO một bút toán đã ghi sai
--
-- REVERSAL tồn tại vì sổ cái BẤT BIẾN (ADR-0008): ghi sai thì không xóa
-- được, chỉ ghi bút toán ngược. 3110 bút toán đã ghi sai đang chờ đúng
-- quy trình này.
ALTER TABLE ledger_entry DROP CONSTRAINT IF EXISTS ledger_entry_entry_type_check;
ALTER TABLE ledger_entry
    ADD CONSTRAINT ledger_entry_entry_type_check CHECK (entry_type IN (
        'ORDER_REVENUE', 'COGS', 'REFUND', 'PAYOUT', 'ADJUSTMENT', 'FEE',
        'SELLER_RELEASE', 'PAYMENT_RECEIVED', 'REVERSAL'
    ));

-- Bút toán đảo trỏ về bút toán GỐC nó đảo.
--
-- Không có cột này thì "đã đảo cái nào" chỉ nằm trong phần mô tả bằng chữ,
-- và không truy vấn nào trả lời được "bút toán này còn hiệu lực không" —
-- câu hỏi đầu tiên của mọi lần đối chiếu.
ALTER TABLE ledger_entry
    ADD COLUMN IF NOT EXISTS reverses_entry_id TEXT;

COMMENT ON COLUMN ledger_entry.reverses_entry_id IS
    'Bút toán gốc mà bút toán này ĐẢO. Chỉ có giá trị với entry_type = REVERSAL.';

-- Một bút toán chỉ được đảo MỘT lần.
--
-- Đảo hai lần là ghi ngược số tiền hai lần: sổ cái mất cân đối theo đúng
-- số tiền đó, và không ai phát hiện cho tới kỳ đối chiếu.
CREATE UNIQUE INDEX IF NOT EXISTS ledger_entry_reverses_unique
    ON ledger_entry (reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;
