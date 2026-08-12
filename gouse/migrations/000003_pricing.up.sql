-- Module: pricing — bảng giá, khung giá ràng buộc seller, lịch sử giá.
--
-- KHÔNG có khóa ngoại tới sku: sku thuộc module product.

CREATE TABLE price (
    id             TEXT PRIMARY KEY CHECK (id LIKE 'prc\_%'),

    -- Tham chiếu VƯỢT MODULE — không có REFERENCES.
    sku_id         TEXT NOT NULL,

    price_type     TEXT NOT NULL CHECK (
        price_type IN ('BASE', 'MEMBER', 'CLEARANCE', 'CAMPAIGN', 'FLASH')
    ),

    -- Tiền: BIGINT + CHAR(3), KHÔNG BAO GIỜ dùng FLOAT.
    -- Sai số dấu phẩy động tích lũy thành lệch đối soát, và lệch đối soát
    -- phải điều tra thủ công từng đơn.
    amount         BIGINT NOT NULL CHECK (amount > 0),
    currency       CHAR(3) NOT NULL,

    -- Giá gạch ngang. 0 nghĩa là không hiển thị.
    compare_at     BIGINT NOT NULL DEFAULT 0 CHECK (compare_at >= 0),

    -- NULL = có hiệu lực ngay / vô thời hạn.
    valid_from     TIMESTAMPTZ,
    valid_until    TIMESTAMPTZ,

    customer_tier  TEXT NOT NULL DEFAULT '',
    campaign_id    TEXT NOT NULL DEFAULT '',

    is_active      BOOLEAN NOT NULL DEFAULT TRUE,

    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,

    -- Giá gạch ngang phải CAO HƠN giá bán, nếu không thì trang hiển thị
    -- "giảm -20%".
    CONSTRAINT price_compare_at_higher CHECK (compare_at = 0 OR compare_at > amount),

    -- Khoảng hiệu lực hợp lệ.
    CONSTRAINT price_period_valid CHECK (
        valid_from IS NULL OR valid_until IS NULL OR valid_from < valid_until
    ),

    -- Giá flash và chiến dịch PHẢI có thời hạn kết thúc.
    --
    -- Giá flash quên tắt sẽ bán lỗ vô hạn — loại lỗi không ai phát hiện
    -- cho tới khi đối soát cuối tháng. Ràng buộc ở database là chốt chặn
    -- cuối cùng, kể cả khi có đường ghi nào đó bỏ qua kiểm tra ở domain.
    CONSTRAINT price_temporary_needs_end CHECK (
        price_type NOT IN ('FLASH', 'CAMPAIGN') OR valid_until IS NOT NULL
    )
);

-- Tra giá theo SKU là truy vấn nóng nhất của module này.
CREATE INDEX price_sku_idx ON price (sku_id) WHERE is_active;

CREATE INDEX price_campaign_idx ON price (campaign_id) WHERE campaign_id <> '';

CREATE TABLE price_constraint (
    id                  TEXT PRIMARY KEY CHECK (id LIKE 'pcs\_%'),

    -- MỘT khung giá cho mỗi SKU. Nhiều khung sẽ mâu thuẫn nhau và không
    -- có cách nào quyết định khung nào thắng.
    sku_id              TEXT NOT NULL UNIQUE,

    -- 0 nghĩa là không giới hạn phía đó.
    min_price           BIGINT NOT NULL DEFAULT 0 CHECK (min_price >= 0),
    max_price           BIGINT NOT NULL DEFAULT 0 CHECK (max_price >= 0),
    reference_price     BIGINT NOT NULL DEFAULT 0 CHECK (reference_price >= 0),
    currency            CHAR(3) NOT NULL,

    -- Ngưỡng cảnh báo theo phần vạn so với giá tham chiếu.
    suspicious_below_bp INT NOT NULL DEFAULT 5000 CHECK (suspicious_below_bp BETWEEN 0 AND 10000),

    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,

    -- Khung giá phải có ít nhất một giới hạn, nếu không nó tạo cảm giác
    -- an toàn giả rằng giá đang được kiểm soát.
    CONSTRAINT price_constraint_not_empty CHECK (min_price > 0 OR max_price > 0),

    -- Min > Max thì KHÔNG giá nào hợp lệ và seller không đăng bán được
    -- mà không hiểu vì sao.
    CONSTRAINT price_constraint_min_below_max CHECK (
        min_price = 0 OR max_price = 0 OR min_price <= max_price
    )
);

-- ---------------------------------------------------------------------
-- Lịch sử giá — BẤT BIẾN, chỉ ghi thêm.
-- ---------------------------------------------------------------------
--
-- VÌ SAO CẦN (docs/04-modules/pricing.md mục 6):
--   1. Phát hiện thao túng giá — tăng rồi giảm giả vờ khuyến mãi
--   2. Phân tích độ co giãn của cầu
--   3. Nghĩa vụ minh bạch giá ở một số thị trường
--
-- Điểm 3 là lý do không thể bỏ qua: một số nơi yêu cầu công bố "giá thấp
-- nhất 30 ngày qua" khi quảng cáo giảm giá. Lịch sử SỬA ĐƯỢC thì không
-- còn giá trị làm bằng chứng.
CREATE TABLE price_history (
    id          TEXT PRIMARY KEY CHECK (id LIKE 'prc\_%'),
    sku_id      TEXT NOT NULL,

    price_type  TEXT NOT NULL,
    amount      BIGINT NOT NULL CHECK (amount > 0),
    currency    CHAR(3) NOT NULL,
    compare_at  BIGINT NOT NULL DEFAULT 0,

    -- Lý do là BẮT BUỘC. Rà soát thao túng giá cần biết VÌ SAO mỗi lần
    -- đổi, không chỉ biết giá mới là bao nhiêu.
    reason      TEXT NOT NULL CHECK (length(trim(reason)) > 0),

    -- Rỗng nghĩa là hệ thống tự động.
    changed_by  TEXT NOT NULL DEFAULT '',

    recorded_at TIMESTAMPTZ NOT NULL
);

-- Truy vấn "giá thấp nhất 30 ngày qua" quét theo (sku_id, recorded_at).
CREATE INDEX price_history_sku_time_idx ON price_history (sku_id, recorded_at DESC);

-- RULE chặn UPDATE và DELETE ở TẦNG DATABASE.
--
-- Đây là điểm khác biệt so với kho in-memory: in-memory chỉ "không cung
-- cấp phương thức sửa", còn ở đây database TỪ CHỐI THI HÀNH kể cả khi ai
-- đó gõ SQL trực tiếp bằng tài khoản quản trị.
--
-- Cùng cách làm với sổ cái tài chính — xem docs/adr/0008-financial-ledger.md.
--
-- Dùng TRIGGER báo lỗi thay vì RULE ... DO INSTEAD NOTHING: im lặng bỏ qua
-- sẽ khiến người sửa tưởng đã thành công và đi tiếp với giả định sai.
-- Báo lỗi tường minh dừng họ lại đúng chỗ.
CREATE OR REPLACE FUNCTION price_history_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'price_history là bảng bất biến: không được % (bản ghi %)',
        TG_OP, COALESCE(OLD.id, '?')
        USING HINT = 'Ghi thêm bản ghi mới thay vì sửa bản ghi cũ';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER price_history_no_update
    BEFORE UPDATE ON price_history
    FOR EACH ROW EXECUTE FUNCTION price_history_immutable();

CREATE TRIGGER price_history_no_delete
    BEFORE DELETE ON price_history
    FOR EACH ROW EXECUTE FUNCTION price_history_immutable();
