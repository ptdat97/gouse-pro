-- Một phiên thanh toán giữ MỖI bản ghi tồn kho nhiều nhất MỘT lần.
--
-- VÌ SAO Ở TẦNG DỮ LIỆU: tầng ứng dụng đã có bảo vệ (một giỏ chỉ có một
-- phiên đang chạy), nhưng đó là kiểm-rồi-ghi. Hai request song song cùng
-- vượt qua bước kiểm. Ràng buộc UNIQUE là thứ DUY NHẤT không có khe hở
-- giữa kiểm và ghi.
--
-- Kiểm chứng trước khi thêm: gọi Reserve hai lần với cùng checkout_id và
-- cùng inventory_item_id thì CẢ HAI thành công, và số hàng bị khóa gấp
-- đôi số khách thật sự cần. Số thừa treo tới khi hết hạn — với hàng bán
-- chạy đó là cách tự tạo ra tình trạng hết hàng giả.
--
-- CÓ ĐIỀU KIỆN trên status = 'ACTIVE': một phiên hết hạn rồi mở lại, hoặc
-- giữ hàng bị nhả rồi giữ lại, đều là chuyện hợp lệ. Chỉ những lần giữ
-- ĐANG HIỆU LỰC mới không được trùng.
--
-- checkout_id rỗng bị loại trừ: nhập kho và điều chỉnh thủ công không
-- thuộc phiên nào, và chúng được phép có nhiều bản ghi.
CREATE UNIQUE INDEX reservation_active_uniq
    ON reservation (checkout_id, inventory_item_id)
 WHERE status = 'ACTIVE' AND checkout_id <> '';
