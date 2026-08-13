-- Module: marketplace — Offer, buy box, hoa hồng.
--
-- Offer là ĐƠN VỊ KHÁCH THỰC SỰ MUA: khách chọn mua từ một nhà bán cụ thể
-- với giá cụ thể. Xem docs/adr/0007-marketplace-order-model.md.

CREATE TABLE offer (
    id        TEXT PRIMARY KEY CHECK (id LIKE 'off\_%' AND length(id) = 30),

    -- Tham chiếu VƯỢT MODULE — không có REFERENCES.
    sku_id    TEXT NOT NULL,
    seller_id TEXT NOT NULL,

    -- Tiền: BIGINT + CHAR(3), KHÔNG BAO GIỜ dùng FLOAT.
    price_amount      BIGINT NOT NULL CHECK (price_amount > 0),
    price_currency    CHAR(3) NOT NULL,
    compare_at_amount BIGINT NOT NULL DEFAULT 0 CHECK (compare_at_amount >= 0),

    condition TEXT NOT NULL DEFAULT 'NEW'
        CHECK (condition IN ('NEW', 'USED_LIKE_NEW', 'USED_GOOD')),

    handling_time_hours INT NOT NULL DEFAULT 24 CHECK (handling_time_hours > 0),

    min_order_quantity INT NOT NULL DEFAULT 1 CHECK (min_order_quantity > 0),
    -- 0 nghĩa là không giới hạn.
    max_order_quantity INT NOT NULL DEFAULT 0 CHECK (max_order_quantity >= 0),

    status TEXT NOT NULL CHECK (
        status IN ('DRAFT', 'ACTIVE', 'OUT_OF_STOCK', 'SUSPENDED', 'ARCHIVED')
    ),

    -- Khóa lạc quan: seller sửa offer rất thường xuyên, hai lần sửa đồng
    -- thời không được ghi đè nhau âm thầm.
    version BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    -- Giá gạch ngang phải CAO HƠN giá bán, nếu không trang hiển thị "giảm -20%".
    CONSTRAINT offer_compare_at_higher CHECK (
        compare_at_amount = 0 OR compare_at_amount > price_amount
    ),

    CONSTRAINT offer_quantity_range CHECK (
        max_order_quantity = 0 OR max_order_quantity >= min_order_quantity
    )
);

-- QUY TẮC 1 (mục 11): một seller chỉ có MỘT offer ACTIVE cho một SKU.
--
-- Chỉ mục duy nhất CÓ ĐIỀU KIỆN thực thi bất biến này ở tầng database.
-- Hai offer ACTIVE cùng lúc thì không biết giá nào là giá thật, và buy box
-- chọn nhầm sẽ bán với giá seller không định.
--
-- Kiểm tra ở tầng ứng dụng KHÔNG chặn được hai request đồng thời.
CREATE UNIQUE INDEX offer_active_uniq
    ON offer (sku_id, seller_id) WHERE status = 'ACTIVE';

-- Truy vấn nóng nhất: lấy mọi offer đang bán của một SKU để chọn buy box.
CREATE INDEX offer_sku_active_idx ON offer (sku_id) WHERE status = 'ACTIVE';

CREATE INDEX offer_seller_idx ON offer (seller_id);

-- ---------------------------------------------------------------------
-- Lịch sử giá offer — BẤT BIẾN.
-- ---------------------------------------------------------------------
--
-- Quy tắc 5 (mục 11): lưu lịch sử mọi lần đổi giá.
--
-- Cần cho việc phát hiện thao túng giá (tăng giá rồi giảm để giả vờ khuyến
-- mãi) và cho phân tích cạnh tranh. Sửa được thì mất cả hai khả năng đó.
CREATE TABLE offer_price_history (
    id       TEXT PRIMARY KEY CHECK (id LIKE 'off\_%'),
    offer_id TEXT NOT NULL REFERENCES offer (id),
    sku_id   TEXT NOT NULL,
    seller_id TEXT NOT NULL,

    price_amount      BIGINT NOT NULL CHECK (price_amount > 0),
    price_currency    CHAR(3) NOT NULL,
    compare_at_amount BIGINT NOT NULL DEFAULT 0,

    changed_by  TEXT NOT NULL DEFAULT '',
    recorded_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX offer_price_history_offer_idx
    ON offer_price_history (offer_id, recorded_at DESC);
CREATE INDEX offer_price_history_sku_idx
    ON offer_price_history (sku_id, recorded_at DESC);

CREATE OR REPLACE FUNCTION offer_price_history_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'offer_price_history là bảng bất biến: không được % (bản ghi %)',
        TG_OP, COALESCE(OLD.id, '?')
        USING HINT = 'Ghi thêm bản ghi mới thay vì sửa bản ghi cũ';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER offer_price_history_no_update
    BEFORE UPDATE ON offer_price_history
    FOR EACH ROW EXECUTE FUNCTION offer_price_history_immutable();

CREATE TRIGGER offer_price_history_no_delete
    BEFORE DELETE ON offer_price_history
    FOR EACH ROW EXECUTE FUNCTION offer_price_history_immutable();
