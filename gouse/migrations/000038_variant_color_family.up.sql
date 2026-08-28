-- Nhóm màu cho biến thể — bù dữ liệu cũ và thêm chỉ mục lọc.
--
-- VÌ SAO CẦN: khách lọc theo "màu xanh", không lọc theo "Xanh navy đậm".
-- Một sàn thời trang có hàng trăm tên màu do người bán tự đặt, và bộ lọc
-- liệt kê từng tên là bộ lọc không ai dùng.
-- Xem docs/02-domain/value-objects.md mục 3.2.
--
-- Nhóm màu được suy ra TỰ ĐỘNG từ tên màu lúc tạo biến thể
-- (domain.SuyRaNhomMau). Migration này bù cho biến thể đã có.
--
-- # Vì sao bù bằng SQL chứ không gọi lại hàm Go
--
-- Chạy một lần trên dữ liệu có sẵn thì SQL đơn giản hơn nhiều so với viết
-- một công cụ riêng. Bảng ánh xạ dưới đây CỐ Ý ngắn hơn bản Go: nó chỉ cần
-- đúng với dữ liệu đang có, còn dữ liệu mới đi qua hàm Go.
--
-- Tên không khớp từ nào vào OTHER — vẫn duyệt danh mục thấy được, chỉ là
-- không lọc theo màu.
UPDATE variant
   SET attributes = attributes || jsonb_build_object('color_family',
       CASE
           WHEN lower(attributes->>'color') LIKE '%xanh lá%' THEN 'GREEN'
           WHEN lower(attributes->>'color') LIKE '%denim%'   THEN 'BLUE'
           WHEN lower(attributes->>'color') LIKE '%navy%'    THEN 'BLUE'
           WHEN lower(attributes->>'color') LIKE '%xanh%'    THEN 'BLUE'
           WHEN lower(attributes->>'color') LIKE '%trắng%'   THEN 'WHITE'
           WHEN lower(attributes->>'color') LIKE '%đen%'     THEN 'BLACK'
           WHEN lower(attributes->>'color') LIKE '%xám%'     THEN 'GREY'
           WHEN lower(attributes->>'color') LIKE '%đỏ%'      THEN 'RED'
           WHEN lower(attributes->>'color') LIKE '%hồng%'    THEN 'PINK'
           WHEN lower(attributes->>'color') LIKE '%cam%'     THEN 'ORANGE'
           WHEN lower(attributes->>'color') LIKE '%vàng%'    THEN 'YELLOW'
           WHEN lower(attributes->>'color') LIKE '%tím%'     THEN 'PURPLE'
           WHEN lower(attributes->>'color') LIKE '%nâu%'     THEN 'BROWN'
           WHEN lower(attributes->>'color') LIKE '%kem%'     THEN 'BEIGE'
           WHEN lower(attributes->>'color') LIKE '%be%'      THEN 'BEIGE'
           ELSE 'OTHER'
       END)
 WHERE attributes ? 'color'
   AND NOT attributes ? 'color_family';

-- Chỉ mục GIN cho phép lọc theo thuộc tính mà không quét cả bảng.
--
-- Không có nó, mỗi lần khách lọc size là một lần đọc toàn bộ bảng biến
-- thể — và bộ lọc là thao tác được dùng nhiều nhất của trang danh mục.
CREATE INDEX IF NOT EXISTS variant_attributes_gin
    ON variant USING GIN (attributes jsonb_path_ops);
