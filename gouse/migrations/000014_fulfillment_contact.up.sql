-- Thông tin liên hệ trên đơn thực hiện.
--
-- VÌ SAO NHÂN BẢN DỮ LIỆU KHÁCH XUỐNG ĐÂY:
--
-- Module `notification` KHÔNG ĐƯỢC gọi bất kỳ module nghiệp vụ nào
-- (notification.md quy tắc 1). Nếu nó phải gọi `order` hay `customer` để
-- lấy email, nó phụ thuộc toàn hệ thống — và một module lỗi sẽ làm hỏng
-- việc gửi email cho MỌI loại thông báo.
--
-- Hệ quả: event `fulfillment.progress_changed` phải mang theo email. Mà
-- fulfillment chỉ biết những gì có trong bảng của nó.
--
-- BA LỰA CHỌN, và lý do chọn cái thứ ba:
--
--   1. notification gọi order lấy email
--      → vi phạm quy tắc 1, notification phụ thuộc toàn hệ thống
--
--   2. fulfillment gọi order khi phát event
--      → tạo phụ thuộc order ← fulfillment, và đã có chiều ngược lại
--        (order nghe event từ fulfillment) → phụ thuộc VÒNG
--
--   3. Lưu email vào đơn thực hiện lúc tách  ← ĐÃ CHỌN
--      → nhân bản dữ liệu, nhưng KHÔNG tạo phụ thuộc nào
--
-- ĐÂY LÀ NHÂN BẢN CÓ CHỦ Ý, cùng loại với việc đóng băng giá ở order_line:
-- dữ liệu được sao chép tại thời điểm giao dịch và không đổi sau đó.
--
-- Khách đổi email trong hồ sơ SAU KHI đặt hàng thì thông báo về đơn cũ vẫn
-- gửi tới địa chỉ họ đã dùng lúc mua. Đó là hành vi ĐÚNG — giống như hóa
-- đơn giấy đã in không đổi theo sổ địa chỉ.

ALTER TABLE fulfillment_order
    ADD COLUMN IF NOT EXISTS customer_id TEXT NOT NULL DEFAULT '',

    -- notify_email là địa chỉ nhận thông báo về ĐƠN NÀY.
    --
    -- Với khách đã đăng ký, đây là email lúc đặt hàng. Với khách vãng lai,
    -- đây là email họ nhập ở màn hình thanh toán — thứ duy nhất liên hệ được.
    ADD COLUMN IF NOT EXISTS notify_email TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS notify_phone TEXT NOT NULL DEFAULT '';

-- KHÔNG có ràng buộc NOT NULL hay CHECK bắt buộc phải có email.
--
-- Lý do: đơn thực hiện tạo từ event, và event cũ (trước migration này)
-- không mang email. Bắt buộc sẽ làm hỏng việc tách đơn cho những đơn đó —
-- và không giao được hàng là hậu quả nặng hơn nhiều so với không gửi được
-- email.
--
-- Module notification tự bỏ qua khi thiếu địa chỉ, có ghi log.
