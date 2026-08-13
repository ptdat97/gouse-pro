-- Module: cart — giỏ hàng, Ý ĐỊNH mua chứ không phải hợp đồng.
--
-- ĐỐI CHIẾU VỚI BẢNG "order" Ở MIGRATION 000008:
--
--     order_line                    | cart_item
--     ------------------------------|------------------------------
--     product_name  ĐÓNG BĂNG       | product_name  BẢN CHỤP, làm mới
--     unit_price    ĐÓNG BĂNG       | unit_price    CẬP NHẬT ĐỘNG
--     commission_rate ĐÓNG BĂNG     | KHÔNG lưu
--
-- Hai bảng trông giống nhau nhưng ý nghĩa ngược nhau. Sửa giá trong
-- order_line là làm sai hóa đơn cũ; sửa giá trong cart_item là hành vi
-- ĐÚNG và diễn ra thường xuyên.
--
-- ĐIỀU KHÔNG CÓ Ở ĐÂY MỚI LÀ QUAN TRỌNG NHẤT: không có cột nào trỏ tới
-- reservation, không có cột nào khóa tồn kho. Giỏ hàng KHÔNG GIỮ HÀNG.
-- Nếu giữ, khách thêm rồi bỏ quên hai tuần sẽ khóa hàng hai tuần, và với
-- hàng khan hiếm thì vài trăm giỏ bỏ quên = hết hàng ảo.

CREATE TABLE cart (
    id          TEXT PRIMARY KEY CHECK (id LIKE 'crt\_%' AND length(id) = 30),

    -- Một trong hai phải có. Khách vãng lai dùng session_id; khi đăng nhập
    -- thì giỏ theo phiên được GỘP vào giỏ của tài khoản.
    customer_id TEXT NOT NULL DEFAULT '',
    session_id  TEXT NOT NULL DEFAULT '',

    CONSTRAINT cart_needs_owner CHECK (
        length(trim(customer_id)) > 0 OR length(trim(session_id)) > 0
    ),

    currency    CHAR(3) NOT NULL,

    status      TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN (
        'ACTIVE', 'CONVERTED', 'ABANDONED', 'MERGED'
    )),

    -- DÀI: 30 ngày, khác hẳn checkout (15 phút). Giỏ không giữ hàng nên
    -- để lâu không hại ai; ngược lại, giỏ sống lâu là thứ khách quay lại
    -- và mua tiếp.
    expires_at  TIMESTAMPTZ NOT NULL,

    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

-- QUY TẮC 5: một khách chỉ có MỘT giỏ ACTIVE.
--
-- Chỉ mục UNIQUE CÓ ĐIỀU KIỆN, không phải UNIQUE thường: giỏ đã chuyển
-- đổi hoặc bỏ quên vẫn ở lại (chúng là dữ liệu phân tích), nên khách có
-- nhiều giỏ trong lịch sử nhưng chỉ một giỏ đang dùng.
CREATE UNIQUE INDEX cart_one_active_per_customer
    ON cart (customer_id) WHERE status = 'ACTIVE' AND customer_id <> '';

CREATE UNIQUE INDEX cart_one_active_per_session
    ON cart (session_id) WHERE status = 'ACTIVE' AND session_id <> '';

-- Tìm giỏ bị bỏ quên để gửi nhắc nhở.
CREATE INDEX cart_abandoned_idx
    ON cart (updated_at) WHERE status = 'ACTIVE';

CREATE INDEX cart_expiry_idx
    ON cart (expires_at) WHERE status = 'ACTIVE';

CREATE TABLE cart_item (
    id      TEXT PRIMARY KEY CHECK (id LIKE 'cit\_%' AND length(id) = 30),
    cart_id TEXT NOT NULL REFERENCES cart (id) ON DELETE CASCADE,

    -- Thứ khách chọn mua: lời chào bán cụ thể của một seller.
    offer_id  TEXT NOT NULL,
    sku_id    TEXT NOT NULL DEFAULT '',
    seller_id TEXT NOT NULL DEFAULT '',

    -- BẢN CHỤP để hiển thị nhanh, LÀM MỚI mỗi lần đồng bộ.
    --
    -- Khác hẳn order_line: ở đó các cột này là nguồn sự thật đóng băng, ở
    -- đây chúng chỉ là bộ nhớ đệm của dữ liệu sống ở marketplace/product.
    product_name        TEXT NOT NULL DEFAULT '',
    variant_description TEXT NOT NULL DEFAULT '',
    image_url           TEXT NOT NULL DEFAULT '',
    unit_price          BIGINT NOT NULL CHECK (unit_price > 0),
    currency            CHAR(3) NOT NULL,

    quantity INT NOT NULL CHECK (quantity > 0),

    -- Giới hạn của offer, chụp lại để kiểm tra số lượng mà không phải gọi
    -- marketplace ở mọi thao tác tăng giảm.
    --
    -- max = 0 nghĩa là KHÔNG giới hạn — cùng quy ước với marketplace, để
    -- hai module không diễn giải cùng một con số theo hai cách.
    min_order_quantity INT NOT NULL DEFAULT 0 CHECK (min_order_quantity >= 0),
    max_order_quantity INT NOT NULL DEFAULT 0 CHECK (max_order_quantity >= 0),

    -- QUY TẮC 6: không tự động xóa món không hợp lệ, chỉ ĐÁNH DẤU.
    --
    -- Xóa im lặng làm khách bối rối: họ nhớ đã thêm món đó, không hiểu vì
    -- sao nó biến mất, rồi nghi ngờ cả những món còn lại.
    availability TEXT NOT NULL DEFAULT 'AVAILABLE' CHECK (availability IN (
        'AVAILABLE', 'OUT_OF_STOCK', 'UNAVAILABLE', 'QUANTITY_REDUCED'
    )),

    -- Số lượng bán được tại lần đồng bộ gần nhất — THÔNG TIN THAM KHẢO.
    --
    -- Giỏ không giữ hàng nên con số này có thể sai ngay khi khách đọc nó.
    -- Cam kết chỉ có ở checkout.
    available_quantity INT NOT NULL DEFAULT 0,

    -- Nguồn giới thiệu — mắt xích của bánh đà creator commerce.
    --
    -- Ghi ngay lúc THÊM GIỎ, không đợi lúc mua: nhờ vậy đo được tỷ lệ
    -- "thêm giỏ" của từng nội dung (tín hiệu ý định mua mạnh hơn lượt xem
    -- nhiều) và quy kết đúng khi khách mua sau vài ngày.
    source_content_id TEXT NOT NULL DEFAULT '',
    source_creator_id TEXT NOT NULL DEFAULT '',

    added_at   TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Một offer chỉ xuất hiện MỘT LẦN trong một giỏ: thêm lại thì CỘNG DỒN.
--
-- Khách thêm cùng offer hai lần mong thấy "số lượng 2", không phải hai
-- dòng giống hệt nhau.
CREATE UNIQUE INDEX cart_item_one_per_offer ON cart_item (cart_id, offer_id);

CREATE INDEX cart_item_cart_idx ON cart_item (cart_id);

-- Tín hiệu nhu cầu theo nội dung: "nội dung nào tạo ý định mua".
CREATE INDEX cart_item_source_idx
    ON cart_item (source_content_id, added_at DESC)
    WHERE source_content_id <> '';

-- Tín hiệu nhu cầu theo SKU, đầu vào cho supply-chain.
CREATE INDEX cart_item_sku_idx ON cart_item (sku_id, added_at DESC);
