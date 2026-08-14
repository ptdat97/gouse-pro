-- Audit log — nhật ký thao tác, BẤT BIẾN.
--
-- Theo ADR-0011: đây là năng lực PLATFORM, không phải module nghiệp vụ.
-- Nó bị mọi module gọi và không sở hữu khái niệm nghiệp vụ nào — giống
-- logger và eventbus.
--
-- HAI YÊU CẦU TỪ TÀI LIỆU (docs/06-api/admin-api.md mục 2 và 6):
--
--     1. Bảy endpoint nhạy cảm BẮT BUỘC trường `reason`
--        (ledger.adjust · inventory.adjust · seller.suspend ·
--         creator.suspend · content.take-down · order.cancel · refund)
--
--     2. MỌI lần đọc dữ liệu cá nhân khách hàng đều ghi audit
--
-- VÌ SAO KHÔNG CÓ KHÓA NGOẠI TỚI BẢNG NÀO:
--
-- ADR-0005 cấm khóa ngoại vượt ranh giới module. Nhưng ở đây còn một lý do
-- mạnh hơn: audit log phải SỐNG LÂU HƠN thứ nó ghi lại. Xóa một seller mà
-- vết kiểm toán "ai đã đình chỉ seller đó" biến mất theo thì audit log vô
-- dụng đúng vào lúc cần nhất.

CREATE TABLE audit_log (
    id TEXT PRIMARY KEY CHECK (id LIKE 'aud\_%' AND length(id) = 30),

    -- Ai thực hiện.
    --
    -- actor_type phân biệt người thật với hệ thống: "đơn bị hủy" do nhân
    -- viên bấm nút khác hẳn "đơn bị hủy" do job quá hạn chạy. Điều tra sự
    -- cố mà không phân biệt được hai thứ này thì mất rất nhiều thời gian.
    actor_type TEXT NOT NULL CHECK (actor_type IN (
        'USER',        -- người thật, có tài khoản
        'SYSTEM',      -- job nền, tiến trình tự động
        'API_CLIENT'   -- tích hợp bên ngoài dùng khóa API
    )),

    -- KHÔNG có khóa ngoại tới bảng "user": xem ghi chú đầu file.
    -- Rỗng khi actor_type = 'SYSTEM'.
    actor_id TEXT NOT NULL DEFAULT '',

    -- Làm gì. Dạng "danh_từ.động_từ": ledger.adjust, seller.suspend,
    -- customer.view.
    action TEXT NOT NULL CHECK (length(trim(action)) > 0),

    -- Trên tài nguyên nào.
    --
    -- Chuỗi thuần, KHÔNG phải enum của database: thêm loại tài nguyên mới
    -- không được buộc phải chạy migration. Danh sách giá trị hợp lệ nằm ở
    -- tầng ứng dụng (internal/platform/audit).
    resource_type TEXT NOT NULL CHECK (length(trim(resource_type)) > 0),
    resource_id TEXT NOT NULL DEFAULT '',

    -- Vì sao.
    --
    -- BẮT BUỘC với thao tác nhạy cảm, cưỡng chế ở tầng ứng dụng chứ không
    -- ở đây: database không biết action nào là nhạy cảm, và nhét danh sách
    -- đó vào CHECK nghĩa là mỗi lần thêm một endpoint nhạy cảm lại phải
    -- chạy migration.
    reason TEXT NOT NULL DEFAULT '',

    -- Nối với chuỗi truy vết của request.
    --
    -- Đây là cầu nối giữa audit log và log ứng dụng: có request_id thì tra
    -- ngược được toàn bộ những gì đã xảy ra trong cùng request đó.
    request_id TEXT NOT NULL DEFAULT '',

    -- Chi tiết bổ sung, tùy thao tác. Ví dụ số tiền điều chỉnh, trạng thái
    -- trước và sau.
    --
    -- JSONB chứ không phải cột riêng: mỗi loại thao tác cần thông tin khác
    -- nhau, và thêm cột cho từng loại sẽ tạo một bảng toàn cột NULL.
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- Chỉ mục theo ĐÚNG các đường tra cứu của trang audit log.
-- ---------------------------------------------------------------------
--
-- docs/08-frontend/admin.md mục 7: lọc theo loại tài nguyên và hành động,
-- trên một khoảng ngày. Chỉ mục phải phục vụ đúng truy vấn đó, không phải
-- mọi tổ hợp có thể nghĩ ra — chỉ mục thừa làm chậm việc GHI, mà đây là
-- bảng chỉ có ghi và đọc rất thưa.

-- Đường tra cứu chính: lọc theo loại tài nguyên, mới nhất trước.
CREATE INDEX audit_log_resource_time_idx
    ON audit_log (resource_type, occurred_at DESC);

-- "Chuyện gì đã xảy ra với bản ghi CỤ THỂ này" — dùng khi điều tra một
-- seller hay một đơn hàng.
CREATE INDEX audit_log_resource_id_idx
    ON audit_log (resource_id, occurred_at DESC)
    WHERE resource_id <> '';

-- "Nhân viên này đã làm những gì" — dùng cho cảnh báo ở admin-api.md mục
-- 6: nhân viên truy cập nhiều hồ sơ khách trong thời gian ngắn.
CREATE INDEX audit_log_actor_time_idx
    ON audit_log (actor_id, occurred_at DESC)
    WHERE actor_id <> '';

-- Lọc theo hành động trên khoảng thời gian.
CREATE INDEX audit_log_action_time_idx
    ON audit_log (action, occurred_at DESC);

-- ---------------------------------------------------------------------
-- LỚP BẢO VỆ CUỐI CÙNG: chặn UPDATE và DELETE.
-- ---------------------------------------------------------------------
--
-- docs/08-frontend/admin.md mục 7: audit log CHỈ ĐỌC. Nếu sửa được, nó
-- mất hết giá trị — một bản ghi kiểm toán chỉ đáng tin khi không ai, kể
-- cả người có quyền cao nhất, sửa được nó sau khi sự việc xảy ra.
--
-- Tầng Go không có hàm Update hay Delete, nên không có đường nào để gọi.
-- Trigger này chặn cả những đường KHÔNG đi qua code của chúng ta: thao
-- tác thủ công bằng psql, script di trú dữ liệu viết vội, hoặc một lỗi
-- code trong tương lai.
--
-- Dùng TRIGGER báo lỗi thay vì RULE ... DO INSTEAD NOTHING, cùng lý do
-- với sổ cái ở migration 000007: im lặng bỏ qua khiến người sửa tưởng đã
-- thành công và đi tiếp với giả định sai.
CREATE OR REPLACE FUNCTION audit_log_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit log là bất biến: không được % trên bảng %',
        TG_OP, TG_TABLE_NAME
        USING HINT = 'Vết kiểm toán sửa được là vết kiểm toán vô giá trị';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

CREATE TRIGGER audit_log_no_delete
    BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
