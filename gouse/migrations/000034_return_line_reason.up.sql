-- Lý do trả hàng theo TỪNG DÒNG.
--
-- VÌ SAO: khách trả hai món trong một đơn vì hai lý do khác nhau là chuyện
-- thường — cái áo chật, cái quần lỗi đường may. Gộp thành một lý do chung
-- làm mất đúng thứ mà lý do sinh ra để mang: hàng lỗi thì truy vết lô sản
-- xuất, sai size thì sửa bảng size. Trộn hai việc lại thì không làm được
-- việc nào.
--
-- Đặc tả (api/paths/orders.yaml#requestReturn) đặt reason_code ở cấp dòng
-- ngay từ đầu; cài đặt ban đầu của tôi đặt ở cấp yêu cầu và đã lệch.
ALTER TABLE return_line
    ADD COLUMN reason_code TEXT NOT NULL DEFAULT 'CHANGED_MIND'
        CHECK (reason_code IN (
            'SIZE_TOO_SMALL', 'SIZE_TOO_LARGE',
            'NOT_AS_DESCRIBED', 'COLOR_DIFFERENT',
            'QUALITY_ISSUE', 'DEFECTIVE',
            'WRONG_ITEM_SENT', 'DAMAGED_IN_TRANSIT',
            'CHANGED_MIND', 'LATE_DELIVERY'
        )),
    ADD COLUMN reason_detail TEXT NOT NULL DEFAULT '';

-- return_request.reason_code giữ lại làm LÝ DO CHÍNH — lý do của dòng đầu
-- tiên. Cố ý trùng lặp dữ liệu: danh sách yêu cầu của một gian hàng lọc
-- theo lý do rất thường xuyên, và JOIN sang bảng dòng cho mỗi lần lọc là
-- cái giá không đáng.
COMMENT ON COLUMN return_request.reason_code IS
    'Lý do CHÍNH (của dòng đầu). Lý do đầy đủ nằm ở return_line.reason_code.';
