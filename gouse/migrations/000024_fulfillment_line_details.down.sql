-- Gỡ thông tin nhặt hàng.
--
-- CẢNH BÁO: đây là ẢNH CHỤP tại thời điểm tách đơn, không phải bộ nhớ đệm.
-- Gỡ đi là mất con số seller đã thấy lúc giao hàng — order_line hiện tại có
-- thể đã khác (hủy một phần, điều chỉnh).
ALTER TABLE fulfillment_order_line
    DROP COLUMN sku_id,
    DROP COLUMN product_name,
    DROP COLUMN variant_description,
    DROP COLUMN quantity,
    DROP COLUMN unit_price,
    DROP COLUMN line_total;
