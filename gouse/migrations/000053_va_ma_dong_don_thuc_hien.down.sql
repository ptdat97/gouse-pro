-- KHÔNG khôi phục mã cũ.
--
-- Bản lên thay một mã SAI bằng mã đúng và không lưu lại mã sai ở đâu —
-- có chủ ý. Giá trị cũ không dùng được vào việc gì: nó trỏ tới dòng của
-- một phiên thanh toán đã đóng, và không endpoint nào nhận nó.
--
-- Đi xuống rồi đi lên lại vẫn cho cùng kết quả: bản lên là idempotent,
-- nó chỉ chạm những dòng còn mang tiền tố `cln_`.

SELECT 1;
