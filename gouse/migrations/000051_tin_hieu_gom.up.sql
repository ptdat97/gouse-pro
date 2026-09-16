-- Khóa chống trùng cho TÍN HIỆU GOM.
--
-- # Vì sao bảng tín hiệu vốn KHÔNG có khóa nào
--
-- `demand_signal` là nhật ký CHỈ THÊM: mỗi lần thêm giỏ, mỗi lần hết hàng
-- là một sự thật riêng đã xảy ra, và hai sự thật giống hệt nhau vẫn là hai
-- sự thật. Đặt khóa duy nhất lên toàn bảng sẽ làm mất chính điều đó.
--
-- # Vì sao tín hiệu GOM thì khác
--
-- Lượt xem sản phẩm có khối lượng lớn hơn mọi tín hiệu khác hai bậc. Ghi
-- một dòng cho mỗi lượt xem sẽ làm bảng này phình theo lưu lượng đọc chứ
-- không theo nhu cầu — và tổng hợp về sau vẫn phải gom lại.
--
-- Nên lượt xem được GOM theo NGÀY × SẢN PHẨM trước khi ghi. Một dòng gom
-- không phải "một sự thật đã xảy ra" mà là "kết quả đếm của một ngày", và
-- đếm lại cùng một ngày phải CẬP NHẬT chứ không thêm dòng mới.
--
-- Chỉ mục MỘT PHẦN: nó chỉ ràng buộc dòng gom, nên nhật ký chỉ-thêm của
-- mọi loại tín hiệu khác không bị đụng tới.
CREATE UNIQUE INDEX IF NOT EXISTS demand_signal_gom_idx
    ON demand_signal (signal_type, product_id, source_id)
    WHERE source_type = 'view_rollup';
