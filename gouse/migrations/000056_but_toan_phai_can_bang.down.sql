-- Gỡ hàng rào cân bằng sổ cái.
--
-- Xóa trigger TRƯỚC hàm: hàm còn trigger phụ thuộc thì `DROP FUNCTION` bị
-- từ chối.
DROP TRIGGER IF EXISTS ledger_entry_phai_can_bang ON ledger_entry;
DROP FUNCTION IF EXISTS kiem_but_toan_can_bang();
