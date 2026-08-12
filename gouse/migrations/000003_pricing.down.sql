-- Trigger bị xóa theo bảng, nhưng hàm thì không — phải xóa tường minh.
DROP TABLE IF EXISTS price_history;
DROP FUNCTION IF EXISTS price_history_immutable();
DROP TABLE IF EXISTS price_constraint;
DROP TABLE IF EXISTS price;
