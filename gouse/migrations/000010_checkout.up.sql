-- Module: checkout — phiên thanh toán, nơi giá chuyển từ ĐỘNG sang TĨNH.
--
-- BA BẢNG GIỎ–PHIÊN–ĐƠN, ĐỌC LIỀN NHAU THÌ THẤY TOÀN BỘ THIẾT KẾ:
--
--     cart_item      → giá ĐỘNG, không khóa hàng, sống 30 ngày
--     checkout_line  → giá ĐÓNG BĂNG, KHÓA HÀNG, sống 15 phút   ← ở đây
--     order_line     → giá ĐÓNG BĂNG, giữ vĩnh viễn
--
-- Điểm chuyển đổi nằm ở bảng này. Cột reservation_id là thứ cart_item
-- KHÔNG có và order_line KHÔNG cần: nó chỉ tồn tại trong 15 phút giữa lúc
-- khách bấm "Thanh toán" và lúc đơn được tạo.
--
-- VÌ SAO ĐÓNG BĂNG (checkout.md mục 5):
--
--     14:00 — Khách bắt đầu checkout, áo giá 299.000đ
--     14:05 — Seller đổi giá thành 350.000đ
--     14:10 — Khách hoàn tất thanh toán
--
--     Không đóng băng: khách thấy 299.000đ nhưng bị trừ 350.000đ
--     Đóng băng:       khách trả đúng 299.000đ như đã thấy

CREATE TABLE checkout (
    id      TEXT PRIMARY KEY CHECK (id LIKE 'chk\_%' AND length(id) = 30),

    -- Giỏ đã sinh ra phiên này. Giữ lại để đánh dấu CONVERTED sau khi đặt
    -- hàng, và để truy vết đơn hàng đến từ giỏ nào.
    cart_id TEXT NOT NULL,

    customer_id TEXT NOT NULL DEFAULT '',
    guest_email TEXT NOT NULL DEFAULT '',
    guest_phone TEXT NOT NULL DEFAULT '',

    -- Quy tắc 6: khách vãng lai được checkout, nhưng phải liên hệ được.
    CONSTRAINT checkout_needs_customer CHECK (
        length(trim(customer_id)) > 0 OR length(trim(guest_email)) > 0
    ),

    currency CHAR(3) NOT NULL,

    -- Địa chỉ giao hàng, thu thập trong phiên rồi ĐÓNG BĂNG vào đơn.
    ship_recipient_name TEXT NOT NULL DEFAULT '',
    ship_phone          TEXT NOT NULL DEFAULT '',
    ship_street         TEXT NOT NULL DEFAULT '',
    ship_ward           TEXT NOT NULL DEFAULT '',
    ship_district       TEXT NOT NULL DEFAULT '',
    ship_province       TEXT NOT NULL DEFAULT '',
    ship_country_code   TEXT NOT NULL DEFAULT 'VN',
    shipping_method     TEXT NOT NULL DEFAULT '',

    shipping_fee    BIGINT NOT NULL DEFAULT 0 CHECK (shipping_fee >= 0),
    discount_amount BIGINT NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_amount      BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    coupon_code     TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL CHECK (status IN (
        'STARTED', 'PENDING_PAYMENT', 'COMPLETED', 'CANCELLED', 'EXPIRED'
    )),

    -- Thời điểm hàng được nhả — trường quan trọng nhất về vận hành.
    --
    -- Mọi reservation của phiên sống tới đúng lúc này, và tiến trình nền
    -- dựa vào nó để dọn. 15 phút, khác hẳn 30 ngày của giỏ: phiên này
    -- đang KHÓA hàng, giỏ thì không.
    expires_at TIMESTAMPTZ NOT NULL,

    -- Gia hạn có thật (khách đang chuyển khoản) nhưng CÓ GIỚI HẠN: gia hạn
    -- vô hạn nghĩa là khóa hàng vô hạn, đúng thứ mà việc tách checkout
    -- khỏi giỏ sinh ra để tránh.
    extended_times INT NOT NULL DEFAULT 0 CHECK (extended_times BETWEEN 0 AND 2),

    order_id TEXT NOT NULL DEFAULT '',

    -- Khóa idempotency của lần hoàn tất (quy tắc 5).
    --
    -- UNIQUE CÓ ĐIỀU KIỆN bên dưới: nhiều phiên chưa hoàn tất đều có khóa
    -- rỗng, nên không thể dùng UNIQUE thường.
    completion_key TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    -- Đã hoàn tất thì BẮT BUỘC có mã đơn: phiên COMPLETED mà không biết
    -- đơn nào là phiên không truy vết được, và khách hỏi "đơn của tôi đâu"
    -- thì không trả lời được.
    CONSTRAINT checkout_completed_has_order CHECK (
        status <> 'COMPLETED' OR length(trim(order_id)) > 0
    )
);

-- Quy tắc 5: hoàn tất phải idempotent.
CREATE UNIQUE INDEX checkout_completion_key_idx
    ON checkout (completion_key) WHERE completion_key <> '';

-- Một giỏ chỉ có MỘT phiên đang chạy.
--
-- Khách bấm "Thanh toán", quay lại giỏ, bấm lần nữa — phiên thứ hai sẽ giữ
-- hàng LẦN THỨ HAI cho cùng một giỏ, tức là khóa gấp đôi số hàng thật cần.
CREATE UNIQUE INDEX checkout_one_active_per_cart
    ON checkout (cart_id) WHERE status IN ('STARTED', 'PENDING_PAYMENT');

-- Đầu vào của tiến trình dọn. Mỗi hàng khớp chỉ mục này là hàng đang bị
-- KHÓA mà không ai dùng tới.
CREATE INDEX checkout_expiring_idx
    ON checkout (expires_at) WHERE status IN ('STARTED', 'PENDING_PAYMENT');

CREATE INDEX checkout_customer_idx
    ON checkout (customer_id, created_at DESC) WHERE customer_id <> '';

CREATE TABLE checkout_line (
    id          TEXT PRIMARY KEY CHECK (id LIKE 'cln\_%' AND length(id) = 30),
    checkout_id TEXT NOT NULL REFERENCES checkout (id) ON DELETE CASCADE,

    -- Truy vết ngược về giỏ.
    cart_item_id TEXT NOT NULL DEFAULT '',

    offer_id  TEXT NOT NULL,
    sku_id    TEXT NOT NULL DEFAULT '',
    seller_id TEXT NOT NULL DEFAULT '',

    -- ---- ĐÓNG BĂNG tại thời điểm bắt đầu checkout ----
    --
    -- Các cột này được TRUYỀN THẲNG sang order_line, không tính lại. Tính
    -- lại ở bước tạo đơn sẽ phá vỡ toàn bộ ý nghĩa của việc đóng băng.
    product_name        TEXT NOT NULL CHECK (length(trim(product_name)) > 0),
    variant_description TEXT NOT NULL DEFAULT '',
    unit_price          BIGINT NOT NULL CHECK (unit_price > 0),
    currency            CHAR(3) NOT NULL,
    quantity            INT NOT NULL CHECK (quantity > 0),
    commission_rate     INT NOT NULL DEFAULT 0
        CHECK (commission_rate BETWEEN 0 AND 10000),

    -- Mã giữ hàng ở inventory — cột KHÔNG CÓ ở cart_item.
    --
    -- Đó chính là khác biệt cốt lõi giữa hai module: dòng này đang KHÓA
    -- hàng thật, món trong giỏ thì không.
    --
    -- Quy tắc 1: BẮT BUỘC giữ tồn kho trước khi cho checkout. Rỗng nghĩa
    -- là chưa giữ được hàng, và dòng đó không được vào đơn.
    reservation_id TEXT NOT NULL CHECK (length(trim(reservation_id)) > 0),

    -- Kho ĐÃ CHỌN. Một SKU có thể nằm ở nhiều kho; giữ lại để nhả đúng chỗ
    -- và để fulfillment biết xuất hàng từ đâu.
    inventory_item_id TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX checkout_line_checkout_idx ON checkout_line (checkout_id);

-- Tra ngược từ reservation về phiên: khi có hàng bị khóa mà không rõ vì
-- sao, đây là câu hỏi đầu tiên.
CREATE UNIQUE INDEX checkout_line_reservation_idx
    ON checkout_line (reservation_id);
