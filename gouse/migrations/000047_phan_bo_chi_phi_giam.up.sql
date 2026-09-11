-- BẢNG PHÂN BỔ chi phí khuyến mãi, đóng băng vào phiên thanh toán.
--
-- # Vì sao một cột tên-bên-chịu là KHÔNG ĐỦ
--
-- Migration 000046 lưu `discount_cost_bearer` — đúng ba trạng thái
-- PLATFORM/SELLER/SHARED. Nó đủ cho cột `order_line_adjustment.cost_bearer`
-- mà đối soát đọc, nhưng KHÔNG đủ cho sổ cái: chương trình CHIA ĐÔI cần
-- biết mỗi bên gánh bao nhiêu ĐỒNG, không chỉ biết "có chia".
--
-- Hệ quả trước cột này: bút toán giảm giá ghi trọn về một bên, nên phần
-- của nhà bán trong chương trình chia đôi không bao giờ bị trừ — nền tảng
-- gánh hộ. Sai theo hướng đó thì nhà bán không mất tiền nên không ai báo.
--
-- # Vì sao ĐÓNG BĂNG cả bảng chứ không lưu tỷ lệ rồi tính lại
--
-- Cùng lý do với 000046 (nguyên tắc P9), nhưng chặt hơn: `AllocateCost`
-- rải phần dư của phép chia cho bên đầu tiên để tổng luôn bằng ĐÚNG số
-- tiền giảm. Tính lại ở nơi khác là mở đường cho một phép làm tròn thứ
-- hai, và lệch một đồng ở đây là một khoản KHÔNG AI CHỊU — xuất hiện ở
-- mọi đơn dùng mã, cuối tháng thành con số không nhỏ.
--
-- Dạng: [{"bearer":"PLATFORM","seller_id":"","amount":24500}, ...]
ALTER TABLE checkout
    ADD COLUMN IF NOT EXISTS discount_allocations JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN checkout.discount_allocations IS
    'Bảng phân bổ chi phí khuyến mãi do promotion.AllocateCost tính, đóng '
    'băng lúc áp mã. Tổng các phần luôn bằng đúng discount_amount.';
