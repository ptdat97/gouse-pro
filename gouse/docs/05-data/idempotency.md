# Idempotency

## 1. Vì sao bắt buộc

> Một thao tác **idempotent** cho kết quả giống nhau dù thực hiện một lần hay nhiều lần với cùng đầu vào.

Đây không phải tính năng nâng cao — nó là **điều kiện để hệ thống đúng**.

### Những việc chắc chắn sẽ xảy ra

```text
1. Khách bấm nút "Đặt hàng" hai lần
2. Ứng dụng di động mất mạng, tự thử lại
3. Cổng thanh toán gửi webhook trùng
4. Event từ outbox được phát hai lần (mô hình at-least-once)
5. Tiến trình bị khởi động lại giữa chừng
6. Người vận hành chạy lại một job
```

Không có idempotency, mỗi tình huống trên đều dẫn tới: đơn hàng trùng, trừ tiền hai lần, tồn kho trừ hai lần, hoặc trả tiền seller hai lần.

**Trường hợp tốn kém nhất:** chuyển tiền hai lần cho seller do lỗi thử lại — rất khó thu hồi.

---

## 2. Ba nơi bắt buộc idempotent

```text
1. API thay đổi trạng thái      → header Idempotency-Key
2. Bên xử lý event              → kiểm tra event_id đã xử lý
3. Webhook từ bên ngoài         → kiểm tra id giao dịch của nhà cung cấp
```

---

## 3. API idempotency

### Giao thức

```http
POST /api/v1/orders
Idempotency-Key: 01J9XABC123DEF456
Content-Type: application/json

{ "checkout_id": "chk_01J9X..." }
```

### Hành vi

```text
Lần đầu với key này
    → xử lý, lưu kết quả gắn với key
    → trả kết quả

Gọi lại, cùng key, CÙNG nội dung
    → KHÔNG xử lý lại
    → trả kết quả đã lưu (cùng mã HTTP, cùng body)

Gọi lại, cùng key, KHÁC nội dung
    → trả 409 Conflict
    → vì key đã dùng cho request khác

Key hết hạn (ví dụ sau 24 giờ)
    → xử lý như request mới

Request đang xử lý, gọi lại cùng key
    → trả 409 với mã IDEMPOTENT_REQUEST_IN_PROGRESS
```

### Cài đặt

```sql
CREATE TABLE idempotency_key (
    key             TEXT PRIMARY KEY,
    request_hash    TEXT NOT NULL,      -- băm nội dung request
    status          TEXT NOT NULL,      -- IN_PROGRESS | COMPLETED | FAILED
    response_status INT,
    response_body   JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);
```

```text
Luồng xử lý:

1. BEGIN
2. INSERT idempotency_key (status=IN_PROGRESS)
   → nếu trùng khóa:
       - đã COMPLETED  → trả response đã lưu
       - đang IN_PROGRESS → trả 409
3. Kiểm tra request_hash khớp không → khác thì 409
4. Xử lý nghiệp vụ
5. UPDATE idempotency_key (status=COMPLETED, lưu response)
6. COMMIT
```

**Điểm mấu chốt ở bước 5–6:** lưu kết quả và xử lý nghiệp vụ phải trong **cùng một giao dịch**. Nếu tách, có thể xử lý xong nhưng chưa kịp lưu kết quả — lần thử lại sẽ xử lý lần nữa.

---

## 4. Idempotency cho bên xử lý event

```go
func (h *FinancialHandler) HandleOrderPlaced(ctx context.Context, e DomainEvent) error {
    return h.db.InTransaction(ctx, func(tx Tx) error {
        // Đánh dấu đã xử lý — nếu trùng, INSERT thất bại
        inserted, err := h.markProcessed(tx, e.EventID, "financial")
        if err != nil {
            return err
        }
        if !inserted {
            return nil  // đã xử lý rồi, bỏ qua
        }

        // Xử lý trong CÙNG giao dịch
        return h.recordLedgerEntry(tx, e)
    })
}
```

```sql
CREATE TABLE processed_event (
    event_id     UUID NOT NULL,
    handler_name TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, handler_name)
);
```

**Vì sao khóa chính gồm cả `handler_name`:** một event có nhiều bên nhận. Mỗi bên xử lý độc lập và phải theo dõi riêng.

---

## 5. Idempotency cho webhook

```text
Webhook từ cổng thanh toán:

1. Xác minh chữ ký  ← bắt buộc, chống giả mạo
2. Lấy id giao dịch của nhà cung cấp
3. Kiểm tra đã xử lý id này chưa
   → rồi: trả 200 ngay, không xử lý lại
4. Xử lý + đánh dấu trong cùng giao dịch
5. Trả 200

Nguyên tắc: trả 200 NHANH, xử lý nặng chuyển sang bất đồng bộ.
Nhà cung cấp thường có timeout ngắn và sẽ gửi lại nếu chậm.
```

---

## 6. Idempotency tự nhiên — ưu tiên khi có thể

Một số thao tác **tự nhiên idempotent** nếu thiết kế đúng:

```text
Không idempotent:
    UPDATE inventory SET quantity = quantity - 10

Idempotent tự nhiên:
    UPDATE inventory SET quantity = 90 WHERE version = 5
    → chạy lại không đổi kết quả
```

```text
Không idempotent:
    INSERT INTO ledger_entry (...)

Idempotent tự nhiên:
    INSERT INTO ledger_entry (..., idempotency_key)
    ON CONFLICT (idempotency_key) DO NOTHING
```

**Nguyên tắc:** thiết kế thao tác idempotent tự nhiên tốt hơn là thêm lớp kiểm tra bên ngoài. Ít code hơn, ít chỗ sai hơn.

---

## 7. Sinh idempotency key

### Client sinh

```text
Client tạo ULID mới mỗi khi NGƯỜI DÙNG thực hiện hành động
    → giữ nguyên key khi THỬ LẠI cùng hành động đó
    → key mới khi người dùng bấm lại có chủ đích
```

**Điểm quan trọng:** key phải gắn với **ý định của người dùng**, không phải với lần gọi mạng.

```text
SAI:  sinh key mới mỗi lần gọi HTTP
      → thử lại tạo key mới → tạo đơn trùng

ĐÚNG: sinh key khi người dùng bấm nút
      → mọi lần thử lại dùng cùng key
```

### Server sinh (cho luồng nội bộ)

```text
Sinh từ dữ liệu nghiệp vụ, đảm bảo ổn định:

    ledger_entry cho đơn hàng:
        key = "order_revenue:" + order_id

    hoa hồng creator:
        key = "creator_commission:" + order_line_id + ":" + creator_id
```

Cách này đảm bảo dù xử lý bao nhiêu lần cũng chỉ tạo một bút toán.

---

## 8. Danh sách thao tác bắt buộc idempotent

| Thao tác | Cơ chế |
|---|---|
| Tạo đơn hàng | Idempotency-Key |
| Thanh toán | Idempotency-Key + id giao dịch PSP |
| **Chi trả cho seller** | **Idempotency-Key + kiểm tra trạng thái trước khi gọi ngân hàng** |
| Hoàn tiền | Idempotency-Key |
| Ghi bút toán | Khóa duy nhất trên `idempotency_key` |
| Giữ tồn kho | Kiểm tra reservation đã tồn tại |
| Tạo fulfillment order | Kiểm tra đã có FO cho order_id chưa |
| Ghi nhận quy kết | Khóa duy nhất trên (order_line_id, creator_id) |
| Tích/tiêu điểm | Idempotency key theo tham chiếu |
| Gửi thông báo | Kiểm tra đã gửi cho event này chưa |
| Ghi nhận dùng mã giảm giá | Khóa duy nhất (coupon_id, order_id) |

Dòng in đậm là thao tác nguy hiểm nhất — chuyển tiền thật ra ngoài hệ thống.

---

## 9. Dọn dẹp

```text
idempotency_key:  xóa sau 24–48 giờ
processed_event:  giữ lâu hơn (7–30 ngày), tùy khả năng event đến trễ

Lưu ý: giữ quá ngắn → event đến trễ bị xử lý lại
       giữ quá lâu  → bảng phình
```

---

## 10. Kiểm thử

Mỗi thao tác idempotent cần test:

```text
1. Gọi hai lần cùng key, cùng nội dung
   → chỉ tạo một bản ghi, trả cùng kết quả

2. Gọi hai lần cùng key, khác nội dung
   → lần hai trả 409

3. Gọi đồng thời hai lần cùng key
   → chỉ một thành công, một trả 409

4. Xử lý event hai lần
   → chỉ tác động một lần

5. Giao dịch thất bại giữa chừng, thử lại
   → không để lại trạng thái nửa vời
```

Trường hợp 3 (đồng thời) là dễ bỏ sót nhất và cũng dễ gây lỗi nhất trong thực tế.

---

## 11. Giới hạn của khóa idempotency

Khóa idempotency bảo vệ **một ý định của người dùng khỏi bị gửi lặp**. Nó
KHÔNG bảo vệ được gì trước **hai ý định thật trên cùng một đối tượng**.

Khác biệt ấy nghe nhỏ, nhưng nó đúng bằng khoảng cách giữa "khách có một
đơn" và "khách có năm đơn".

```text
Khóa idempotency BẮT được:
    một lần bấm → mạng lỗi → tự thử lại → cùng key
    → một đơn

Khóa idempotency KHÔNG bắt được:
    tab 1 bấm "Đặt hàng"  → key A ─┐ cùng một giỏ
    tab 2 bấm "Đặt hàng"  → key B ─┘ cùng một phiên
    → hai key khác nhau, đúng theo định nghĩa ở mục 7
    → HAI ĐƠN
```

Đo thật trên hệ thống này: 8 request `POST /checkout/{id}/complete` chạy
song song với 8 khóa khác nhau tạo ra **3–7 đơn hàng** cho một giỏ.

### Quy tắc rút ra

Với mỗi thao tác ghi, phải trả lời được hai câu hỏi RIÊNG BIỆT:

| Câu hỏi | Công cụ |
|---------|---------|
| Cùng một ý định gửi nhiều lần thì sao? | Khóa idempotency |
| Cùng một đối tượng nhận nhiều ý định thì sao? | Ràng buộc trên chính đối tượng |

Câu thứ hai thường bị bỏ quên vì câu thứ nhất đã được trả lời, và người
viết code tưởng là đã xong.

Với việc hoàn tất thanh toán, câu trả lời cho câu thứ hai là chỉ mục
`order_one_per_checkout` — xem [ADR-0013](../adr/0013-write-transaction-boundary.md).

---

## 12. Tài liệu liên quan

- [consistency.md](consistency.md) — vì sao at-least-once cần idempotency
- [../02-domain/domain-events.md](../02-domain/domain-events.md) mục 6
- [../06-api/api-guidelines.md](../06-api/api-guidelines.md) — giao thức API
- [../04-modules/payment.md](../04-modules/payment.md) — thao tác tài chính
- [../adr/0013-write-transaction-boundary.md](../adr/0013-write-transaction-boundary.md) — ranh giới giao dịch của phép đọc-rồi-ghi
