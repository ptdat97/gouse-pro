-- Transactional Outbox — ADR-0006.
--
-- BÀI TOÁN: ghi database thành công nhưng phát event thất bại (hoặc ngược
-- lại) làm hệ thống mất nhất quán.
--
--     Đơn hàng tạo rồi nhưng không ai xử lý:
--       → không chuyển Reserved sang Committed
--       → không ghi sổ tài chính
--       → khách trả tiền nhưng không nhận hàng
--
-- CÁCH GIẢI: ghi event vào bảng này TRONG CÙNG giao dịch với thay đổi
-- nghiệp vụ. Một tiến trình riêng đọc bảng và phát đi sau.
--
--     Giao dịch thành công → event CHẮC CHẮN được phát (sớm hay muộn)
--     Giao dịch thất bại   → event KHÔNG BAO GIỜ được phát
--
-- Đây là mô hình AT-LEAST-ONCE, không phải exactly-once: event có thể phát
-- nhiều lần. Vì vậy mọi bên nhận PHẢI idempotent — đó là yêu cầu bắt buộc,
-- không phải tùy chọn.
--
-- BẢNG NÀY THUỘC platform, KHÔNG thuộc module nào. Mọi module ghi vào đây,
-- và đó là ngoại lệ có chủ ý duy nhất với quy tắc "mỗi bảng thuộc một
-- module" — vì nó là cơ chế truyền tải, không phải dữ liệu nghiệp vụ.

CREATE TABLE event_outbox (
    -- Khóa chính tăng dần: bộ đọc lấy theo thứ tự ghi.
    --
    -- Dùng BIGSERIAL chứ không phải ULID vì đây là hàng đợi, không phải
    -- thực thể nghiệp vụ — không ai tham chiếu tới một dòng outbox.
    id BIGSERIAL PRIMARY KEY,

    -- event_id là định danh NGHIỆP VỤ của event, do bên phát sinh.
    --
    -- Bên nhận dùng nó để bỏ qua event trùng. UNIQUE ở đây chặn việc cùng
    -- một event bị ghi hai lần vào outbox — chuyện xảy ra khi code gọi
    -- publish hai lần trong cùng giao dịch.
    event_id TEXT NOT NULL UNIQUE
        CHECK (event_id LIKE 'evt\_%' AND length(event_id) = 30),

    -- event_type dạng "order.placed", "inventory.reserved".
    --
    -- Chuỗi chứ không phải enum: thêm loại event mới là việc thường xuyên,
    -- và enum ở database buộc phải migration cho mỗi lần thêm.
    event_type TEXT NOT NULL CHECK (length(trim(event_type)) > 0),

    -- event_version cho phép tiến hóa schema mà không phá bên nhận cũ.
    event_version INT NOT NULL DEFAULT 1 CHECK (event_version > 0),

    aggregate_type TEXT NOT NULL CHECK (length(trim(aggregate_type)) > 0),
    aggregate_id   TEXT NOT NULL CHECK (length(trim(aggregate_id)) > 0),

    -- payload chứa ĐỦ thông tin để bên nhận xử lý mà KHÔNG phải gọi ngược
    -- lại bên phát.
    --
    -- Nếu mọi bên nhận đều phải gọi ngược để lấy chi tiết thì event trở
    -- nên vô dụng và tạo đúng thứ ghép nối mà nó sinh ra để tránh.
    payload JSONB NOT NULL,

    -- Truy vết chuỗi nghiệp vụ.
    --
    -- correlation_id: toàn bộ chuỗi từ MỘT hành động của khách
    -- causation_id:   event nào sinh ra event này
    --
    -- Hai trường này là thứ duy nhất cho phép trả lời "vì sao bút toán
    -- này tồn tại" khi có tranh chấp ba tháng sau.
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id   TEXT NOT NULL DEFAULT '',

    occurred_at TIMESTAMPTZ NOT NULL,

    -- published_at NULL nghĩa là CHƯA phát.
    published_at TIMESTAMPTZ,

    -- attempts đếm số lần thử phát.
    --
    -- Vượt ngưỡng thì chuyển sang dead letter: KHÔNG thử lại vô hạn. Một
    -- event hỏng thử lại mãi sẽ chặn hàng đợi và làm mọi event sau nó
    -- không bao giờ được phát.
    attempts   INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT NOT NULL DEFAULT '',

    -- dead_lettered_at đánh dấu event bỏ cuộc, chờ người vận hành.
    dead_lettered_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Đã phát thì không thể đồng thời là dead letter.
    CONSTRAINT event_outbox_not_both CHECK (
        published_at IS NULL OR dead_lettered_at IS NULL
    )
);

-- Truy vấn NÓNG NHẤT của hệ thống outbox: "event nào chưa phát".
--
-- Chỉ mục CÓ ĐIỀU KIỆN nên nó chỉ chứa các dòng chưa xử lý — với bảng có
-- hàng triệu dòng đã phát, chỉ mục này vẫn nhỏ và nằm gọn trong bộ nhớ.
CREATE INDEX event_outbox_unpublished_idx
    ON event_outbox (id)
    WHERE published_at IS NULL AND dead_lettered_at IS NULL;

-- Giám sát: event kẹt lâu nhất chưa phát.
--
-- Độ trễ vượt 60 giây là dấu hiệu bộ đọc đã chết hoặc không theo kịp.
CREATE INDEX event_outbox_lag_idx
    ON event_outbox (occurred_at)
    WHERE published_at IS NULL AND dead_lettered_at IS NULL;

-- Dead letter cần người xem: truy vấn này chạy khi có cảnh báo.
CREATE INDEX event_outbox_dead_idx
    ON event_outbox (dead_lettered_at DESC)
    WHERE dead_lettered_at IS NOT NULL;

-- Truy vết: "aggregate này đã phát những event gì".
CREATE INDEX event_outbox_aggregate_idx
    ON event_outbox (aggregate_type, aggregate_id, occurred_at);

-- ---------------------------------------------------------------------
-- Đánh dấu event đã xử lý — cơ chế idempotency của BÊN NHẬN.
-- ---------------------------------------------------------------------
--
-- Outbox đảm bảo at-least-once, nên bên nhận sẽ thấy event trùng. Bảng này
-- là cách bên nhận nhớ "tôi đã xử lý event này rồi".
--
-- ĐIỂM MẤU CHỐT: bên nhận phải ghi vào bảng này TRONG CÙNG GIAO DỊCH với
-- việc xử lý nghiệp vụ.
--
--     Ghi sổ thành công + đánh dấu thất bại
--       → lần thử lại ghi sổ LẦN HAI
--       → tiền bị nhân đôi
--
-- Khóa chính là cặp (event_id, handler): mỗi bên nhận xử lý độc lập, nên
-- notification đã xử lý không có nghĩa là payment cũng đã xử lý.
CREATE TABLE event_processed (
    event_id TEXT NOT NULL,

    -- handler là tên bên nhận, ví dụ "inventory.commit_on_order_placed".
    handler TEXT NOT NULL CHECK (length(trim(handler)) > 0),

    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (event_id, handler)
);

-- Dọn bản ghi cũ: sau khi outbox đã xóa event, bản ghi ở đây vô dụng.
CREATE INDEX event_processed_age_idx ON event_processed (processed_at);
