# ADR-0006: Domain event nội bộ + Transactional Outbox

**Trạng thái:** Accepted — **đã triển khai** (xem mục cuối)

---

## Context

Trong modular monolith, các module cần giao tiếp với nhau. Một sự kiện nghiệp vụ như "đơn hàng được đặt" kéo theo **chín** module khác nhau phải phản ứng:

```text
order.placed
    ├──→ inventory     : Reserved → Committed
    ├──→ fulfillment   : tạo FulfillmentOrder theo seller
    ├──→ payment       : ghi bút toán doanh thu, hoa hồng
    ├──→ affiliate     : xác nhận quy kết creator
    ├──→ notification  : gửi email khách, thông báo seller
    ├──→ loyalty       : tích điểm
    ├──→ analytics     : ghi nhận chuyển đổi
    ├──→ supply-chain  : ghi DemandSignal
    └──→ promotion     : cập nhật số lần dùng mã
```

Nếu module `order` gọi trực tiếp chín module này:

```text
− order phụ thuộc vào 9 module → vi phạm ranh giới nghiêm trọng
− Thêm bên nhận thứ 10 phải sửa module order
− Một module lỗi làm hỏng việc đặt hàng
− Giao dịch kéo dài, tranh chấp khóa
```

Câu hỏi thứ hai: làm sao đảm bảo **ghi database và phát event luôn nhất quán**?

---

## Decision

**Dùng domain event trong tiến trình, kết hợp Transactional Outbox.**

### Phần 1: Domain event

```text
Event mô tả SỰ THẬT ĐÃ XẢY RA, ở thì quá khứ:
    ✓ OrderPlaced, PaymentCaptured, QualityApproved
    ✗ SendEmail, UpdateInventory, ProcessOrder
```

**Vì sao khác biệt này quan trọng:** `SendEmail` là mệnh lệnh trá hình — bên phát phải biết bên nhận làm gì, tạo ghép nối chặt. `OrderPlaced` là sự thật — thêm bên nhận mới không cần sửa bên phát.

### Phần 2: Transactional Outbox

```text
TRONG MỘT GIAO DỊCH DATABASE:
    1. Ghi thay đổi aggregate
    2. Ghi event vào bảng event_outbox
    COMMIT

SAU ĐÓ (tiến trình worker riêng):
    3. Đọc outbox chưa phát
    4. Phát event tới các bên nghe
    5. Đánh dấu đã phát
```

**Đảm bảo:**

```text
✓ Giao dịch thành công → event CHẮC CHẮN được phát (sớm hay muộn)
✓ Giao dịch thất bại   → event KHÔNG BAO GIỜ được phát
✓ Event có thể phát nhiều lần → bên nhận PHẢI idempotent
```

Đây là mô hình **at-least-once**, không phải exactly-once. Vì vậy idempotency ở bên nhận là bắt buộc, không phải tùy chọn.

### Phần 3: Chọn đồng bộ hay event

| Dùng gọi đồng bộ khi | Dùng event khi |
|---|---|
| Cần kết quả để quyết định tiếp | Chỉ thông báo việc đã xảy ra |
| Cần biết thành công/thất bại ngay | Nhiều bên quan tâm |
| Kiểm tra ràng buộc trước khi cho phép | Bên nhận có thể chậm hoặc lỗi |

**Ví dụ trong cùng một luồng:**

```text
ĐỒNG BỘ: order → inventory.Reserve()
         (phải biết có hàng không mới cho đặt)

EVENT:   order.placed → 9 bên nghe
         (đơn đã đặt rồi, các việc này không được cản trở)
```

---

## Alternatives

### A. Gọi trực tiếp giữa các module — **bị loại**

```text
Ưu:
    + Đơn giản, dễ theo dõi luồng
    + Biết ngay thành công hay thất bại

Nhược (quyết định):
    − order phụ thuộc 9 module
    − Thêm bên nhận phải sửa order
    − Một module lỗi làm hỏng đặt hàng
    − Giao dịch kéo dài
```

### B. Message broker riêng ngay từ đầu — **bị loại**

```text
Ưu:
    + Tách biệt hoàn toàn
    + Sẵn sàng cho microservices
    + Có sẵn cơ chế thử lại, dead letter queue

Nhược (quyết định):
    − Thêm hạ tầng phải vận hành, giám sát, sao lưu
    − Không cần thiết khi mọi bên nhận ở CÙNG tiến trình
    − Phức tạp khi phát triển và kiểm thử cục bộ
    − Vi phạm nguyên tắc P15: không đưa công nghệ vào
      vì "sau này có thể cần"
```

**Điểm quan trọng:** thiết kế event contract như thể **sẽ vượt tiến trình**, nhưng cài đặt truyền tải đơn giản nhất cho hiện tại.

Khi cần chuyển sang message broker thật: **chỉ thay bộ đọc outbox**, không sửa module nghiệp vụ.

### C. Event Sourcing toàn hệ thống — **bị loại**

```text
Ưu:
    + Lịch sử đầy đủ
    + Tái dựng trạng thái tại bất kỳ thời điểm

Nhược (quyết định):
    − Độ phức tạp cao cho toàn bộ hệ thống
    − Truy vấn khó
    − Đội chưa có kinh nghiệm
    − Lợi ích không tương xứng với đa số module
```

**Áp dụng có chọn lọc:** mô hình append-only chỉ dùng ở nơi có lý do rõ ràng — `ledger_entry`, `inventory_movement`, `attribution`, `point_transaction`.

### D. Chỉ ghi database, không có outbox — **bị loại**

```text
Vấn đề: ghi database thành công nhưng phát event thất bại
    → đơn hàng "mồ côi": tạo rồi nhưng không ai xử lý
    → không tạo fulfillment order
    → không ghi sổ tài chính
    → khách trả tiền nhưng không nhận hàng
```

---

## Consequences

### Tích cực

```text
✓ Module order KHÔNG biết 9 bên nghe tồn tại
✓ Thêm bên nghe mới không sửa bên phát
✓ Lỗi ở bên nghe không làm hỏng luồng chính
✓ Giao dịch ngắn, ít tranh chấp
✓ Sẵn sàng cho việc tách service (chỉ thay bộ đọc outbox)
✓ Phá vỡ phụ thuộc vòng giữa các module
```

### Tiêu cực

```text
− Luồng nghiệp vụ khó theo dõi hơn (không đọc thẳng được)
− Nhất quán cuối, không tức thời
− Mọi bên nhận phải idempotent
− Cần giám sát độ trễ outbox
```

### Biện pháp giảm rủi ro

```text
1. correlation_id và causation_id trong mọi event
   → truy vết được toàn bộ chuỗi

2. Distributed tracing ngay từ monolith
   → thấy được request đi qua đâu

3. Giám sát:
   - Độ trễ outbox > 60 giây → cảnh báo
   - Dead letter queue > 10 → cảnh báo

4. Tài liệu hóa: bảng "event nào, ai nghe, làm gì"
   tại 03-architecture/module-boundaries.md
```

---

## Xử lý thất bại

```text
Lỗi tạm thời (mạng, timeout)
    → thử lại với khoảng chờ tăng dần

Lỗi vĩnh viễn (dữ liệu sai, logic lỗi)
    → dead letter queue
    → cảnh báo người vận hành
    → KHÔNG thử lại vô hạn

Nguyên tắc: event thất bại KHÔNG được làm hỏng bên phát
```

---

## Tiến hóa schema event

```text
ĐƯỢC PHÉP (tương thích ngược):
    ✓ Thêm trường tùy chọn
    ✓ Thêm giá trị enum (bên nhận phải xử lý giá trị lạ)
    ✓ Thêm loại event mới

KHÔNG ĐƯỢC:
    ✗ Xóa/đổi tên trường
    ✗ Đổi kiểu dữ liệu
    ✗ Đổi ý nghĩa trường

Nếu bắt buộc: tăng event_version, phát cả hai phiên bản
trong thời gian chuyển tiếp
```

---

## Nguyên tắc thiết kế payload

```text
Payload phải chứa ĐỦ thông tin để bên nhận xử lý
mà KHÔNG phải gọi ngược lại bên phát.
```

**Ví dụ cụ thể:** `notification` cần tên sản phẩm để gửi email.

```text
❌ SAI:  notification gọi catalog.GetProduct()
         → notification phụ thuộc catalog → không tách được

✅ ĐÚNG: order.placed đã chứa product_name
         (đã đóng băng trong OrderLine theo nguyên tắc P9)
```

Nhưng cũng **không nhồi toàn bộ aggregate** — chỉ những gì bên nhận cần.

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Luồng khó theo dõi hơn | Module độc lập, thêm bên nghe dễ |
| Nhất quán cuối | Giao dịch ngắn, ít tranh chấp |
| Mọi bên nhận phải idempotent | Đảm bảo không mất event |
| Cần giám sát outbox | Nhất quán giữa database và event |

---

## Tài liệu liên quan

- [../02-domain/domain-events.md](../02-domain/domain-events.md)
- [../05-data/idempotency.md](../05-data/idempotency.md)
- [../05-data/consistency.md](../05-data/consistency.md)
- [ADR-0005](0005-module-boundaries.md)


---

## Trạng thái triển khai (14/08/2026)

**Đã cài đặt.** `internal/platform/eventbus/` có `Event`, `Outbox`,
`Dispatcher`; migration `000011_event_outbox` tạo hai bảng `event_outbox`
và `event_processed`. Worker chạy job "phát domain event" mỗi 5 giây.

### Bên nhận đầu tiên: Reserved → Committed

```text
checkout.CompleteCheckout
    ↓  ghi trạng thái phiên + ghi event vào outbox — CÙNG một giao dịch
checkout.completed  (outbox)
    ↓  worker phát, mỗi 5 giây
inventory.CommitOnCheckoutCompleted
    ↓  chạy trong giao dịch của dispatcher
Reserved → Committed
```

**Vì sao bên nhận này phải có trước mọi bên nhận khác:** tiến trình dọn
reservation quá hạn sẽ NHẢ hàng còn ở trạng thái Reserved. Đơn đã thanh
toán mà hàng vẫn Reserved nghĩa là tiến trình đó biến một đơn đã thu tiền
thành đơn không có hàng — và bán hàng đó cho khách khác.

### Vì sao nghe `checkout.completed` chứ không phải `order.placed`

Đặc tả ở [../02-domain/domain-events.md](../02-domain/domain-events.md)
ghi `order.placed → inventory: Reserved → Committed`. Khi triển khai thì
thấy không làm được: inventory cần `reservation_id`, mà `Order` không giữ
nó — reservation là dữ liệu **vận hành**, không thuộc hợp đồng với khách.

Hai lựa chọn, và lựa chọn thứ hai được chọn:

```text
1. Nhồi reservation_id vào Order
   → làm bẩn hợp đồng với khách bằng chi tiết vận hành
   → Order mang dữ liệu chỉ một bên nhận dùng tới

2. Nghe checkout.completed         ← ĐÃ CHỌN
   → checkout biết CẢ HAI đầu: mã đơn vừa tạo và các mã giữ hàng
   → Order giữ nguyên vai trò "hợp đồng với khách"
```

Đây là ví dụ của nguyên tắc P17: tài liệu mô tả ý định, triển khai cho thấy
một chi tiết chưa được nghĩ tới, và tài liệu được cập nhật thay vì ép code
mang dữ liệu không thuộc về nó.

### Ba bất biến, đều kiểm chứng ngược

| Phá gì | Test bắt được |
|---|---|
| `PublishTx` ghi bằng kết nối riêng thay vì tx của bên gọi | Giao dịch rollback mà event vẫn phát — bên nhận xử lý sự thật chưa xảy ra |
| Bỏ cưỡng chế `event_processed` | Bên nhận xử lý 2 lần — với tiền là ghi sổ hai lần |
| Bỏ giới hạn số lần thử | Event hỏng thử lại 10+ lần, kẹt hàng đợi vĩnh viễn |

Và ở tầng nghiệp vụ:

| Phá gì | Test bắt được |
|---|---|
| Không phát event khi hoàn tất | Hàng kẹt ở Reserved: 7/3/0 thay vì 7/0/3 |
| Handler không commit | Như trên |
| `SaveWithEvents` phát ngoài giao dịch | `eventPublisher` từ chối phát, báo lỗi rõ ràng |

### Điều KHÔNG làm

```text
✗ Kafka / NATS / RabbitMQ — một tiến trình, một database, chưa cần
✓ Bảng event_outbox trong PostgreSQL + worker đọc bảng đó
```

Khi thật sự cần broker: **chỉ thay `Dispatcher`**. Module phát event vẫn
phát như cũ, bên nhận vẫn nhận như cũ — đó là lý do outbox được chọn thay
vì gọi handler trực tiếp lúc phát.

### Còn thiếu

| Việc | Nghe event nào | Trạng thái |
|---|---|---|
| Ghi `demand_signal` | `cart.item_added`, `order.placed` | **Chưa — rủi ro đã biết** |
| Gửi email xác nhận | `order.placed` | Chưa có module notification |
| Ghi nhận chuyển đổi | `order.placed` | Chưa có module analytics |
| Quy kết creator | `order.placed` | Phase 2 |
