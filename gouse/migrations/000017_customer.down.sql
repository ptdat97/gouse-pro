-- Đảo migration 000017.
--
-- Thứ tự ngược với lúc tạo: bảng con trước, bảng cha sau.
--
-- LƯU Ý: đảo migration này XÓA nhật ký đồng ý (customer_consent) — thứ
-- có nghĩa vụ pháp lý phải chứng minh được. Chỉ chạy ở môi trường phát
-- triển. Trên môi trường thật, hãy sao lưu customer_consent trước.

DROP TABLE IF EXISTS customer_merge_log;
DROP TABLE IF EXISTS wishlist_item;
DROP TABLE IF EXISTS wishlist;
DROP TABLE IF EXISTS customer_consent;
DROP TABLE IF EXISTS customer_address;
DROP TABLE IF EXISTS customer;
