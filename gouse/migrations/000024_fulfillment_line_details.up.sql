-- Thông tin nhặt hàng trên dòng đơn thực hiện.
--
-- # Vì sao seller cần những cột này
--
-- Bảng trước đây chỉ có hai khóa: (fulfillment_order_id, order_line_id).
-- Seller mở đơn ra và thấy một danh sách mã — không biết phải nhặt gì.
--
-- Với thời trang, TÊN SẢN PHẨM KHÔNG ĐỦ: cùng một chiếc áo có năm size và
-- ba màu nằm ở năm ô kệ khác nhau. Thiếu `variant_description` thì seller
-- phải mở đơn hàng gốc — mà quy tắc bảo mật KHÔNG cho họ xem đơn gốc (họ sẽ
-- thấy cả hàng của seller khác, email khách, tổng tiền đơn).
--
-- # Vì sao SAO CHÉP thay vì tham chiếu order_line
--
-- Cùng lý do order_line sao chép từ offer: đây là ẢNH CHỤP tại thời điểm
-- tách đơn. Seller đối soát phần của mình bằng con số họ đã thấy lúc giao
-- hàng, không phải con số hiện tại của một bảng khác.
--
-- Dữ liệu đến từ payload event `checkout.completed` — nó đã mang sẵn mọi
-- trường này, bên nhận chỉ chưa đọc.
--
-- # Vì sao có CẢ unit_price lẫn line_total
--
-- Chia line_total cho quantity là phép chia số nguyên và nó làm tròn sai
-- với những giá không chia hết. Lưu cả hai thì không ai phải chia.
ALTER TABLE fulfillment_order_line
    ADD COLUMN sku_id              TEXT   NOT NULL DEFAULT '',
    ADD COLUMN product_name        TEXT   NOT NULL DEFAULT '',
    ADD COLUMN variant_description TEXT   NOT NULL DEFAULT '',
    ADD COLUMN quantity            INT    NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    ADD COLUMN unit_price          BIGINT NOT NULL DEFAULT 0 CHECK (unit_price >= 0),
    ADD COLUMN line_total          BIGINT NOT NULL DEFAULT 0 CHECK (line_total >= 0);
