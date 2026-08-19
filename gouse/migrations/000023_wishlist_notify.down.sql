-- Gỡ cờ đăng ký nhận thông báo.
--
-- CẢNH BÁO: đây là dữ liệu nghiệp vụ, không phải bộ nhớ đệm. Gỡ nó là mất
-- danh sách khách đang chờ hàng về, và không nguồn nào dựng lại được.
ALTER TABLE wishlist_item
    DROP COLUMN notify_when_available;
