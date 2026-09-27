-- Đảo: bỏ hai bảng của module affiliate.
--
-- Thứ tự NGƯỢC với lúc tạo: `commission_record` tham chiếu `attribution`.
DROP TABLE IF EXISTS commission_record;
DROP TABLE IF EXISTS attribution;
