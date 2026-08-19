-- Địa chỉ giao trên đơn thực hiện.
--
-- # Vì sao seller cần
--
-- Seller biết NHẶT GÌ (migration 000024) nhưng không biết GỬI ĐI ĐÂU. Không
-- có địa chỉ thì họ không in được phiếu giao hàng — luồng 6 chạy đúng về
-- mặt dữ liệu nhưng không giao được hàng thật.
--
-- Họ KHÔNG được tra đơn hàng gốc: ở đó có cả dòng hàng của seller khác,
-- email khách và tổng tiền đơn.
--
-- # KHÔNG có email khách
--
-- Chỉ những trường CẦN cho việc giao hàng: người nhận, số điện thoại (gọi
-- trước khi giao), và địa chỉ. Email không giúp giao hàng, nên nó không
-- được đi tới seller.
--
-- Cột notify_email đã có sẵn trên bảng này phục vụ module notification —
-- nó KHÔNG được lộ ra API của seller.
--
-- # SAO CHÉP, không tham chiếu
--
-- Cùng lý do order sao chép địa chỉ từ sổ địa chỉ: đây là nơi hàng ĐÃ được
-- gửi tới. Khách sửa sổ địa chỉ sau đó không được làm đổi phiếu giao hàng
-- đã in.
ALTER TABLE fulfillment_order
    ADD COLUMN ship_recipient_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN ship_phone          TEXT NOT NULL DEFAULT '',
    ADD COLUMN ship_street         TEXT NOT NULL DEFAULT '',
    ADD COLUMN ship_ward           TEXT NOT NULL DEFAULT '',
    ADD COLUMN ship_district       TEXT NOT NULL DEFAULT '',
    ADD COLUMN ship_province       TEXT NOT NULL DEFAULT '',
    ADD COLUMN ship_country_code   TEXT NOT NULL DEFAULT 'VN';
