# Module: Fulfillment

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | **Core** |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Tách đơn hàng của khách thành các đơn thực hiện theo từng nguồn hàng
- Phân bổ nguồn hàng (kho nào, seller nào xuất)
- Quản lý tiến trình lấy hàng, đóng gói, bàn giao vận chuyển
- Theo dõi trạng thái vận chuyển
- Là **góc nhìn vận hành** của đơn hàng

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Hợp đồng với khách (mua gì, giá bao nhiêu) | `order` |
| Số lượng tồn kho | `inventory` |
| Thao tác vật lý trong kho | `warehouse` |
| Xử lý hàng trả về | `return` |
| Chi trả cho seller | `payment` |

---

## 3. Tách Order và FulfillmentOrder

Đây là quyết định trung tâm của module.

```text
Khách nhìn thấy:
    Order #1000 — 1.250.000đ, đặt 10/08

Hệ thống thực thi:
    Order #1000
    ├── FulfillmentOrder #1000-A  (own brand, kho HN)    → giao 11/08
    ├── FulfillmentOrder #1000-B  (Seller A, TP.HCM)     → giao 13/08
    └── FulfillmentOrder #1000-C  (Seller B, Đà Nẵng)    → giao 14/08
```

### Năm lý do tách

```text
1. Chủ sở hữu khác nhau
   Order thuộc khách hàng · FulfillmentOrder thuộc seller/kho

2. Vòng đời khác nhau
   Order gần như bất biến sau khi đặt
   FulfillmentOrder thay đổi liên tục theo tiến trình

3. Ràng buộc bảo mật
   Seller chỉ được xem FulfillmentOrder của mình
   Seller KHÔNG được xem Order (chứa hàng của seller khác)

4. Tranh chấp ghi
   Ba seller cập nhật đồng thời sẽ tranh chấp nếu cùng một bản ghi Order

5. Hỗ trợ xử lý từng phần
   Giao/hủy/hoàn từng phần chỉ khả thi khi tách
```

Lý do 3 và 4 là quyết định — không giải quyết được nếu gộp.

Xem [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md).

---

## 4. Ba mô hình fulfillment

| Mô hình | Ai giữ hàng | Ai đóng gói | Ghi chú |
|---|---|---|---|
| `PLATFORM` | Nền tảng | Nền tảng | Own brand |
| `SELLER` | Seller | Seller | Đa số marketplace |
| `PLATFORM_SERVICE` | **Seller sở hữu**, để ở kho nền tảng | Nền tảng | Dịch vụ thu phí |

Mô hình thứ ba là lý do `InventoryItem` phải tách `inventory_owner_id` khỏi `stock_location_id`. Xem [inventory.md](inventory.md) mục 3.1.

---

## 5. Trạng thái FulfillmentOrder

```text
    PENDING (chờ xử lý)
        │
        ▼
    ALLOCATED (đã phân bổ nguồn hàng, tồn kho committed)
        │
        ▼
    PICKING (đang lấy hàng)
        │
        ▼
    PACKED (đã đóng gói)
        │
        ▼
    HANDED_OVER (đã bàn giao vận chuyển)
        │
        ▼
    IN_TRANSIT (đang vận chuyển)
        │
        ├──→ DELIVERY_FAILED
        │        ├──→ thử lại → IN_TRANSIT
        │        └──→ RETURNED_TO_SENDER
        │
        ▼
    DELIVERED (đã giao)
        │
        ▼ (hết hạn đổi trả)
    COMPLETED

Nhánh hủy:
    PENDING / ALLOCATED ──→ CANCELLED (giải phóng tồn kho)
    Sau PACKED          ──→ cần quy trình riêng, có chi phí
```

### Ý nghĩa tài chính của `COMPLETED`

```text
DELIVERED   → số dư seller vẫn Pending
COMPLETED   → số dư chuyển Available, được payout
```

Đây là cơ chế bảo vệ nền tảng khỏi rủi ro hoàn hàng sau khi đã trả tiền seller.

---

## 6. Phân bổ nguồn hàng (Sourcing)

Khi own brand có nhiều kho, phải quyết định kho nào xuất.

```text
Tiêu chí (theo thứ tự ưu tiên):
    1. Có đủ hàng không
    2. Khoảng cách tới khách (thời gian và chi phí giao)
    3. Cân bằng tồn kho giữa các kho
    4. Ưu tiên giải phóng hàng sắp hết mùa
```

**Nguyên tắc P14:** MVP dùng quy tắc đơn giản — kho gần nhất có đủ hàng. Tối ưu phức tạp hơn chỉ làm khi có nhiều kho và khối lượng lớn.

**Cảnh báo:** chia một đơn thành nhiều gói làm tăng chi phí vận chuyển và giảm trải nghiệm. Chỉ chia khi thật sự cần (không kho nào có đủ toàn bộ đơn).

---

## 7. Tích hợp đơn vị vận chuyển

Nguyên tắc P13 — nằm sau interface:

```go
// domain định nghĩa
type ShippingProvider interface {
    Quote(ctx, req QuoteRequest) (*QuoteResult, error)
    CreateShipment(ctx, req ShipmentRequest) (*ShipmentResult, error)
    Track(ctx, trackingNumber string) (*TrackingStatus, error)
    Cancel(ctx, trackingNumber string) error
}
```

Module `fulfillment` **không biết tên nhà vận chuyển nào** trong domain logic. Việc chọn nhà vận chuyển là cấu hình và quy tắc.

**Lý do thực tế:** giá và chất lượng dịch vụ của các đối tác thay đổi thường xuyên. Nền tảng cần đổi hoặc dùng đồng thời nhiều đối tác.

### Cập nhật trạng thái vận chuyển

```text
Hai cơ chế, cần cả hai:

1. Webhook từ đối tác  → cập nhật thời gian thực
2. Hỏi định kỳ         → phòng khi webhook mất

Yêu cầu: idempotent — cùng một cập nhật có thể đến hai lần
```

---

## 8. Dữ liệu sở hữu

```sql
fulfillment_order       -- đơn thực hiện
fulfillment_line        -- dòng hàng trong đơn thực hiện
shipment                -- lô hàng gửi đi
shipment_tracking       -- lịch sử trạng thái vận chuyển
```

```sql
CREATE TABLE fulfillment_order (
    id                     UUID PRIMARY KEY,
    fulfillment_number     TEXT NOT NULL UNIQUE,
    order_id               UUID NOT NULL,
    seller_id              UUID NOT NULL,
    stock_location_id      UUID,
    fulfillment_type       TEXT NOT NULL,
    status                 TEXT NOT NULL,
    shipping_method        TEXT,
    shipping_provider      TEXT,
    tracking_number        TEXT,
    estimated_delivery_date DATE,
    shipped_at             TIMESTAMPTZ,
    delivered_at           TIMESTAMPTZ,
    completed_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_fo_order ON fulfillment_order (order_id);
CREATE INDEX idx_fo_seller ON fulfillment_order (seller_id, status);
CREATE INDEX idx_fo_pending ON fulfillment_order (status)
    WHERE status IN ('PENDING','ALLOCATED','PICKING');
```

Chỉ mục `idx_fo_seller` phục vụ màn hình chính của Seller Center — danh sách đơn cần xử lý.

`fulfillment_line` có trường `production_batch_id` để truy vết lô sản xuất, phục vụ tính COGS chính xác và thu hồi khi cần.

---

## 9. Interface công khai

```go
type PublicAPI interface {
    GetFulfillmentOrder(ctx, foID string) (*FulfillmentOrderView, error)
    GetFulfillmentOrdersByOrder(ctx, orderID string) ([]FulfillmentOrderView, error)
    GetFulfillmentOrdersBySeller(ctx, sellerID string, filter Filter, page Pagination) (*FOList, error)

    // Thao tác vận hành
    AllocateInventory(ctx, foID string) error
    MarkPicked(ctx, foID string) error
    MarkPacked(ctx, req PackRequest) error
    CreateShipment(ctx, req CreateShipmentRequest) (*Shipment, error)
    UpdateTrackingStatus(ctx, req TrackingUpdate) error
    CancelFulfillmentOrder(ctx, foID string, reason string) error

    // Ước tính cho checkout
    EstimateShipping(ctx, req ShippingEstimateRequest) (*ShippingEstimate, error)
}
```

---

## 10. Event

### Phát ra

| Event | Khi nào | Bên nghe |
|---|---|---|
| `fulfillment_order.created` | Tách đơn xong | inventory, seller, notification |
| `fulfillment_order.allocated` | Phân bổ nguồn hàng | inventory |
| `fulfillment_order.packed` | Đóng gói xong | notification |
| `fulfillment_order.shipped` | Bàn giao vận chuyển | order, notification, analytics |
| `fulfillment_order.delivered` | Giao thành công | order, return (đếm hạn), payment |
| `fulfillment_order.delivery_failed` | Giao thất bại | notification, order |
| `fulfillment_order.completed` | Hết hạn đổi trả | **payment (chuyển số dư seller)** |
| `fulfillment_order.cancelled` | Hủy | inventory, order, payment |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `order.paid` | order | **Tạo FulfillmentOrder cho từng seller** |
| `order.cancelled` | order | Hủy các FO chưa xuất |
| `order.line_cancelled` | order | Hủy phần tương ứng |

---

## 11. Use case chính: tách đơn

```text
Nhận event order.paid
    ↓
Nhóm OrderLine theo (seller_id, nguồn hàng)
    ↓
Với mỗi nhóm:
    1. Xác định fulfillment_type
       (own brand → PLATFORM, seller → SELLER hoặc PLATFORM_SERVICE)
    2. Chọn stock_location (phân bổ nguồn hàng)
    3. Tạo FulfillmentOrder + FulfillmentLine
    4. Sinh fulfillment_number
    ↓
GIAO DỊCH: lưu tất cả FO + ghi outbox
    ↓
Phát fulfillment_order.created cho mỗi FO
```

**Lưu ý:** việc tách phải **idempotent** — nếu event `order.paid` đến hai lần, không được tạo hai bộ FulfillmentOrder.

Cách xử lý: kiểm tra đã có FO cho `order_id` này chưa trước khi tạo.

---

## 12. Xử lý giao hàng thất bại

Tình huống phổ biến hơn dự kiến, đặc biệt với thanh toán khi nhận hàng.

```text
Nguyên nhân: khách không có nhà, sai địa chỉ, từ chối nhận, đổi ý

Xử lý:
    Lần 1 → liên hệ khách, hẹn lại
    Lần 2 → liên hệ, hẹn lại
    Lần 3 → hoàn về người gửi

Hệ quả:
    - Chi phí vận chuyển hai chiều
    - Hàng phải nhập lại kho, KIỂM TRA LẠI
    - Nếu đã thanh toán → hoàn tiền
```

**Quy tắc bắt buộc:** hàng hoàn về **không tự động cộng lại** tồn kho khả dụng. Phải qua kiểm định — hàng có thể hư hỏng trong quá trình vận chuyển hai chiều.

---

## 13. Quy tắc nghiệp vụ quan trọng

| # | Quy tắc |
|---|---|
| 1 | Σ số lượng các FulfillmentLine = số lượng OrderLine (trừ phần hủy) |
| 2 | Seller chỉ truy cập được FO của mình |
| 3 | Tách đơn phải idempotent |
| 4 | Hủy sau khi PACKED cần quy trình riêng |
| 5 | Hàng hoàn về phải qua kiểm định |
| 6 | `COMPLETED` mới kích hoạt chuyển số dư seller |
| 7 | Không mã cứng tên nhà vận chuyển trong domain |
| 8 | Cập nhật trạng thái vận chuyển phải idempotent |

---

## 14. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Tách đơn, một kho, một đối tác vận chuyển, trạng thái cơ bản |
| **Phase 2** | Nhiều kho, phân bổ nguồn hàng, nhiều đối tác, xử lý giao thất bại |
| **Phase 3** | Dịch vụ fulfillment cho seller, tối ưu chi phí vận chuyển |
| **Phase 4** | Giao nhanh, giao theo khung giờ |

---

## 15. Tài liệu liên quan

- [../01-business/fulfillment.md](../01-business/fulfillment.md) — nghiệp vụ
- [order.md](order.md) — quan hệ với đơn hàng
- [warehouse.md](warehouse.md) — thao tác vật lý
- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md)
