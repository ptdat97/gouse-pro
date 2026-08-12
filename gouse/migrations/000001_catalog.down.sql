-- Thứ tự xóa NGƯỢC với thứ tự tạo: bảng phụ thuộc xóa trước bảng được
-- tham chiếu, nếu không khóa ngoại sẽ chặn.
DROP TABLE IF EXISTS size_chart_entry;
DROP TABLE IF EXISTS size_chart;
DROP TABLE IF EXISTS category;
DROP TABLE IF EXISTS collection;
DROP TABLE IF EXISTS brand_authorization;
DROP TABLE IF EXISTS brand;
