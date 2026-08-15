-- Tên seller trong giỏ hàng.
--
-- # Vì sao cần cột này
--
-- Đặc tả (api/components/schemas.yaml#/Cart) trả giỏ NHÓM THEO SELLER, và
-- SellerRef bắt buộc có `name`. Không có cột này thì tầng HTTP phải gọi
-- module seller mỗi lần dựng response — thêm một lượt đi-về cho mọi thao
-- tác giỏ hàng, cho một chuỗi hiển thị.
--
-- # Đây là BẢN CHỤP, giống product_name ngay phía trên
--
-- Không phải nguồn sự thật: seller đổi tên thì lần đồng bộ kế tiếp cập
-- nhật lại. Khác hẳn order_line, nơi tên được ĐÓNG BĂNG vĩnh viễn vì đơn
-- hàng là hợp đồng.
ALTER TABLE cart_item
    ADD COLUMN seller_name TEXT NOT NULL DEFAULT '';
