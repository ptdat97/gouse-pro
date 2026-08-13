-- Module: seller — danh tính và chính sách nhà bán.
--
-- Module này KHÔNG sở hữu offer, đơn hàng, hay tiền. Muốn biết số dư →
-- gọi payment.GetBalance(). Không lưu trùng: hai nơi cùng lưu một sự thật
-- thì sớm muộn chúng lệch nhau, và khi lệch thì không biết bên nào đúng.

CREATE TABLE seller (
    id         TEXT PRIMARY KEY CHECK (id LIKE 'sel\_%' AND length(id) = 30),
    name       TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug       TEXT NOT NULL UNIQUE CHECK (length(trim(slug)) > 0),

    -- Bốn loại khác nhau ở CHÍNH SÁCH, không ở CẤU TRÚC (mục 4 của đặc tả).
    -- Một bảng, phân biệt bằng cột này: seller cá nhân phát triển thành
    -- local brand chỉ là đổi thuộc tính, không phải di trú dữ liệu.
    --
    -- INTERNAL là own brand của nền tảng — một seller nội bộ, không phải
    -- đường đi riêng. Nhờ vậy đơn lẫn own brand và hàng seller đi CHUNG
    -- một luồng.
    seller_type TEXT NOT NULL CHECK (seller_type IN (
        'INTERNAL', 'INDIVIDUAL', 'BUSINESS', 'LOCAL_BRAND', 'STRATEGIC_PARTNER'
    )),

    status     TEXT NOT NULL CHECK (status IN (
        'APPLIED', 'PENDING_REVIEW', 'APPROVED', 'ACTIVE',
        'REJECTED', 'SUSPENDED', 'ON_VACATION', 'TERMINATED'
    )),

    legal_name TEXT NOT NULL DEFAULT '',
    tax_code   TEXT NOT NULL DEFAULT '',
    email      TEXT NOT NULL DEFAULT '',
    phone      TEXT NOT NULL DEFAULT '',

    -- Tỷ lệ hoa hồng theo phần vạn (1000 = 10%). KHÔNG dùng số thực: sai
    -- số dấu phẩy động tích lũy thành lệch đối soát.
    commission_rate INT NOT NULL DEFAULT 0
        CHECK (commission_rate BETWEEN 0 AND 10000),

    -- QUY TẮC 1: nhà bán ACTIVE phải có tài khoản ngân hàng đã xác minh.
    --
    -- KHÔNG lưu số tài khoản ở bảng này — thông tin ngân hàng thuộc bảng
    -- riêng và không bao giờ được ghi log.
    bank_account_verified BOOLEAN NOT NULL DEFAULT FALSE,

    suspension_reason TEXT NOT NULL DEFAULT '',
    approved_by       TEXT NOT NULL DEFAULT '',
    approved_at       TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    -- Own brand KHÔNG chịu hoa hồng: nền tảng không thu của chính mình.
    -- Doanh thu own brand ghi toàn phần ở tầng ledger.
    CONSTRAINT seller_internal_no_commission CHECK (
        seller_type <> 'INTERNAL' OR commission_rate = 0
    ),

    -- QUY TẮC 1 ở tầng database — chốt chặn cuối.
    --
    -- Không có tài khoản đã xác minh thì seller bán được hàng nhưng không
    -- nhận được tiền. INTERNAL được miễn: own brand nhận tiền qua sổ cái
    -- nội bộ, không qua chuyển khoản.
    CONSTRAINT seller_active_needs_bank CHECK (
        status <> 'ACTIVE' OR seller_type = 'INTERNAL' OR bank_account_verified
    ),

    -- Đình chỉ/từ chối/chấm dứt phải có lý do: seller cần biết vì sao để
    -- khắc phục, và nền tảng cần vết để trả lời khi có tranh chấp.
    CONSTRAINT seller_negative_status_needs_reason CHECK (
        status NOT IN ('REJECTED', 'SUSPENDED', 'TERMINATED')
        OR length(trim(suspension_reason)) > 0
    )
);

CREATE INDEX seller_status_idx ON seller (status);

-- Chỉ mục có điều kiện: truy vấn "seller đang hoạt động" là truy vấn nóng
-- nhất (marketplace kiểm tra trước khi hiển thị offer).
CREATE INDEX seller_active_idx ON seller (id) WHERE status = 'ACTIVE';
