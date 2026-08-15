-- Gỡ bản chụp tên seller.
--
-- An toàn để gỡ: cột là bộ nhớ đệm hiển thị, không có dữ liệu nào chỉ tồn
-- tại ở đây. Lần đồng bộ giỏ kế tiếp dựng lại từ module seller.
ALTER TABLE cart_item
    DROP COLUMN seller_name;
