-- Gỡ theo thứ tự ngược với khóa ngoại: bảng nối trước, bảng gốc sau.
DROP TABLE IF EXISTS fulfillment_order_line;
DROP TABLE IF EXISTS fulfillment_order;
DROP TABLE IF EXISTS order_line_adjustment;
DROP TABLE IF EXISTS order_line;
DROP TABLE IF EXISTS "order";
DROP SEQUENCE IF EXISTS order_number_seq;
