-- Ghi PHƯƠNG THỨC THANH TOÁN khách đã chọn vào đơn (P3-9).
--
-- Trước đây `payment_method` được tầng HTTP kiểm tra hợp lệ rồi VỨT ĐI: nó
-- không có chỗ nào để đi tới. Hệ quả là đơn COD và đơn chờ chuyển khoản
-- giống hệt nhau trong database, nên không ai trả lời được câu hỏi mà kho
-- thật sự cần: đơn này có phải thu tiền lúc giao không?
--
-- CHO PHÉP NULL, có chủ ý. Hai đường tạo đơn không giống nhau:
--
--     POST /api/v1/checkout/{id}/complete  → đặc tả BẮT BUỘC payment_method
--     POST /api/v1/orders    (placeOrder)  → đặc tả chỉ có checkout_id
--
-- Nên "chưa có lựa chọn" là một trạng thái CÓ THẬT, không phải dữ liệu
-- thiếu. Đặt DEFAULT 'COD' cho gọn sẽ khiến kho đi thu tiền của đơn đã trả
-- trước — bịa dữ liệu nguy hiểm hơn hẳn việc thừa nhận mình chưa biết.
--
-- Đơn CŨ (trước migration này) cũng là NULL và cũng vì đúng lý do đó:
-- lựa chọn của những khách ấy đã bị vứt đi, không có gì để khôi phục.
ALTER TABLE "order"
    ADD COLUMN payment_method TEXT
        CHECK (payment_method IN ('CARD', 'BANK_TRANSFER', 'E_WALLET', 'COD'));

-- Ràng buộc CHECK chứ không chỉ kiểm ở Go: cùng lý do với migration 000040.
-- Kiểm ở một tầng thì tầng khác vẫn ghi được giá trị lạ vào, và một giá trị
-- domain không có tên tương ứng sẽ hỏng ÂM THẦM lúc đọc. Siết ở database
-- biến nó thành lỗi ồn ào lúc ghi.

COMMENT ON COLUMN "order".payment_method IS
    'Phương thức khách chọn, ĐÓNG BĂNG lúc đặt. NULL = đơn tạo qua placeOrder, chưa chọn.';
