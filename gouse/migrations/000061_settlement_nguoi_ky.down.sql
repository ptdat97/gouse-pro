-- Đảo: bỏ hai ràng buộc rồi bỏ hai cột.
--
-- Thứ tự NGƯỢC lúc tạo: ràng buộc tham chiếu cột.
ALTER TABLE settlement
    DROP CONSTRAINT IF EXISTS settlement_da_tra_phai_co_nguoi_ky,
    DROP CONSTRAINT IF EXISTS settlement_xac_nhan_phai_co_nguoi_ky;

ALTER TABLE settlement
    DROP COLUMN IF EXISTS paid_by,
    DROP COLUMN IF EXISTS confirmed_by;
