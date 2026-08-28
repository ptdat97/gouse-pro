-- Kết quả kiểm định hàng hoàn, theo TỪNG DÒNG.
--
-- VÌ SAO CẦN: hàng hoàn về kho nằm ở trạng thái Returned và KHÔNG BAO GIỜ
-- tự động vào Available — bán lại hàng hỏng cho khách khác gây thiệt hại
-- uy tín lớn hơn nhiều lần giá trị món hàng
-- (docs/07-workflows/return.md mục 4).
--
-- Nhưng không có bước ghi kết quả kiểm định thì hàng nằm CHẾT vĩnh viễn:
-- nhà bán mất cả hàng lẫn tiền, và con số tồn kho ngày càng xa thực tế.
--
-- Theo từng DÒNG chứ không phải cả yêu cầu: một yêu cầu trả hai món hoàn
-- toàn có thể một món bán lại được và một món hỏng.
ALTER TABLE return_line
    ADD COLUMN inspection TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (inspection IN ('PENDING', 'PASSED', 'FAILED')),
    ADD COLUMN inspection_note TEXT NOT NULL DEFAULT '',
    ADD COLUMN inspected_at TIMESTAMPTZ;

-- Loại hàng thì PHẢI nêu lý do.
--
-- Lý do loại là đầu vào cho việc làm việc với nhà cung cấp và cho việc
-- quyết ai chịu chi phí. "Hỏng" không có mô tả thì không làm được gì với nó.
ALTER TABLE return_line
    ADD CONSTRAINT return_line_fail_needs_note CHECK (
        inspection <> 'FAILED' OR length(trim(inspection_note)) > 0
    );

-- Tìm dòng chờ kiểm định — chỉ báo giám sát: con số tăng dần nghĩa là
-- hàng đang dồn trong kho mà không ai xử lý.
CREATE INDEX return_line_cho_kiem_dinh
    ON return_line (return_request_id)
 WHERE inspection = 'PENDING';
