-- Phiên thanh toán MANG THEO nguồn quy công từ giỏ sang đơn.
--
-- # Một chuỗi có hai đầu mà không có giữa
--
-- Hạ tầng quy công đã tồn tại ở CẢ HAI ĐẦU từ lâu:
--
--     cart_item.source_content_id      migration 000009
--     cart_item.source_creator_id      migration 000009 (có cả index)
--     order_line.attributed_creator_id migration 000008 (có cả index)
--     order_line.creator_commission_rate
--
-- Và `order.PlaceOrderLineInput` đã có sẵn hai trường `AttributedCreatorID`,
-- `CreatorCommissionRate`.
--
-- Nhưng KHÔNG gì nối hai đầu ấy. `checkout.domain.Line` không mang trường
-- quy công, nên khi phiên thanh toán dựng đơn hàng, thông tin creator biến
-- mất ở chính giữa chuỗi.
--
-- Hệ quả: mọi cột và index nói trên chưa bao giờ có dữ liệu khác rỗng. Một
-- hạ tầng hoàn chỉnh ở hai đầu và trống ở giữa — dạng lỗi hay gặp nhất của
-- dự án này, lần này trải dài qua ba module.
--
-- Tìm ra ngày 25/09/2026 khi dựng `affiliate` làm PHÉP THỬ cho ADR-0021:
-- "nếu ngày mai cần xây Affiliate, phải sửa Core bao nhiêu?". Câu trả lời
-- là đây — và nó là khoản sửa MỘT LẦN: sau khi chuỗi nối xong, mọi bên
-- dùng quy công sau này (creator, livestream, campaign) không cần sửa core
-- thêm dòng nào.
--
-- # Vì sao KHÔNG mang theo tỷ lệ hoa hồng creator
--
-- Bản đầu của migration này có thêm `creator_commission_rate`, cho cân với
-- `commission_rate` của nhà bán ngay bên trên. Bỏ đi, vì tài liệu affiliate
-- mục 7 đã quyết: bảng `attribution` của CHÍNH NÓ đóng băng tỷ lệ
-- (`commission_rate INT NOT NULL -- basis points, ĐÓNG BĂNG`).
--
-- Thêm một cột thứ hai cho cùng con số là tạo hai nguồn sự thật, và cột ở
-- kernel sẽ KHÔNG AI GHI — đúng dạng lỗi hay gặp nhất của dự án này.
--
-- ADR-0021 cho phép đúng thay đổi còn lại: "thêm TRƯỜNG quy công vào
-- primitive, nếu nó chỉ là THAM CHIẾU". Hai cột dưới đây là hai cái mã.
-- Không một dòng logic affiliate nào đi vào kernel.

ALTER TABLE checkout_line
    ADD COLUMN source_content_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_creator_id TEXT NOT NULL DEFAULT '';

-- Index CHỈ trên dòng CÓ quy công.
--
-- Phần lớn dòng không đến từ nội dung creator, nên một index đầy đủ sẽ gần
-- như toàn bản ghi rỗng. Index từng phần giữ cùng khuôn với
-- `cart_item.source_content_id` và `order_line.attributed_creator_id`.
CREATE INDEX checkout_line_creator_idx
    ON checkout_line (source_creator_id)
    WHERE source_creator_id <> '';
