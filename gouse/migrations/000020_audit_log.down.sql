-- Đảo migration 000020.
--
-- LƯU Ý: đảo migration này XÓA toàn bộ vết kiểm toán — thứ KHÔNG TẠO
-- NGƯỢC ĐƯỢC. Không có cách nào dựng lại "ai đã điều chỉnh sổ cái tháng
-- trước, và vì lý do gì".
--
-- Ở nhiều thị trường, vết kiểm toán còn là nghĩa vụ lưu trữ theo quy định.
--
-- Chỉ chạy ở môi trường phát triển.

DROP TRIGGER IF EXISTS audit_log_no_delete ON audit_log;
DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;
DROP FUNCTION IF EXISTS audit_log_immutable();
DROP TABLE IF EXISTS audit_log;
