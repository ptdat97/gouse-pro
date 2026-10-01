-- Đảo: trả cột về ĐÚNG định nghĩa cũ của migration 000008.
--
-- Mặc định 0 và CHECK y như cũ, nên lùi rồi tiến lại cho cùng lược đồ. Dữ
-- liệu không mất gì vì cột rỗng ở mọi dòng trước khi bỏ.
ALTER TABLE order_line
    ADD COLUMN creator_commission_rate INT NOT NULL DEFAULT 0
        CHECK (creator_commission_rate BETWEEN 0 AND 10000);
