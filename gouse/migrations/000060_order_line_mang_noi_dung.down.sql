-- Đảo: bỏ cột nội dung và chỉ mục của nó.
--
-- Chỉ mục bị bỏ theo cột, nhưng ghi tường minh để lùi rồi tiến lại cho đúng
-- cùng lược đồ.
DROP INDEX IF EXISTS order_line_content_idx;

ALTER TABLE order_line
    DROP COLUMN IF EXISTS attributed_content_id;
