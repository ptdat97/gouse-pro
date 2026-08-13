-- Module: order — HỢP ĐỒNG với khách và ĐƠN VỊ CÔNG VIỆC vận hành.
--
-- Cấu trúc bảng ở đây phản ánh trực tiếp quyết định cốt lõi của ADR-0007:
--
--     order              = hợp đồng với khách hàng
--     fulfillment_order  = đơn vị công việc của MỘT nguồn hàng
--
-- Tách làm hai bảng, chứ không phải một bảng có cột seller_id, vì hai lý do
-- QUYẾT ĐỊNH:
--
--   3. RÀNG BUỘC BẢO MẬT — seller truy vấn fulfillment_order theo seller_id
--      thì tự nhiên chỉ thấy phần của mình. Nếu seller đọc thẳng "order",
--      phải lọc dữ liệu ở tầng hiển thị, và QUÊN MỘT LẦN là rò rỉ dữ liệu
--      đối thủ. Ở đây ranh giới nằm trong CẤU TRÚC DỮ LIỆU.
--
--   4. TRANH CHẤP GHI — ba seller cập nhật đồng thời sẽ tranh nhau cùng một
--      hàng "order" nếu gộp chung. Tách ra thì mỗi seller ghi hàng riêng.

-- Bộ đếm mã đơn hiển thị.
--
-- Dùng SEQUENCE chứ không phải MAX(order_number)+1: hai request song song
-- đọc cùng một MAX sẽ sinh hai đơn CÙNG MÃ, và khi đó khách đọc mã qua
-- tổng đài sẽ ra hai đơn khác nhau.
--
-- SEQUENCE có thể nhảy số khi giao dịch bị hủy — chấp nhận được: mã đơn
-- cần DUY NHẤT, không cần liên tục.
CREATE SEQUENCE order_number_seq START 1;

CREATE TABLE "order" (
    id              TEXT PRIMARY KEY CHECK (id LIKE 'ord\_%' AND length(id) = 30),

    -- Mã hiển thị cho khách: FC-2026-08-000001. Khách đọc mã này qua điện
    -- thoại khi khiếu nại, nên nó phải ngắn và đọc được — khác với id.
    order_number    TEXT NOT NULL UNIQUE CHECK (length(trim(order_number)) > 0),

    -- Khách đã đăng ký HOẶC khách vãng lai (quy tắc 6). Tham chiếu vượt
    -- module nên không có REFERENCES.
    customer_id     TEXT NOT NULL DEFAULT '',
    guest_email     TEXT NOT NULL DEFAULT '',
    guest_phone     TEXT NOT NULL DEFAULT '',

    -- Đơn phải biết thuộc về ai và liên hệ được bằng cách nào. Đơn không
    -- liên hệ được là đơn không giao được.
    CONSTRAINT order_needs_customer CHECK (
        length(trim(customer_id)) > 0 OR length(trim(guest_email)) > 0
    ),

    -- Địa chỉ ĐÓNG BĂNG: lưu thẳng giá trị, KHÔNG trỏ tới sổ địa chỉ.
    -- Khách sửa sổ địa chỉ sau này không được làm đổi nơi đơn cũ đã giao tới.
    ship_recipient_name TEXT NOT NULL DEFAULT '',
    ship_phone          TEXT NOT NULL DEFAULT '',
    ship_street         TEXT NOT NULL DEFAULT '',
    ship_ward           TEXT NOT NULL DEFAULT '',
    ship_district       TEXT NOT NULL DEFAULT '',
    ship_province       TEXT NOT NULL DEFAULT '',
    ship_country_code   TEXT NOT NULL DEFAULT 'VN',

    bill_recipient_name TEXT NOT NULL DEFAULT '',
    bill_phone          TEXT NOT NULL DEFAULT '',
    bill_street         TEXT NOT NULL DEFAULT '',
    bill_ward           TEXT NOT NULL DEFAULT '',
    bill_district       TEXT NOT NULL DEFAULT '',
    bill_province       TEXT NOT NULL DEFAULT '',
    bill_country_code   TEXT NOT NULL DEFAULT '',

    currency        CHAR(3) NOT NULL,

    -- Các khoản ở mức đơn hàng. Không âm: chiều đã nằm ở ý nghĩa của cột.
    -- Chỗ DUY NHẤT cho phép số âm là order_line_adjustment.amount.
    shipping_fee    BIGINT NOT NULL DEFAULT 0 CHECK (shipping_fee >= 0),
    discount_amount BIGINT NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_amount      BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),

    status          TEXT NOT NULL CHECK (status IN (
        'PENDING_PAYMENT', 'PAID', 'PROCESSING',
        'PARTIALLY_SHIPPED', 'SHIPPED',
        'PARTIALLY_DELIVERED', 'DELIVERED',
        'PARTIALLY_CANCELLED', 'CANCELLED', 'COMPLETED'
    )),

    -- Quy tắc 5: PlaceOrder phải idempotent. Khách bấm "Đặt hàng" hai lần,
    -- hoặc client thử lại sau timeout — không được tạo hai đơn.
    --
    -- UNIQUE ở đây là lớp bảo vệ CUỐI CÙNG: kiểm tra ở tầng ứng dụng vẫn
    -- lọt khi hai request chạy song song, ràng buộc này thì không.
    idempotency_key TEXT NOT NULL UNIQUE CHECK (length(trim(idempotency_key)) > 0),

    placed_at       TIMESTAMPTZ NOT NULL,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX order_customer_idx ON "order" (customer_id, placed_at DESC)
    WHERE customer_id <> '';
CREATE INDEX order_status_idx ON "order" (status, placed_at DESC);
CREATE INDEX order_placed_idx ON "order" (placed_at DESC);

-- ---------------------------------------------------------------------
-- Dòng hàng — mọi thông tin giao dịch ĐÓNG BĂNG (nguyên tắc P9).
-- ---------------------------------------------------------------------
--
-- KIỂM CHỨNG BẰNG TÌNH HUỐNG THỰC TẾ:
--
--     10/08: Khách mua áo 299.000đ, hoa hồng 10%
--     15/08: Seller giảm giá còn 249.000đ
--     20/08: Nền tảng đổi chính sách hoa hồng thành 12%
--     25/08: Chạy đối soát cho kỳ 01–15/08
--
--     Tham chiếu động: 249.000 × 12% = 29.880đ    ← SAI
--     Đóng băng:       299.000 × 10% = 29.900đ    ← ĐÚNG
--
-- Vì vậy product_name, unit_price, commission_rate là CỘT DỮ LIỆU, không
-- phải JOIN sang product/pricing. Sai lệch này không chỉ là con số — nó
-- phá vỡ niềm tin của seller và không giải thích được khi có tranh chấp.
CREATE TABLE order_line (
    id          TEXT PRIMARY KEY CHECK (id LIKE 'oln\_%' AND length(id) = 30),
    order_id    TEXT NOT NULL REFERENCES "order" (id),

    -- Thứ khách THỰC SỰ mua: một lời chào bán cụ thể của một seller.
    offer_id    TEXT NOT NULL,
    sku_id      TEXT NOT NULL DEFAULT '',

    -- Sao chép lại để truy vấn theo seller không phải JOIN ngược.
    seller_id   TEXT NOT NULL,

    -- ---- Các cột ĐÓNG BĂNG ----
    product_name        TEXT NOT NULL CHECK (length(trim(product_name)) > 0),
    variant_description TEXT NOT NULL DEFAULT '',
    unit_price          BIGINT NOT NULL CHECK (unit_price > 0),
    currency            CHAR(3) NOT NULL,
    quantity            INT NOT NULL CHECK (quantity > 0),

    -- Tỷ lệ theo ĐIỂM CƠ BẢN (10.000 = 100%), không phải số thực: sai số
    -- dấu phẩy động tích lũy thành lệch đối soát.
    commission_rate     INT NOT NULL DEFAULT 0
        CHECK (commission_rate BETWEEN 0 AND 10000),

    -- Tính MỘT LẦN lúc đặt hàng rồi lưu lại. Tính lúc đọc nghĩa là tham
    -- chiếu động, và đối soát sẽ ra số khác khi chính sách đổi.
    commission_amount   BIGINT NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),

    attributed_creator_id   TEXT NOT NULL DEFAULT '',
    creator_commission_rate INT NOT NULL DEFAULT 0
        CHECK (creator_commission_rate BETWEEN 0 AND 10000),

    status      TEXT NOT NULL CHECK (status IN ('ACTIVE', 'CANCELLED', 'RETURNED')),

    cancelled_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,

    -- Quy tắc 3: KHÔNG xóa dữ liệu đơn hàng. Dòng bị hủy vẫn ở lại, chỉ
    -- đổi trạng thái — hóa đơn cũ và đối soát phải giải thích được.
    CONSTRAINT order_line_cancelled_has_time CHECK (
        status <> 'CANCELLED' OR cancelled_at IS NOT NULL
    )
);

CREATE INDEX order_line_order_idx ON order_line (order_id);

-- Đối soát của seller: "tháng này tôi bán được gì". Truy vấn nóng.
CREATE INDEX order_line_seller_idx ON order_line (seller_id, created_at DESC);

CREATE INDEX order_line_creator_idx ON order_line (attributed_creator_id, created_at DESC)
    WHERE attributed_creator_id <> '';

-- ---------------------------------------------------------------------
-- Khoản điều chỉnh — THỰC THỂ HẠNG NHẤT, không phải một cột.
-- ---------------------------------------------------------------------
--
-- Nếu chỉ có order.discount_amount = 50000:
--     đơn 3 món 500.000đ giảm 50.000đ, khách trả món C giá 100.000đ —
--     hoàn bao nhiêu? Phải tính lại tỷ lệ, dễ sai.
--
-- Có bảng này (phân bổ ngay lúc đặt hàng):
--     dòng A → −20.000, B → −20.000, C → −10.000
--     khách trả món C → hoàn 100.000 − 10.000 = 90.000đ, ĐỌC TRỰC TIẾP.
CREATE TABLE order_line_adjustment (
    id            TEXT PRIMARY KEY,
    order_line_id TEXT NOT NULL REFERENCES order_line (id),

    adjustment_type TEXT NOT NULL CHECK (adjustment_type IN (
        'PROMOTION', 'TAX', 'SHIPPING', 'COMMISSION', 'FEE', 'MANUAL'
    )),

    -- Nhãn là thứ KHÁCH NHÌN THẤY trên hóa đơn. Không có nhãn thì hiện một
    -- khoản trừ vô danh — khách không hiểu và sẽ khiếu nại.
    label   TEXT NOT NULL CHECK (length(trim(label)) > 0),

    -- ÂM là giảm, DƯƠNG là tăng. Chỗ DUY NHẤT trong hệ thống cho phép số
    -- tiền âm: một khoản điều chỉnh vốn dĩ có hai chiều, và tách thành hai
    -- loại sẽ nhân đôi số nhánh xử lý ở mọi nơi cộng dồn.
    --
    -- Bằng 0 thì bị chặn: khoản điều chỉnh không tác dụng chỉ làm rối hóa đơn.
    amount   BIGINT NOT NULL CHECK (amount <> 0),
    currency CHAR(3) NOT NULL,

    source_type TEXT NOT NULL DEFAULT '',
    source_id   TEXT NOT NULL DEFAULT '',

    -- AI CHỊU CHI PHÍ. Không có cột này thì không trả lời được "tổng giảm
    -- giá do seller chịu trong kỳ này là bao nhiêu" — câu hỏi bắt buộc khi
    -- đối soát với seller.
    cost_bearer TEXT NOT NULL DEFAULT 'PLATFORM'
        CHECK (cost_bearer IN ('PLATFORM', 'SELLER', 'SHARED')),

    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX order_line_adjustment_line_idx
    ON order_line_adjustment (order_line_id);

-- Đối soát: tổng khoản giảm theo bên chịu chi phí trong kỳ.
CREATE INDEX order_line_adjustment_bearer_idx
    ON order_line_adjustment (cost_bearer, created_at DESC);

-- ---------------------------------------------------------------------
-- Đơn vị công việc vận hành — RANH GIỚI BẢO MẬT của seller.
-- ---------------------------------------------------------------------
--
-- Giỏ hàng:
--   ├── Áo own brand   (kho nền tảng, Hà Nội)
--   ├── Giày Seller A  (kho seller A, TP.HCM)
--   └── Túi Seller B   (kho seller B, Đà Nẵng)
--
-- Ba món KHÔNG THỂ đóng chung một gói. Mỗi nguồn hàng là một hàng ở đây.
--
-- Own brand cũng được tách như seller bình thường: nó là một seller nội bộ
-- (INTERNAL), nên đơn lẫn own brand và hàng seller đi CHUNG một luồng.
CREATE TABLE fulfillment_order (
    id        TEXT PRIMARY KEY CHECK (id LIKE 'ful\_%' AND length(id) = 30),
    order_id  TEXT NOT NULL REFERENCES "order" (id),

    -- Mã hiển thị cho seller: <order_number>-A, -B, -C. Seller thấy mã của
    -- mình mà KHÔNG cần biết có bao nhiêu seller khác trong đơn.
    fo_number TEXT NOT NULL UNIQUE CHECK (length(trim(fo_number)) > 0),

    -- CỘT TẠO NÊN RANH GIỚI BẢO MẬT. Mọi truy vấn của seller lọc theo cột
    -- này, ngay trong SQL — không phụ thuộc vào việc tầng hiển thị có nhớ
    -- lọc hay không.
    seller_id TEXT NOT NULL CHECK (length(trim(seller_id)) > 0),

    status    TEXT NOT NULL CHECK (status IN (
        'PENDING', 'CONFIRMED', 'PACKED', 'SHIPPED', 'DELIVERED', 'CANCELLED'
    )),

    -- Số tiền của RIÊNG phần này, để seller đối soát mà không cần thấy đơn.
    subtotal          BIGINT NOT NULL DEFAULT 0 CHECK (subtotal >= 0),
    commission_amount BIGINT NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),
    currency          CHAR(3) NOT NULL,

    -- Lý do hủy BẮT BUỘC khi đã hủy: seller cần biết vì sao, và khách cần
    -- lời giải thích khi nhận thông báo.
    cancel_reason TEXT NOT NULL DEFAULT '',
    CONSTRAINT fulfillment_order_cancel_needs_reason CHECK (
        status <> 'CANCELLED' OR length(trim(cancel_reason)) > 0
    ),

    confirmed_at TIMESTAMPTZ,
    packed_at    TIMESTAMPTZ,
    shipped_at   TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Truy vấn nóng nhất của seller: "đơn nào tôi cần xử lý hôm nay".
CREATE INDEX fulfillment_order_seller_idx
    ON fulfillment_order (seller_id, status, created_at DESC);

CREATE INDEX fulfillment_order_order_idx ON fulfillment_order (order_id);

-- Một seller CHỈ có MỘT đơn thực hiện trong một đơn hàng: gom theo seller
-- là nguyên tắc tách đơn, hai hàng cho cùng seller nghĩa là tách sai.
CREATE UNIQUE INDEX fulfillment_order_one_per_seller
    ON fulfillment_order (order_id, seller_id);

-- Dòng hàng nào thuộc đơn thực hiện nào.
--
-- Bảng nối riêng thay vì cột fulfillment_order_id trên order_line: dòng
-- hàng thuộc về HỢP ĐỒNG với khách, còn việc gom nó vào gói nào là chuyện
-- VẬN HÀNH. Giữ hai thứ tách nhau cho phép tách lại đơn (ví dụ seller chia
-- hai lô giao) mà không đụng vào hợp đồng.
CREATE TABLE fulfillment_order_line (
    fulfillment_order_id TEXT NOT NULL REFERENCES fulfillment_order (id),
    order_line_id        TEXT NOT NULL REFERENCES order_line (id),

    PRIMARY KEY (fulfillment_order_id, order_line_id)
);

CREATE INDEX fulfillment_order_line_line_idx
    ON fulfillment_order_line (order_line_id);
