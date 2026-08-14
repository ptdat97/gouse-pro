-- Module: identity — tài khoản đăng nhập, xác thực, vai trò.
--
-- RANH GIỚI QUAN TRỌNG NHẤT (identity.md mục 2):
--
--     identity trả lời:  "Đây là user_id=123, có vai trò SELLER"
--                        → hạ tầng, TRUNG LẬP với domain
--
--     module order:      "user_id=123 có được xem order #1000 không?"
--                        → quyết định NGHIỆP VỤ
--
-- Nếu identity phải biết mọi quy tắc truy cập dữ liệu, nó sẽ phụ thuộc
-- toàn hệ thống — vi phạm nguyên tắc P12.
--
-- TÁCH User KHỎI HỒ SƠ NGHIỆP VỤ:
--
--     User (identity)
--       ├── Customer profile  (module customer)
--       ├── Seller profile    (module seller)
--       └── Creator profile   (module creator)
--
-- Một người có thể vừa là khách, vừa là creator, vừa là seller — ví dụ
-- một KOC bán hàng trên sàn, làm nội dung affiliate, và mua sắm cho bản
-- thân. Gộp User với Customer thì không mô hình hóa được.

CREATE TABLE "user" (
    id TEXT PRIMARY KEY CHECK (id LIKE 'usr\_%' AND length(id) = 30),

    -- Email là định danh đăng nhập.
    --
    -- CITEXT sẽ tiện hơn nhưng cần extension; dùng TEXT và CHUẨN HÓA VỀ
    -- CHỮ THƯỜNG ở tầng ứng dụng. Khách gõ "Khach@Example.com" và
    -- "khach@example.com" phải vào cùng một tài khoản — không thì họ tạo
    -- hai tài khoản rồi không hiểu vì sao mất đơn hàng cũ.
    email TEXT NOT NULL UNIQUE CHECK (email = lower(email)),

    phone TEXT NOT NULL DEFAULT '',

    -- Tên hiển thị. KHÔNG phải hồ sơ nghiệp vụ — địa chỉ, ngày sinh, số
    -- đo cơ thể thuộc module customer.
    display_name TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN (
        'ACTIVE',
        'SUSPENDED',   -- bị khóa, không đăng nhập được
        'DELETED'      -- đã xóa (mềm) — không xóa cứng vì có tham chiếu
    )),

    email_verified_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX user_status_idx ON "user" (status) WHERE status <> 'ACTIVE';

-- ---------------------------------------------------------------------
-- Mật khẩu — BẢNG RIÊNG, không nằm trong "user".
-- ---------------------------------------------------------------------
--
-- VÌ SAO TÁCH BẢNG: truy vấn thông tin người dùng xảy ra ở khắp nơi
-- (hiển thị tên, gửi email). Nếu hash nằm cùng bảng, mỗi lần `SELECT *`
-- là một lần hash đi qua tầng ứng dụng và có thể lọt vào log.
--
-- Tách bảng khiến việc đọc hash trở thành hành động CÓ CHỦ Ý.
CREATE TABLE user_credential (
    user_id TEXT PRIMARY KEY REFERENCES "user" (id),

    -- password_hash là chuỗi bcrypt đầy đủ, đã gồm muối và chi phí.
    --
    -- KHÔNG BAO GIỜ ghi cột này ra log. Xem docs/09-operations/security.md.
    password_hash TEXT NOT NULL CHECK (length(password_hash) > 0),

    -- Đổi mật khẩu phải thu hồi mọi phiên đang mở: nếu tài khoản bị lộ,
    -- đổi mật khẩu mà phiên cũ vẫn sống thì kẻ tấn công vẫn vào được.
    password_changed_at TIMESTAMPTZ NOT NULL,

    -- Chống thử vét cạn.
    failed_attempts   INT NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    locked_until      TIMESTAMPTZ,

    updated_at TIMESTAMPTZ NOT NULL
);

-- ---------------------------------------------------------------------
-- Vai trò và quyền.
-- ---------------------------------------------------------------------
--
-- Quyền không chỉ là "được làm gì" mà còn "TRÊN PHẠM VI NÀO":
--
--     Khách hàng:     order.read scope=OWN     (chỉ đơn của mình)
--     Seller:         order.read scope=SELLER  (chỉ đơn thuộc gian hàng)
--     Nhân viên CSKH: order.read scope=ALL
--
-- QUAN TRỌNG: phạm vi do identity CUNG CẤP, nhưng việc ÁP DỤNG nó vào
-- truy vấn là trách nhiệm của module sở hữu dữ liệu. Module fulfillment
-- lọc theo seller_id trong SQL của chính nó — identity không biết bảng
-- fulfillment_order tồn tại.
CREATE TABLE user_role (
    user_id TEXT NOT NULL REFERENCES "user" (id),

    role TEXT NOT NULL CHECK (role IN (
        'CUSTOMER',
        'SELLER_OWNER',       -- chủ gian hàng
        'SELLER_STAFF',       -- nhân viên gian hàng
        'CREATOR',
        'ADMIN',              -- quản trị nền tảng
        'OPS_WAREHOUSE',
        'OPS_MERCHANDISING',
        'OPS_FINANCE',
        'OPS_SUPPORT'
    )),

    -- scope_id là phạm vi cụ thể: seller_id với vai trò SELLER_*.
    --
    -- Rỗng với vai trò không gắn thực thể nào (CUSTOMER, ADMIN).
    scope_id TEXT NOT NULL DEFAULT '',

    granted_at TIMESTAMPTZ NOT NULL,
    granted_by TEXT NOT NULL DEFAULT '',

    PRIMARY KEY (user_id, role, scope_id)
);

CREATE INDEX user_role_role_idx ON user_role (role, scope_id);

-- ---------------------------------------------------------------------
-- Phiên đăng nhập.
-- ---------------------------------------------------------------------
--
-- Refresh token PHẢI THU HỒI ĐƯỢC: khi tài khoản bị lộ, người dùng cần
-- đăng xuất mọi thiết bị. Token tự chứa (JWT thuần) không làm được điều
-- đó — vì vậy refresh token là bản ghi trong bảng này.
CREATE TABLE session (
    id TEXT PRIMARY KEY CHECK (id LIKE 'ses\_%' AND length(id) = 30),

    user_id TEXT NOT NULL REFERENCES "user" (id),

    -- refresh_token_hash: BĂM chứ không lưu nguyên văn.
    --
    -- Rò rỉ database mà token lưu nguyên văn nghĩa là kẻ tấn công đăng
    -- nhập được vào mọi tài khoản mà không cần mật khẩu.
    refresh_token_hash TEXT NOT NULL UNIQUE,

    -- Ngữ cảnh để người dùng nhận ra phiên nào là của thiết bị nào khi
    -- xem danh sách "các phiên đang đăng nhập".
    user_agent TEXT NOT NULL DEFAULT '',
    ip_hash    TEXT NOT NULL DEFAULT '',

    expires_at TIMESTAMPTZ NOT NULL,

    -- revoked_at khác NULL nghĩa là đã thu hồi.
    --
    -- KHÔNG xóa hàng: cần biết phiên bị thu hồi lúc nào khi điều tra sự cố.
    revoked_at TIMESTAMPTZ,

    created_at    TIMESTAMPTZ NOT NULL,
    last_used_at  TIMESTAMPTZ NOT NULL
);

-- Truy vấn nóng: xác thực refresh token.
CREATE INDEX session_active_idx ON session (user_id)
    WHERE revoked_at IS NULL;

-- Dọn phiên hết hạn.
CREATE INDEX session_expiry_idx ON session (expires_at)
    WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------
-- Nhật ký đăng nhập.
-- ---------------------------------------------------------------------
--
-- Ghi CẢ THÀNH CÔNG LẪN THẤT BẠI. Chỉ ghi thất bại thì không phát hiện
-- được "đăng nhập thành công từ một quốc gia lạ lúc 3 giờ sáng" — loại
-- bất thường nguy hiểm nhất.
CREATE TABLE login_attempt (
    id BIGSERIAL PRIMARY KEY,

    -- email chứ không phải user_id: lần thử với email không tồn tại cũng
    -- phải ghi, vì đó là dấu hiệu dò tài khoản.
    email TEXT NOT NULL,

    user_id TEXT NOT NULL DEFAULT '',

    succeeded BOOLEAN NOT NULL,

    -- failure_reason chỉ dùng cho ĐIỀU TRA, KHÔNG trả về cho client.
    --
    -- Trả "sai mật khẩu" hay "email không tồn tại" cho client là để lộ
    -- tài khoản nào có thật — kẻ tấn công dùng nó để thu hẹp danh sách.
    failure_reason TEXT NOT NULL DEFAULT '',

    -- IP đã BĂM: lưu nguyên văn là dữ liệu cá nhân, cần cơ sở pháp lý.
    ip_hash    TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',

    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX login_attempt_email_idx ON login_attempt (email, attempted_at DESC);

-- Phát hiện bất thường: nhiều lần thất bại trong thời gian ngắn.
CREATE INDEX login_attempt_failed_idx ON login_attempt (attempted_at DESC)
    WHERE succeeded = false;
