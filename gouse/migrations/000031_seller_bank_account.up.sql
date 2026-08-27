-- Tài khoản ngân hàng của nhà bán.
--
-- VÌ SAO CẦN: không có nó thì không trả tiền cho nhà bán được, và endpoint
-- đăng ký `applyAsSeller` không cài được vì đặc tả bắt buộc `bank_account`.
-- Trước migration này bảng `seller` chỉ có cờ `bank_account_verified` —
-- biết là "đã xác minh" mà không biết đã xác minh CÁI GÌ.
--
-- # Vì sao ba cột chứ không phải một
--
--   bank_account_number_enc  bản MÃ HÓA, chỉ đường chi trả đọc tới
--   bank_account_last4       bốn số cuối, RÕ, để hiển thị
--   bank_account_holder      tên chủ tài khoản, RÕ
--
-- Bốn số cuối lưu riêng ở dạng rõ là chủ ý, theo đúng quy tắc đã áp cho
-- thẻ ở docs/09-operations/security.md mục 7. Không có nó thì mọi màn hình
-- muốn hiện "…4567" đều phải giải mã, và đường giải mã càng nhiều nơi gọi
-- thì càng khó canh.
--
-- Tên chủ tài khoản để RÕ vì nó phải đối chiếu được với tên đăng ký ngay
-- lúc duyệt hồ sơ — mã hóa nó chỉ làm khâu duyệt phải giải mã, tức là mở
-- thêm một đường đọc mà không giấu được gì: tên doanh nghiệp vốn đã nằm
-- ngay cột bên cạnh.
--
-- Mã hóa: AES-256-GCM, khóa từ ENCRYPTION_KEY. Xem
-- docs/adr/0014-ma-hoa-truong-nhay-cam.md để biết vì sao chọn thế và
-- những gì CHƯA làm (xoay khóa, KMS).
ALTER TABLE seller
    ADD COLUMN bank_code               TEXT NOT NULL DEFAULT '',
    ADD COLUMN bank_account_number_enc TEXT NOT NULL DEFAULT '',
    ADD COLUMN bank_account_last4      TEXT NOT NULL DEFAULT '',
    ADD COLUMN bank_account_holder     TEXT NOT NULL DEFAULT '';

-- Nhà bán NỘI BỘ không có tài khoản để xác minh: hàng là của nền tảng và
-- tiền không đi đâu cả. Domain đã miễn cho họ (Seller.Activate bỏ qua kiểm
-- tra khi IsInternal), và ràng buộc `seller_active_needs_bank` ở migration
-- 000005 cũng miễn — nên ràng buộc ở đây phải miễn y hệt. Ba nơi nói khác
-- nhau thì sớm muộn một nơi chặn thứ nơi kia cho phép.
--
-- Hợp với ràng buộc cũ, chuỗi thành: ACTIVE ⇒ đã xác minh ⇒ CÓ tài khoản.

-- Bù dữ liệu, CHỈ với nhà bán chưa hoạt động.
--
-- Cờ "đã xác minh" của họ được bật mà chưa từng có tài khoản nào — hạ
-- xuống là ghi lại sự thật, không phải làm cho ràng buộc chạy được.
UPDATE seller
   SET bank_account_verified = false
 WHERE bank_account_verified
   AND status <> 'ACTIVE'
   AND seller_type <> 'INTERNAL'
   AND length(bank_account_number_enc) = 0;

-- NOT VALID: cưỡng chế với dòng MỚI và dòng được SỬA, không kiểm dòng cũ.
--
-- # Vì sao không kiểm dòng cũ ngay
--
-- Một nhà bán đang ACTIVE mà thiếu tài khoản là vấn đề THẬT — họ bán được
-- hàng nhưng không nhận được tiền. Có đúng hai cách xử lý, và cả hai đều
-- tệ nếu làm trong migration:
--
--   kiểm ngay      → migration thất bại, chặn cả đợt triển khai
--   tự hạ trạng thái → âm thầm ngắt một gian hàng đang bán, không ai biết
--
-- Cách thứ ba: chặn từ đây trở đi, và để việc dọn dữ liệu cũ thành một
-- thao tác vận hành CÓ NGƯỜI nhìn. Tìm các dòng cần dọn:
--
--   SELECT id, name FROM seller
--    WHERE status = 'ACTIVE' AND seller_type <> 'INTERNAL'
--      AND length(bank_account_number_enc) = 0;
--
-- Dọn xong thì khoá lại vĩnh viễn:
--
--   ALTER TABLE seller VALIDATE CONSTRAINT seller_verified_needs_account;
ALTER TABLE seller
    ADD CONSTRAINT seller_verified_needs_account CHECK (
        NOT bank_account_verified
        OR seller_type = 'INTERNAL'
        OR length(bank_account_number_enc) > 0
    ) NOT VALID;
