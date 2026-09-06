-- Bỏ cột phương thức thanh toán.
--
-- MẤT DỮ LIỆU: lựa chọn của khách trên mọi đơn đã đặt biến mất và không
-- khôi phục được từ đâu khác — không có bút toán nào ghi lại nó, vì COD
-- không sinh bút toán nào lúc đặt hàng.
ALTER TABLE "order" DROP COLUMN payment_method;
