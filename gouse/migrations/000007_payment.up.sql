-- Module: payment — sổ cái tài chính BẤT BIẾN.
--
-- Nền tảng GIỮ TIỀN HỘ nhiều bên: tiền khách trả KHÔNG thuộc về nền tảng
-- cho tới khi đối soát xong. Vì vậy đây là bút toán kép, không phải bảng
-- giao dịch đơn giản. Xem docs/adr/0008-financial-ledger.md.

CREATE TABLE ledger_entry (
    id              TEXT PRIMARY KEY CHECK (id LIKE 'led\_%' AND length(id) = 30),

    entry_type      TEXT NOT NULL CHECK (entry_type IN (
        'ORDER_REVENUE', 'COGS', 'REFUND', 'PAYOUT', 'ADJUSTMENT', 'FEE'
    )),

    -- Nguồn gốc sự kiện. Tham chiếu VƯỢT MODULE — không có REFERENCES.
    reference_type  TEXT NOT NULL CHECK (length(trim(reference_type)) > 0),
    reference_id    TEXT NOT NULL CHECK (length(trim(reference_id)) > 0),

    description     TEXT NOT NULL DEFAULT '',

    -- Chống ghi trùng. Ghi hai lần cùng một sự kiện tài chính sẽ NHÂN ĐÔI
    -- số tiền — loại lỗi tệ nhất có thể xảy ra ở module này.
    idempotency_key TEXT NOT NULL UNIQUE CHECK (length(trim(idempotency_key)) > 0),

    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL
    -- KHÔNG có updated_at — bản ghi BẤT BIẾN.
);

CREATE INDEX ledger_entry_reference_idx ON ledger_entry (reference_type, reference_id);
CREATE INDEX ledger_entry_created_idx ON ledger_entry (created_at DESC);

CREATE TABLE ledger_line (
    id           BIGSERIAL PRIMARY KEY,
    entry_id     TEXT NOT NULL REFERENCES ledger_entry (id),

    account_type TEXT NOT NULL CHECK (account_type IN (
        'PLATFORM_CASH', 'PLATFORM_REVENUE', 'SELLER_PAYABLE',
        'CREATOR_PAYABLE', 'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE',
        'COGS', 'FEE_EXPENSE', 'INVENTORY_ASSET'
    )),

    -- Rỗng với tài khoản của nền tảng. Bắt buộc với tài khoản phải trả:
    -- SELLER_PAYABLE mà không biết seller nào thì không đối soát được.
    account_owner_id TEXT NOT NULL DEFAULT '',

    direction    TEXT NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),

    -- LUÔN DƯƠNG. Hướng nằm ở cột direction, không nằm ở dấu của số —
    -- một dấu trừ đặt sai chỗ làm bút toán vẫn "cân bằng" nhưng sai hoàn toàn.
    amount       BIGINT NOT NULL CHECK (amount > 0),
    currency     CHAR(3) NOT NULL,

    description  TEXT NOT NULL DEFAULT '',

    -- Tài khoản phải trả BẮT BUỘC có chủ sở hữu.
    CONSTRAINT ledger_line_payable_needs_owner CHECK (
        account_type NOT IN ('SELLER_PAYABLE', 'CREATOR_PAYABLE',
                             'CUSTOMER_REFUND_PAYABLE', 'SUPPLIER_PAYABLE')
        OR length(trim(account_owner_id)) > 0
    )
);

CREATE INDEX ledger_line_entry_idx ON ledger_line (entry_id);

-- Truy vấn số dư: gom theo tài khoản. Đây là truy vấn nóng nhất của module.
CREATE INDEX ledger_line_account_idx
    ON ledger_line (account_type, account_owner_id);

-- ---------------------------------------------------------------------
-- LỚP BẢO VỆ CUỐI CÙNG: chặn UPDATE và DELETE.
-- ---------------------------------------------------------------------
--
-- ADR-0008 quyết định 1: KHÔNG BAO GIỜ sửa hay xóa một bút toán đã ghi.
-- Sửa sai bằng cách ghi bút toán ĐIỀU CHỈNH mới.
--
-- Kể cả khi có lỗi code hoặc thao tác thủ công nhầm, database vẫn từ chối.
--
-- Dùng TRIGGER báo lỗi thay vì RULE ... DO INSTEAD NOTHING: im lặng bỏ qua
-- khiến người sửa tưởng đã thành công và đi tiếp với giả định sai. Với sổ
-- sách tài chính, một giả định sai kéo dài là điều tệ nhất.
CREATE OR REPLACE FUNCTION ledger_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Sổ cái là bất biến: không được % trên bảng %',
        TG_OP, TG_TABLE_NAME
        USING HINT = 'Ghi bút toán ĐIỀU CHỈNH (ADJUSTMENT) mới thay vì sửa bút toán cũ';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_entry_no_update
    BEFORE UPDATE ON ledger_entry
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

CREATE TRIGGER ledger_entry_no_delete
    BEFORE DELETE ON ledger_entry
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

CREATE TRIGGER ledger_line_no_update
    BEFORE UPDATE ON ledger_line
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

CREATE TRIGGER ledger_line_no_delete
    BEFORE DELETE ON ledger_line
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

-- ---------------------------------------------------------------------
-- Bản chụp số dư — CACHE, không phải nguồn sự thật.
-- ---------------------------------------------------------------------
--
-- Số dư = Σ CREDIT − Σ DEBIT, tính từ bút toán. Bảng này chỉ để tăng tốc:
-- tổng hàng triệu bút toán mỗi lần truy vấn là chậm.
--
-- ĐIỂM MẤU CHỐT: snapshot có thể TÍNH LẠI được. Job hàng ngày kiểm tra
-- snapshot có khớp với tổng bút toán không — lệch = cảnh báo NGHIÊM TRỌNG.
CREATE TABLE balance_snapshot (
    account_type          TEXT NOT NULL,
    account_owner_id      TEXT NOT NULL DEFAULT '',

    -- Chốt tại bút toán nào: số dư hiện tại = snapshot + tổng bút toán sau đó.
    as_of_entry_created_at TIMESTAMPTZ NOT NULL,

    balance   BIGINT NOT NULL,
    currency  CHAR(3) NOT NULL,

    computed_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (account_type, account_owner_id)
);
