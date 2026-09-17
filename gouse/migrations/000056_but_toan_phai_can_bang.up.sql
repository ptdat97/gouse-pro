-- Bút toán KHÔNG cân bằng: từ chối ở tầng DATABASE, không chỉ theo dõi.
--
-- VÌ SAO KHÔNG CHỌN "THÊM MỘT CHỈ SỐ"
--
-- mvp.md mục 7 ghi tiêu chí "Bút toán không cân bằng = 0". Đo tay ngày
-- 17/09/2026 ra 0/6.229 — đúng, mà KHÔNG AI CANH: bộ đếm
-- `gouse_business_failures_total{reason="unbalanced"}` đếm những lượt ghi
-- bị TỪ CHỐI, tức đường ghi đang làm đúng việc, chứ không đếm dữ liệu đã
-- lưu. PH-15 mở ra để sửa chỗ đó.
--
-- Cách rẻ nhất là thêm một job quét và một cảnh báo. Nó sai theo hai
-- hướng:
--
--	phát hiện SAU khi đã ghi   Sổ cái BẤT BIẾN (ADR-0008). Bút toán lệch
--	                           không xóa được, chỉ đảo được — tức mọi báo
--	                           cáo tài chính giữa lúc ghi và lúc đảo đều
--	                           sai, và không ai biết mình đang đọc số sai.
--	cửa sổ quét hữu hạn        Quét toàn bảng mỗi 10 phút là lãng phí;
--	                           quét cửa sổ thì bút toán lệch trôi ra ngoài
--	                           cửa sổ sẽ làm chỉ số tụt về 0 trong khi dữ
--	                           liệu hỏng vẫn nằm đó. Cảnh báo TỰ TẮT còn
--	                           tệ hơn không có cảnh báo.
--
-- Tồn kho đã có câu trả lời đúng từ migration 000004: `CHECK (… >= 0)` ở
-- tầng database, gọi thẳng là "LỚP BẢO VỆ CUỐI CÙNG". Nhờ nó mà tiêu chí
-- "tồn kho âm = 0" không cần ai canh — nó không xảy ra được.
--
-- Bút toán cân bằng cũng đáng được như vậy.
--
-- VÌ SAO PHẢI LÀ CONSTRAINT TRIGGER HOÃN
--
-- `CHECK` không làm được: điều kiện trải trên NHIỀU DÒNG của `ledger_line`,
-- còn CHECK chỉ nhìn được một dòng.
--
-- Trigger thường cũng không: bút toán được ghi TRƯỚC các dòng của nó trong
-- cùng một giao dịch (xem payment/infrastructure/postgres/store.go). Một
-- trigger `AFTER INSERT` thường sẽ chạy lúc bảng dòng còn rỗng và từ chối
-- MỌI bút toán.
--
-- `DEFERRABLE INITIALLY DEFERRED` chạy ở thời điểm COMMIT — lúc đó bút
-- toán và mọi dòng của nó đều đã có mặt. Đây là công cụ duy nhất của
-- PostgreSQL kiểm được một bất biến trải trên nhiều bảng.

-- kiem_but_toan_can_bang kiểm BA bất biến, không phải một.
--
-- Ba cái tách riêng vì chúng hỏng theo ba kiểu khác nhau và cần ba thông
-- điệp khác nhau. Gộp thành một câu "bút toán không hợp lệ" là buộc người
-- trực sự cố tài chính phải tự đoán.
CREATE OR REPLACE FUNCTION kiem_but_toan_can_bang()
RETURNS TRIGGER AS $$
DECLARE
    so_dong   INT;
    lech      RECORD;
BEGIN
    SELECT count(*) INTO so_dong FROM ledger_line WHERE entry_id = NEW.id;

    -- 1. Bút toán RỖNG.
    --
    -- Phải kiểm riêng: Σ của tập rỗng bằng 0 ở cả hai vế, nên phép so
    -- cân bằng sẽ CHO QUA một bút toán không có dòng nào. Một bút toán
    -- rỗng không sai số học, nó chỉ không nói gì — và nó chiếm mất một
    -- khóa idempotency, nên lần ghi lại ĐÚNG sẽ bị từ chối là trùng.
    IF so_dong = 0 THEN
        RAISE EXCEPTION 'Bút toán % không có dòng nào', NEW.id
            USING HINT = 'Mọi bút toán phải có ít nhất hai dòng: một NỢ, một CÓ';
    END IF;

    -- 2 và 3. Cân bằng THEO TỪNG ĐƠN VỊ TIỀN.
    --
    -- Gom theo `currency` chứ không cộng tất: 100 JPY nợ và 100 VND có sẽ
    -- "cân bằng" nếu chỉ cộng cột `amount`, và đó là một bút toán vô
    -- nghĩa. Gom theo đơn vị tiền bắt được CẢ hai lỗi bằng một phép kiểm:
    -- bút toán lẫn đơn vị tiền sẽ có ít nhất một nhóm không cân.
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

-- FOR EACH ROW trên `ledger_entry`: MỘT lần kiểm cho mỗi bút toán.
--
-- Đặt trên `ledger_line` sẽ kiểm lại cùng một bút toán N lần — cùng kết
-- quả, N lần công.
CREATE CONSTRAINT TRIGGER ledger_entry_phai_can_bang
    AFTER INSERT ON ledger_entry
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION kiem_but_toan_can_bang();
