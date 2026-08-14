-- Đảo migration 000018.
--
-- Thứ tự ngược với lúc tạo: bảng con trước, bảng cha sau.
--
-- LƯU Ý: đảo migration này XÓA lịch sử sử dụng mã giảm giá — thứ dùng để
-- tra khi khách khiếu nại "tôi đã dùng mã rồi". Chỉ chạy ở môi trường
-- phát triển.

DROP TABLE IF EXISTS coupon_usage;
DROP TABLE IF EXISTS coupon;
DROP TABLE IF EXISTS promotion;
