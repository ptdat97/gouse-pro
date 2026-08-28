-- Đối soát: gom các khoản RÚT ĐƯỢC của một nhà bán thành một đợt chi trả.
--
-- Xem docs/04-modules/payment.md mục 7.

CREATE TABLE settlement (
    id TEXT PRIMARY KEY CHECK (id LIKE 'stl\_%' AND length(id) = 30),

    seller_id TEXT NOT NULL,

    -- Kỳ đối soát. Mốc CUỐI là thứ quyết định bút toán nào được gom.
    period_start TIMESTAMPTZ NOT NULL,
    period_end   TIMESTAMPTZ NOT NULL,
    CONSTRAINT settlement_period_valid CHECK (period_end > period_start),

    status TEXT NOT NULL CHECK (status IN ('DRAFT', 'CONFIRMED', 'PAID')),

    -- gross_amount là tổng các khoản RÚT ĐƯỢC gom trong đợt này.
    gross_amount BIGINT NOT NULL CHECK (gross_amount >= 0),

    -- deficit_amount là phần ÂM của tài khoản đang chờ, số DƯƠNG.
    --
    -- # Vì sao cần cột riêng
    --
    -- Hoàn tiền thu hồi từ SELLER_PAYABLE (đang chờ). Nhưng khách có thể
    -- xin trả ngày 6 và hàng về ngày 10, khi tiền ĐÃ chuyển sang rút
    -- được — khoản thu hồi khi ấy rơi vào một tài khoản đã rỗng và làm
    -- nó âm.
    --
    -- Chi trả trọn phần rút được trong tình huống đó là trả cả khoản vừa
    -- hoàn cho khách. Nên số thực chi = rút được TRỪ phần âm này.
    deficit_amount BIGINT NOT NULL DEFAULT 0 CHECK (deficit_amount >= 0),

    -- net_amount = gross − deficit. Đây là số ĐEM ĐI CHI TRẢ.
    net_amount BIGINT NOT NULL,

    currency TEXT NOT NULL DEFAULT 'VND',

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ,
    paid_at      TIMESTAMPTZ,
    version      BIGINT NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX settlement_seller_idx ON settlement (seller_id, created_at DESC);

-- Dòng đối soát: MỘT bút toán rút được thuộc về MỘT đợt.
CREATE TABLE settlement_line (
    id TEXT PRIMARY KEY CHECK (id LIKE 'stl\_%' AND length(id) = 30),

    settlement_id TEXT NOT NULL REFERENCES settlement (id) ON DELETE CASCADE,

    -- ledger_entry_id là bút toán SELLER_RELEASE được gom.
    --
    -- UNIQUE TOÀN CỤC, không phải theo từng đợt: một bút toán lọt vào hai
    -- đợt đối soát nghĩa là nhà bán được trả tiền hai lần cho cùng một
    -- đơn hàng. Kiểm ở tầng ứng dụng không đủ — hai lượt chạy job chồng
    -- nhau đều thấy "chưa gom".
    ledger_entry_id TEXT NOT NULL UNIQUE
        REFERENCES ledger_entry (id),

    amount   BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'VND',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX settlement_line_settlement_idx ON settlement_line (settlement_id);
