-- Đổi tên định danh DATABASE sang tiếng Anh.
--
-- # Vì sao một migration MỚI, không sửa migration cũ
--
-- `migrate` theo dõi theo PHIÊN BẢN, không theo nội dung file. Sửa
-- `000039_ops_config.up.sql` sẽ không chạy lại ở database nào đã áp nó, nên
-- file và database sẽ nói hai chuyện khác nhau — và đó là trạng thái tệ nhất
-- của một lược đồ.
--
-- Nên các file cũ GIỮ NGUYÊN. Chúng là lịch sử đúng của những gì đã chạy.
--
-- # Phạm vi
--
--	1 bảng · 9 cột · 5 ràng buộc · 5 chỉ mục · 1 hàm · 1 trigger
--
-- Quy ước: xem docs/03-architecture/quy-uoc-dat-ten.md. Định danh tiếng Anh;
-- chú thích GIỮ tiếng Việt, kể cả trong file này.

-- ---------------------------------------------------------------------
-- 1. Bảng quan_sat_size → size_observation
-- ---------------------------------------------------------------------
--
-- PostgreSQL KHÔNG đổi tên chỉ mục khi đổi tên bảng, nên hai chỉ mục phải
-- đổi tường minh ở dưới. Quên chúng để lại `quan_sat_size_khach_idx` trên
-- một bảng không còn tên ấy — một cái tên trỏ vào quá khứ.
ALTER TABLE quan_sat_size RENAME TO size_observation;

ALTER TABLE size_observation RENAME COLUMN ket_qua      TO outcome;
ALTER TABLE size_observation RENAME COLUMN nguon_loai   TO source_type;
ALTER TABLE size_observation RENAME COLUMN nguon_id     TO source_id;
ALTER TABLE size_observation RENAME COLUMN quan_sat_luc TO observed_at;

ALTER INDEX quan_sat_size_nguon_idx RENAME TO size_observation_source_idx;
ALTER INDEX quan_sat_size_khach_idx RENAME TO size_observation_customer_idx;

-- ---------------------------------------------------------------------
-- 2. ops_config — năm cột
-- ---------------------------------------------------------------------
--
-- `key` và `value` là từ KHÔNG dành riêng trong PostgreSQL, nên dùng được
-- không cần dấu nháy. Kiểm trước khi đặt: một cột phải nháy ở mọi truy vấn
-- là một cột sẽ có người quên nháy.
ALTER TABLE ops_config RENAME COLUMN khoa    TO key;
ALTER TABLE ops_config RENAME COLUMN gia_tri TO value;
ALTER TABLE ops_config RENAME COLUMN sua_luc TO updated_at;
ALTER TABLE ops_config RENAME COLUMN sua_boi TO updated_by;
ALTER TABLE ops_config RENAME COLUMN ly_do   TO reason;

-- ---------------------------------------------------------------------
-- 3. Ràng buộc CHECK
-- ---------------------------------------------------------------------
--
-- Năm tên này được NHẮC TRONG MÃ GO dưới dạng chuỗi: `isUnique(err, "...")`
-- và các phép so tên ràng buộc để dịch lỗi database thành lỗi miền. Đổi ở
-- đây mà quên ở đó làm một lỗi được xử đúng thành một lỗi 500 — xem phần Go
-- của cùng commit.
ALTER TABLE commission_record
    RENAME CONSTRAINT commission_dao_phai_co_ly_do
                   TO commission_reversed_needs_reason;

ALTER TABLE payment_intent
    RENAME CONSTRAINT payment_intent_captured_co_moc
                   TO payment_intent_captured_has_time;

ALTER TABLE settlement
    RENAME CONSTRAINT settlement_xac_nhan_phai_co_nguoi_ky
                   TO settlement_confirmed_needs_signer;

ALTER TABLE settlement
    RENAME CONSTRAINT settlement_da_tra_phai_co_nguoi_ky
                   TO settlement_paid_needs_signer;

-- ---------------------------------------------------------------------
-- 4. Chỉ mục một phần
-- ---------------------------------------------------------------------
-- `email_verification_token_con_hieu_luc` là CHỈ MỤC một phần, không phải
-- ràng buộc CHECK — bản đầu của migration này xếp nó vào khối ràng buộc và
-- PostgreSQL từ chối: `constraint ... does not exist`.
ALTER INDEX email_verification_token_con_hieu_luc
    RENAME TO email_verification_token_still_valid;

ALTER INDEX payment_intent_cho_thu     RENAME TO payment_intent_awaiting_capture;
ALTER INDEX return_line_cho_kiem_dinh  RENAME TO return_line_awaiting_inspection;
ALTER INDEX webhook_event_chua_xu_ly   RENAME TO webhook_event_unprocessed;

-- ---------------------------------------------------------------------
-- 5. Hàm kiểm cân bằng bút toán + trigger của nó
-- ---------------------------------------------------------------------
--
-- # Vì sao DỰNG LẠI hàm thay vì ALTER FUNCTION ... RENAME TO
--
-- `RENAME TO` đổi được tên hàm, nhưng KHÔNG đổi được tên biến cục bộ trong
-- thân nó (`so_dong`, `lech`). Mà thân hàm là chỗ người đọc thật sự đọc.
--
-- Nên: tạo hàm tên mới với thân đã đổi, trỏ trigger sang nó, rồi bỏ hàm cũ.
-- Thứ tự ấy giữ cho bất biến "mọi bút toán phải cân" KHÔNG có khoảng trống
-- nào — kể cả trong một giao dịch migration.
--
-- Thân hàm giữ NGUYÊN logic và nguyên chú thích tiếng Việt: đây là một lần
-- đổi tên, không phải một lần sửa quy tắc tiền.
CREATE OR REPLACE FUNCTION check_ledger_entry_balanced()
RETURNS TRIGGER AS $$
DECLARE
    line_count INT;
    imbalance  RECORD;
BEGIN
    SELECT count(*) INTO line_count FROM ledger_line WHERE entry_id = NEW.id;

    -- 1. Bút toán RỖNG.
    --
    -- Phải kiểm riêng: Σ của tập rỗng bằng 0 ở cả hai vế, nên phép so
    -- cân bằng sẽ CHO QUA một bút toán không có dòng nào. Một bút toán
    -- rỗng không sai số học, nó chỉ không nói gì — và nó chiếm mất một
    -- khóa idempotency, nên lần ghi lại ĐÚNG sẽ bị từ chối là trùng.
    IF line_count = 0 THEN
        RAISE EXCEPTION 'Bút toán % không có dòng nào', NEW.id
            USING HINT = 'Mọi bút toán phải có ít nhất hai dòng: một NỢ, một CÓ';
    END IF;

    -- 2 và 3. Cân bằng THEO TỪNG ĐƠN VỊ TIỀN.
    --
    -- Gom theo `currency` chứ không cộng tất: 100 JPY nợ và 100 VND có sẽ
    -- "cân bằng" nếu chỉ cộng cột `amount`, và đó là một bút toán vô
    -- nghĩa. Gom theo đơn vị tiền bắt được CẢ hai lỗi bằng một phép kiểm:
    -- bút toán lẫn đơn vị tiền sẽ có ít nhất một nhóm không cân.
    FOR imbalance IN
        SELECT currency,
               sum(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END) AS debit,
               sum(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END) AS credit
        FROM ledger_line
        WHERE entry_id = NEW.id
        GROUP BY currency
        HAVING sum(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END)
            <> sum(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END)
    LOOP
        RAISE EXCEPTION
            'Bút toán % không cân bằng ở đơn vị %: NỢ % ≠ CÓ %',
            NEW.id, imbalance.currency, imbalance.debit, imbalance.credit
            USING HINT = 'Σ DEBIT phải bằng Σ CREDIT trong TỪNG đơn vị tiền';
    END LOOP;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ledger_entry_phai_can_bang ON ledger_entry;

-- FOR EACH ROW trên `ledger_entry`: MỘT lần kiểm cho mỗi bút toán.
--
-- Đặt trên `ledger_line` sẽ kiểm lại cùng một bút toán N lần — cùng kết
-- quả, N lần công.
CREATE CONSTRAINT TRIGGER ledger_entry_must_balance
    AFTER INSERT ON ledger_entry
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_ledger_entry_balanced();

DROP FUNCTION IF EXISTS kiem_but_toan_can_bang();
