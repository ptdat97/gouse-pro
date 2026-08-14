-- Trả lại trạng thái trước khi chuyển quyền sở hữu.
--
-- MIGRATION NÀY THẤT BẠI KHI ĐÃ CÓ DỮ LIỆU MỚI, và đó là hành vi ĐÚNG:
--
--   1. Đơn ở trạng thái ALLOCATED, PICKING, HANDED_OVER, IN_TRANSIT,
--      DELIVERY_FAILED hoặc COMPLETED vi phạm CHECK cũ
--   2. Đơn thực hiện trỏ tới đơn hàng không còn tồn tại vi phạm khóa ngoại
--
-- Cả hai đều báo "dữ liệu đã đi xa hơn schema cũ". Ép lùi bằng cách xóa
-- ràng buộc sẽ để lại trạng thái mà code cũ không hiểu — im lặng và tệ hơn
-- nhiều so với một lỗi migration.
--
-- Cách lùi khi THỰC SỰ cần: đưa các đơn về trạng thái cũ trước, rồi mới
-- chạy migration này.
--
--   UPDATE fulfillment_order SET status = 'SHIPPED'
--    WHERE status IN ('HANDED_OVER','IN_TRANSIT','DELIVERY_FAILED');
--   UPDATE fulfillment_order SET status = 'DELIVERED'
--    WHERE status = 'COMPLETED';
--   UPDATE fulfillment_order SET status = 'PENDING'
--    WHERE status IN ('ALLOCATED','PICKING');

DROP INDEX IF EXISTS fulfillment_order_tracking_idx;
DROP INDEX IF EXISTS fulfillment_order_awaiting_completion_idx;

ALTER TABLE fulfillment_order
    DROP CONSTRAINT IF EXISTS fulfillment_order_completed_has_time;

ALTER TABLE fulfillment_order
    DROP CONSTRAINT IF EXISTS fulfillment_order_failure_needs_reason;

ALTER TABLE fulfillment_order
    DROP CONSTRAINT IF EXISTS fulfillment_order_status_check;

ALTER TABLE fulfillment_order
    ADD CONSTRAINT fulfillment_order_status_check CHECK (status IN (
        'PENDING', 'CONFIRMED', 'PACKED', 'SHIPPED', 'DELIVERED', 'CANCELLED'
    ));

ALTER TABLE fulfillment_order
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS estimated_delivery_date,
    DROP COLUMN IF EXISTS tracking_number,
    DROP COLUMN IF EXISTS shipping_provider,
    DROP COLUMN IF EXISTS shipping_method,
    DROP COLUMN IF EXISTS fulfillment_type,
    DROP COLUMN IF EXISTS stock_location_id;

ALTER TABLE fulfillment_order
    ADD CONSTRAINT fulfillment_order_order_id_fkey
        FOREIGN KEY (order_id) REFERENCES "order" (id);

ALTER TABLE fulfillment_order_line
    ADD CONSTRAINT fulfillment_order_line_order_line_id_fkey
        FOREIGN KEY (order_line_id) REFERENCES order_line (id);
