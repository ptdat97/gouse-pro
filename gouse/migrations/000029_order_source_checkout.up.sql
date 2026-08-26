-- Một phiên thanh toán sinh ra TỐI ĐA MỘT đơn hàng.
--
-- # Vì sao ràng buộc này phải nằm ở database
--
-- Bất biến đã có ba lớp phòng vệ ở tầng ứng dụng và cả ba đều thủng khi
-- hai request chạy chồng nhau:
--
--   Lớp 1 — CompleteCheckout đọc trạng thái phiên rồi mới kiểm. Giữa lúc
--           đọc và lúc ghi COMPLETED có một khoảng trống, và mọi request
--           vào trong khoảng đó đều thấy "phiên chưa hoàn tất".
--   Lớp 2 — order.PlaceOrder idempotent theo khóa idempotency. Khóa bảo
--           vệ trước việc CÙNG một lần bấm bị gửi lặp, không bảo vệ được
--           gì trước hai lần bấm THẬT từ hai tab: hai khóa khác nhau.
--   Lớp 3 — phiên chuyển COMPLETED sau khi đơn đã tạo xong. Quá muộn.
--
-- Kiểm chứng trước khi thêm: 8 lượt POST /checkout/{id}/complete song
-- song với 8 khóa idempotency khác nhau tạo ra 3–5 ĐƠN HÀNG cho cùng một
-- giỏ. Xem internal/app/api_idempotency_test.go.
--
-- Chỉ ở trong database thì việc KIỂM và việc GHI mới nằm cùng một giao
-- dịch — đúng lý do đã ghi trong internal/platform/httpserver/idempotency.go.
--
-- Không có khóa ngoại: order không được phụ thuộc checkout. Cùng cách
-- reservation.checkout_id đã làm ở migration 000004.
ALTER TABLE "order"
    ADD COLUMN source_checkout_id TEXT NOT NULL DEFAULT '';

-- Có ĐIỀU KIỆN: đơn tạo bằng đường quản trị hoặc nhập liệu không đi qua
-- phiên thanh toán nào, và chuỗi rỗng thì không được coi là trùng nhau.
CREATE UNIQUE INDEX order_one_per_checkout
    ON "order" (source_checkout_id)
 WHERE source_checkout_id <> '';
