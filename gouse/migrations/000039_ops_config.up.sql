-- Tham số vận hành sửa được lúc chạy.
--
-- # Vì sao chỉ MỘT SỐ hằng số nằm ở đây
--
-- Sổ đăng ký ĐÓNG nằm trong mã (internal/platform/opsconfig/registry.go),
-- không phải trong bảng này. Bảng chỉ giữ GIÁ TRỊ; danh sách khóa hợp lệ,
-- kiểu, biên và mặc định đều ở mã nguồn.
--
-- Hệ quả có chủ ý: KHÔNG có đường nào thêm tham số mới từ giao diện quản
-- trị. Thêm tham số là việc của người viết mã, có review — vì mỗi tham số
-- mới là một câu hỏi "sửa được lúc chạy có an toàn không", và câu hỏi đó
-- không trả lời được bằng một form.
--
-- Dòng có khóa không còn trong sổ đăng ký sẽ bị BỎ QUA lúc nạp, không gây
-- lỗi: xóa khóa khỏi mã mà database còn dòng cũ là chuyện bình thường khi
-- triển khai lại.
CREATE TABLE ops_config (
    khoa    TEXT PRIMARY KEY CHECK (length(trim(khoa)) > 0),

    -- Một cột số duy nhất cho mọi kiểu.
    --
    -- Thời lượng lưu bằng GIỜ, tỷ lệ lưu 0..1, số nguyên lưu chính nó —
    -- kiểu thật do sổ đăng ký trong mã quyết định. Cột `jsonb` linh hoạt
    -- hơn nhưng mở cửa cho giá trị hình dạng bất kỳ, và ở đây mọi tham số
    -- đều là một con số.
    gia_tri DOUBLE PRECISION NOT NULL,

    sua_luc TIMESTAMPTZ NOT NULL,

    -- Người thực hiện và lý do BẮT BUỘC không rỗng.
    --
    -- Đổi tham số vận hành ảnh hưởng tới người NGOÀI công ty: hạ hạn giao
    -- hàng làm hàng loạt gian hàng đột ngột bị chấm là giao trễ. Một lần
    -- đổi không có người chịu trách nhiệm và không có lý do thì không giải
    -- thích được khi nhà bán khiếu nại.
    sua_boi TEXT NOT NULL CHECK (length(trim(sua_boi)) > 0),
    ly_do   TEXT NOT NULL CHECK (length(trim(ly_do)) >= 20)
);
