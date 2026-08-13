-- Module: inventory — tồn kho theo SKU × địa điểm × chủ sở hữu.
--
-- KHÔNG có khóa ngoại tới sku: sku thuộc module product.

CREATE TABLE stock_location (
    id         TEXT PRIMARY KEY CHECK (id LIKE 'loc\_%'),
    name       TEXT NOT NULL CHECK (length(trim(name)) > 0),
    code       TEXT NOT NULL UNIQUE,

    -- PLATFORM: kho của nền tảng. SELLER: kho riêng của seller.
    kind       TEXT NOT NULL CHECK (kind IN ('PLATFORM', 'SELLER')),

    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE inventory_item (
    id                  TEXT PRIMARY KEY CHECK (id LIKE 'inv\_%'),

    -- Tham chiếu VƯỢT MODULE — không có REFERENCES.
    sku_id              TEXT NOT NULL,

    stock_location_id   TEXT NOT NULL REFERENCES stock_location (id),

    -- Chủ sở hữu: 'own_platform' hoặc định danh seller.
    --
    -- TÁCH KHỎI location là điểm mấu chốt: hàng của seller gửi ở kho nền
    -- tảng vẫn THUỘC SỞ HỮU seller, không được ghi nhận là tài sản của nền
    -- tảng. Xem docs/04-modules/inventory.md mục 3.1.
    inventory_owner_id  TEXT NOT NULL CHECK (length(trim(inventory_owner_id)) > 0),

    -- SÁU trạng thái. CHECK >= 0 ở tầng database là LỚP BẢO VỆ CUỐI CÙNG:
    -- kể cả khi có lỗi logic ở tầng ứng dụng, database vẫn từ chối số âm.
    --
    -- Chỉ báo "số SKU có tồn kho âm" phải LUÔN bằng 0 (mục 13 của đặc tả).
    quantity_available  INT NOT NULL DEFAULT 0 CHECK (quantity_available >= 0),
    quantity_reserved   INT NOT NULL DEFAULT 0 CHECK (quantity_reserved >= 0),
    quantity_committed  INT NOT NULL DEFAULT 0 CHECK (quantity_committed >= 0),
    quantity_in_transit INT NOT NULL DEFAULT 0 CHECK (quantity_in_transit >= 0),
    quantity_damaged    INT NOT NULL DEFAULT 0 CHECK (quantity_damaged >= 0),
    quantity_returned   INT NOT NULL DEFAULT 0 CHECK (quantity_returned >= 0),

    production_batch_id TEXT NOT NULL DEFAULT '',

    -- version cho KHÓA LẠC QUAN.
    --
    -- Mỗi lần ghi thành công tăng 1. Câu UPDATE mang theo version đã đọc;
    -- nếu không khớp nghĩa là có tiến trình khác vừa sửa và thao tác bị
    -- từ chối. Xem mục 5.2 và infrastructure/postgres/item.go.
    version             BIGINT NOT NULL DEFAULT 0,

    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,

    -- Khóa định danh nghiệp vụ: một bản ghi cho mỗi tổ hợp.
    --
    -- Thiếu ràng buộc này thì cùng một SKU ở cùng một kho có thể có hai
    -- bản ghi, và tổng tồn kho tính ra sai mà không ai biết.
    CONSTRAINT inventory_item_key_uniq
        UNIQUE (sku_id, stock_location_id, inventory_owner_id)
);

CREATE INDEX inventory_item_sku_idx ON inventory_item (sku_id);
CREATE INDEX inventory_item_owner_idx ON inventory_item (inventory_owner_id);

-- Chỉ mục có điều kiện cho cảnh báo sắp hết hàng: nhỏ hơn nhiều so với
-- chỉ mục trên toàn bảng vì phần lớn SKU không ở trạng thái sắp hết.
CREATE INDEX inventory_item_low_stock_idx ON inventory_item (sku_id)
    WHERE quantity_available <= 10;

CREATE TABLE reservation (
    id                TEXT PRIMARY KEY CHECK (id LIKE 'rsv\_%'),
    inventory_item_id TEXT NOT NULL REFERENCES inventory_item (id),

    -- checkout_id vượt module — không có khóa ngoại.
    checkout_id       TEXT NOT NULL DEFAULT '',

    quantity          INT NOT NULL CHECK (quantity > 0),

    -- Quy tắc 5 (mục 12): reservation PHẢI có thời hạn.
    --
    -- NOT NULL cưỡng chế điều đó ở tầng database: không có đường nào tạo
    -- được reservation giữ hàng vĩnh viễn.
    expires_at        TIMESTAMPTZ NOT NULL,

    status            TEXT NOT NULL CHECK (
        status IN ('ACTIVE', 'CONVERTED', 'EXPIRED', 'RELEASED')
    ),
    extensions        INT NOT NULL DEFAULT 0 CHECK (extensions >= 0),

    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL
);

-- Tiến trình nền tìm reservation quá hạn. Chỉ mục có điều kiện vì chỉ
-- bản ghi ACTIVE mới cần dọn.
CREATE INDEX reservation_expiry_idx ON reservation (expires_at)
    WHERE status = 'ACTIVE';

CREATE INDEX reservation_checkout_idx ON reservation (checkout_id)
    WHERE checkout_id <> '';

CREATE INDEX reservation_item_idx ON reservation (inventory_item_id);

-- ---------------------------------------------------------------------
-- Nhật ký biến động — BẤT BIẾN, chỉ ghi thêm.
-- ---------------------------------------------------------------------
--
-- Quy tắc 4 (mục 12): mọi biến động ghi vào đây.
--
-- Nhật ký cho phép tái dựng trạng thái tại bất kỳ thời điểm nào và điều
-- tra khi có sai lệch kiểm kê. Sửa được thì mất cả hai khả năng đó.
CREATE TABLE inventory_movement (
    id                TEXT PRIMARY KEY CHECK (id LIKE 'imv\_%'),
    inventory_item_id TEXT NOT NULL REFERENCES inventory_item (id),
    sku_id            TEXT NOT NULL,

    movement_type     TEXT NOT NULL CHECK (movement_type IN (
        'RECEIVE', 'RESERVE', 'RELEASE', 'COMMIT', 'UNCOMMIT', 'SHIP',
        'RETURN', 'INSPECT_PASS', 'INSPECT_FAIL', 'DAMAGE',
        'TRANSFER_OUT', 'TRANSFER_IN', 'ADJUST'
    )),

    -- LUÔN dương. Hướng biến động nằm ở movement_type, không nằm ở dấu —
    -- dấu âm rất dễ đọc nhầm khi cộng dồn báo cáo.
    quantity          INT NOT NULL CHECK (quantity > 0),

    -- Số lượng khả dụng SAU biến động, để đối chiếu với cộng dồn nhật ký.
    quantity_after    INT NOT NULL CHECK (quantity_after >= 0),

    reason            TEXT NOT NULL DEFAULT '',
    performed_by      TEXT NOT NULL DEFAULT '',
    reference_id      TEXT NOT NULL DEFAULT '',

    occurred_at       TIMESTAMPTZ NOT NULL,

    -- Quy tắc 7: điều chỉnh thủ công phải có lý do.
    --
    -- Điều chỉnh không lý do là điểm mù trong kiểm toán — không phân biệt
    -- được sai sót kiểm kê với thất thoát.
    CONSTRAINT inventory_movement_adjust_needs_reason CHECK (
        movement_type NOT IN ('ADJUST', 'DAMAGE') OR length(trim(reason)) > 0
    )
);

CREATE INDEX inventory_movement_item_idx ON inventory_movement (inventory_item_id, occurred_at DESC);
CREATE INDEX inventory_movement_sku_time_idx ON inventory_movement (sku_id, occurred_at DESC);

-- Trigger chặn sửa/xóa nhật ký. Cùng cách làm với price_history và sổ cái.
CREATE OR REPLACE FUNCTION inventory_movement_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'inventory_movement là nhật ký bất biến: không được % (bản ghi %)',
        TG_OP, COALESCE(OLD.id, '?')
        USING HINT = 'Ghi thêm bản ghi điều chỉnh thay vì sửa bản ghi cũ';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER inventory_movement_no_update
    BEFORE UPDATE ON inventory_movement
    FOR EACH ROW EXECUTE FUNCTION inventory_movement_immutable();

CREATE TRIGGER inventory_movement_no_delete
    BEFORE DELETE ON inventory_movement
    FOR EACH ROW EXECUTE FUNCTION inventory_movement_immutable();
