-- Đảo: bỏ hai cột quy công khỏi `checkout_line`.
--
-- Dữ liệu MẤT khi đảo, và đó là đúng: ba cột này chỉ mang bản chụp trên
-- đường từ giỏ sang đơn. Nguồn sự thật nằm ở `cart_item` (lúc khách thêm)
-- và `order_line` (sau khi đặt) — hai chỗ ấy không bị migration này chạm.
DROP INDEX IF EXISTS checkout_line_creator_idx;

ALTER TABLE checkout_line
    DROP COLUMN IF EXISTS source_creator_id,
    DROP COLUMN IF EXISTS source_content_id;
