-- Ý ĐỊNH THANH TOÁN: số tiền hệ thống CHỜ THU cho một đơn (ADR-0017).
--
-- Đây là con số mà webhook thanh toán đối chiếu vào — lớp bảo vệ thứ ba
-- trong ba lớp mà api/paths/webhooks.yaml quy định. Thiếu bảng này thì
-- endpoint webhook chỉ có chữ ký và idempotency, tức là nó TIN số tiền do
-- bên ngoài gửi vào. Chữ ký đúng chỉ chứng minh NGUỒN, không chứng minh
-- NỘI DUNG: lỗi tích hợp phía nhà cung cấp, hoặc một khóa HMAC bị lộ, đều
-- thành tiền ghi sai vào một cuốn sổ BẤT BIẾN.
CREATE TABLE payment_intent (
    id          TEXT PRIMARY KEY
                CHECK (id LIKE 'pin\_%' AND length(id) = 30),

    -- Một đơn có TỐI ĐA một intent. Ràng buộc UNIQUE chứ không chỉ kiểm ở
    -- Go: hai request hoàn tất song song đều qua được mọi cửa kiểm ở tầng
    -- ứng dụng, và hai intent cho một đơn nghĩa là thu tiền hai lần.
    order_id    TEXT NOT NULL UNIQUE
                CHECK (order_id LIKE 'ord\_%' AND length(order_id) = 30),

    -- amount ĐÓNG BĂNG từ tổng đơn lúc đặt: cùng con số khách đã nhìn
    -- thấy. KHÔNG tính lại lúc webhook về — tính lại thì một thay đổi giá
    -- giữa chừng làm phép đối chiếu tự nói dối.
    amount      BIGINT NOT NULL CHECK (amount > 0),
    currency    TEXT NOT NULL CHECK (length(currency) = 3),

    -- Phương thức TRẢ TRƯỚC. COD không có intent (ADR-0017 phần 2): không
    -- có cuộc trao đổi nào với cổng thanh toán để mà có ý định.
    payment_method TEXT NOT NULL
                CHECK (payment_method IN ('CARD', 'BANK_TRANSFER', 'E_WALLET')),

    status      TEXT NOT NULL DEFAULT 'REQUIRES_PAYMENT'
                CHECK (status IN ('REQUIRES_PAYMENT', 'CAPTURED', 'FAILED', 'CANCELLED')),

    -- Mã của NHÀ CUNG CẤP. Để trống cho tới khi có PSP thật — hôm nay
    -- chưa có adapter nào, nên webhook tra intent bằng mã nội bộ. Cột có
    -- sẵn nên ngày nối PSP là thay đổi CỘNG THÊM, không phải migration
    -- phá vỡ.
    provider            TEXT NOT NULL DEFAULT '',
    provider_intent_id  TEXT NOT NULL DEFAULT '',

    -- failure_reason là lý do nhà cung cấp báo, giữ nguyên văn để đối
    -- soát. Rỗng khi chưa thất bại.
    failure_reason TEXT NOT NULL DEFAULT '',

    captured_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- CAPTURED thì bắt buộc có mốc thu tiền, và chưa CAPTURED thì không
    -- được có. Hai nửa của cùng một sự thật lệch nhau là chuyện xảy ra khi
    -- có đường ghi thứ hai không đi qua domain.
    CONSTRAINT payment_intent_captured_co_moc CHECK (
        (status = 'CAPTURED' AND captured_at IS NOT NULL)
        OR (status <> 'CAPTURED' AND captured_at IS NULL)
    )
);

-- Danh sách intent quá hạn chưa thu = danh sách phải đi hỏi PSP.
--
-- Đó là chỗ đứng cho job đối chiếu định kỳ (yêu cầu 5 của webhooks.yaml),
-- thứ CHƯA có: không có nó thì một webhook MẤT để đơn treo vĩnh viễn mà
-- không ai biết.
CREATE INDEX payment_intent_cho_thu
    ON payment_intent (created_at)
    WHERE status = 'REQUIRES_PAYMENT';

-- Tra theo mã nhà cung cấp — đường mà PSP thật sẽ dùng.
CREATE INDEX payment_intent_theo_nha_cung_cap
    ON payment_intent (provider, provider_intent_id)
    WHERE provider_intent_id <> '';
