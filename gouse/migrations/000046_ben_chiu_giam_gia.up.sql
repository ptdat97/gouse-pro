-- BÊN CHỊU khoản giảm giá, đóng băng vào phiên thanh toán.
--
-- # Vì sao cần
--
-- `order_line_adjustment.cost_bearer` đã có từ migration 000008 và đối
-- soát cuối kỳ đọc nó để biết trừ tiền ai. Nhưng giá trị ghi vào đó đang
-- được GÁN CỨNG "PLATFORM" ở `checkout/adapters.go`, kèm đúng một chú
-- thích nói ra điều đó:
--
--   "Khi có chương trình do nhà bán tự chạy, chỗ này phải phân biệt —
--    nếu không thì đối soát cuối kỳ tính nhầm bên chịu chi phí."
--
-- Chương trình do nhà bán chạy đã tạo được (`CreatePromotion` nhận
-- `CostBearer: SELLER`), nên đây không còn là chuyện tương lai.
--
-- # Vì sao ĐÓNG BĂNG chứ không tra lại lúc đặt đơn
--
-- Nguyên tắc P9: tỷ lệ chia có thể đổi khi thỏa thuận với nhà bán thay
-- đổi. Đọc cấu hình HIỆN TẠI lúc hoàn tất phiên nghĩa là một lần sửa thỏa
-- thuận làm đổi số tiền của những đơn đã đặt trước đó — và không ai giải
-- thích được chênh lệch khi nhà bán khiếu nại.
ALTER TABLE checkout
    ADD COLUMN IF NOT EXISTS discount_cost_bearer TEXT NOT NULL DEFAULT 'PLATFORM'
        CHECK (discount_cost_bearer IN ('PLATFORM', 'SELLER', 'SHARED'));

COMMENT ON COLUMN checkout.discount_cost_bearer IS
    'Bên chịu khoản giảm giá, đóng băng lúc áp mã. Chảy xuống '
    'order_line_adjustment.cost_bearer khi đặt đơn.';
