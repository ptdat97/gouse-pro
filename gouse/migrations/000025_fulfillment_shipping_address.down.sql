-- Gỡ địa chỉ giao khỏi đơn thực hiện.
--
-- CẢNH BÁO: đây là ẢNH CHỤP nơi hàng đã được gửi tới, không phải bộ nhớ
-- đệm. Đơn hàng gốc có thể đã đổi địa chỉ, nên gỡ đi là mất bằng chứng về
-- nơi seller thật sự đã giao.
ALTER TABLE fulfillment_order
    DROP COLUMN ship_recipient_name,
    DROP COLUMN ship_phone,
    DROP COLUMN ship_street,
    DROP COLUMN ship_ward,
    DROP COLUMN ship_district,
    DROP COLUMN ship_province,
    DROP COLUMN ship_country_code;
