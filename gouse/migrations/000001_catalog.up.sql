-- Module: catalog — thương hiệu, ủy quyền, bộ sưu tập, danh mục, bảng size.
--
-- QUY ƯỚC ĐỊNH DANH (khác docs/05-data/data-model.md mục 12 một cách có
-- chủ đích): định danh lưu dạng TEXT chứ không phải UUID.
--
-- Lý do: định danh của hệ thống này có TIỀN TỐ LOẠI ("brd_01J9X..."), xem
-- internal/kernel/ids. Tiền tố là thứ ngăn việc truyền nhầm brand_id vào
-- chỗ cần category_id — lỗi mà kiểu UUID thuần không bắt được. Đặc tả
-- OpenAPI cũng công bố định dạng này ra ngoài (pattern ^[a-z]+_[0-9A-HJKMNP-TV-Z]{26}$).
--
-- Đánh đổi: tốn 30 byte/định danh thay vì 16, và không dùng được toán tử
-- UUID sẵn có. Chấp nhận được vì lợi ích an toàn kiểu lớn hơn.
--
-- CHECK độ dài chặn dữ liệu rác lọt vào ngay ở tầng database.

CREATE TABLE brand (
    id                TEXT PRIMARY KEY CHECK (id LIKE 'brd\_%' AND length(id) = 30),
    name              TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug              TEXT NOT NULL UNIQUE CHECK (length(trim(slug)) > 0),
    description       TEXT NOT NULL DEFAULT '',
    logo_url          TEXT NOT NULL DEFAULT '',

    -- Enum lưu TEXT + CHECK, không dùng số: đọc dữ liệu trực tiếp hiểu ngay,
    -- thêm giá trị mới không phải nhớ số nào chưa dùng.
    brand_type        TEXT NOT NULL CHECK (brand_type IN ('OWN', 'THIRD_PARTY')),

    -- Mức bảo vệ thương hiệu — cơ chế chống hàng giả.
    protection_level  TEXT NOT NULL CHECK (protection_level IN ('OPEN', 'VERIFIED_ONLY', 'RESTRICTED')),

    -- Rỗng nghĩa là không thuộc seller nào (thương hiệu của nền tảng).
    owner_seller_id   TEXT NOT NULL DEFAULT '',
    country_of_origin TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL CHECK (status IN ('ACTIVE', 'INACTIVE')),

    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL
);

-- Tra thương hiệu theo slug là truy vấn của MỌI trang thương hiệu.
-- UNIQUE ở trên đã tạo chỉ mục nên không cần thêm.

CREATE INDEX brand_owner_seller_idx ON brand (owner_seller_id)
    WHERE owner_seller_id <> '';

-- Ủy quyền bán thương hiệu — link table có hiệu lực theo thời gian.
--
-- KHÔNG có khóa ngoại tới seller: seller thuộc module khác (quy tắc ở
-- docs/05-data/data-model.md mục 3.2). Tham chiếu chỉ bằng định danh.
CREATE TABLE brand_authorization (
    id           TEXT PRIMARY KEY CHECK (id LIKE 'aut\_%'),

    -- Khóa ngoại tới brand ĐƯỢC PHÉP: cùng module catalog.
    brand_id     TEXT NOT NULL REFERENCES brand (id),

    -- seller_id KHÔNG có khóa ngoại — khác module.
    seller_id    TEXT NOT NULL,

    status       TEXT NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'REVOKED')),
    document_url TEXT NOT NULL DEFAULT '',

    valid_from   TIMESTAMPTZ NOT NULL,
    valid_until  TIMESTAMPTZ NOT NULL,

    -- Ai duyệt và duyệt lúc nào — cần cho truy vết khi có tranh chấp về
    -- hàng giả. NULL khi chưa được duyệt.
    approved_by  TEXT NOT NULL DEFAULT '',
    approved_at  TIMESTAMPTZ,

    created_at   TIMESTAMPTZ NOT NULL,

    -- Khoảng hiệu lực phải hợp lệ. Không có ràng buộc này thì một bản ghi
    -- hết hạn trước khi bắt đầu sẽ nằm im trong bảng và không ai hiểu vì
    -- sao seller không bán được.
    CONSTRAINT brand_authorization_period_valid CHECK (valid_from < valid_until)
);

-- Một seller chỉ có MỘT ủy quyền đang hiệu lực cho một thương hiệu.
-- Hai bản ghi APPROVED cùng lúc sẽ khiến việc thu hồi không dứt điểm.
CREATE UNIQUE INDEX brand_authorization_active_uniq
    ON brand_authorization (brand_id, seller_id)
    WHERE status = 'APPROVED';

CREATE INDEX brand_authorization_seller_idx ON brand_authorization (seller_id);

-- Tìm ủy quyền sắp hết hạn để nhắc gia hạn.
CREATE INDEX brand_authorization_expiry_idx ON brand_authorization (valid_until)
    WHERE status = 'APPROVED';

CREATE TABLE collection (
    id                 TEXT PRIMARY KEY CHECK (id LIKE 'col\_%'),
    brand_id           TEXT NOT NULL REFERENCES brand (id),

    name               TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug               TEXT NOT NULL UNIQUE,
    season             TEXT NOT NULL DEFAULT '',
    theme              TEXT NOT NULL DEFAULT '',

    launch_date        TIMESTAMPTZ NOT NULL,
    end_of_season_date TIMESTAMPTZ NOT NULL,

    status             TEXT NOT NULL CHECK (status IN ('PLANNING', 'ACTIVE', 'ENDING', 'ARCHIVED')),

    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT collection_period_valid CHECK (launch_date < end_of_season_date)
);

CREATE INDEX collection_brand_idx ON collection (brand_id);

-- Tìm bộ sưu tập tới hạn ra mắt / kết thúc mùa (worker chạy định kỳ).
CREATE INDEX collection_launch_idx ON collection (launch_date)
    WHERE status = 'PLANNING';
CREATE INDEX collection_ending_idx ON collection (end_of_season_date)
    WHERE status = 'ACTIVE';

CREATE TABLE category (
    id            TEXT PRIMARY KEY CHECK (id LIKE 'cat\_%'),

    -- Rỗng nghĩa là danh mục gốc. Dùng chuỗi rỗng thay vì NULL để tránh
    -- ba trạng thái (có giá trị / rỗng / NULL) — hai là đủ.
    parent_id     TEXT NOT NULL DEFAULT '',

    name          TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug          TEXT NOT NULL UNIQUE,
    depth         INT NOT NULL CHECK (depth >= 0),
    display_order INT NOT NULL DEFAULT 0,
    status        TEXT NOT NULL CHECK (status IN ('ACTIVE', 'INACTIVE')),

    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,

    -- Danh mục không được là cha của chính nó.
    CONSTRAINT category_not_self_parent CHECK (parent_id <> id)
);

CREATE INDEX category_parent_idx ON category (parent_id);

CREATE TABLE size_chart (
    id           TEXT PRIMARY KEY CHECK (id LIKE 'szc\_%'),
    brand_id     TEXT NOT NULL REFERENCES brand (id),

    product_type TEXT NOT NULL CHECK (
        product_type IN ('TOP', 'BOTTOM', 'DRESS', 'OUTERWEAR', 'SHOES', 'BAG', 'ACCESSORY')
    ),
    system       TEXT NOT NULL CHECK (
        system IN ('ALPHA', 'NUMERIC', 'EU', 'US', 'UK', 'JP', 'FREE')
    ),
    note         TEXT NOT NULL DEFAULT '',

    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,

    -- Một thương hiệu chỉ có MỘT bảng size cho mỗi loại sản phẩm.
    -- Hai bảng cùng (brand, product_type) thì không biết dùng bảng nào,
    -- và khách sẽ thấy số đo khác nhau ở các trang khác nhau.
    CONSTRAINT size_chart_brand_type_uniq UNIQUE (brand_id, product_type)
);

-- Dòng trong bảng size.
--
-- Bảng riêng thay vì JSONB: số đo được truy vấn và so sánh (tìm size vừa
-- theo số đo khách), và JSONB làm việc đó chậm hơn nhiều.
CREATE TABLE size_chart_entry (
    size_chart_id TEXT NOT NULL REFERENCES size_chart (id) ON DELETE CASCADE,
    size          TEXT NOT NULL,

    -- measurements linh hoạt theo loại sản phẩm: áo có chest_cm, quần có
    -- waist_cm, giày có foot_length_cm. Đây là trường hợp JSONB hợp lý —
    -- khóa không cố định và không cần ràng buộc từng khóa.
    measurements  JSONB NOT NULL DEFAULT '{}'::jsonb,

    display_order INT NOT NULL DEFAULT 0,

    PRIMARY KEY (size_chart_id, size)
);
