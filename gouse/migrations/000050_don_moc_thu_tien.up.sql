-- Mốc THU ĐƯỢC TIỀN của đơn hàng.
--
-- # Vì sao trạng thái không đủ để nói "đã trả tiền"
--
-- Cột `status` mang TIẾN ĐỘ GIAO HÀNG: PENDING_PAYMENT → PAID → PROCESSING
-- → SHIPPED → DELIVERED. Với đơn TRẢ TRƯỚC thì tiền về ở đầu chuỗi, nên
-- một cột diễn đạt được cả hai việc.
--
-- Với COD thì không. Tiền về lúc GIAO — tức là ở CUỐI chuỗi, khi đơn đã
-- mang trạng thái DELIVERED. Ghi nhận thu tiền bằng cách đặt status = PAID
-- sẽ đẩy đơn LÙI lại và xóa mất sự thật "đã giao xong".
--
-- Hai trục khác nhau thì cần hai cột. `paid_at` là trục TIỀN; `status` giữ
-- nguyên nghĩa là trục HÀNG.
--
-- # Vì sao là mốc thời gian chứ không phải cờ boolean
--
-- Đối soát cần biết tiền về LÚC NÀO, không chỉ "đã về chưa": chênh lệch
-- giữa ngày giao và ngày hãng vận chuyển chuyển tiền COD về là một khoản
-- phải thu có tuổi, và tuổi của nó là thứ người vận hành đi đòi.
--
-- NULL nghĩa là chưa thu được tiền.
ALTER TABLE "order"
    ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;

-- Đơn đã thu tiền thì phải tra nhanh được: đối soát chạy theo khoảng ngày.
CREATE INDEX IF NOT EXISTS order_paid_at_idx ON "order" (paid_at)
    WHERE paid_at IS NOT NULL;
