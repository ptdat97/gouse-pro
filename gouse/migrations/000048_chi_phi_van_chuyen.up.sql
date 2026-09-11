-- CHI PHÍ trả hãng vận chuyển — vế còn thiếu của mảng vận chuyển.
--
-- # Vì sao cần
--
-- Từ ADR-0018, phí vận chuyển khách trả được ghi là doanh thu nền tảng
-- (`NewShippingRevenueEntry`). Nhưng khoản nền tảng TRẢ cho hãng vận
-- chuyển chưa có bút toán nào, nên:
--
--   · doanh thu nền tảng đang bị THỔI LÊN đúng bằng chi phí chưa ghi
--   · lãi/lỗ mảng vận chuyển không đọc được — con số duy nhất trả lời
--     "thu phí ship như vậy là lãi hay lỗ" không tồn tại
--
-- # Hai tài khoản, không phải một
--
--   SHIPPING_EXPENSE  chi phí đã phát sinh   (CHI PHÍ, tăng khi ghi nợ)
--   CARRIER_PAYABLE   nợ hãng chưa trả       (NỢ PHẢI TRẢ)
--
-- Tách khỏi FEE_EXPENSE đang dùng cho phí cổng thanh toán: gộp lại thì
-- báo cáo không tách được lãi/lỗ vận chuyển khỏi chi phí thanh toán, mà
-- hai thứ đó do hai quyết định kinh doanh khác nhau chi phối.
--
-- CARRIER_PAYABLE KHÔNG bắt buộc chủ sở hữu ở migration này: hôm nay hệ
-- thống chưa có hồ sơ hãng vận chuyển (`shipping_provider` là CHUỖI, xem
-- HandOverRequest). Khi có, thêm ràng buộc chủ sở hữu là thay đổi cộng thêm.
ALTER TABLE ledger_line DROP CONSTRAINT IF EXISTS ledger_line_account_type_check;
ALTER TABLE ledger_line
    ADD CONSTRAINT ledger_line_account_type_check CHECK (account_type IN (
        'PLATFORM_CASH', 'PLATFORM_REVENUE', 'ACCOUNTS_RECEIVABLE',
        'SELLER_PAYABLE', 'SELLER_AVAILABLE',
        'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE',
        'CARRIER_PAYABLE',
        'COGS', 'FEE_EXPENSE', 'SHIPPING_EXPENSE', 'INVENTORY_ASSET'
    ));

-- Loại bút toán mới: ghi nhận nghĩa vụ trả hãng lúc BÀN GIAO.
--
-- Ghi lúc bàn giao chứ không chờ hóa đơn hãng: nghĩa vụ phát sinh khi hàng
-- rời kho, và chờ hóa đơn (thường về cuối tháng) nghĩa là mọi báo cáo
-- trong tháng đều thiếu vế chi phí — đúng tình trạng cần sửa.
ALTER TABLE ledger_entry DROP CONSTRAINT IF EXISTS ledger_entry_entry_type_check;
ALTER TABLE ledger_entry
    ADD CONSTRAINT ledger_entry_entry_type_check CHECK (entry_type IN (
        'ORDER_REVENUE', 'COGS', 'REFUND', 'PAYOUT', 'ADJUSTMENT', 'FEE',
        'SELLER_RELEASE', 'PAYMENT_RECEIVED', 'REVERSAL', 'SHIPPING_COST'
    ));
