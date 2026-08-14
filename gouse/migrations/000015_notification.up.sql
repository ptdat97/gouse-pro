-- Module: notification — gửi thông báo và GHI LOG mọi lần gửi.
--
-- RÀNG BUỘC KIẾN TRÚC QUAN TRỌNG NHẤT (notification.md quy tắc 1):
--
--     Module này KHÔNG GỌI bất kỳ module nghiệp vụ nào.
--
-- Nếu nó phải gọi `order` để lấy tên sản phẩm và `customer` để lấy email,
-- nó phụ thuộc toàn hệ thống — và một module lỗi sẽ làm hỏng việc gửi mọi
-- loại thông báo, kể cả mã OTP.
--
-- Hệ quả cho thiết kế: payload event phải chứa ĐỦ thông tin. Đó là lý do
-- `checkout.completed` mang theo email và tên sản phẩm, còn
-- `fulfillment.progress_changed` mang theo mã vận đơn.

CREATE TABLE notification_log (
    -- BIGSERIAL: đây là bảng ghi nhiều, và không thực thể nào tham chiếu
    -- tới một dòng cụ thể.
    id BIGSERIAL PRIMARY KEY,

    -- event_id là định danh của SỰ KIỆN sinh ra thông báo này.
    --
    -- Cùng với recipient và template, nó tạo nên khóa CHỐNG GỬI TRÙNG:
    -- event có thể được phát lại (mô hình at-least-once), và gửi hai email
    -- xác nhận cho một đơn làm khách tưởng bị tính tiền hai lần.
    event_id TEXT NOT NULL DEFAULT '',

    channel TEXT NOT NULL CHECK (channel IN ('EMAIL', 'SMS', 'PUSH', 'IN_APP')),

    -- category quyết định có được gửi hay không:
    --
    --   TRANSACTIONAL  xác nhận đơn, giao hàng, hoàn tiền
    --                  → KHÔNG cần đồng ý marketing, LUÔN gửi
    --                  → khách không tắt được (là thông tin thiết yếu)
    --
    --   MARKETING      khuyến mãi, sản phẩm mới
    --                  → BẮT BUỘC có đồng ý
    --
    -- Nhầm lẫn hai loại này là vi phạm pháp luật ở nhiều thị trường.
    category TEXT NOT NULL CHECK (category IN ('TRANSACTIONAL', 'MARKETING', 'SOCIAL')),

    -- template là mã mẫu, ví dụ "order_confirmed", "order_shipped".
    template TEXT NOT NULL CHECK (length(trim(template)) > 0),

    -- recipient là địa chỉ nhận: email, số điện thoại, hoặc device token.
    --
    -- Lưu NGUYÊN VĂN chứ không băm: bộ phận hỗ trợ cần trả lời được "chúng
    -- tôi đã gửi email tới đâu" khi khách nói không nhận được.
    recipient TEXT NOT NULL CHECK (length(trim(recipient)) > 0),

    -- Người nhận, nếu là khách đã đăng ký. Rỗng với khách vãng lai.
    user_id TEXT NOT NULL DEFAULT '',

    subject TEXT NOT NULL DEFAULT '',
    body    TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL CHECK (status IN (
        'PENDING',   -- chờ gửi
        'SENT',      -- đã giao cho nhà cung cấp
        'FAILED',    -- gửi thất bại
        'SKIPPED'    -- cố ý không gửi (thiếu địa chỉ, không có đồng ý)
    )),

    -- provider_message_id để tra cứu với nhà cung cấp khi có khiếu nại.
    provider_message_id TEXT NOT NULL DEFAULT '',

    -- skip_reason và error là hai chuyện KHÁC NHAU:
    --
    --   skip_reason  quyết định CÓ CHỦ Ý không gửi — không phải sự cố
    --   error        gửi thất bại — cần xem và có thể thử lại
    --
    -- Gộp chung sẽ làm cảnh báo vận hành kêu vì những việc hoàn toàn bình
    -- thường, và rồi không ai đọc cảnh báo nữa.
    skip_reason TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',

    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),

    -- Nguồn gốc, để truy vết: "đơn hàng này đã gửi những email nào".
    reference_type TEXT NOT NULL DEFAULT '',
    reference_id   TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ
);

-- CHỐNG GỬI TRÙNG — bất biến quan trọng nhất của module này.
--
-- Event ở mô hình at-least-once sẽ được phát lại. Không có chỉ mục này,
-- mỗi lần phát lại là một email nữa gửi cho khách — và khách nhận ba email
-- "đơn hàng đã đặt" sẽ gọi tổng đài hỏi mình bị tính tiền mấy lần.
--
-- Khóa gồm cả recipient vì một event có thể sinh nhiều thông báo cho những
-- người khác nhau: khách nhận "đơn đã đặt", seller nhận "có đơn mới".
CREATE UNIQUE INDEX notification_log_dedup_idx
    ON notification_log (event_id, template, recipient)
    WHERE event_id <> '';

-- Lịch sử thông báo của một người dùng.
CREATE INDEX notification_log_user_idx
    ON notification_log (user_id, created_at DESC)
    WHERE user_id <> '';

-- Truy vết: "đơn hàng này đã gửi những gì".
CREATE INDEX notification_log_reference_idx
    ON notification_log (reference_type, reference_id, created_at DESC);

-- Giám sát: thông báo thất bại cần xem.
CREATE INDEX notification_log_failed_idx
    ON notification_log (created_at DESC)
    WHERE status = 'FAILED';

-- ---------------------------------------------------------------------
-- Tùy chọn nhận thông báo.
-- ---------------------------------------------------------------------
--
-- Ở MVP chỉ có email giao dịch, nên bảng này chưa được dùng tới. Tạo sẵn
-- vì cấu trúc đã rõ và việc thêm sau sẽ phải sửa mọi chỗ gọi.
--
-- LƯU Ý PHÁP LÝ: bảng này KHÔNG áp dụng cho TRANSACTIONAL. Khách không
-- được tắt thông báo "đơn đã giao" — đó là thông tin thiết yếu về giao
-- dịch họ đã trả tiền.
CREATE TABLE notification_preference (
    user_id  TEXT NOT NULL,
    channel  TEXT NOT NULL CHECK (channel IN ('EMAIL', 'SMS', 'PUSH', 'IN_APP')),

    -- Chỉ MARKETING và SOCIAL tắt được. TRANSACTIONAL không có ở đây.
    category TEXT NOT NULL CHECK (category IN ('MARKETING', 'SOCIAL')),

    enabled BOOLEAN NOT NULL DEFAULT true,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, channel, category)
);
