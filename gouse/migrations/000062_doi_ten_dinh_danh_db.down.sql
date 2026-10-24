-- Đảo: trả mọi định danh về tên tiếng Việt cũ.
--
-- Thứ tự NGƯỢC lúc tiến. Hàm và trigger làm TRƯỚC: chúng phụ thuộc bảng
-- `ledger_line`, không phụ thuộc các phép đổi tên khác, nên đặt đâu cũng
-- được — nhưng giữ thứ tự ngược làm file này đọc đối chiếu được với file
-- `up` từng khối một.

-- ---- 5. Hàm + trigger ----
CREATE OR REPLACE FUNCTION kiem_but_toan_can_bang()
RETURNS TRIGGER AS $$
DECLARE
    so_dong   INT;
    lech      RECORD;
BEGIN
    SELECT count(*) INTO so_dong FROM ledger_line WHERE entry_id = NEW.id;

    IF so_dong = 0 THEN
        RAISE EXCEPTION 'Bút toán % không có dòng nào', NEW.id
            USING HINT = 'Mọi bút toán phải có ít nhất hai dòng: một NỢ, một CÓ';
    END IF;

    FOR lech IN
        SELECT currency,
               sum(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END) AS no,
               sum(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END) AS co
        FROM ledger_line
        WHERE entry_id = NEW.id
        GROUP BY currency
        HAVING sum(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END)
            <> sum(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END)
    LOOP
        RAISE EXCEPTION
            'Bút toán % không cân bằng ở đơn vị %: NỢ % ≠ CÓ %',
            NEW.id, lech.currency, lech.no, lech.co
            USING HINT = 'Σ DEBIT phải bằng Σ CREDIT trong TỪNG đơn vị tiền';
    END LOOP;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ledger_entry_must_balance ON ledger_entry;

CREATE CONSTRAINT TRIGGER ledger_entry_phai_can_bang
    AFTER INSERT ON ledger_entry
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION kiem_but_toan_can_bang();

DROP FUNCTION IF EXISTS check_ledger_entry_balanced();

-- ---- 4. Chỉ mục một phần ----
ALTER INDEX payment_intent_awaiting_capture  RENAME TO payment_intent_cho_thu;
ALTER INDEX return_line_awaiting_inspection  RENAME TO return_line_cho_kiem_dinh;
ALTER INDEX webhook_event_unprocessed        RENAME TO webhook_event_chua_xu_ly;
ALTER INDEX email_verification_token_still_valid
    RENAME TO email_verification_token_con_hieu_luc;

-- ---- 3. Ràng buộc CHECK ----
ALTER TABLE settlement
    RENAME CONSTRAINT settlement_paid_needs_signer
                   TO settlement_da_tra_phai_co_nguoi_ky;

ALTER TABLE settlement
    RENAME CONSTRAINT settlement_confirmed_needs_signer
                   TO settlement_xac_nhan_phai_co_nguoi_ky;

ALTER TABLE payment_intent
    RENAME CONSTRAINT payment_intent_captured_has_time
                   TO payment_intent_captured_co_moc;

ALTER TABLE commission_record
    RENAME CONSTRAINT commission_reversed_needs_reason
                   TO commission_dao_phai_co_ly_do;

-- ---- 2. ops_config ----
ALTER TABLE ops_config RENAME COLUMN reason     TO ly_do;
ALTER TABLE ops_config RENAME COLUMN updated_by TO sua_boi;
ALTER TABLE ops_config RENAME COLUMN updated_at TO sua_luc;
ALTER TABLE ops_config RENAME COLUMN value      TO gia_tri;
ALTER TABLE ops_config RENAME COLUMN key        TO khoa;

-- ---- 1. Bảng size_observation ----
ALTER INDEX size_observation_customer_idx RENAME TO quan_sat_size_khach_idx;
ALTER INDEX size_observation_source_idx   RENAME TO quan_sat_size_nguon_idx;

ALTER TABLE size_observation RENAME COLUMN observed_at TO quan_sat_luc;
ALTER TABLE size_observation RENAME COLUMN source_id   TO nguon_id;
ALTER TABLE size_observation RENAME COLUMN source_type TO nguon_loai;
ALTER TABLE size_observation RENAME COLUMN outcome     TO ket_qua;

ALTER TABLE size_observation RENAME TO quan_sat_size;
