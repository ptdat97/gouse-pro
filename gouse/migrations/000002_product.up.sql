-- Module: product — Product → Variant → SKU.
--
-- KHÔNG có khóa ngoại tới brand, category, size_chart: chúng thuộc module
-- catalog. Tham chiếu chỉ bằng định danh (docs/05-data/data-model.md mục 3.2).
--
-- Đánh đổi đã chấp nhận: database không chặn được việc trỏ tới brand_id
-- không tồn tại. Việc kiểm tra nằm ở tầng application (CatalogPort). Bù
-- lại, hai module tách thành service riêng được mà không phải gỡ khóa ngoại.

CREATE TABLE product (
    id                   TEXT PRIMARY KEY CHECK (id LIKE 'prd\_%' AND length(id) = 30),

    -- Tham chiếu VƯỢT MODULE — không có REFERENCES.
    brand_id             TEXT NOT NULL,
    collection_id        TEXT NOT NULL DEFAULT '',
    category_id          TEXT NOT NULL,
    size_chart_id        TEXT NOT NULL DEFAULT '',

    name                 TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug                 TEXT NOT NULL UNIQUE,
    description          TEXT NOT NULL DEFAULT '',

    -- Ba trường đặc thù thời trang. Ảnh hưởng TRỰC TIẾP tỷ lệ hoàn hàng,
    -- nên là cột riêng chứ không nhét vào JSONB thuộc tính.
    care_instructions    TEXT NOT NULL DEFAULT '',
    material_composition TEXT NOT NULL DEFAULT '',
    origin_country       TEXT NOT NULL DEFAULT '',

    product_type         TEXT NOT NULL CHECK (
        product_type IN ('TOP', 'BOTTOM', 'DRESS', 'OUTERWEAR', 'SHOES', 'BAG', 'ACCESSORY')
    ),
    gender_target        TEXT NOT NULL CHECK (gender_target IN ('MEN', 'WOMEN', 'UNISEX', 'KIDS')),

    status               TEXT NOT NULL CHECK (
        status IN ('DRAFT', 'PENDING_REVIEW', 'ACTIVE', 'INACTIVE', 'ARCHIVED')
    ),
    rejection_reason     TEXT NOT NULL DEFAULT '',

    -- Rỗng nghĩa là danh mục chuẩn của nền tảng.
    created_by_seller_id TEXT NOT NULL DEFAULT '',

    images               TEXT[] NOT NULL DEFAULT '{}',

    -- NULL khi chưa từng xuất bản. Đây là chỗ NULL đúng nghĩa: "chưa xảy ra"
    -- khác với "xảy ra lúc 0".
    published_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,

    -- Sản phẩm ACTIVE phải có mô tả và chất liệu (quy tắc 3, 4 mục 12).
    -- Ràng buộc ở tầng database là chốt chặn cuối: kể cả khi có đường đi
    -- nào đó bỏ qua kiểm tra ở domain, dữ liệu hỏng vẫn không vào được.
    CONSTRAINT product_active_requires_content CHECK (
        status <> 'ACTIVE'
        OR (length(trim(description)) > 0 AND length(trim(material_composition)) > 0)
    ),

    -- Đã xuất bản thì phải có mốc thời gian.
    CONSTRAINT product_published_has_timestamp CHECK (
        status NOT IN ('ACTIVE', 'INACTIVE') OR published_at IS NOT NULL
    )
);

-- Lọc theo seller là truy vấn của MỌI trang Seller Center. Chỉ mục có điều
-- kiện vì phần lớn sản phẩm là của nền tảng (seller_id rỗng).
CREATE INDEX product_seller_idx ON product (created_by_seller_id)
    WHERE created_by_seller_id <> '';

CREATE INDEX product_brand_idx ON product (brand_id);
CREATE INDEX product_category_idx ON product (category_id);

CREATE INDEX product_collection_idx ON product (collection_id)
    WHERE collection_id <> '';

-- Truy vấn storefront: chỉ sản phẩm đang hiển thị. Chỉ mục có điều kiện
-- nhỏ hơn nhiều so với chỉ mục trên toàn bảng.
CREATE INDEX product_visible_idx ON product (created_at DESC)
    WHERE status = 'ACTIVE';

CREATE TABLE variant (
    id            TEXT PRIMARY KEY CHECK (id LIKE 'var\_%'),
    product_id    TEXT NOT NULL REFERENCES product (id) ON DELETE CASCADE,

    -- attributes là tổ hợp thuộc tính: {"color": "Trắng", "size": "M"}.
    -- JSONB hợp lý ở đây: khóa khác nhau theo loại sản phẩm (áo có độ dài
    -- tay, giày không có), không thể cố định thành cột.
    attributes    JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- attribute_key là chuỗi chuẩn hóa của tổ hợp, dùng để chặn trùng.
    -- Sinh ở tầng ứng dụng (Variant.AttributeKey) vì cần chuẩn hóa chữ
    -- hoa/thường và sắp xếp khóa — logic đó thuộc domain.
    attribute_key TEXT NOT NULL,

    images        TEXT[] NOT NULL DEFAULT '{}',
    display_order INT NOT NULL DEFAULT 0,
    status        TEXT NOT NULL CHECK (status IN ('ACTIVE', 'INACTIVE')),

    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,

    -- Quy tắc 2 (mục 12): không trùng tổ hợp thuộc tính trong một Product.
    --
    -- Ràng buộc này ở tầng database mới thực sự chắc: kiểm tra ở domain
    -- không chặn được hai request đồng thời cùng thêm biến thể "Trắng/M".
    CONSTRAINT variant_attributes_uniq UNIQUE (product_id, attribute_key)
);

CREATE INDEX variant_product_idx ON variant (product_id);

CREATE TABLE sku (
    id          TEXT PRIMARY KEY CHECK (id LIKE 'sku\_%'),
    variant_id  TEXT NOT NULL REFERENCES variant (id) ON DELETE CASCADE,

    -- Quy tắc 1 (mục 12): sku_code duy nhất TOÀN HỆ THỐNG.
    --
    -- UNIQUE ở đây là chốt chặn thật. Kiểm tra ở tầng application chỉ cho
    -- thông báo lỗi đẹp hơn; nó KHÔNG chặn được hai request đồng thời.
    sku_code    TEXT NOT NULL UNIQUE CHECK (sku_code = upper(trim(sku_code))),

    barcode     TEXT NOT NULL DEFAULT '',

    -- Cần cho tính phí vận chuyển và xếp kho — không phải thông tin phụ.
    -- Thiếu thì hãng vận chuyển tính theo mặc định, thường cao hơn thực tế.
    weight_gram INT NOT NULL DEFAULT 0 CHECK (weight_gram >= 0),
    length_mm   INT NOT NULL DEFAULT 0 CHECK (length_mm >= 0),
    width_mm    INT NOT NULL DEFAULT 0 CHECK (width_mm >= 0),
    height_mm   INT NOT NULL DEFAULT 0 CHECK (height_mm >= 0),

    status      TEXT NOT NULL CHECK (status IN ('ACTIVE', 'DISCONTINUED')),

    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX sku_variant_idx ON sku (variant_id);
