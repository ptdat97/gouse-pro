-- Chuyển quyền sở hữu `fulfillment_order` từ module `order` sang `fulfillment`.
--
-- VÌ SAO: migration 000008 tạo hai bảng này cùng lúc với `order`, vì lúc đó
-- module `fulfillment` chưa tồn tại. Nhưng tài liệu ranh giới module
-- (docs/03-architecture/module-boundaries.md mục 3) và ADR-0007 đều ghi rõ
-- chúng thuộc `fulfillment`:
--
--     order        = HỢP ĐỒNG với khách  (Order, OrderLine)
--     fulfillment  = GÓC NHÌN VẬN HÀNH   (FulfillmentOrder, Shipment)
--
-- Việc chuyển không đổi cấu trúc bảng — chỉ đổi ai được ghi vào nó, và bỏ
-- các khóa ngoại giờ đã vượt ranh giới module.
--
-- HỆ QUẢ THẬT của việc để sai chỗ: khi tách service, module `order` sẽ
-- mang theo dữ liệu vận hành của seller — đúng thứ ADR-0007 tách ra để
-- tránh.

-- ---------------------------------------------------------------------
-- Bỏ khóa ngoại VƯỢT MODULE.
-- ---------------------------------------------------------------------
--
-- Sau khi chuyển, `fulfillment_order.order_id` trỏ sang bảng của module
-- `order`, và `fulfillment_order_line.order_line_id` cũng vậy. Khóa ngoại
-- cứng giữa hai module ngăn việc tách service và buộc migration phải điều
-- phối giữa hai bên.
--
-- Đánh đổi: database không còn đảm bảo toàn vẹn tham chiếu ở hai chỗ này.
-- Bù lại bằng kiểm tra ở tầng ứng dụng và job đối soát — cùng cách đã áp
-- dụng cho mọi tham chiếu vượt module khác (05-data/data-model.md mục 3).

ALTER TABLE fulfillment_order
    DROP CONSTRAINT IF EXISTS fulfillment_order_order_id_fkey;

ALTER TABLE fulfillment_order_line
    DROP CONSTRAINT IF EXISTS fulfillment_order_line_order_line_id_fkey;

-- Khóa ngoại NỘI BỘ module fulfillment thì GIỮ NGUYÊN:
-- fulfillment_order_line.fulfillment_order_id → fulfillment_order.id

-- ---------------------------------------------------------------------
-- Bổ sung các cột của giai đoạn 5.
-- ---------------------------------------------------------------------

-- Kho xuất hàng. NULL với đơn của seller tự giao — nền tảng không biết và
-- không cần biết seller lấy hàng từ đâu.
ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS stock_location_id TEXT NOT NULL DEFAULT '';

-- Ba mô hình thực hiện (fulfillment.md mục 4):
--
--   PLATFORM         nền tảng giữ hàng, nền tảng đóng gói (own brand)
--   SELLER           seller giữ hàng, seller đóng gói (đa số marketplace)
--   PLATFORM_SERVICE seller SỞ HỮU hàng, để ở kho nền tảng, nền tảng đóng
--
-- Mô hình thứ ba là lý do InventoryItem phải tách owner_id khỏi location_id:
-- hàng nằm ở kho nền tảng nhưng KHÔNG phải tài sản của nền tảng.
ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS fulfillment_type TEXT NOT NULL DEFAULT 'SELLER'
        CHECK (fulfillment_type IN ('PLATFORM', 'SELLER', 'PLATFORM_SERVICE'));

-- Lý do giao thất bại.
--
-- Khách cần biết vì sao chưa nhận được hàng, và bộ phận vận hành cần biết
-- có nên giao lại hay trả về người gửi.
ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS failure_reason TEXT NOT NULL DEFAULT '';

-- Đã giao thất bại thì BẮT BUỘC có lý do.
ALTER TABLE fulfillment_order
    ADD CONSTRAINT fulfillment_order_failure_needs_reason CHECK (
        status <> 'DELIVERY_FAILED' OR length(trim(failure_reason)) > 0
    );

-- Vận chuyển. Tên nhà vận chuyển là DỮ LIỆU, không phải mã nguồn: giá và
-- chất lượng của các đối tác thay đổi thường xuyên, và nền tảng cần đổi
-- hoặc dùng đồng thời nhiều đối tác (nguyên tắc P13).
ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS shipping_method TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS shipping_provider TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tracking_number TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS estimated_delivery_date DATE;

-- Trạng thái COMPLETED có Ý NGHĨA TÀI CHÍNH (fulfillment.md mục 5):
--
--     DELIVERED   → số dư seller vẫn Pending
--     COMPLETED   → số dư chuyển Available, được chi trả
--
-- Đây là cơ chế bảo vệ nền tảng khỏi rủi ro hoàn hàng sau khi đã trả tiền
-- seller: phần lớn yêu cầu hoàn xảy ra trong thời hạn đổi trả.
ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

-- Mở rộng danh sách trạng thái theo vòng đời đầy đủ ở fulfillment.md mục 5.
--
-- PICKING, HANDED_OVER, IN_TRANSIT, DELIVERY_FAILED và COMPLETED chưa có ở
-- giai đoạn 4 vì lúc đó chưa có module vận hành.
ALTER TABLE fulfillment_order
    DROP CONSTRAINT IF EXISTS fulfillment_order_status_check;

ALTER TABLE fulfillment_order
    ADD CONSTRAINT fulfillment_order_status_check CHECK (status IN (
        'PENDING',          -- chờ xử lý
        'ALLOCATED',        -- đã phân bổ nguồn hàng, tồn kho committed
        'CONFIRMED',        -- seller xác nhận sẽ giao
        'PICKING',          -- đang lấy hàng
        'PACKED',           -- đã đóng gói
        'HANDED_OVER',      -- đã bàn giao vận chuyển
        'IN_TRANSIT',       -- đang vận chuyển
        'DELIVERY_FAILED',  -- giao thất bại
        'DELIVERED',        -- đã giao
        'COMPLETED',        -- hết hạn đổi trả — số dư seller chuyển Available
        'CANCELLED'
    ));

-- Đã hoàn tất thì BẮT BUỘC có mốc thời gian: đó là mốc tính hạn chi trả
-- cho seller, và thiếu nó thì không biết khi nào tiền được giải phóng.
ALTER TABLE fulfillment_order
    ADD CONSTRAINT fulfillment_order_completed_has_time CHECK (
        status <> 'COMPLETED' OR completed_at IS NOT NULL
    );

-- Truy vấn của tiến trình nền: đơn đã giao, chờ hết hạn đổi trả.
CREATE INDEX IF NOT EXISTS fulfillment_order_awaiting_completion_idx
    ON fulfillment_order (delivered_at)
    WHERE status = 'DELIVERED';

-- Tra cứu theo mã vận đơn: webhook của đối tác vận chuyển gửi về mã này,
-- không gửi mã đơn của ta.
CREATE INDEX IF NOT EXISTS fulfillment_order_tracking_idx
    ON fulfillment_order (tracking_number)
    WHERE tracking_number <> '';
