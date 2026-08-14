-- Module: analytics — ghi sự kiện hành vi và tính chỉ số cốt lõi.
--
-- PHẠM VI MVP (analytics.md mục 13): ghi sự kiện cơ bản, chỉ số cốt lõi
-- (GMV, AOV, chuyển đổi). Phễu chi tiết, cohort, kho dữ liệu là Phase 2+.
--
-- NGUYÊN TẮC QUAN TRỌNG NHẤT (mục 4):
--
--     analytics KHÔNG PHẢI NGUỒN SỰ THẬT.
--
--     "GMV tháng này bao nhiêu?"      → analytics (có thể trễ vài phút)
--     "Seller A được trả bao nhiêu?"  → payment  (nguồn sự thật)
--
-- Không bao giờ dùng số liệu ở đây để ra quyết định tài chính. Đây là bản
-- sao đọc, chấp nhận trễ và chấp nhận mất mát ở mức nhỏ.
--
-- HỆ QUẢ CHO THIẾT KẾ BẢNG: không có khóa ngoại tới bất kỳ bảng nghiệp vụ
-- nào. Ràng buộc tham chiếu ở đây sẽ biến một sự cố ghi log thành một sự
-- cố bán hàng.

CREATE TABLE event_log (
    id BIGSERIAL PRIMARY KEY,

    -- event_name là tên sự kiện: "product_view", "order.placed".
    --
    -- KHÔNG dùng CHECK liệt kê: sự kiện mới xuất hiện liên tục, và một
    -- ràng buộc chặn sự kiện lạ sẽ làm hỏng luồng chính — đúng thứ quy
    -- tắc 3 cấm.
    event_name TEXT NOT NULL CHECK (length(event_name) > 0),

    -- category phân nhóm để truy vấn nhanh: BEHAVIOR hoặc BUSINESS.
    --
    --     BEHAVIOR  hành vi người dùng, khối lượng RẤT LỚN
    --     BUSINESS  từ domain event, khối lượng nhỏ hơn nhiều
    --
    -- Tách nhóm vì hai loại có vòng đời khác nhau: hành vi có thể dọn sau
    -- 90 ngày, nghiệp vụ phải giữ lâu hơn.
    category TEXT NOT NULL DEFAULT 'BEHAVIOR' CHECK (category IN (
        'BEHAVIOR', 'BUSINESS'
    )),

    -- ------------------------------------------------------------------
    -- Ai gây ra sự kiện.
    -- ------------------------------------------------------------------

    -- customer_id RỖNG với khách chưa đăng nhập.
    --
    -- KHÔNG có khóa ngoại: xem ghi chú đầu file.
    customer_id TEXT NOT NULL DEFAULT '',

    -- session_id nối các sự kiện của MỘT lượt truy cập.
    --
    -- Đây là thứ cho phép đo phễu chuyển đổi: không có nó thì biết có
    -- 1000 lượt xem và 50 đơn hàng, nhưng KHÔNG biết 50 đơn đó đến từ
    -- những lượt xem nào.
    session_id TEXT NOT NULL DEFAULT '',

    -- ------------------------------------------------------------------
    -- Đối tượng của sự kiện.
    -- ------------------------------------------------------------------

    -- subject_type: "product", "order", "variant"...
    subject_type TEXT NOT NULL DEFAULT '',
    subject_id   TEXT NOT NULL DEFAULT '',

    seller_id TEXT NOT NULL DEFAULT '',

    -- ------------------------------------------------------------------
    -- Giá trị tiền, nếu sự kiện có.
    -- ------------------------------------------------------------------

    -- amount là số nguyên đơn vị nhỏ nhất, NULL nếu sự kiện không có tiền.
    --
    -- NULL chứ không phải 0: "đơn hàng 0đ" và "sự kiện xem sản phẩm không
    -- liên quan tới tiền" là hai chuyện khác nhau, và cộng nhầm loại thứ
    -- hai vào GMV làm sai mọi con số.
    amount   BIGINT,
    currency TEXT NOT NULL DEFAULT 'VND',

    -- ------------------------------------------------------------------
    -- Ngữ cảnh bổ sung.
    -- ------------------------------------------------------------------

    -- properties là dữ liệu tự do dạng JSON.
    --
    -- KHÔNG ĐƯỢC chứa dữ liệu cá nhân nhạy cảm: số đo cơ thể, thông tin
    -- thanh toán, mật khẩu. Xem quy tắc 4 và
    -- docs/09-operations/security.md.
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- ip_hash: BĂM, không lưu nguyên văn (quy tắc bảo mật, mục 8).
    ip_hash    TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',

    -- event_id là id của domain event sinh ra bản ghi này.
    --
    -- Rỗng với sự kiện hành vi (đến thẳng từ giao diện). Với sự kiện
    -- nghiệp vụ, đây là khóa CHỐNG GHI TRÙNG khi handler xử lý lại cùng
    -- một event — xem chỉ mục bên dưới.
    event_id TEXT NOT NULL DEFAULT '',

    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- CHỐNG GHI TRÙNG cho sự kiện nghiệp vụ.
--
-- Handler event xử lý lại cùng một event là chuyện bình thường (giao
-- ít-nhất-một-lần). Không có chỉ mục này, mỗi lần xử lý lại là một đơn
-- hàng nữa cộng vào GMV — và số liệu phình lên không ai giải thích được.
--
-- Chỉ mục MỘT PHẦN: sự kiện hành vi có event_id rỗng và KHÔNG cần chống
-- trùng (hai lần xem sản phẩm thật sự là hai lần xem).
CREATE UNIQUE INDEX event_log_event_id_key ON event_log (event_id, event_name)
    WHERE event_id <> '';

-- Truy vấn nóng nhất: chỉ số theo khoảng thời gian.
CREATE INDEX event_log_time_idx ON event_log (occurred_at DESC, event_name);

-- Phễu chuyển đổi: gom sự kiện theo phiên.
CREATE INDEX event_log_session_idx ON event_log (session_id, occurred_at)
    WHERE session_id <> '';

-- Dashboard seller: chỉ số của một gian hàng.
CREATE INDEX event_log_seller_idx ON event_log (seller_id, occurred_at DESC)
    WHERE seller_id <> '';

-- Ẩn danh hóa khi khách yêu cầu xóa (quy tắc bảo mật, mục 8).
CREATE INDEX event_log_customer_idx ON event_log (customer_id)
    WHERE customer_id <> '';

-- ---------------------------------------------------------------------
-- Chỉ số tính sẵn.
-- ---------------------------------------------------------------------
--
-- VÌ SAO CẦN BẢNG NÀY: đếm lại từ event_log mỗi lần mở dashboard là quét
-- hàng triệu hàng cho một con số. Bảng này giữ kết quả đã tính, worker
-- cập nhật định kỳ.
--
-- CHẤP NHẬN TRỄ: đây là bản sao đọc, không phải nguồn sự thật (mục 4).
CREATE TABLE metric_snapshot (
    -- metric_name: "gmv", "aov", "conversion_rate", "order_count".
    metric_name TEXT NOT NULL CHECK (length(metric_name) > 0),

    -- period_start là ĐẦU khoảng thời gian, đã cắt tròn theo granularity.
    period_start TIMESTAMPTZ NOT NULL,

    -- granularity: HOUR, DAY, MONTH.
    granularity TEXT NOT NULL CHECK (granularity IN ('HOUR', 'DAY', 'MONTH')),

    -- dimension cho phép cắt lát chỉ số: "seller", "" (toàn sàn).
    --
    -- Hai cột thay vì một bảng riêng: MVP chỉ cần cắt theo seller, và một
    -- bảng chiều riêng cho một chiều duy nhất chỉ thêm một phép JOIN.
    dimension       TEXT NOT NULL DEFAULT '',
    dimension_value TEXT NOT NULL DEFAULT '',

    -- value là số nguyên.
    --
    -- Với chỉ số tiền tệ, đây là đơn vị nhỏ nhất. Với tỷ lệ, đây là ĐIỂM
    -- CƠ BẢN (1000 = 10%) — KHÔNG dùng số thực, vì tỷ lệ chuyển đổi hiển
    -- thị sai ở chữ số thứ mười lăm vẫn là hiển thị sai.
    value BIGINT NOT NULL,

    -- sample_size là số bản ghi đã dùng để tính.
    --
    -- Cần để đọc chỉ số cho đúng: tỷ lệ chuyển đổi 50% từ 2 lượt truy cập
    -- không nói lên điều gì, còn từ 20.000 lượt thì có.
    sample_size BIGINT NOT NULL DEFAULT 0 CHECK (sample_size >= 0),

    currency TEXT NOT NULL DEFAULT 'VND',

    computed_at TIMESTAMPTZ NOT NULL,

    -- Một chỉ số, một khoảng, một lát cắt — ĐÚNG MỘT hàng.
    --
    -- Tính lại phải GHI ĐÈ chứ không thêm hàng mới: hai giá trị GMV cho
    -- cùng một ngày là hai câu trả lời cho cùng một câu hỏi, và không có
    -- cách nào biết cái nào đúng.
    PRIMARY KEY (metric_name, period_start, granularity, dimension, dimension_value)
);

CREATE INDEX metric_snapshot_lookup_idx
    ON metric_snapshot (metric_name, granularity, period_start DESC);
