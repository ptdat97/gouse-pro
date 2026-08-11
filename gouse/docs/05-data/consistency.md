# Nhất quán dữ liệu

## 1. Mô hình nhất quán

Hệ thống dùng **hai mức** nhất quán, áp dụng có chủ đích:

```text
NHẤT QUÁN MẠNH (trong một aggregate)
    → giao dịch database
    → bất biến luôn được giữ

NHẤT QUÁN CUỐI (giữa các aggregate/module)
    → domain event
    → có độ trễ ngắn, chấp nhận được
```

**Nguyên tắc P11:** một giao dịch database chỉ sửa **một** aggregate.

---

## 2. Vì sao không dùng nhất quán mạnh khắp nơi

```text
Nếu mọi thứ nhất quán mạnh:
    - Giao dịch trải rộng nhiều module → tranh chấp khóa
    - Một module chậm làm chậm tất cả
    - Không thể tách service sau này
    - Điểm nghẽn ghi ở các bảng trung tâm

Nếu nhất quán cuối giữa module:
    - Mỗi module giao dịch độc lập
    - Lỗi ở một module không chặn module khác
    - Tách service chỉ là thay cơ chế truyền event
```

---

## 3. Cái gì PHẢI nhất quán mạnh

Ba nhóm bất biến không được phép vi phạm dù chỉ trong khoảnh khắc:

### 3.1 Tồn kho

```text
available + reserved + committed + in_transit + damaged + returned
    = tổng số lượng vật lý
mọi thành phần ≥ 0
```

Vi phạm → bán hàng không có, hoặc hàng bị khóa vĩnh viễn.

### 3.2 Bút toán tài chính

```text
Trong mỗi LedgerEntry: Σ DEBIT = Σ CREDIT
```

Vi phạm → sổ sách sai, không đối soát được.

### 3.3 Tổng đơn hàng

```text
Order.total = Σ OrderLine.line_total + phí − giảm giá
```

Vi phạm → khách bị tính sai tiền.

---

## 4. Cái gì CHẤP NHẬN nhất quán cuối

| Trường hợp | Độ trễ chấp nhận | Hậu quả nếu trễ |
|---|---|---|
| Trạng thái offer sau khi hết hàng | Vài giây | Khách thấy "còn hàng" rồi báo hết ở checkout |
| Ghi sổ sau khi đặt hàng | Vài giây | Báo cáo trễ chút |
| Gửi thông báo | Vài phút | Khách nhận email chậm |
| Cập nhật thống kê | Vài phút | Dashboard trễ |
| Tín hiệu nhu cầu | Vài phút | Không ảnh hưởng |
| Trạng thái tổng hợp đơn hàng | Vài giây | Hiển thị trạng thái cũ |

**Nguyên tắc đánh giá:** nếu độ trễ vài giây gây thiệt hại tài chính hoặc mất dữ liệu, cần nhất quán mạnh. Nếu chỉ gây bất tiện nhỏ, nhất quán cuối là đủ.

---

## 5. Transactional Outbox — cơ chế cốt lõi

**Vấn đề:** nếu ghi database thành công nhưng phát event thất bại (hoặc ngược lại), hệ thống không nhất quán.

**Giải pháp:**

```text
TRONG MỘT GIAO DỊCH:
    1. Ghi thay đổi aggregate
    2. Ghi event vào bảng outbox
    COMMIT

SAU ĐÓ (tiến trình riêng):
    3. Đọc outbox chưa phát
    4. Phát event
    5. Đánh dấu đã phát
```

```sql
CREATE TABLE event_outbox (
    id              UUID PRIMARY KEY,
    event_id        UUID NOT NULL UNIQUE,
    event_type      TEXT NOT NULL,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    UUID NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL,
    published_at    TIMESTAMPTZ
);

CREATE INDEX idx_outbox_unpublished ON event_outbox (occurred_at)
    WHERE published_at IS NULL;
```

### Đảm bảo

```text
✓ Giao dịch thành công → event CHẮC CHẮN được phát (sớm hay muộn)
✓ Giao dịch thất bại   → event KHÔNG BAO GIỜ được phát
✓ Event có thể phát nhiều lần → bên nhận phải idempotent
```

Đây là mô hình "at-least-once" (ít nhất một lần), không phải "exactly-once". Vì vậy idempotency ở bên nhận là **bắt buộc**, không phải tùy chọn.

---

## 6. Khóa lạc quan

Dùng cho các bảng có tranh chấp ghi cao: `inventory_item`, `offer`, `order`.

```sql
UPDATE inventory_item
SET quantity_available = quantity_available - $qty,
    quantity_reserved  = quantity_reserved + $qty,
    version = version + 1
WHERE id = $id
  AND version = $expected_version
  AND quantity_available >= $qty;

-- affected rows = 0 → xung đột hoặc không đủ hàng
```

### Vì sao không dùng khóa bi quan

```text
SELECT ... FOR UPDATE tạo hàng đợi tuần tự.

Với live commerce (hàng nghìn người mua cùng một SKU trong vài giây):
    - Mọi request xếp hàng chờ
    - Độ trễ tăng vọt
    - Kết nối database cạn kiệt

Khóa lạc quan cho phép xử lý song song,
chỉ request thật sự xung đột mới phải thử lại.
```

### Chiến lược thử lại

```text
Xung đột phiên bản  → thử lại tối đa 3 lần, khoảng chờ ngẫu nhiên ngắn
Không đủ hàng       → KHÔNG thử lại, trả lỗi ngay
```

Phân biệt hai loại lỗi này quan trọng: thử lại khi hết hàng chỉ lãng phí tài nguyên và làm chậm phản hồi cho khách.

---

## 7. Mẫu bù trừ (Saga)

Khi một luồng nghiệp vụ trải nhiều module, không dùng giao dịch phân tán. Dùng **bù trừ**.

### Ví dụ: đặt hàng

```text
Bước 1: inventory.Reserve()          → thành công
Bước 2: payment.CreateIntent()       → thành công
Bước 3: order.PlaceOrder()           → THẤT BẠI

Bù trừ:
    - Reservation TỰ HẾT HẠN sau TTL  ← không cần hành động chủ động
    - Payment intent bị hủy hoặc tự hết hạn
```

**Nguyên tắc thiết kế quan trọng:** ưu tiên **bù trừ thụ động** (tự hết hạn) hơn **bù trừ chủ động** (phải gọi ngược để hoàn tác).

Lý do: bù trừ chủ động cũng có thể thất bại, và khi đó phải bù trừ cho việc bù trừ — chuỗi này không có điểm dừng.

### Bảng chiến lược bù trừ

| Thao tác | Bù trừ | Kiểu |
|---|---|---|
| Giữ tồn kho | Tự hết hạn sau TTL | Thụ động ✓ |
| Tạo payment intent | Tự hết hạn | Thụ động ✓ |
| Tạo đơn hàng | Hủy đơn + hoàn tiền | Chủ động |
| Ghi bút toán | Bút toán đảo ngược | Chủ động |
| Trừ điểm thưởng | Giao dịch điểm ngược | Chủ động |

---

## 8. Xử lý event không đúng thứ tự

Không được giả định event đến đúng thứ tự.

```text
Có thể xảy ra:
    order.paid đến TRƯỚC order.placed

Cách xử lý:
    1. Thiết kế bên nhận không phụ thuộc thứ tự (tốt nhất)
    2. Kiểm tra trạng thái aggregate trước khi xử lý
    3. Nếu chưa sẵn sàng: hoãn, thử lại sau
    4. Đảm bảo thứ tự trong phạm vi một aggregate_id
```

### Ví dụ xử lý đúng

```go
func (h *Handler) HandleOrderPaid(ctx context.Context, e Event) error {
    order, err := h.orders.FindByID(ctx, e.OrderID)
    if errors.Is(err, ErrNotFound) {
        // order.placed chưa được xử lý → hoãn
        return ErrRetryLater
    }
    // xử lý bình thường
}
```

---

## 9. Đọc dữ liệu của module khác

```text
Cần dữ liệu MỚI NHẤT (quyết định nghiệp vụ)
    → gọi đồng bộ interface công khai
    → ví dụ: kiểm tra tồn kho trước khi cho đặt hàng

Chấp nhận dữ liệu HƠI CŨ (hiển thị)
    → dùng bản sao cục bộ cập nhật qua event
    → ví dụ: offer.status = OUT_OF_STOCK
```

### Bản sao cục bộ — quy tắc

```text
✓ Được phép giữ bản sao dữ liệu module khác để hiển thị
✓ Bản sao cập nhật qua event
✗ Bản sao KHÔNG phải nguồn sự thật
✗ KHÔNG ra quyết định tài chính dựa trên bản sao
```

Ví dụ: `offer.status = OUT_OF_STOCK` là bản sao. Nguồn sự thật là `inventory_item.quantity_available`. Khi checkout, phải hỏi `inventory` thật, không tin `offer.status`.

---

## 10. Điều hòa dữ liệu (reconciliation)

Nhất quán cuối có thể thất bại vĩnh viễn nếu event bị mất. Cần cơ chế phát hiện.

```text
Job định kỳ kiểm tra:

1. Tài chính:
   - Mọi ledger_entry có cân bằng?
   - balance_snapshot khớp tổng bút toán?
   - Tổng số dư seller khớp với đơn hàng đã hoàn tất?

2. Tồn kho:
   - Tổng các trạng thái = số lượng vật lý?
   - Có inventory_item nào âm?
   - Reservation quá hạn chưa được giải phóng?

3. Đơn hàng:
   - Order nào PAID nhưng không có FulfillmentOrder?
   - Σ FulfillmentLine = Σ OrderLine?

4. Event:
   - Outbox có event chưa phát quá lâu?
   - Event nào thất bại vĩnh viễn (dead letter)?
```

**Nguyên tắc:** job điều hòa **phát hiện và cảnh báo**, không tự động sửa. Tự sửa có thể che giấu lỗi thật hoặc làm hỏng thêm.

Ngoại lệ: giải phóng reservation quá hạn có thể tự động, vì hành động này an toàn và có tính lặp lại.

---

## 11. Tài liệu liên quan

- [idempotency.md](idempotency.md) — điều kiện bắt buộc cho nhất quán cuối
- [../02-domain/domain-events.md](../02-domain/domain-events.md) — cơ chế event
- [../02-domain/aggregates.md](../02-domain/aggregates.md) — ranh giới giao dịch
- [../04-modules/inventory.md](../04-modules/inventory.md) — ví dụ tranh chấp cao
