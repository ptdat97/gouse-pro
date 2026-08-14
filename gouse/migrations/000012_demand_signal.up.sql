-- Module: supply-chain — tín hiệu nhu cầu (demand signal).
--
-- VÌ SAO BẢNG NÀY TỒN TẠI Ở MVP DÙ CHƯA AI DÙNG TỚI NÓ:
--
--     DỮ LIỆU LỊCH SỬ KHÔNG TẠO NGƯỢC ĐƯỢC.
--
-- Chuỗi cung ứng tới Phase 3 mới làm, nhưng tới lúc đó mà không có dữ liệu
-- hành vi của 12 tháng trước thì không dự báo được gì. Không có cách nào
-- dựng lại "tháng 3 có bao nhiêu người tìm áo khoác dạ mà không thấy".
--
-- Đây là một trong bốn thứ "sửa sau là viết lại" ở docs/10-roadmap/mvp.md.
--
-- SAI LẦM KINH ĐIỂN mà bảng này sinh ra để tránh (supply-chain.md mục 4.2):
--
--     Chỉ nhìn doanh số:  "Áo khoác bán 200 chiếc" → nhu cầu là 200
--
--     Thực tế:            bán 200, HẾT HÀNG từ tuần 3
--                         1.500 lượt tìm sau khi hết
--                         400 lượt đăng ký báo có hàng
--                         → nhu cầu thật gần 800
--
-- Lập kế hoạch chỉ dựa vào doanh số lịch sử sẽ LIÊN TỤC SẢN XUẤT THIẾU
-- đúng những mặt hàng bán chạy nhất.
--
-- BẤT BIẾN: bảng chỉ GHI THÊM. Tín hiệu là quan sát về một thời điểm đã
-- qua; sửa nó nghĩa là sửa lịch sử.

CREATE TABLE demand_signal (
    -- BIGSERIAL chứ không phải ULID: đây là bảng ghi nhiều nhất hệ thống
    -- (mỗi lượt xem, mỗi lần thêm giỏ), và không thực thể nào tham chiếu
    -- tới một dòng cụ thể. Trả 16 byte cho mỗi dòng để lấy một định danh
    -- không ai dùng là lãng phí ở quy mô này.
    id BIGSERIAL PRIMARY KEY,

    signal_type TEXT NOT NULL CHECK (signal_type IN (
        'VIEW',              -- xem sản phẩm
        'SEARCH',            -- tìm kiếm có kết quả
        'SEARCH_NO_RESULT',  -- tìm KHÔNG có kết quả  ← nhu cầu chưa đáp ứng
        'CLICK',             -- click từ nội dung/danh mục
        'ADD_TO_CART',       -- thêm giỏ — mạnh hơn view nhiều
        'WISHLIST',          -- lưu để mua sau — ý định rõ ràng
        'ORDER',             -- đơn hàng thật
        'STOCKOUT',          -- hết hàng          ← nhu cầu chưa đáp ứng
        'RETURN',            -- hoàn hàng (kèm lý do trong metadata)
        'NOTIFY_REQUEST'     -- đăng ký báo có hàng ← nhu cầu chưa đáp ứng
    )),

    -- Ba loại tín hiệu quan trọng nhất là SEARCH_NO_RESULT, STOCKOUT và
    -- NOTIFY_REQUEST: chúng đo NHU CẦU KHÔNG ĐƯỢC ĐÁP ỨNG — thứ không bao
    -- giờ xuất hiện trong dữ liệu bán hàng.

    -- Cả ba đều nullable: tín hiệu tìm kiếm không có kết quả thì KHÔNG có
    -- sku_id (đó chính là ý nghĩa của nó), còn tín hiệu xem sản phẩm thì
    -- không cần search_term.
    --
    -- Tham chiếu VƯỢT MODULE nên không có REFERENCES.
    sku_id      TEXT NOT NULL DEFAULT '',
    product_id  TEXT NOT NULL DEFAULT '',
    category_id TEXT NOT NULL DEFAULT '',
    search_term TEXT NOT NULL DEFAULT '',

    -- Tín hiệu phải chỉ vào MỘT THỨ GÌ ĐÓ. Không có cả bốn thì nó không
    -- nói lên điều gì và chỉ làm phồng bảng.
    CONSTRAINT demand_signal_needs_subject CHECK (
        length(trim(sku_id)) > 0
        OR length(trim(product_id)) > 0
        OR length(trim(category_id)) > 0
        OR length(trim(search_term)) > 0
    ),

    -- quantity: số lượng liên quan. Với VIEW là 1; với ORDER là số lượng
    -- đặt; với STOCKOUT là số lượng KHÔNG đáp ứng được.
    quantity INT NOT NULL DEFAULT 1 CHECK (quantity > 0),

    -- occurred_at là thời điểm NGHIỆP VỤ, khác thời điểm ghi.
    --
    -- Tín hiệu đi qua outbox nên có độ trễ vài giây; tổng hợp theo tuần mà
    -- dùng thời điểm ghi sẽ đẩy nhầm tín hiệu cuối tuần sang tuần sau.
    occurred_at TIMESTAMPTZ NOT NULL,

    -- Nguồn gốc, để truy vết và để khử trùng lặp.
    source_type TEXT NOT NULL DEFAULT '',
    source_id   TEXT NOT NULL DEFAULT '',

    -- metadata giữ phần đặc thù theo loại: lý do hoàn hàng, từ khóa gốc,
    -- kênh phát sinh. JSONB vì mỗi loại tín hiệu cần trường khác nhau, và
    -- thêm cột cho từng loại sẽ tạo một bảng phần lớn là NULL.
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    -- KHÔNG có updated_at: bảng chỉ ghi thêm.
);

-- Truy vấn chính của Phase 3: "SKU này có bao nhiêu tín hiệu trong kỳ".
CREATE INDEX demand_signal_sku_idx
    ON demand_signal (sku_id, occurred_at DESC)
    WHERE sku_id <> '';

CREATE INDEX demand_signal_product_idx
    ON demand_signal (product_id, occurred_at DESC)
    WHERE product_id <> '';

-- Truy vấn nhu cầu chưa đáp ứng: "tìm gì mà không thấy", "hết hàng gì".
--
-- Chỉ mục riêng cho ba loại này vì chúng là câu hỏi được hỏi thường xuyên
-- nhất, và chúng chiếm tỷ lệ nhỏ trong tổng số tín hiệu.
CREATE INDEX demand_signal_unmet_idx
    ON demand_signal (signal_type, occurred_at DESC)
    WHERE signal_type IN ('SEARCH_NO_RESULT', 'STOCKOUT', 'NOTIFY_REQUEST');

-- Tìm kiếm không kết quả, gom theo từ khóa.
CREATE INDEX demand_signal_search_idx
    ON demand_signal (search_term, occurred_at DESC)
    WHERE search_term <> '';

-- Phân vùng theo thời gian và chính sách lưu trữ sẽ thêm ở Phase 2, khi
-- có số liệu thật về tốc độ ghi. Thêm bây giờ là tối ưu hóa dựa trên phỏng
-- đoán — xem docs/04-modules/supply-chain.md mục 8.
CREATE INDEX demand_signal_occurred_idx ON demand_signal (occurred_at DESC);
