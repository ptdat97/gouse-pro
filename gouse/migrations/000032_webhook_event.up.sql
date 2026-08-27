-- Nhật ký webhook đến.
--
-- # Vì sao ghi MỌI webhook, kể cả loại không xử lý
--
-- Yêu cầu số 4 của api/paths/webhooks.yaml. Khi có tranh chấp — khách nói
-- đã trả tiền, hãng vận chuyển nói đã giao — thứ duy nhất trả lời được là
-- bản ghi nguyên văn những gì họ đã gửi và lúc nào.
--
-- Loại không xử lý cũng phải ghi: nó là bằng chứng nhà cung cấp CÓ gửi,
-- và là cách phát hiện ta đang bỏ sót một loại sự kiện mới.
--
-- # Chỉ mục UNIQUE là cơ chế idempotency
--
-- Nhà cung cấp SẼ gửi trùng — vì timeout, vì thử lại, vì lỗi phía họ. Kiểm
-- tra ở tầng ứng dụng không đủ: hai webhook trùng tới cùng lúc thì cả hai
-- cùng thấy "chưa xử lý". Chỉ ràng buộc ở database mới nằm cùng giao dịch
-- với việc ghi.
CREATE TABLE webhook_event (
    id TEXT PRIMARY KEY CHECK (id LIKE 'whk\_%' AND length(id) = 30),

    -- provider là định danh nhà cung cấp: "ghn", "vnpay"… DỮ LIỆU, không
    -- phải mã nguồn — đối tác đổi thường xuyên.
    provider TEXT NOT NULL CHECK (length(trim(provider)) > 0),

    -- provider_event_id là mã sự kiện BÊN HỌ đặt.
    provider_event_id TEXT NOT NULL CHECK (length(trim(provider_event_id)) > 0),

    event_type TEXT NOT NULL DEFAULT '',

    -- payload nguyên văn. Giữ để đối chiếu khi có tranh chấp.
    payload JSONB NOT NULL,

    -- processed_at rỗng nghĩa là ĐÃ NHẬN nhưng CHƯA xử lý xong.
    --
    -- Phân biệt hai việc là quan trọng: nhận được thì trả 200 ngay để nhà
    -- cung cấp thôi gửi lại, còn xử lý có thể hỏng và cần thử lại. Gộp
    -- chúng làm một nghĩa là mỗi lần xử lý hỏng lại kéo theo một lượt gửi
    -- trùng từ phía họ.
    processed_at TIMESTAMPTZ,

    -- last_error giữ lý do lần xử lý gần nhất thất bại.
    last_error TEXT NOT NULL DEFAULT '',

    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (provider, provider_event_id)
);

-- Tìm webhook đã nhận mà chưa xử lý xong — đầu vào của job thử lại.
CREATE INDEX webhook_event_chua_xu_ly
    ON webhook_event (received_at)
 WHERE processed_at IS NULL;
