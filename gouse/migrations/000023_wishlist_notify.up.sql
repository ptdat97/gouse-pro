-- Đăng ký nhận thông báo khi có hàng.
--
-- # Vì sao phải LƯU
--
-- Đặc tả (api/paths/account.yaml, addWishlistItem) cho khách bật cờ này.
-- Nhận rồi vứt đi thì nút bấm là nút giả: khách tưởng đã đăng ký và chờ
-- một thông báo không bao giờ tới.
--
-- # Vì sao nó đáng lưu về mặt kinh doanh
--
-- Đây là thước đo NHU CẦU KHÔNG ĐƯỢC ĐÁP ỨNG — khác hẳn lượt xem hay lượt
-- thích. Khách để lại lời hứa "có hàng là tôi mua", nói chính xác nên sản
-- xuất lại mã nào và bao nhiêu (docs/04-modules/supplychain.md).
--
-- Mặc định FALSE: thêm vào yêu thích KHÔNG mặc nhiên là đồng ý nhận
-- thông báo — đó là hai ý định khác nhau.
ALTER TABLE wishlist_item
    ADD COLUMN notify_when_available BOOLEAN NOT NULL DEFAULT FALSE;
