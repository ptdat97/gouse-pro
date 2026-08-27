-- Yêu cầu trả hàng.
--
-- VÌ SAO LÀ LUỒNG CHÍNH, KHÔNG PHẢI NGOẠI LỆ: tỷ lệ hoàn hàng ngành thời
-- trang cao hơn hẳn các ngành khác vì khách không thử được trước khi mua.
-- Xem docs/07-workflows/return.md mục 1.

CREATE TABLE return_request (
    id TEXT PRIMARY KEY CHECK (id LIKE 'ret\_%' AND length(id) = 30),

    -- Tham chiếu vượt module — không có khóa ngoại, cùng cách
    -- reservation.checkout_id đã làm.
    order_id  TEXT NOT NULL,
    seller_id TEXT NOT NULL,

    -- customer_id rỗng với khách vãng lai; khi đó liên hệ qua đơn hàng.
    customer_id TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL CHECK (status IN (
        'REQUESTED', 'APPROVED', 'REJECTED', 'RECEIVED', 'REFUNDED', 'CANCELLED'
    )),

    -- reason_code CHUẨN HÓA, không phải văn bản tự do.
    --
    -- Lý do hoàn quyết định DÒNG TIỀN và HÀNH ĐỘNG KHẮC PHỤC: hàng lỗi thì
    -- truy vết lô sản xuất, sai size thì sửa bảng size, hỏng khi vận
    -- chuyển thì khiếu nại đối tác. Văn bản tự do không phân tích được và
    -- không ra được quyết định nào.
    reason_code TEXT NOT NULL CHECK (reason_code IN (
        'SIZE_TOO_SMALL', 'SIZE_TOO_LARGE',
        'NOT_AS_DESCRIBED', 'COLOR_DIFFERENT',
        'QUALITY_ISSUE', 'DEFECTIVE',
        'WRONG_ITEM_SENT', 'DAMAGED_IN_TRANSIT',
        'CHANGED_MIND', 'LATE_DELIVERY'
    )),

    -- customer_note là mô tả thêm của khách. KHÔNG thay thế reason_code.
    customer_note TEXT NOT NULL DEFAULT '',

    -- reject_reason bắt buộc khi REJECTED: khách cần biết vì sao.
    reject_reason TEXT NOT NULL DEFAULT '',
    CONSTRAINT return_reject_needs_reason CHECK (
        status <> 'REJECTED' OR length(trim(reject_reason)) > 0
    ),

    -- refund_amount là số tiền ĐÃ HOÀN, tính theo GIÁ THỰC TRẢ.
    --
    -- Xem docs/07-workflows/return.md mục 5: hoàn theo giá niêm yết thay
    -- vì giá thực trả là điểm dễ sai nhất của cả luồng, và nó làm nền
    -- tảng trả ra nhiều hơn đã thu vào.
    refund_amount   BIGINT NOT NULL DEFAULT 0 CHECK (refund_amount >= 0),
    refund_currency TEXT NOT NULL DEFAULT 'VND',

    requested_at TIMESTAMPTZ NOT NULL,
    decided_at   TIMESTAMPTZ,
    received_at  TIMESTAMPTZ,
    refunded_at  TIMESTAMPTZ,

    version    BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX return_request_order_idx  ON return_request (order_id);
CREATE INDEX return_request_seller_idx ON return_request (seller_id, status);

-- Dòng hàng xin trả.
CREATE TABLE return_line (
    id TEXT PRIMARY KEY CHECK (id LIKE 'rtl\_%' AND length(id) = 30),

    return_request_id TEXT NOT NULL
        REFERENCES return_request (id) ON DELETE CASCADE,

    -- order_line_id vượt module, không khóa ngoại.
    order_line_id TEXT NOT NULL,
    sku_id        TEXT NOT NULL,

    quantity INT NOT NULL CHECK (quantity > 0),

    -- line_refund là phần tiền của dòng này, ĐÓNG BĂNG lúc tạo yêu cầu.
    --
    -- Đóng băng chứ không tính lại lúc hoàn: giữa lúc khách xin trả và lúc
    -- hàng về kho, giá có thể đã đổi và khuyến mãi có thể đã kết thúc.
    line_refund   BIGINT NOT NULL CHECK (line_refund >= 0),
    line_currency TEXT NOT NULL DEFAULT 'VND',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- MỘT dòng hàng chỉ được xin trả MỘT LẦN đang còn hiệu lực.
    --
    -- Không có ràng buộc này, khách gửi hai yêu cầu cho cùng một món và
    -- được hoàn tiền hai lần cho một lần mua.
    UNIQUE (return_request_id, order_line_id)
);

CREATE INDEX return_line_order_line_idx ON return_line (order_line_id);
