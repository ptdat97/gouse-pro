-- settlement ghi AI KÝ ở mỗi bước của vòng đời.
--
-- # Vì sao vòng đời này tới hôm nay mới khép
--
-- `DoiSoat.XacNhan` và `DoiSoat.DanhDauDaTra` có từ migration 000036, với
-- đủ hàng rào trạng thái, và **không lời gọi nào**. Mọi đợt đối soát nằm
-- `DRAFT` vĩnh viễn — nhà bán không bao giờ được chi.
--
-- Khép nó cần một quyết định không thuộc về mã: AI ĐƯỢC KÝ CHI. Quyết ngày
-- 01/10/2026 — hai bước, có người ký, mỗi bước ghi ai bấm và lúc nào (P3-89).
--
-- # Vì sao LƯU người ký, không chỉ ghi vết kiểm toán
--
-- `platform/audit` đã ghi mọi thao tác quản trị, nên trên nguyên tắc người
-- ký có thể tra từ đó. Nhưng vết kiểm toán là một bảng CHUNG, và câu hỏi
-- "ai ký đợt này" là câu hỏi về CHÍNH đợt ấy — nó xuất hiện trên màn hình
-- chi tiết đợt, trong báo cáo đối soát, và trong tranh chấp với nhà bán.
--
-- Bắt mọi bên đọc đi JOIN `audit_log` theo chuỗi mã là biến một thuộc tính
-- thành một truy vấn. Hai nguồn ở đây KHÔNG phải hai nguồn sự thật: audit
-- ghi SỰ KIỆN (ai gọi API nào, lúc nào), cột này ghi TRẠNG THÁI (đợt này
-- do ai ký).
--
-- NULL nghĩa là bước ấy chưa xảy ra, đúng như `confirmed_at`/`paid_at`.
ALTER TABLE settlement
    ADD COLUMN confirmed_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN paid_by      TEXT NOT NULL DEFAULT '';

-- Một bước ĐÃ XẢY RA thì PHẢI có người ký.
--
-- # Vì sao ràng buộc ở tầng database
--
-- Đây là tiền RỜI nền tảng. Một đợt `PAID` mà không biết ai ký là một khoản
-- chi không ai chịu trách nhiệm — và vết kiểm toán không tạo ngược được.
--
-- Hàng rào ở tầng ứng dụng vẫn cần (báo lỗi rõ hơn), nhưng nó không phải
-- chốt chặn cuối: một câu UPDATE viết tay lúc xử lý sự cố cũng đi qua nó.
--
-- `DEFAULT ''` ở trên cho phép ALTER chạy trên dữ liệu cũ; ràng buộc này
-- chỉ áp khi trạng thái đã TIẾN, nên đợt DRAFT cũ không bị chặn.
ALTER TABLE settlement
    ADD CONSTRAINT settlement_xac_nhan_phai_co_nguoi_ky CHECK (
        confirmed_at IS NULL OR confirmed_by <> ''
    ),
    ADD CONSTRAINT settlement_da_tra_phai_co_nguoi_ky CHECK (
        paid_at IS NULL OR paid_by <> ''
    );
