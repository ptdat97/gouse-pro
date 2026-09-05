-- Bỏ OUT_OF_STOCK khỏi trạng thái offer (P3-23).
--
-- Hết hàng là sự thật của TỒN KHO, không phải một trạng thái của lời chào
-- bán. Offer hết hàng vẫn ACTIVE và vẫn hiển thị (để khách biết có tổ hợp
-- màu/size đó); `is_sellable`, tính lúc đọc từ offer + tồn kho, mới là thứ
-- tắt nút mua.
--
-- VÌ SAO PHẢI SIẾT RÀNG BUỘC, KHÔNG CHỈ SỬA CODE
--
-- Ràng buộc cũ vẫn cho ghi 'OUT_OF_STOCK'. Một bản ghi như vậy lọt vào —
-- do SQL thủ công, do script nhập liệu, do một nhánh code cũ — thì domain
-- không còn trạng thái tương ứng: offer đó vừa không bán được vừa không
-- hiển thị, tức âm thầm biến mất khỏi trang sản phẩm mà không có lỗi nào.
--
-- Siết lại biến hỏng-âm-thầm-lúc-đọc thành lỗi-ồn-ào-lúc-ghi.

-- Dữ liệu hiện tại: 0 bản ghi (giá trị này chưa bao giờ được ghi, vì
-- `MarkOutOfStock` không có bên gọi). Câu lệnh vẫn để đây cho môi trường
-- nào lỡ có: ACTIVE là ánh xạ đúng, vì offer vẫn đang được chào bán và
-- tồn kho mới quyết định mua được hay không.
UPDATE offer
   SET status = 'ACTIVE', updated_at = now()
 WHERE status = 'OUT_OF_STOCK';

ALTER TABLE offer DROP CONSTRAINT offer_status_check;

ALTER TABLE offer ADD CONSTRAINT offer_status_check
    CHECK (status IN ('DRAFT', 'ACTIVE', 'SUSPENDED', 'ARCHIVED'));
