-- Trả lại ràng buộc cũ (cho phép OUT_OF_STOCK).
--
-- KHÔNG khôi phục dữ liệu: bản lên đã đổi OUT_OF_STOCK thành ACTIVE và
-- không lưu lại giá trị cũ ở đâu cả. Đó là chủ ý — giá trị đó chưa bao giờ
-- được ghi, nên không có gì để mất; giữ một bảng lưu vết cho 0 bản ghi là
-- phức tạp vô ích.

ALTER TABLE offer DROP CONSTRAINT offer_status_check;

ALTER TABLE offer ADD CONSTRAINT offer_status_check
    CHECK (status IN ('DRAFT', 'ACTIVE', 'OUT_OF_STOCK', 'SUSPENDED', 'ARCHIVED'));
