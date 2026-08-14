-- Module: promotion — khuyến mãi và mã giảm giá.
--
-- PHẠM VI MVP (promotion.md mục 11): mã giảm giá cơ bản và miễn phí vận
-- chuyển theo ngưỡng. Khuyến mãi tự động, combo/outfit, mua X tặng Y
-- thuộc Phase 2 trở đi.
--
-- VẤN ĐỀ CỐT LÕI CỦA KHUYẾN MÃI TRÊN MARKETPLACE (mục 3):
--
--     Khách dùng mã giảm 50.000đ cho đơn của Seller A. AI CHỊU 50.000đ?
--
-- Không trả lời được câu này thì không tính được seller thực nhận bao
-- nhiêu, và đối soát cuối tháng sẽ lệch đúng bằng tổng tiền khuyến mãi.
-- Vì vậy cost_bearer là cột BẮT BUỘC ngay từ MVP, dù MVP chỉ dùng
-- PLATFORM.

CREATE TABLE promotion (
    id TEXT PRIMARY KEY CHECK (id LIKE 'pmo\_%' AND length(id) = 30),

    name        TEXT NOT NULL CHECK (length(name) > 0),
    description TEXT NOT NULL DEFAULT '',

    -- Loại khuyến mãi. MVP chỉ cài COUPON và FREE_SHIPPING.
    --
    -- Các giá trị còn lại có mặt trong CHECK từ đầu để thêm loại mới
    -- không phải sửa ràng buộc — nhưng KHÔNG có nghĩa là đã cài đặt.
    kind TEXT NOT NULL CHECK (kind IN (
        'COUPON',         -- khách nhập mã                      (MVP)
        'FREE_SHIPPING',  -- miễn phí ship theo ngưỡng          (MVP)
        'AUTO',           -- tự áp khi đủ điều kiện         (Phase 2)
        'BUY_X_GET_Y',                                   -- (Phase 3)
        'OUTFIT_BUNDLE'   -- mua cả bộ giảm giá             (Phase 3)
    )),

    -- Cách tính giảm.
    --
    --     PERCENTAGE  giảm theo phần trăm, dùng discount_bps
    --     FIXED       giảm số tiền cố định, dùng discount_amount
    --     FREE_SHIP   miễn phí vận chuyển, không dùng cột nào
    discount_type TEXT NOT NULL CHECK (discount_type IN (
        'PERCENTAGE', 'FIXED', 'FREE_SHIP'
    )),

    -- discount_bps là điểm cơ bản: 1000 = 10%.
    --
    -- SỐ NGUYÊN, không phải số thực. 10% của 999.999đ tính bằng float sẽ
    -- ra 99999.90000000001 — và với hàng triệu giao dịch, sai số tích lũy
    -- thành tiền thật.
    discount_bps INT NOT NULL DEFAULT 0
        CHECK (discount_bps >= 0 AND discount_bps <= 10000),

    discount_amount BIGINT NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),

    -- max_discount_amount CHẶN TRÊN cho giảm theo phần trăm.
    --
    -- "Giảm 50%, tối đa 100.000đ" là cách viết thông thường của khuyến
    -- mãi thật. Không có nó, một đơn 10 triệu sẽ được giảm 5 triệu.
    --
    -- 0 nghĩa là KHÔNG giới hạn.
    max_discount_amount BIGINT NOT NULL DEFAULT 0
        CHECK (max_discount_amount >= 0),

    currency TEXT NOT NULL DEFAULT 'VND',

    -- ------------------------------------------------------------------
    -- Điều kiện áp dụng (mục 5). MVP chỉ có ngưỡng giá trị đơn.
    -- ------------------------------------------------------------------

    -- min_order_amount là ngưỡng giá trị đơn tối thiểu.
    --
    -- Đây là điều kiện của MIỄN PHÍ SHIP THEO NGƯỠNG — tính năng MVP.
    min_order_amount BIGINT NOT NULL DEFAULT 0 CHECK (min_order_amount >= 0),

    -- ------------------------------------------------------------------
    -- Bên chịu chi phí (mục 3) — xem ghi chú đầu file.
    -- ------------------------------------------------------------------

    cost_bearer TEXT NOT NULL DEFAULT 'PLATFORM' CHECK (cost_bearer IN (
        'PLATFORM',  -- nền tảng chịu, trừ vào chi phí marketing
        'SELLER',    -- seller chịu, trừ vào số tiền seller nhận
        'SHARED'     -- chia theo tỷ lệ                     (Phase 2)
    )),

    -- Tỷ lệ chia khi cost_bearer = SHARED, tính bằng điểm cơ bản.
    --
    -- Hai cột PHẢI cộng lại đúng 10000 (100%) — ràng buộc ở cuối bảng.
    -- Lệch một điểm cơ bản là một khoản tiền không ai chịu, và đối soát
    -- cuối tháng sẽ không khớp.
    platform_share_bps INT NOT NULL DEFAULT 10000
        CHECK (platform_share_bps >= 0 AND platform_share_bps <= 10000),
    seller_share_bps INT NOT NULL DEFAULT 0
        CHECK (seller_share_bps >= 0 AND seller_share_bps <= 10000),

    -- seller_id khác rỗng nghĩa là khuyến mãi CHỈ áp cho gian hàng đó.
    --
    -- KHÔNG có khóa ngoại tới bảng seller: đó là module khác, và khóa
    -- ngoại vượt ranh giới module biến hai module thành một (quy tắc R2).
    seller_id TEXT NOT NULL DEFAULT '',

    -- ------------------------------------------------------------------
    -- Giới hạn sử dụng và ngân sách.
    -- ------------------------------------------------------------------

    -- max_uses là số lượt tối đa TOÀN CỤC. 0 = không giới hạn.
    max_uses INT NOT NULL DEFAULT 0 CHECK (max_uses >= 0),

    -- max_uses_per_customer là số lượt tối đa MỖI KHÁCH. 0 = không giới hạn.
    --
    -- Thiếu cột này thì một người dùng mã "giảm 100k cho khách mới" được
    -- vô số lần bằng cách tạo đơn liên tục.
    max_uses_per_customer INT NOT NULL DEFAULT 0
        CHECK (max_uses_per_customer >= 0),

    -- used_count là bộ đếm, tăng bằng UPDATE có điều kiện.
    --
    -- KHÔNG đếm lại từ bảng coupon_usage mỗi lần kiểm tra: truy vấn đó
    -- chạy ở MỌI lần khách nhập mã, và nó quét toàn bộ lịch sử sử dụng.
    used_count INT NOT NULL DEFAULT 0 CHECK (used_count >= 0),

    -- max_budget là tổng số tiền tối đa được giảm. 0 = không giới hạn.
    --
    -- Khác max_uses: một mã giảm 10% không biết trước mỗi lượt tốn bao
    -- nhiêu, nên giới hạn theo lượt KHÔNG chặn được chi phí.
    max_budget  BIGINT NOT NULL DEFAULT 0 CHECK (max_budget >= 0),
    used_budget BIGINT NOT NULL DEFAULT 0 CHECK (used_budget >= 0),

    -- ------------------------------------------------------------------
    -- Hiệu lực.
    -- ------------------------------------------------------------------

    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
        'DRAFT',      -- đang soạn, chưa áp được
        'ACTIVE',
        'PAUSED',     -- tạm dừng, có thể bật lại
        'EXHAUSTED',  -- hết lượt hoặc hết ngân sách
        'EXPIRED'     -- quá hạn
    )),

    starts_at TIMESTAMPTZ NOT NULL,
    ends_at   TIMESTAMPTZ NOT NULL,

    version INT NOT NULL DEFAULT 1 CHECK (version > 0),

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    -- Khoảng thời gian phải hợp lệ.
    CHECK (ends_at > starts_at),

    -- Tỷ lệ chia phải cộng đúng 100%.
    CHECK (platform_share_bps + seller_share_bps = 10000),

    -- Giảm theo phần trăm phải có tỷ lệ; giảm cố định phải có số tiền.
    --
    -- Thiếu ràng buộc này thì tạo được khuyến mãi "giảm 0đ" — nó qua mọi
    -- kiểm tra, khách áp được, và không giảm gì cả.
    CHECK (
        (discount_type = 'PERCENTAGE' AND discount_bps > 0) OR
        (discount_type = 'FIXED'      AND discount_amount > 0) OR
        (discount_type = 'FREE_SHIP')
    )
);

CREATE INDEX promotion_active_idx ON promotion (status, starts_at, ends_at)
    WHERE status = 'ACTIVE';

CREATE INDEX promotion_seller_idx ON promotion (seller_id)
    WHERE seller_id <> '';

-- ---------------------------------------------------------------------
-- Mã giảm giá.
-- ---------------------------------------------------------------------
--
-- TÁCH KHỎI promotion có chủ ý: một chương trình khuyến mãi có thể phát
-- hành NHIỀU mã khác nhau — mã chung "SALE10" và mã riêng cho từng khách.
-- Gộp vào một bảng thì mỗi mã riêng là một bản sao đầy đủ của toàn bộ
-- cấu hình khuyến mãi.
CREATE TABLE coupon (
    id TEXT PRIMARY KEY CHECK (id LIKE 'cpn\_%' AND length(id) = 30),

    promotion_id TEXT NOT NULL REFERENCES promotion (id),

    -- code là thứ khách GÕ VÀO, luôn CHỮ HOA.
    --
    -- Khách gõ "sale10" và "SALE10" phải ra cùng một mã — không thì họ
    -- nghĩ mã hỏng. Chuẩn hóa ở tầng ứng dụng, ràng buộc ở đây.
    code TEXT NOT NULL UNIQUE CHECK (code = upper(code) AND length(code) > 0),

    -- customer_id khác rỗng nghĩa là mã RIÊNG cho một khách.
    --
    -- Dùng cho mã xin lỗi sau sự cố, hoặc mã tặng khách VIP. Người khác
    -- biết mã cũng không dùng được.
    customer_id TEXT NOT NULL DEFAULT '',

    used_count INT NOT NULL DEFAULT 0 CHECK (used_count >= 0),

    active BOOLEAN NOT NULL DEFAULT true,

    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX coupon_promotion_idx ON coupon (promotion_id);

CREATE INDEX coupon_customer_idx ON coupon (customer_id)
    WHERE customer_id <> '';

-- ---------------------------------------------------------------------
-- Lượt sử dụng.
-- ---------------------------------------------------------------------
--
-- BẢNG NÀY LÀ NGUỒN SỰ THẬT về việc mã đã dùng chưa.
--
-- Bộ đếm used_count trên promotion và coupon chỉ là bản tóm tắt để đọc
-- nhanh; khi hai con số lệch nhau, bảng này đúng.
CREATE TABLE coupon_usage (
    id BIGSERIAL PRIMARY KEY,

    coupon_id    TEXT NOT NULL REFERENCES coupon (id),
    promotion_id TEXT NOT NULL REFERENCES promotion (id),

    -- customer_id có thể RỖNG: khách vãng lai cũng dùng mã được.
    customer_id TEXT NOT NULL DEFAULT '',

    order_id TEXT NOT NULL CHECK (length(order_id) > 0),

    discount_amount BIGINT NOT NULL CHECK (discount_amount >= 0),
    currency        TEXT NOT NULL DEFAULT 'VND',

    used_at TIMESTAMPTZ NOT NULL,

    -- released_at khác NULL nghĩa là đã giải phóng (đơn bị hủy).
    --
    -- KHÔNG xóa hàng: cần biết mã từng được dùng cho đơn nào và vì sao
    -- được trả lại — nếu không, tranh chấp "tôi đã dùng mã rồi" không có
    -- gì để tra.
    released_at TIMESTAMPTZ,

    -- IDEMPOTENCY Ở TẦNG DATABASE.
    --
    -- Ghi nhận sử dụng phải idempotent (quy tắc 4). Kiểm tra "đã ghi
    -- chưa" ở tầng ứng dụng KHÔNG cứu được khi handler xử lý cùng một
    -- event hai lần: cả hai lần cùng đọc thấy chưa có rồi cùng ghi.
    --
    -- Khi đó ngân sách khuyến mãi bị trừ hai lần cho một đơn.
    UNIQUE (coupon_id, order_id)
);

-- Đếm số lần MỘT KHÁCH đã dùng MỘT MÃ — kiểm tra max_uses_per_customer.
CREATE INDEX coupon_usage_customer_idx
    ON coupon_usage (coupon_id, customer_id)
    WHERE released_at IS NULL;

-- Tra theo đơn: "đơn này đã áp mã nào".
CREATE INDEX coupon_usage_order_idx ON coupon_usage (order_id);
