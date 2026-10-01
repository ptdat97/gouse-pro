-- order_line chở thêm MÃ NỘI DUNG của nguồn quy công.
--
-- # Chuỗi dừng ở giữa, lần thứ hai
--
-- Migration 000057 nối chuỗi quy công giỏ→phiên→đơn, nhưng chỉ cho CREATOR:
--
--	cart_item.source_content_id       migration 000009   ✓
--	checkout_line.source_content_id   migration 000057   ✓
--	order_line                        KHÔNG có cột nội dung
--
-- Nên `attribution.content_id` của module affiliate LUÔN RỖNG, và câu hỏi
-- *"video nào bán được hàng"* chưa trả lời được — dù `cart_item` đã ghi
-- `source_content_id` từ tháng 8. Xem P3-84.
--
-- # Vì sao kernel ĐƯỢC chở cột này
--
-- ADR-0022 cho phép kernel CHỞ ĐỊNH DANH, và cấm nó chở Ý NGHĨA. Cột này là
-- một MÃ: kernel không tra nó, không biết nội dung là gì, không biết nội
-- dung ấy thuộc creator nào.
--
--	kernel ĐƯỢC chở     attributed_creator_id, attributed_content_id
--	kernel KHÔNG chở    tỷ lệ hoa hồng, bên chịu chi phí, mô hình quy công
--
-- Đối chiếu: `creator_commission_rate` từng ở bảng này và đã bị bỏ ở
-- migration 000059 — nó là một TỶ LỆ TIỀN, tức ý nghĩa kinh doanh, và nó
-- thuộc bảng `attribution` của affiliate (P3-83).
--
-- # Vì sao không chờ module `creator` (Phase 2)
--
-- Đường "affiliate tự có bảng `click` rồi tra nội dung từ đó" tốn 0 dòng
-- kernel và là đường ADR-0021 ưa về nguyên tắc. Nó bị bỏ vì **dữ liệu nội
-- dung không tạo ngược được**: không ghi hôm nay thì Phase 3 khởi động với
-- lịch sử trống — đúng lý do `search.no_result` được ghi từ MVP dù chưa ai
-- dùng.
ALTER TABLE order_line
    ADD COLUMN attributed_content_id TEXT NOT NULL DEFAULT '';

-- Chỉ mục CHỈ cho dòng CÓ nội dung.
--
-- Phần lớn dòng đơn không đến từ nội dung creator, nên một chỉ mục đầy đủ
-- sẽ phần lớn là chuỗi rỗng. Cùng khuôn `order_line_creator_idx` của
-- migration 000008.
CREATE INDEX order_line_content_idx
    ON order_line (attributed_content_id, created_at DESC)
    WHERE attributed_content_id <> '';
