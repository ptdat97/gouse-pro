-- Module affiliate: quy công đơn hàng cho creator và tính hoa hồng.
--
-- # Vì sao CHỈ hai bảng, không phải năm
--
-- `docs/04-modules/affiliate.md` mục 7 khai năm bảng: `affiliate_link`,
-- `click`, `attribution`, `commission_record`, `fraud_signal`.
--
-- Migration này dựng HAI. Ba bảng còn lại chưa dựng, có lý do:
--
--	affiliate_link  cần HTTP cho creator tạo link, mà module `creator` là
--	                Phase 2. Và quy công KHÔNG cần link: chuỗi
--	                `source.creator_id` → cart → checkout → order_line đã
--	                chạy từ migration 000057, nên affiliate quy công được
--	                NGAY hôm nay. Link là cách theo dõi click qua URL
--	                chuyển hướng — một tối ưu, không phải điều kiện.
--	click           cùng lý do: không có link thì không có click.
--	fraud_signal    chưa có phép phát hiện gian lận nào. Một bảng không ai
--	                ghi là dạng lỗi hay gặp nhất của dự án này — dựng nó
--	                trước khi có mã ghi vào là tự tạo ra lỗi ấy.
--
-- Thêm khi có bên ghi thật. Đó là quy tắc P13 và §17 của đề xuất Commerce
-- Kernel: không dựng thứ chưa có nhu cầu.

-- attribution — BẤT BIẾN sau khi tạo (quy tắc 2 của affiliate.md).
--
-- Sửa một bản quy công nghĩa là sửa một khoản tiền đã hứa với creator. Nên
-- nó bất biến, và "sửa" là ghi một bản mới — cùng nguyên tắc với sổ cái
-- (ADR-0008).
CREATE TABLE attribution (
    id       TEXT PRIMARY KEY CHECK (id LIKE 'atr\_%' AND length(id) = 30),
    order_id TEXT NOT NULL,

    -- order_line_id: quy công theo TỪNG DÒNG, không theo cả đơn.
    --
    -- Một đơn có thể có dòng đến từ nội dung creator và dòng khách tự tìm.
    -- Quy công cả đơn cho creator là trả hoa hồng trên phần họ không tạo ra.
    order_line_id TEXT NOT NULL,

    -- seller_id: nhà bán của dòng hàng, CHÉP LẠI từ đơn.
    --
    -- # Vì sao lưu lại thứ module order đã có
    --
    -- Tín hiệu "hết hạn đổi trả" của hệ thống là `fulfillment_order.completed`,
    -- và nó mang `(order_id, seller_id)` — không mang dòng hàng. Một đơn
    -- nhiều nhà bán hết hạn theo TỪNG nhà bán, nên không có cột này thì
    -- affiliate chỉ còn hai đường: mở khóa hoa hồng của CẢ đơn khi nhà bán
    -- ĐẦU TIÊN giao xong (trả tiền cho hàng chưa hết hạn trả), hoặc gọi
    -- ngược module order giữa lúc xử lý event.
    --
    -- Chép một định danh KHÔNG phải nhân đôi nguồn sự thật: nó bất biến
    -- cùng dòng hàng, và đây là bản ĐÓNG BĂNG như `commission_rate`.
    seller_id TEXT NOT NULL DEFAULT '',

    creator_id TEXT NOT NULL CHECK (creator_id <> ''),

    -- content_id: nội dung nào dẫn tới lượt mua. RỖNG ở phiên bản này.
    --
    -- `cart_item.source_content_id` và `checkout_line.source_content_id` có
    -- từ migration 000057, nhưng chuỗi DỪNG ở `checkout_line`:
    -- `order_line` chỉ có `attributed_creator_id`, không có cột nội dung.
    --
    -- Để rỗng chứ không thêm cột vào `order_line`, vì đó là bảng của KERNEL
    -- và nội dung là việc của affiliate (ADR-0021). Khi module `creator` có
    -- bảng `click`, affiliate tra nội dung từ bảng CỦA MÌNH và cột này đầy
    -- lên mà kernel không phải đổi một dòng nào. Xem P3-84.
    content_id TEXT NOT NULL DEFAULT '',

    -- commission_rate ĐÓNG BĂNG tại thời điểm quy công (quy tắc 3).
    --
    -- Điểm cơ bản: 500 = 5%. Không dùng số thực — sai số dấu phẩy động tích
    -- lũy thành lệch đối soát.
    commission_rate INT NOT NULL CHECK (commission_rate BETWEEN 0 AND 10000),

    -- commission_base ĐÓNG BĂNG: số tiền mà tỷ lệ áp lên.
    --
    -- Là tiền THỰC TRẢ của dòng (giá niêm yết trừ phần giảm đã phân bổ),
    -- không phải giá niêm yết. Tính hoa hồng trên giá niêm yết của một đơn
    -- có giảm giá là trả nhiều hơn phần creator thực sự tạo ra doanh thu.
    commission_base BIGINT NOT NULL CHECK (commission_base >= 0),
    currency        CHAR(3) NOT NULL,

    -- cost_bearer: ai CHỊU chi phí hoa hồng (affiliate.md mục 5).
    --
    -- Đóng băng tại đây vì nó là một thỏa thuận tại thời điểm quy công.
    -- `campaign` (Phase 2) sẽ quyết theo chiến dịch; tới lúc đó mặc định là
    -- nền tảng chịu.
    cost_bearer TEXT NOT NULL CHECK (cost_bearer IN ('PLATFORM', 'SELLER')),

    -- attribution_model: cách chọn creator khi có nhiều người trong chuỗi.
    --
    -- `LAST_CLICK` là quyết định khởi điểm của affiliate.md mục 3.3. Ghi ra
    -- vì đổi mô hình sau này phải giải thích được số cũ tính theo cách nào.
    attribution_model TEXT NOT NULL CHECK (attribution_model IN ('LAST_CLICK')),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- MỘT dòng đơn chỉ được quy công MỘT lần.
    --
    -- `order.paid` giao ít nhất một lần, nên bên nhận sẽ thấy cùng một đơn
    -- hai lần. Không có ràng buộc này thì creator nhận hoa hồng gấp đôi —
    -- và chống trùng ở tầng ứng dụng không đủ, vì hai lượt worker chồng
    -- nhau cùng thấy "chưa quy công".
    UNIQUE (order_line_id)
);

CREATE INDEX attribution_creator_idx ON attribution (creator_id, created_at DESC);
CREATE INDEX attribution_order_idx ON attribution (order_id);

-- Chỉ mục cho phép mở khóa hoa hồng theo (đơn, nhà bán) — đúng hình dạng
-- khóa mà `fulfillment_order.completed` mang tới.
CREATE INDEX attribution_order_seller_idx ON attribution (order_id, seller_id);

-- commission_record — TRẠNG THÁI của khoản hoa hồng.
--
-- Tách khỏi `attribution` vì hai thứ có vòng đời khác nhau: quy công bất
-- biến, còn khoản tiền thì đi qua các trạng thái và có thể bị đảo.
CREATE TABLE commission_record (
    id              TEXT PRIMARY KEY CHECK (id LIKE 'crc\_%' AND length(id) = 30),
    attribution_id  TEXT NOT NULL REFERENCES attribution (id),
    creator_id      TEXT NOT NULL CHECK (creator_id <> ''),

    amount   BIGINT NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL,

    -- status: PENDING → AVAILABLE, hoặc → REVERSED.
    --
    -- Hoa hồng chỉ AVAILABLE sau khi hết hạn đổi trả (quy tắc 4), giống số
    -- dư nhà bán. Nhờ đó phần lớn hoàn hàng xảy ra TRƯỚC khi tiền được chi,
    -- và việc đảo ngược không phải đòi tiền đã trả.
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'AVAILABLE', 'REVERSED')),

    -- reversed_reason BẮT BUỘC khi đã đảo.
    --
    -- Một khoản tiền biến mất mà không nêu lý do là một điểm mù trong kiểm
    -- toán — cùng quy tắc với bút toán điều chỉnh của payment.
    reversed_reason TEXT NOT NULL DEFAULT '',
    reversed_at     TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Một bản quy công sinh ĐÚNG MỘT khoản hoa hồng.
    UNIQUE (attribution_id),

    CONSTRAINT commission_dao_phai_co_ly_do CHECK (
        (status <> 'REVERSED') OR
        (reversed_reason <> '' AND reversed_at IS NOT NULL)
    )
);

CREATE INDEX commission_record_creator_idx
    ON commission_record (creator_id, status, created_at DESC);
