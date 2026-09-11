-- MỐC GIAO HÀNG của đơn — điều kiện để cưỡng chế hạn đổi trả.
--
-- # Vì sao cần
--
-- `returns.XinTra` chỉ kiểm "đơn đã giao chưa", KHÔNG kiểm hạn nào cả —
-- `DonHang` của nó chỉ có `DaGiao bool`. Nên khách xin trả hàng sau bao
-- lâu cũng được, kể cả khi đơn đã COMPLETED.
--
-- Hệ quả tiền, và nó nặng: `CompleteDelivered` chuyển số dư nhà bán sang
-- KHẢ DỤNG sau hạn đổi trả. Quá hạn mà vẫn cho trả nghĩa là nền tảng hoàn
-- tiền cho khách trong khi tiền nhà bán đã rút được — đúng thứ mà chú
-- thích của `Order.Complete` nói là "rất khó thu hồi".
--
-- # Vì sao ở ĐƠN chứ không tra ngược fulfillment
--
-- `order` KHÔNG hỏi ngược `fulfillment` (ADR-0007); chiều đó đi bằng event.
-- Order đã nghe `fulfillment.progress_changed` để tính trạng thái tổng
-- hợp, nên nó là nơi DUY NHẤT biết "mọi gói đã giao lúc nào" mà không phá
-- ranh giới.
ALTER TABLE "order"
    ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;

COMMENT ON COLUMN "order".delivered_at IS
    'Thời điểm đơn chuyển sang DELIVERED. Mốc tính hạn đổi trả. '
    'NULL với đơn chưa giao xong.';

-- Chỉ mục cho việc tìm đơn sắp hết hạn đổi trả.
CREATE INDEX IF NOT EXISTS order_delivered_at_idx
    ON "order" (delivered_at)
    WHERE delivered_at IS NOT NULL;
