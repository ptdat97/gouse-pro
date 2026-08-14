-- Module: customer — hồ sơ khách hàng, sổ địa chỉ, wishlist, đồng ý.
--
-- RANH GIỚI VỚI identity (customer.md mục 2):
--
--     identity  "ai đang gọi, họ được phép làm gì"   → hạ tầng
--     customer  "người này là ai"                    → nghiệp vụ
--
-- Cầu nối là user_id. customer GIỮ user_id; identity KHÔNG biết customer
-- tồn tại. Một chiều, không tạo vòng.
--
-- VÌ SAO KHÁCH VÃNG LAI CÓ HÀNG TRONG BẢNG NÀY:
--
-- Khách chưa đăng ký vẫn đặt hàng được (quy tắc 1). Đơn hàng phải trỏ tới
-- một customer_id — không thì không có chỗ nào giữ địa chỉ giao hàng và
-- lịch sử mua. Vì vậy user_id ĐỂ TRỐNG với khách vãng lai, và được điền
-- khi họ đăng ký.

CREATE TABLE customer (
    id TEXT PRIMARY KEY CHECK (id LIKE 'cus\_%' AND length(id) = 30),

    -- user_id RỖNG nghĩa là khách vãng lai.
    --
    -- KHÔNG có khóa ngoại tới "user": đó là bảng của module khác, và khóa
    -- ngoại vượt ranh giới module biến hai module thành một (quy tắc R2).
    user_id TEXT NOT NULL DEFAULT '',

    -- email là thứ DUY NHẤT nối khách vãng lai với tài khoản sau này.
    --
    -- Chuẩn hóa về chữ thường như identity — hai module lưu email khác
    -- định dạng thì không bao giờ gộp được danh tính.
    email TEXT NOT NULL CHECK (email = lower(email)),

    phone        TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',

    -- Bốn TRẠNG THÁI của MỘT khái niệm, không phải bốn entity.
    --
    -- Một người đi từ GUEST qua REGISTERED lên MEMBER mà vẫn giữ nguyên
    -- customer_id và toàn bộ lịch sử mua hàng. Tách thành nhiều bảng thì
    -- mỗi lần lên hạng là một lần chuyển dữ liệu — và mỗi lần chuyển là
    -- một lần có thể mất.
    status TEXT NOT NULL DEFAULT 'GUEST' CHECK (status IN (
        'GUEST',        -- chưa đăng ký, đã đặt hàng
        'REGISTERED',   -- đã có tài khoản
        'MEMBER',       -- đã mua hàng
        'VIP',
        'ANONYMIZED'    -- đã yêu cầu xóa; dữ liệu định danh đã gỡ
    )),

    -- Thống kê cập nhật khi nghe order.completed.
    --
    -- LƯU Ở ĐÂY CÓ CHỦ Ý dù đây là dữ liệu suy ra được: đếm lại từ bảng
    -- order mỗi lần hiển thị hồ sơ là một truy vấn xuyên module cho một
    -- con số hiển thị.
    order_count   INT NOT NULL DEFAULT 0 CHECK (order_count >= 0),
    total_spent   BIGINT NOT NULL DEFAULT 0 CHECK (total_spent >= 0),
    currency      TEXT NOT NULL DEFAULT 'VND',

    -- version cho khóa lạc quan.
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Email DUY NHẤT trong số khách CHƯA ẩn danh.
--
-- Chỉ mục một phần chứ không phải UNIQUE thường: khách đã ẩn danh có email
-- đặt lại thành giá trị giả, và nhiều người cùng ẩn danh sẽ đụng nhau.
CREATE UNIQUE INDEX customer_email_key ON customer (email)
    WHERE status <> 'ANONYMIZED';

-- Tra hồ sơ từ tài khoản đăng nhập — truy vấn nóng nhất.
CREATE UNIQUE INDEX customer_user_id_key ON customer (user_id)
    WHERE user_id <> '';

-- ---------------------------------------------------------------------
-- Sổ địa chỉ.
-- ---------------------------------------------------------------------
--
-- ĐỊA CHỈ Ở ĐÂY KHÁC ĐỊA CHỈ TRONG ĐƠN HÀNG.
--
--     customer_address  địa chỉ HIỆN TẠI, sửa được, xóa được
--     order.shipping_*  bản SAO ĐÓNG BĂNG lúc đặt hàng (P9)
--
-- Khách chuyển nhà rồi sửa sổ địa chỉ thì đơn cũ vẫn phải hiện địa chỉ đã
-- giao tới. Nếu order tham chiếu tới bảng này, mọi đơn cũ sẽ đổi địa chỉ
-- theo — và đối soát với đơn vị vận chuyển sẽ không khớp.
CREATE TABLE customer_address (
    id TEXT PRIMARY KEY CHECK (id LIKE 'adr\_%' AND length(id) = 30),

    customer_id TEXT NOT NULL REFERENCES customer (id),

    -- Người nhận có thể KHÁC chủ tài khoản: mua tặng, giao tới văn phòng.
    recipient_name  TEXT NOT NULL CHECK (length(recipient_name) > 0),
    recipient_phone TEXT NOT NULL CHECK (length(recipient_phone) > 0),

    -- Địa chỉ dạng dòng tự do + đơn vị hành chính.
    --
    -- KHÔNG tách thành số nhà / tên đường: định dạng địa chỉ khác nhau
    -- theo từng nước, và tách sai còn tệ hơn không tách.
    line1     TEXT NOT NULL CHECK (length(line1) > 0),
    line2     TEXT NOT NULL DEFAULT '',
    ward      TEXT NOT NULL DEFAULT '',
    district  TEXT NOT NULL DEFAULT '',
    province  TEXT NOT NULL DEFAULT '',
    postcode  TEXT NOT NULL DEFAULT '',
    country   TEXT NOT NULL DEFAULT 'VN' CHECK (length(country) = 2),

    note TEXT NOT NULL DEFAULT '',

    is_default BOOLEAN NOT NULL DEFAULT false,

    -- XÓA MỀM: đơn hàng cũ không tham chiếu tới đây, nhưng khách vẫn muốn
    -- thấy lại địa chỉ đã dùng khi đặt lại đơn.
    deleted_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- ĐÚNG MỘT địa chỉ mặc định cho mỗi khách — ràng buộc ở DATABASE.
--
-- Kiểm tra ở tầng ứng dụng KHÔNG cứu được: hai request cùng đặt mặc định
-- sẽ cùng đọc thấy "chưa có mặc định nào khác" rồi cùng ghi. Khi đó đơn
-- hàng lấy địa chỉ nào là do thứ tự sắp xếp quyết định.
CREATE UNIQUE INDEX customer_address_default_key
    ON customer_address (customer_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX customer_address_customer_idx
    ON customer_address (customer_id)
    WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------
-- Đồng ý xử lý dữ liệu.
-- ---------------------------------------------------------------------
--
-- BẢNG NÀY LÀ YÊU CẦU PHÁP LÝ, không phải tính năng.
--
-- Ở nhiều thị trường, nền tảng phải CHỨNG MINH ĐƯỢC khách đã đồng ý, vào
-- lúc nào, và ở đâu. Một cột boolean trên bảng customer không chứng minh
-- được gì — nó chỉ nói trạng thái hiện tại.
--
-- Vì vậy bảng này là NHẬT KÝ CHỈ GHI THÊM: mỗi lần đổi ý là một hàng mới.
-- Trạng thái hiện tại là hàng mới nhất của mỗi loại.
CREATE TABLE customer_consent (
    id TEXT PRIMARY KEY CHECK (id LIKE 'cst\_%' AND length(id) = 30),

    customer_id TEXT NOT NULL REFERENCES customer (id),

    consent_type TEXT NOT NULL CHECK (consent_type IN (
        'MARKETING_EMAIL',
        'MARKETING_SMS',
        'DATA_PROCESSING',
        'PERSONALIZATION'
    )),

    granted BOOLEAN NOT NULL,

    -- source là NƠI khách đồng ý: "checkout", "signup_form", "settings".
    --
    -- Bắt buộc không rỗng: "khách đã đồng ý" mà không nói được ở đâu thì
    -- không dùng được làm bằng chứng.
    source TEXT NOT NULL CHECK (length(source) > 0),

    -- Ghi lại NGUYÊN VĂN nội dung khách đã đọc.
    --
    -- Điều khoản thay đổi theo thời gian. Không lưu phiên bản thì không
    -- trả lời được "khách đồng ý với điều gì" — chỉ biết là họ đã bấm.
    policy_version TEXT NOT NULL DEFAULT '',

    ip_hash    TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',

    recorded_at TIMESTAMPTZ NOT NULL
);

-- Truy vấn nóng: "khách này có đồng ý nhận email marketing không".
CREATE INDEX customer_consent_lookup_idx
    ON customer_consent (customer_id, consent_type, recorded_at DESC);

-- ---------------------------------------------------------------------
-- Wishlist.
-- ---------------------------------------------------------------------
--
-- MỘT wishlist mặc định cho mỗi khách ở MVP. Nhiều danh sách có tên
-- ("mùa hè", "quà tặng") là Phase 2 — nhưng bảng tách sẵn từ đầu vì gộp
-- danh sách vào chính bảng item là thứ phải viết lại khi thêm tính năng.
CREATE TABLE wishlist (
    id TEXT PRIMARY KEY CHECK (id LIKE 'wsh\_%' AND length(id) = 30),

    customer_id TEXT NOT NULL REFERENCES customer (id),

    name       TEXT NOT NULL DEFAULT 'Yêu thích',
    is_default BOOLEAN NOT NULL DEFAULT true,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX wishlist_default_key ON wishlist (customer_id)
    WHERE is_default;

CREATE TABLE wishlist_item (
    wishlist_id TEXT NOT NULL REFERENCES wishlist (id),

    -- product_id, KHÔNG phải variant_id.
    --
    -- Khách yêu thích "chiếc áo này", không phải "chiếc áo này size M màu
    -- đen". Lưu theo variant thì hết size M là món đồ biến mất khỏi danh
    -- sách yêu thích.
    product_id TEXT NOT NULL,

    -- variant_id TÙY CHỌN: khách CÓ THỂ nói rõ size mình muốn, và đó là
    -- tín hiệu nhu cầu mạnh hơn hẳn.
    variant_id TEXT NOT NULL DEFAULT '',

    note TEXT NOT NULL DEFAULT '',

    added_at TIMESTAMPTZ NOT NULL,

    -- KHÔNG có id riêng: cặp (wishlist, product, variant) đã là danh tính.
    --
    -- Nhờ vậy "thêm lại món đã có" KHÔNG tạo bản sao — ràng buộc ở
    -- database, không phải ở tầng ứng dụng. Khách bấm tim hai lần là
    -- chuyện thường; hiện hai lần trong danh sách là lỗi.
    PRIMARY KEY (wishlist_id, product_id, variant_id)
);

CREATE INDEX wishlist_item_product_idx ON wishlist_item (product_id);

-- ---------------------------------------------------------------------
-- Nhật ký gộp danh tính.
-- ---------------------------------------------------------------------
--
-- Gộp danh tính là thao tác KHÔNG ĐẢO NGƯỢC ĐƯỢC và chạm tới lịch sử mua
-- hàng của người khác nếu làm sai. Bảng này để trả lời "vì sao đơn hàng
-- này lại thuộc về tài khoản kia".
CREATE TABLE customer_merge_log (
    id BIGSERIAL PRIMARY KEY,

    -- Hồ sơ khách vãng lai BỊ GỘP VÀO hồ sơ đích.
    source_customer_id TEXT NOT NULL,
    target_customer_id TEXT NOT NULL REFERENCES customer (id),

    -- Vì sao được phép gộp: "email_verified" là đường duy nhất ở MVP.
    --
    -- Không xác minh email thì bất kỳ ai đăng ký bằng email người khác
    -- đều đọc được lịch sử mua hàng của họ, kể cả địa chỉ nhà.
    reason TEXT NOT NULL CHECK (length(reason) > 0),

    merged_at TIMESTAMPTZ NOT NULL,

    CHECK (source_customer_id <> target_customer_id)
);

CREATE INDEX customer_merge_log_target_idx
    ON customer_merge_log (target_customer_id);
