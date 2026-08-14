-- Đảo migration 000019.
--
-- LƯU Ý: đảo migration này XÓA toàn bộ dữ liệu hành vi — thứ KHÔNG TẠO
-- NGƯỢC ĐƯỢC (quy tắc 6). Không có cách nào dựng lại "tháng trước có bao
-- nhiêu người xem sản phẩm này mà không mua".
--
-- Chỉ chạy ở môi trường phát triển.

DROP TABLE IF EXISTS metric_snapshot;
DROP TABLE IF EXISTS event_log;
