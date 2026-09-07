-- Đơn thực hiện CHỜ THANH TOÁN — ADR-0018 phần A2.
--
-- VÌ SAO CẦN: trước cột này, không chỗ nào trong fulfillment biết đơn đã
-- trả tiền chưa, nên nhà bán đưa được một đơn CARD chưa trả đồng nào đi
-- hết tới HANDED_OVER — hàng rời kho cho một khoản tiền không tới.
--
-- Đo lúc thêm cột: 3166 đơn PENDING_PAYMENT và 3171 đơn thực hiện.
--
-- # Vì sao là CỜ trên đơn thực hiện, không phải hỏi ngược module order
--
-- `fulfillment` KHÔNG gọi ngược `order` (ADR-0007): chiều đó đi bằng event.
-- Cờ này là hình chiếu cục bộ của một sự kiện đã xảy ra — `order.paid` —
-- nên module tự trả lời được "đơn này được giao chưa" mà không phải hỏi ai
-- cho từng thao tác của nhà bán.
ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS cho_thanh_toan BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN fulfillment_order.cho_thanh_toan IS
    'TRUE = đơn trả trước chưa thu được tiền, nhà bán chưa được phép xử lý. '
    'Mở khóa bởi event order.paid. COD luôn FALSE — tiền về lúc giao hàng.';

-- MẶC ĐỊNH FALSE, có chủ ý.
--
-- Dữ liệu cũ không có `payment_method` trên phần lớn đơn (đo được: 3153
-- đơn để trống), nên không suy ngược được đơn nào là trả trước. Đặt TRUE
-- cho tất cả sẽ khóa hàng nghìn đơn mà không ai mở nổi — kể cả đơn COD
-- hợp lệ đang chờ giao.
--
-- Hệ quả phải ghi rõ: quy tắc mới chỉ áp cho đơn tạo TỪ ĐÂY TRỞ ĐI. Đơn
-- cũ giữ nguyên hành vi cũ. Đây là đánh đổi có ý thức, không phải sơ suất.

-- Chỉ mục cho việc tìm đơn đang bị khóa: nó là danh sách vận hành cần xem
-- (tiền chưa về mà hàng đã sẵn), và luôn nhỏ hơn hẳn bảng.
CREATE INDEX IF NOT EXISTS fulfillment_order_cho_thanh_toan_idx
    ON fulfillment_order (cho_thanh_toan)
    WHERE cho_thanh_toan;
