-- Hai phép đổi DỮ LIỆU, không phải đổi tên.
--
-- Khác 000062/000063 ở một điểm quyết định: ở đây giá trị TRONG CÁC HÀNG
-- đổi, nên phép đảo phải đổi lại được từng hàng — và bộ CHECK phải đi cùng
-- chiều với dữ liệu trong CÙNG giao dịch, không thì một trong hai bước sẽ
-- thấy hàng không thoả ràng buộc.

-- ========================= 1. enum tiếng Việt cuối cùng của hệ thống
--
-- `size_observation.outcome` là enum DUY NHẤT còn giá trị tiếng Việt.
-- Mọi enum khác dùng tiếng Anh: `PURCHASED`, `WRONG_ITEM_SENT`, `ACTIVE`.
--
-- # Vì sao nó là việc đáng làm, không phải thẩm mỹ
--
-- Giá trị enum đi RA NGOÀI: nó vào `metadata` của event, vào log, và sẽ vào
-- phản hồi API ngày nào gợi ý size có endpoint riêng. Một tập enum nửa
-- tiếng Anh nửa tiếng Việt buộc mọi bên đọc phải biết CẢ HAI quy ước —
-- cùng dạng lỗi mà `apicheck` tầng enum dựng ra để chặn.
--
-- Bảng này chỉ do `recommendation` ghi và đọc; không có khoá ngoại nào trỏ
-- tới nó, nên đổi giá trị là phép cục bộ.

ALTER TABLE size_observation DROP CONSTRAINT size_observation_outcome_check;

UPDATE size_observation SET outcome = CASE outcome
    WHEN 'DA_MUA' THEN 'PURCHASED'
    WHEN 'CHAT'   THEN 'TIGHT'
    WHEN 'RONG'   THEN 'LOOSE'
    ELSE outcome
END;

ALTER TABLE size_observation
    ADD CONSTRAINT size_observation_outcome_check
    CHECK (outcome IN ('PURCHASED', 'TIGHT', 'LOOSE'));

-- ========================= 2. P3-82: hai hằng số dùng chung tiền tố `evt`
--
-- `ids.PrefixEmailToken` và `ids.PrefixEvent` đều là "evt", nên một mã token
-- xác minh email và một mã domain event không phân biệt được bằng tiền tố —
-- thứ mà mọi `ids.Parse` dựa vào.
--
-- # Backlog hoãn việc này vì một tiền đề SAI
--
-- P3-82 ghi: "sửa nó cần migration dữ liệu: đổi tiền tố nghĩa là mọi mã đã
-- phát ra không parse được nữa". Đọc lại đường xác minh thì không phải:
--
--   identity/infrastructure/postgres/email_token.go:47
--     WHERE token_hash = $1
--
-- Liên kết gửi cho khách mang TOKEN THÔ, và tra cứu đi theo `token_hash`.
-- Cột `id` chỉ là khoá chính nội bộ, không xuất hiện trong liên kết nào và
-- không có khoá ngoại nào trỏ tới (kiểm bằng `pg_constraint`, contype='f').
--
-- Nên đổi nó KHÔNG làm hỏng một liên kết đã gửi nào. Việc bị hoãn một tháng
-- vì một câu phỏng đoán không ai kiểm lại.
--
-- `emt` cho email token là lựa chọn ít va đập hơn: bảng nhỏ, token có hạn
-- dùng ngắn, và `evt` giữ nguyên nghĩa "event" ở nơi nó đông hàng nhất
-- (`event_outbox.event_id`, 16.061 hàng trên môi trường phát triển).

ALTER TABLE email_verification_token DROP CONSTRAINT email_verification_token_id_check;

UPDATE email_verification_token
   SET id = 'emt_' || substring(id FROM 5)
 WHERE id LIKE 'evt\_%';

ALTER TABLE email_verification_token
    ADD CONSTRAINT email_verification_token_id_check
    CHECK (id LIKE 'emt\_%' AND length(id) = 30);
