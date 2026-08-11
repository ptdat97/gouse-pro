# Module: Order

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | **Core** |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Tạo và quản lý đơn hàng — **hợp đồng với khách hàng**
- Đóng băng thông tin giao dịch tại thời điểm đặt hàng
- Quản lý trạng thái tổng hợp của đơn
- Xử lý hủy toàn phần và từng phần
- Là nguồn sự thật về "khách đã mua gì, giá bao nhiêu"

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Ai giao hàng, giao đến đâu rồi | `fulfillment` |
| Thu tiền, ghi sổ | `payment` |
| Giữ và trừ tồn kho | `inventory` |
| Quyết định tỷ lệ hoa hồng | `marketplace` |
| Xử lý yêu cầu trả hàng | `return` |
| Tính giá và khuyến mãi | `pricing`, `promotion` |

**Ranh giới quan trọng nhất:** `order` giữ **hợp đồng** (khách mua gì, giá nào). `fulfillment` giữ **việc thực thi** (ai giao, đến đâu). Xem [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md).

---

## 3. Khái niệm domain

### 3.1 Aggregate: `Order`

```text
Order {
    id
    order_number                — mã khách nhìn thấy
    customer_id | guest_email   — khách vãng lai được phép đặt hàng
    shipping_address            — ĐÓNG BĂNG
    billing_address             — ĐÓNG BĂNG
    currency
    subtotal, shipping_fee, discount_amount, tax_amount, total_amount
    status
    lines[]                     — OrderLine
    idempotency_key
    placed_at, completed_at
}
```

### 3.2 Entity: `OrderLine`

```text
OrderLine {
    id
    offer_id
    sku_id                      — sao chép để truy vấn nhanh
    seller_id                   — sao chép
    product_name                — ĐÓNG BĂNG
    variant_description         — ĐÓNG BĂNG ("Trắng / M")
    unit_price                  — ĐÓNG BĂNG
    quantity
    line_total
    commission_rate             — ĐÓNG BĂNG
    commission_amount           — ĐÓNG BĂNG
    attributed_creator_id       (nullable)
    creator_commission_rate     — ĐÓNG BĂNG
    status      ACTIVE | CANCELLED | RETURNED
}
```

---

## 4. Nguyên tắc đóng băng dữ liệu

Đây là quy tắc quan trọng nhất của module này (nguyên tắc P9).

### Vì sao phải đóng băng

| Trường | Nếu tham chiếu động | Nếu đóng băng |
|---|---|---|
| `product_name` | Seller đổi tên → hóa đơn cũ sai | Hóa đơn luôn đúng |
| `unit_price` | Giá đổi → tổng tiền đơn cũ không khớp | Nhất quán |
| `commission_rate` | Đổi chính sách → đối soát tháng trước ra số khác | Kiểm toán được |
| `variant_description` | Sửa variant → không biết khách mua size nào | Truy vết được |
| `shipping_address` | Khách sửa sổ địa chỉ → không biết đã giao đâu | Truy vết được |

### Kiểm chứng bằng tình huống thực tế

```text
Ngày 10/08: Khách mua áo giá 299.000đ, hoa hồng 10%
Ngày 15/08: Seller giảm giá còn 249.000đ
Ngày 20/08: Nền tảng đổi chính sách hoa hồng thành 12%
Ngày 25/08: Chạy đối soát cho kỳ 01–15/08

Nếu tham chiếu động:
    → đối soát tính 249.000 × 12% = 29.880đ    ← SAI

Nếu đóng băng:
    → đối soát tính 299.000 × 10% = 29.900đ    ← ĐÚNG
```

Sai lệch này không chỉ là con số — nó phá vỡ niềm tin của seller và không giải thích được khi có tranh chấp.

---

## 5. Trạng thái đơn hàng

### 5.1 Sơ đồ

```text
    PENDING_PAYMENT
        │
        ├──→ CANCELLED (không thanh toán / khách hủy)
        │
        ▼ (payment.captured)
       PAID
        │
        ▼ (fulfillment order được tạo)
    PROCESSING
        │
        ├──→ PARTIALLY_CANCELLED (một seller hủy phần của mình)
        │
        ▼
    PARTIALLY_SHIPPED ──→ SHIPPED
        │
        ▼
    PARTIALLY_DELIVERED ──→ DELIVERED
        │
        ▼ (hết hạn đổi trả)
    COMPLETED
```

### 5.2 Trạng thái tổng hợp — quy tắc tính

Trạng thái của `Order` được **suy ra** từ trạng thái các `FulfillmentOrder`:

```text
Tất cả FO đã giao          → DELIVERED
Một số FO đã giao          → PARTIALLY_DELIVERED
Tất cả FO đã xuất          → SHIPPED
Một số FO đã xuất          → PARTIALLY_SHIPPED
Tất cả FO bị hủy           → CANCELLED
Một số FO bị hủy           → PARTIALLY_CANCELLED
Tất cả FO hoàn tất         → COMPLETED
```

**Cách cài đặt:** `order` lắng nghe event từ `fulfillment` và tính lại trạng thái tổng hợp. Không hỏi ngược `fulfillment` — điều đó tạo phụ thuộc vòng.

### 5.3 Phân biệt DELIVERED và COMPLETED

```text
DELIVERED  — hàng đã đến tay khách
COMPLETED  — đã hết hạn đổi trả, giao dịch chốt

Ý nghĩa tài chính:
    DELIVERED  → số dư seller vẫn ở trạng thái Pending
    COMPLETED  → số dư chuyển sang Available, được payout
```

Phân biệt này bảo vệ nền tảng: nếu trả tiền seller ngay khi giao hàng, khi khách hoàn hàng phải đòi lại tiền — rất khó thu hồi.

---

## 6. Hủy đơn — toàn phần và từng phần

### 6.1 Hủy toàn phần

```text
Điều kiện: chưa có FulfillmentOrder nào ở trạng thái PACKED trở đi

Xử lý:
    1. Chuyển Order → CANCELLED
    2. Chuyển mọi OrderLine → CANCELLED
    3. Phát order.cancelled
    4. inventory giải phóng hàng
    5. payment hoàn tiền
    6. affiliate đảo ngược quy kết
```

### 6.2 Hủy từng phần

Tình huống phổ biến: Seller B hết hàng, hai seller còn lại vẫn giao được.

```text
Order #1000
├── OrderLine 1 (Seller A) → ACTIVE
├── OrderLine 2 (Seller B) → CANCELLED
└── OrderLine 3 (Seller C) → ACTIVE

Order.status = PARTIALLY_CANCELLED
```

Xử lý:

```text
1. Chuyển OrderLine 2 → CANCELLED
2. Tính lại tổng tiền còn hiệu lực
3. Phát order.line_cancelled
4. inventory giải phóng phần của line 2
5. payment hoàn tiền phần của line 2 + phí ship tương ứng
6. affiliate đảo ngược hoa hồng của line 2
```

### 6.3 Vấn đề khó: phí vận chuyển khi hủy một phần

```text
Tình huống:
    Khách mua 600.000đ, được miễn phí ship (ngưỡng 500.000đ)
    Hủy một món 200.000đ → còn 400.000đ, không đạt ngưỡng nữa

Câu hỏi: có thu lại phí ship không?
```

**Quyết định:** **không thu lại**.

Lý do: chi phí xử lý tranh chấp và tổn hại trải nghiệm lớn hơn số tiền thu về. Khách sẽ cảm thấy bị phạt vì một việc không phải lỗi của họ (seller hết hàng).

Quyết định này phải được ghi rõ trong quy tắc nghiệp vụ, không để mỗi trường hợp xử lý một kiểu.

---

## 7. Dữ liệu sở hữu

```sql
"order"                 -- đơn hàng (order là từ khóa SQL, cần đặt trong dấu ngoặc kép)
order_line              -- dòng hàng
order_address           -- địa chỉ đã đóng băng
order_status_history    -- lịch sử chuyển trạng thái
```

### Bảng `order`

```sql
CREATE TABLE "order" (
    id                UUID PRIMARY KEY,
    order_number      TEXT NOT NULL UNIQUE,
    customer_id       UUID,
    guest_email       TEXT,
    guest_phone       TEXT,
    currency          CHAR(3) NOT NULL,
    subtotal          BIGINT NOT NULL,
    shipping_fee      BIGINT NOT NULL DEFAULT 0,
    discount_amount   BIGINT NOT NULL DEFAULT 0,
    tax_amount        BIGINT NOT NULL DEFAULT 0,
    total_amount      BIGINT NOT NULL,
    status            TEXT NOT NULL,
    idempotency_key   TEXT UNIQUE,
    placed_at         TIMESTAMPTZ NOT NULL,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT customer_or_guest CHECK (
        customer_id IS NOT NULL OR guest_email IS NOT NULL
    )
);

CREATE INDEX idx_order_customer ON "order" (customer_id, placed_at DESC);
CREATE INDEX idx_order_status ON "order" (status) WHERE status NOT IN ('COMPLETED','CANCELLED');
```

Ràng buộc `customer_or_guest` thực thi quy tắc: đơn phải thuộc về khách đã đăng ký **hoặc** có thông tin liên hệ của khách vãng lai.

---

## 8. Interface công khai

```go
type PublicAPI interface {
    PlaceOrder(ctx, cmd PlaceOrderCommand) (*OrderResult, error)

    GetOrderSummary(ctx, orderID string) (*OrderSummary, error)
    GetOrdersByCustomer(ctx, customerID string, page Pagination) (*OrderList, error)
    GetOrderByNumber(ctx, orderNumber string) (*OrderSummary, error)

    CancelOrder(ctx, orderID string, reason CancelReason) error
    CancelOrderLine(ctx, orderLineID string, quantity int, reason CancelReason) error

    // Cho module return
    GetOrderLineForReturn(ctx, orderLineID string) (*OrderLineDetail, error)
}
```

**Lưu ý:** không có phương thức trả về aggregate `Order` đầy đủ. Chỉ trả DTO chỉ đọc — ngăn module khác thao tác trực tiếp lên đơn hàng.

---

## 9. Use case chính: `PlaceOrder`

```text
Đầu vào:  checkout_id, idempotency_key

Các bước:
 1. Kiểm tra idempotency_key
    → nếu đã xử lý: trả kết quả cũ, KẾT THÚC

 2. Lấy thông tin checkout (đã có giá đóng băng, đã giữ hàng)

 3. Xác minh reservation còn hiệu lực
    → nếu hết hạn: trả lỗi, yêu cầu làm lại checkout

 4. Lấy tỷ lệ hoa hồng từ marketplace.GetCommissionRate()
    → cho từng dòng hàng

 5. Lấy thông tin quy kết creator từ affiliate (nếu có)

 6. Tạo aggregate Order
    → đóng băng mọi thông tin
    → kiểm tra bất biến

 7. GIAO DỊCH:
    - INSERT order, order_line, order_address
    - INSERT event_outbox (order.placed)
    COMMIT

 8. Trả về order_id, order_number
```

**Điểm quan trọng ở bước 7:** ghi đơn hàng và ghi event trong **cùng một giao dịch**. Đây là outbox pattern — đảm bảo không có trường hợp đơn được tạo mà event không phát, hoặc ngược lại.

Xem [../02-domain/domain-events.md](../02-domain/domain-events.md) mục 7.

---

## 10. Event

### Phát ra

| Event | Khi nào | Bên nghe chính |
|---|---|---|
| `order.placed` | Đơn được tạo | inventory, fulfillment, payment, affiliate, notification, loyalty, analytics, supply-chain, promotion |
| `order.paid` | Thanh toán xong | fulfillment, notification |
| `order.cancelled` | Hủy toàn bộ | inventory, payment, affiliate, notification |
| `order.line_cancelled` | Hủy một phần | inventory, payment, affiliate |
| `order.completed` | Hết hạn đổi trả | payment (chuyển số dư), loyalty |

`order.placed` là event có nhiều bên nghe nhất trong hệ thống. Module `order` **không biết** chín bên này tồn tại — đây chính là giá trị của kiến trúc hướng sự kiện.

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `payment.captured` | payment | Chuyển sang PAID |
| `payment.failed` | payment | Chuyển sang CANCELLED |
| `fulfillment_order.shipped` | fulfillment | Tính lại trạng thái tổng hợp |
| `fulfillment_order.delivered` | fulfillment | Tính lại trạng thái tổng hợp |
| `fulfillment_order.completed` | fulfillment | Kiểm tra để chuyển COMPLETED |
| `return.refunded` | return | Cập nhật trạng thái dòng hàng |

---

## 11. Phụ thuộc

```text
Gọi đồng bộ:   marketplace (lấy tỷ lệ hoa hồng)
               checkout    (lấy thông tin phiên thanh toán)
Nghe event:    payment, fulfillment, return
Được gọi bởi:  checkout, return, fulfillment (chỉ đọc)
```

**Lưu ý:** `order` **không gọi** `inventory` trực tiếp khi đặt hàng. Việc giữ hàng đã do `checkout` làm trước đó. `order` chỉ phát event và `inventory` tự chuyển Reserved → Committed.

Điều này giữ cho module `order` mỏng và không phải xử lý lỗi tồn kho ở thời điểm tạo đơn.

---

## 12. Quy tắc nghiệp vụ quan trọng

| # | Quy tắc | Lý do |
|---|---|---|
| 1 | Mọi thông tin giao dịch phải đóng băng | Kiểm toán, đối soát |
| 2 | Tổng tiền đơn không đổi sau khi đặt | Trừ khi hủy từng phần |
| 3 | Không xóa đơn hàng | Nghĩa vụ lưu trữ |
| 4 | Mọi chuyển trạng thái ghi vào lịch sử | Truy vết |
| 5 | `PlaceOrder` phải idempotent | Chống tạo đơn trùng |
| 6 | Khách vãng lai được đặt hàng | Giảm rào cản chuyển đổi |
| 7 | Trạng thái tổng hợp suy ra từ FO, không tự đặt | Nguồn sự thật duy nhất |
| 8 | Hủy sau khi đóng gói cần quy trình riêng | Có chi phí phát sinh |

---

## 13. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Tạo đơn, hủy toàn phần, trạng thái cơ bản, đơn nhiều seller |
| **Phase 2** | Hủy từng phần, tích hợp trả hàng |
| **Phase 3** | Đổi hàng, đơn đặt trước |
| **Phase 4** | Đơn định kỳ (nếu có mô hình đăng ký) |

---

## 14. Tài liệu liên quan

- [fulfillment.md](fulfillment.md) — thực thi đơn hàng
- [payment.md](payment.md) — thanh toán và ghi sổ
- [checkout.md](checkout.md) — tạo ra đơn hàng
- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md) — quyết định tách Order/FulfillmentOrder
- [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) — luồng đầy đủ
