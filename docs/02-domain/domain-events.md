# Domain Events

## 1. Nguyên tắc

Domain event là **sự thật nghiệp vụ đã xảy ra**, ở thì quá khứ.

```text
Đúng:  OrderPlaced, PaymentCaptured, QualityApproved
Sai:   SendEmail, UpdateInventory, ProcessOrder
```

**Vì sao khác biệt này quan trọng:** `SendEmail` là mệnh lệnh — bên phát phải biết bên nhận làm gì, tạo ghép nối chặt. `OrderPlaced` là sự thật — thêm bên nhận mới (gửi SMS, cập nhật analytics, tính hoa hồng) không cần sửa module đơn hàng.

Xem nguyên tắc P7 tại [../00-overview/principles.md](../00-overview/principles.md).

---

## 2. Event dùng cho việc gì và không dùng cho việc gì

| Dùng event khi | Dùng gọi trực tiếp khi |
|---|---|
| Thông báo việc đã xảy ra | Cần kết quả để quyết định tiếp |
| Nhiều bên quan tâm | Chỉ một bên, quan hệ rõ ràng |
| Bên phát không cần biết ai nghe | Cần biết thành công hay thất bại ngay |
| Chấp nhận độ trễ ngắn | Cần đồng bộ |

**Ví dụ cụ thể:**

```text
Kiểm tra tồn kho trước khi cho đặt hàng
→ GỌI TRỰC TIẾP. Cần biết ngay có hàng không.

Đơn hàng đã đặt → ghi sổ, gửi thông báo, cập nhật thống kê
→ EVENT. Ba việc độc lập, không cần đồng bộ.
```

## 2.1. Quy tắc quyết định: đồng bộ hay event

Một câu hỏi duy nhất:

```text
Bên gọi có cần KẾT QUẢ để quyết định việc tiếp theo không?

    CÓ  → gọi ĐỒNG BỘ qua interface công khai
    KHÔNG → phát DOMAIN EVENT
```

### Áp dụng vào các luồng thật

```text
Checkout  →  Inventory (giữ hàng)
    Cần biết giữ được không mới quyết định mở phiên
    → ĐỒNG BỘ

Checkout  →  Order (tạo đơn)
    Cần mã đơn để trả cho khách và ghi vào phiên
    → ĐỒNG BỘ

Order  →  Marketplace (tỷ lệ hoa hồng)
    Cần con số để đóng băng vào dòng hàng
    → ĐỒNG BỘ

OrderPlaced  →  Notification · Analytics · Search · Attribution
    Không việc nào ảnh hưởng tới việc đơn có được tạo hay không
    → EVENT
```

### Không biến mọi phụ thuộc thành event

Event **không phải** cách để giảm số lượng phụ thuộc trên giấy. Một lời gọi
đồng bộ qua interface công khai đã là phụ thuộc **tường minh và kiểm soát
được** — đó là điều tốt, không phải điều cần né tránh.

```text
✗ Sai: "module A không được biết module B, nên phát event"
       → thực chất A vẫn cần kết quả của B
       → phải chờ event trả về = xây lại lời gọi đồng bộ, phức tạp hơn

✓ Đúng: A cần kết quả  → gọi B trực tiếp, ghi vào đồ thị phụ thuộc
        A chỉ thông báo → phát event, không quan tâm ai nghe
```

**Dấu hiệu dùng event sai:** bên phát cần biết bên nhận đã xử lý xong chưa.
Nếu có, đó là lời gọi đồng bộ được ngụy trang, và nó sẽ mang mọi nhược điểm
của cả hai cách.

---

## 3. Cấu trúc event chuẩn

```go
type DomainEvent struct {
    EventID       string     // định danh duy nhất — dùng cho idempotency
    EventType     string     // "order.placed"
    EventVersion  int        // phiên bản schema
    AggregateType string     // "Order"
    AggregateID   string
    OccurredAt    time.Time
    CorrelationID string     // truy vết chuỗi nghiệp vụ
    CausationID   string     // event nào gây ra event này
    Payload       any
}
```

### Vì sao cần từng trường

| Trường | Lý do |
|---|---|
| `EventID` | Bên nhận dùng để bỏ qua event trùng (nguyên tắc P10) |
| `EventVersion` | Cho phép tiến hóa schema mà không phá bên nhận cũ |
| `CorrelationID` | Truy vết toàn bộ chuỗi từ một hành động của khách |
| `CausationID` | Biết event nào sinh ra event nào — gỡ lỗi chuỗi phức tạp |
| `OccurredAt` | Thứ tự thời gian nghiệp vụ, khác thời điểm xử lý |

**Nguyên tắc thiết kế payload:** chứa đủ thông tin để bên nhận xử lý mà **không phải gọi ngược lại** bên phát. Nếu mọi bên nhận đều phải gọi ngược để lấy chi tiết, event trở nên vô dụng và tạo ghép nối.

Nhưng cũng không nhồi toàn bộ aggregate — chỉ những gì bên nhận cần.

---

## 4. Danh mục event theo context

### 4.1 Commerce Context

| Event | Khi nào | Payload chính | Ai nghe |
|---|---|---|---|
| `product.published` | Sản phẩm lên sàn | product_id, brand_id, category_id | search, recommendation, analytics |
| `product.unpublished` | Ngừng bán | product_id, reason | search, content |
| `offer.created` | Seller tạo offer | offer_id, sku_id, seller_id, price | search, marketplace, analytics |
| `offer.price_changed` | Đổi giá | offer_id, old_price, new_price | search, analytics, promotion |
| `offer.out_of_stock` | Hết hàng | offer_id, sku_id | search, content, supply-chain |
| `cart.item_added` | Thêm giỏ | cart_id, offer_id, source_content_id | analytics, supply-chain |
| `cart.abandoned` | Bỏ giỏ | cart_id, items | notification, analytics |
| `checkout.started` | Bắt đầu thanh toán, **hàng ĐÃ giữ xong** | checkout_id, cart_id | analytics — **KHÔNG phải inventory**, xem ghi chú dưới |
| `checkout.expired` | Hết hạn | checkout_id | analytics, notification — inventory đã được nhả đồng bộ |
| **`order.placed`** | Đơn được tạo | order_id, lines, total, customer | **rất nhiều** — xem mục 5 |
| `order.paid` | Thanh toán thành công | order_id, payment_id, amount | fulfillment, payment, notification |
| `order.cancelled` | Hủy toàn bộ | order_id, reason | inventory, payment, notification |
| `order.line_cancelled` | Hủy một phần | order_id, order_line_id, quantity | inventory, payment |
| `order.completed` | Hoàn tất (hết hạn đổi trả) | order_id | payment (chuyển số dư), loyalty |

### Ghi chú quan trọng: giữ hàng KHÔNG đi qua event

`checkout.started` là **thông báo việc đã xảy ra**, không phải mệnh lệnh
giữ hàng. Thứ tự thật:

```text
1. checkout gọi inventory.Reserve()      ← ĐỒNG BỘ, chờ kết quả
2. Không giữ được hàng → dừng, KHÔNG có phiên nào được tạo
3. Giữ được → tạo phiên → PHÁT checkout.started
```

**Vì sao không thể dùng event ở bước 1:** checkout phải biết **ngay** có
giữ được hàng hay không để quyết định có mở phiên hay không. Nếu phát event
rồi đi tiếp, sẽ có lúc khách thấy màn hình thanh toán cho hàng đã hết —
phát hiện ra sau khi họ đã nhập địa chỉ và thông tin thẻ.

Tương tự với `checkout.expired`: hàng được nhả **đồng bộ** trong cùng thao
tác dọn phiên, không phải qua event. Nhả hàng qua event nghĩa là có khoảng
thời gian phiên đã chết mà hàng vẫn khóa, và nếu event thất bại thì hàng
khóa vĩnh viễn.

Đây là ví dụ trực tiếp của quy tắc ở [mục 2](#2-event-dùng-cho-việc-gì-và-không-dùng-cho-việc-gì).

---


### 4.2 Fulfillment

| Event | Khi nào | Ai nghe |
|---|---|---|
| `fulfillment_order.created` | Tách đơn theo seller | inventory, seller, notification |
| `fulfillment_order.allocated` | Đã phân bổ nguồn hàng | inventory |
| `fulfillment_order.packed` | Đóng gói xong | notification |
| `fulfillment_order.shipped` | Bàn giao vận chuyển | order, notification, analytics |
| `fulfillment_order.delivered` | Giao thành công | order, payment, return (bắt đầu đếm hạn) |
| `fulfillment_order.delivery_failed` | Giao thất bại | notification, order |
| `fulfillment_order.completed` | Hết hạn đổi trả | payment (chuyển số dư seller) |

### 4.3 Inventory

| Event | Khi nào | Ai nghe |
|---|---|---|
| `inventory.reserved` | Giữ hàng cho checkout | checkout |
| `inventory.reservation_released` | Giải phóng | offer (cập nhật trạng thái) |
| `inventory.committed` | Cam kết cho đơn | fulfillment |
| `inventory.received` | Nhập kho | offer, catalog, supply-chain |
| `inventory.depleted` | **Hết hàng** | offer, search, **supply-chain (tín hiệu nhu cầu)** |
| `inventory.low_stock` | Sắp hết | supply-chain (bổ sung hàng) |
| `inventory.adjusted` | Điều chỉnh thủ công | audit, analytics |

**`inventory.depleted` là event quan trọng chiến lược** — nó là tín hiệu nhu cầu bị bỏ lỡ, đầu vào của bánh đà. Xem [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 4.2.

### 4.4 Marketplace

| Event | Khi nào | Ai nghe |
|---|---|---|
| `seller.applied` | Nộp đơn đăng ký | notification, admin |
| `seller.approved` | Được duyệt | identity (cấp quyền), notification, payment (tạo tài khoản) |
| `seller.rejected` | Bị từ chối | notification |
| `seller.suspended` | Đình chỉ | marketplace (ẩn offer), payment (giữ payout), notification |
| `seller.reactivated` | Khôi phục | marketplace, payment |
| `seller.performance_updated` | Cập nhật chỉ số | marketplace (buy box), notification |

### 4.5 Financial

| Event | Khi nào | Ai nghe |
|---|---|---|
| `payment.intent_created` | Tạo ý định thanh toán | order |
| `payment.captured` | Thu tiền thành công | order, payment (ghi sổ) |
| `payment.failed` | Thất bại | order, inventory (giải phóng), notification |
| `ledger.entry_recorded` | Ghi bút toán | analytics |
| `settlement.created` | Tạo đối soát | seller, creator, notification |
| `settlement.confirmed` | Xác nhận | payment (chuẩn bị payout) |
| `payout.executed` | Chuyển tiền | seller, creator, notification |
| `payout.failed` | Chuyển tiền lỗi | notification, admin |
| `refund.issued` | Hoàn tiền | order, payment, notification |

### 4.6 Growth

| Event | Khi nào | Ai nghe |
|---|---|---|
| `creator.approved` | Duyệt creator | identity, payment, notification |
| `content.published` | Nội dung xuất bản | search, recommendation, notification |
| `content.taken_down` | Gỡ nội dung | search, affiliate |
| `affiliate.link_created` | Tạo link | analytics |
| `affiliate.click_recorded` | Ghi nhận click | analytics, supply-chain (tín hiệu) |
| `affiliate.conversion_attributed` | Quy kết đơn | payment (hoa hồng), creator, analytics |
| `affiliate.attribution_reversed` | Đảo ngược (do hoàn hàng) | payment, creator |
| `campaign.started` | Chiến dịch bắt đầu | content, notification |
| `campaign.ended` | Kết thúc | payment (quyết toán) |
| `loyalty.points_earned` | Tích điểm | notification |
| `loyalty.tier_changed` | Đổi hạng | notification, promotion |

### 4.7 Supply Chain

| Event | Khi nào | Ai nghe |
|---|---|---|
| `demand_signal.recorded` | Ghi tín hiệu nhu cầu | supply-chain (tổng hợp) |
| `product_development.approved` | Duyệt mẫu | **catalog (tạo CatalogProduct qua ACL)** |
| `production_order.created` | Tạo đơn sản xuất | payment (đặt cọc), supplier |
| `production_order.confirmed` | Nhà cung cấp xác nhận | supply-chain |
| `production_batch.completed` | Hoàn tất lô | quality |
| `quality.approved` | QC đạt | warehouse, supply-chain |
| `quality.rejected` | QC không đạt | supply-chain, supplier, payment |
| `warehouse.goods_received` | Nhập kho | inventory, catalog, payment (ghi COGS) |
| `replenishment.suggested` | Đề xuất bổ sung | notification (cho người phụ trách) |

### 4.8 Return

| Event | Khi nào | Ai nghe |
|---|---|---|
| `return.requested` | Khách yêu cầu trả | seller, notification, fulfillment |
| `return.approved` | Chấp nhận | notification, warehouse |
| `return.received` | Nhận hàng về | quality |
| `return.inspected` | Kiểm định xong | inventory, payment |
| `return.refunded` | Đã hoàn tiền | order, payment, affiliate (đảo hoa hồng) |
| `return.rejected` | Từ chối | notification |

---

## 5. Ví dụ chi tiết: `order.placed`

Đây là event có nhiều bên nghe nhất.

```json
{
  "event_id": "01J9X...",
  "event_type": "order.placed",
  "event_version": 1,
  "aggregate_type": "Order",
  "aggregate_id": "ord_01J9X...",
  "occurred_at": "2026-08-10T14:23:11Z",
  "correlation_id": "cor_01J9X...",
  "payload": {
    "order_id": "ord_01J9X...",
    "order_number": "FC-2026-08-001234",
    "customer_id": "cus_01J9X...",
    "currency": "VND",
    "subtotal": 1250000,
    "shipping_fee": 30000,
    "discount_amount": 50000,
    "total_amount": 1230000,
    "lines": [
      {
        "order_line_id": "oln_01...",
        "offer_id": "off_01...",
        "sku_id": "sku_01...",
        "seller_id": "sel_01...",
        "quantity": 1,
        "unit_price": 299000,
        "commission_rate": 1000,
        "commission_amount": 29900,
        "attributed_creator_id": "cre_01...",
        "creator_commission_rate": 500
      }
    ],
    "placed_at": "2026-08-10T14:23:11Z"
  }
}
```

### Ai nghe và làm gì

```text
order.placed
    │
    ├──→ (inventory KHÔNG nghe event này — xem ghi chú dưới)
    ├──→ fulfillment      : tạo FulfillmentOrder theo từng seller
    ├──→ payment          : ghi bút toán doanh thu, hoa hồng
    ├──→ affiliate        : xác nhận quy kết, tạo bản ghi hoa hồng creator
    ├──→ notification     : gửi xác nhận cho khách, thông báo cho seller
    ├──→ loyalty          : tích điểm (nếu là member)
    ├──→ analytics        : ghi nhận chuyển đổi
    ├──→ supply-chain     : ghi DemandSignal loại ORDER
    └──→ promotion        : cập nhật số lần dùng mã giảm giá
```

**Quan sát:** module `order` **không biết** các bên này tồn tại. Thêm bên
nghe mới không cần sửa module order. Đây chính là giá trị của event.

### Vì sao `inventory` nghe `checkout.completed`, không phải `order.placed`

Bảng trên từng ghi `order.placed → inventory: Reserved → Committed`. Khi
triển khai thì thấy không làm được: inventory cần `reservation_id` để biết
commit cái nào, mà `Order` **không giữ** nó — reservation là dữ liệu VẬN
HÀNH, không thuộc hợp đồng với khách.

```text
Nhồi reservation_id vào Order        → làm bẩn hợp đồng với khách bằng
                                       chi tiết vận hành
Nghe checkout.completed  ← ĐÃ CHỌN   → checkout biết CẢ HAI đầu:
                                       mã đơn vừa tạo và các mã giữ hàng
```

Payload của `checkout.completed` chứa danh sách reservation, nên inventory
xử lý được mà KHÔNG phải gọi ngược lại checkout.

Xem [../adr/0006-internal-events.md](../adr/0006-internal-events.md) mục
"Trạng thái triển khai".

---

## 6. Xử lý event — yêu cầu bắt buộc

### 6.1 Idempotency

Bên nhận **phải** chịu được việc nhận cùng một event nhiều lần.

```go
func (h *FinancialHandler) HandleOrderPlaced(ctx context.Context, e DomainEvent) error {
    // Kiểm tra đã xử lý chưa
    if h.processedEvents.Exists(e.EventID) {
        return nil  // đã xử lý, bỏ qua
    }

    // Xử lý + đánh dấu đã xử lý trong CÙNG một giao dịch
    return h.db.InTransaction(func(tx) error {
        if err := h.recordLedgerEntry(tx, e); err != nil {
            return err
        }
        return h.processedEvents.Mark(tx, e.EventID)
    })
}
```

**Vì sao "cùng một giao dịch" quan trọng:** nếu ghi sổ thành công nhưng đánh dấu thất bại, lần thử lại sẽ ghi sổ hai lần — tiền bị nhân đôi.

Xem [../05-data/idempotency.md](../05-data/idempotency.md).

### 6.2 Thứ tự event

Không được giả định event đến đúng thứ tự.

```text
Có thể xảy ra:
    order.paid đến TRƯỚC order.placed

Cách xử lý:
    1. Thiết kế bên nhận không phụ thuộc thứ tự nếu được
    2. Nếu bắt buộc thứ tự → kiểm tra trạng thái aggregate, hoãn xử lý
    3. Đảm bảo thứ tự trong phạm vi một aggregate (cùng aggregate_id)
```

### 6.3 Xử lý thất bại

```text
Lỗi tạm thời (mạng, timeout)
    → thử lại với khoảng chờ tăng dần

Lỗi vĩnh viễn (dữ liệu sai, logic lỗi)
    → chuyển vào dead letter queue
    → cảnh báo cho người vận hành
    → KHÔNG thử lại vô hạn

Nguyên tắc: event thất bại KHÔNG được làm hỏng bên phát
```

---

## 7. Cách phát event trong monolith

**Vấn đề:** nếu ghi database thành công nhưng phát event thất bại (hoặc ngược lại), hệ thống không nhất quán.

**Giải pháp: Transactional Outbox**

```text
Trong MỘT giao dịch database:
    1. Ghi thay đổi aggregate
    2. Ghi event vào bảng outbox

Sau đó, tiến trình riêng:
    3. Đọc outbox
    4. Phát event tới các bên nghe
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
    published_at    TIMESTAMPTZ,
    INDEX (published_at) WHERE published_at IS NULL
);
```

**Ưu điểm:** đảm bảo event luôn được phát nếu giao dịch thành công, và không bao giờ phát nếu giao dịch thất bại.

**Quan trọng cho tương lai:** cấu trúc này cho phép chuyển từ phát event trong tiến trình sang message broker thật mà **không sửa module nghiệp vụ** — chỉ thay bộ đọc outbox. Xem [../adr/0006-internal-events.md](../adr/0006-internal-events.md).

---

## 8. Tiến hóa schema event

Event sẽ thay đổi. Quy tắc để không phá vỡ bên nhận:

```text
ĐƯỢC PHÉP (tương thích ngược):
  ✓ Thêm trường tùy chọn
  ✓ Thêm giá trị mới vào enum (bên nhận phải xử lý giá trị lạ)
  ✓ Thêm loại event mới

KHÔNG ĐƯỢC PHÉP:
  ✗ Xóa trường
  ✗ Đổi tên trường
  ✗ Đổi kiểu dữ liệu
  ✗ Đổi ý nghĩa của trường

Nếu bắt buộc thay đổi phá vỡ:
  → tăng event_version
  → phát CẢ HAI phiên bản trong thời gian chuyển tiếp
  → khi mọi bên nhận đã chuyển, ngừng phiên bản cũ
```

**Nguyên tắc thiết kế:** thiết kế event contract như thể chúng **sẽ vượt qua ranh giới tiến trình** — vì trong tương lai chúng sẽ. Điều này có nghĩa: không truyền con trỏ, không truyền đối tượng có hành vi, chỉ truyền dữ liệu thuần.

### 8.1. Thứ tự TRIỂN KHAI, không chỉ thứ tự thay đổi

Các quy tắc trên nói được phép đổi gì. Chúng KHÔNG nói khi nào được triển
khai — và đó mới là chỗ đã gây sự cố thật.

```text
BÊN NHẬN triển khai TRƯỚC, hoặc CÙNG LÚC với bên phát.
KHÔNG BAO GIỜ bên phát trước.
```

**Sự cố ngày 19/08/2026.** Thêm địa chỉ giao vào `checkout.completed`. Một
tiến trình worker CŨ còn sống đã tiêu thụ event mới và **âm thầm bỏ qua**
trường nó không biết. Không lỗi, không log, không cảnh báo — chỉ là đơn
thực hiện thiếu địa chỉ giao, và nhà bán không giao được.

Thứ khiến nó nguy hiểm chính là quy tắc "thêm trường tùy chọn là tương
thích ngược": nó đúng theo nghĩa bên nhận cũ KHÔNG VỠ. Nhưng "không vỡ"
với "làm đúng việc" là hai chuyện khác nhau, và im lặng là cách hỏng tệ
nhất vì không ai biết để đi sửa.

### 8.2. Trạng thái triển khai (20/08/2026)

```text
✅ Trường `event_version` — có ở cả Go (`Event.Version`) và outbox
🔴 Không bên nhận nào ĐỌC version — mọi handler unmarshal thẳng
🔴 Chưa có bên phát nào phát hai phiên bản song song
🔴 Chưa có quy trình triển khai bắt buộc thứ tự bên nhận trước
```

Nghĩa là cơ chế mới có phần KHUNG. Việc hoàn thiện nằm ở
[backlog mục 2.8](../10-roadmap/backlog.md) (PH-7).

Trước mắt, khi payload event đổi: **triển khai worker trước, API sau** —
và kiểm tra không còn tiến trình worker cũ nào đang chạy.

---

## 9. Danh sách event bắt buộc cho MVP

Không cần cài đặt toàn bộ danh mục ở mục 4. MVP chỉ cần:

```text
order.placed
order.paid
order.cancelled
order.completed

fulfillment_order.created
fulfillment_order.shipped
fulfillment_order.delivered

inventory.reserved
inventory.committed
inventory.reservation_released
inventory.depleted

payment.captured
payment.failed

seller.approved

offer.created
offer.price_changed

demand_signal.recorded
```

Event `demand_signal.recorded` có trong MVP dù chuỗi cung ứng chưa làm — vì dữ liệu lịch sử không tạo ngược được. Xem [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 9.

---

## 10. Tài liệu liên quan

- [aggregates.md](aggregates.md) — aggregate phát event
- [../adr/0006-internal-events.md](../adr/0006-internal-events.md) — quyết định về event nội bộ
- [../05-data/idempotency.md](../05-data/idempotency.md) — xử lý trùng lặp
- [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md) — event phá vỡ phụ thuộc vòng
