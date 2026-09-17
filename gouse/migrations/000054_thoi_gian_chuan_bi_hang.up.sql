-- Thời gian CHUẨN BỊ HÀNG đi theo món, từ giỏ vào phiên thanh toán.
--
-- VÌ SAO CẦN
--
-- docs/04-modules/checkout.md mục 7 quyết định: "hiển thị thời gian giao
-- RIÊNG cho từng nhóm hàng, không gộp thành một con số. Khách cần biết
-- món nào đến trước." Trường `shipping_groups` của đặc tả tồn tại cho
-- đúng câu đó.
--
-- Nhưng `fulfillment.EstimateShipping` chỉ trả THỜI GIAN VẬN CHUYỂN, và
-- nó ghi rõ là không gồm thời gian nhà bán chuẩn bị hàng — hai con số
-- thuộc hai bên khác nhau. Trong một phiên thanh toán, mọi nhóm dùng CÙNG
-- phương thức giao nên cùng số ngày vận chuyển. Thứ DUY NHẤT làm các nhóm
-- khác ngày nhau là thời gian chuẩn bị của từng nhà bán.
--
-- Thiếu cột này thì `shipping_groups` hiển thị cùng một ngày cho mọi
-- nhóm, tức trả lời "món nào đến trước" bằng "tất cả cùng lúc" — sai, và
-- sai theo kiểu nhìn vào không biết là sai.
--
-- VÌ SAO ĐÓNG BĂNG THEO MÓN, KHÔNG TRA LÚC ĐỌC
--
-- Cùng lý do với giá và tỷ lệ hoa hồng: `cart` là ranh giới đọc offer,
-- `checkout` đóng băng thứ cart đưa sang. Nhà bán đổi thời gian chuẩn bị
-- giữa lúc khách đang thanh toán KHÔNG được đổi lời hứa đã hiện trên màn
-- hình.
--
-- Mặc định 0 cho dòng cũ: không có nghĩa là "giao ngay" mà là "không
-- biết", và tầng ứng dụng rơi về `marketplace.default_handling_hours`.

ALTER TABLE cart_item
    ADD COLUMN handling_time_hours integer NOT NULL DEFAULT 0;

ALTER TABLE checkout_line
    ADD COLUMN handling_time_hours integer NOT NULL DEFAULT 0;

-- Tên nhà bán để dựng `SellerRef` của mỗi nhóm.
--
-- `cart_item` đã có cột cùng tên và cùng lý do; đây là chép tiếp sang
-- phiên thanh toán, không phải khái niệm mới.
ALTER TABLE checkout_line
    ADD COLUMN seller_name text NOT NULL DEFAULT '';
