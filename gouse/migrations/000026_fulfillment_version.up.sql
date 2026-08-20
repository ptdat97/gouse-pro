-- Khóa lạc quan cho fulfillment_order.
--
-- VÌ SAO CẦN: bước chuyển trạng thái đọc bản ghi, kiểm tra chuyển có hợp
-- lệ không, rồi ghi. Ba việc đó KHÔNG nguyên tử. Hai request song song
-- cùng đọc PACKED, cả hai cùng thấy hợp lệ, cả hai cùng ghi.
--
-- Kiểm chứng bằng test tranh chấp trước khi thêm cột này: 8 lệnh "đã bàn
-- giao vận chuyển" chạy song song thì 2–3 lệnh cùng thành công. Hệ quả
-- không chỉ là một dòng thừa — mỗi lần ghi phát một event tiến độ, nên
-- khách nhận HAI email "đơn đã gửi", analytics đếm hai lần, và mã vận đơn
-- ghi sau đè lên mã ghi trước.
--
-- Cùng cơ chế với inventory_item.version. Dùng chung một mẫu để người đọc
-- không phải học hai cách bảo vệ khác nhau cho cùng một loại vấn đề.
ALTER TABLE fulfillment_order
    ADD COLUMN version BIGINT NOT NULL DEFAULT 0;
