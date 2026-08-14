# Module: Inventory

| | |
|---|---|
| **Bounded Context** | Inventory |
| **Phân loại** | Supporting (nhưng ràng buộc kỹ thuật cao) |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Theo dõi số lượng hàng theo SKU, theo địa điểm, theo chủ sở hữu
- Quản lý chuyển đổi trạng thái tồn kho
- Giữ hàng tạm thời cho checkout
- Cam kết hàng cho đơn đã xác nhận
- Ghi nhật ký mọi biến động tồn kho
- Phát tín hiệu hết hàng và sắp hết hàng

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Biết ai đang mua | `order` |
| Quyết định giá | `pricing`, `marketplace` |
| Quyết định kho nào xuất hàng | `fulfillment` |
| Quyết định khi nào cần nhập thêm | `supply-chain` |
| Kiểm định chất lượng hàng | `quality` |
| Thao tác vật lý trong kho | `warehouse` |

**Ranh giới quan trọng:** inventory chỉ trả lời "còn bao nhiêu, ở đâu, trạng thái gì". Nó **không biết** về đơn hàng, khách hàng, hay lý do nghiệp vụ.

---

## 3. Khái niệm domain

### 3.1 Aggregate: `InventoryItem`

```text
InventoryItem {
    id
    sku_id
    stock_location_id
    inventory_owner_id      ← PLATFORM hoặc seller_id
    quantities              ← value object gồm 6 trạng thái
    production_batch_id     (nullable)
    version                 ← khóa lạc quan
}
```

**Khóa định danh nghiệp vụ:** `(sku_id, stock_location_id, inventory_owner_id)`.

Ba trường này cho phép mô hình hóa cả ba mô hình fulfillment bằng một cấu trúc:

```text
Own brand:              owner=PLATFORM,  location=kho nền tảng
Seller tự giao:         owner=seller_A,  location=kho seller A
Nền tảng giao hộ:       owner=seller_A,  location=kho nền tảng
```

Trường hợp thứ ba là lý do phải tách `owner` khỏi `location` — hàng nằm ở kho nền tảng nhưng **thuộc sở hữu seller**, không được ghi nhận là tài sản của nền tảng.

### 3.2 Aggregate: `Reservation`

```text
Reservation {
    id
    inventory_item_id
    checkout_id
    quantity
    expires_at
    status      ACTIVE | CONVERTED | EXPIRED | RELEASED
}
```

### 3.3 Aggregate: `InventoryMovement`

Sổ nhật ký **bất biến** mọi biến động. Cho phép tái dựng trạng thái tại bất kỳ thời điểm nào và điều tra khi có sai lệch.

---

## 4. Trạng thái tồn kho và chuyển đổi

### 4.1 Sáu trạng thái

```text
Available   — khả dụng, có thể bán
Reserved    — đã giữ tạm cho một checkout đang diễn ra
Committed   — đã cam kết cho đơn hàng xác nhận
In Transit  — đang trung chuyển (giữa kho, hoặc từ nhà cung cấp)
Damaged     — hư hỏng, không bán được
Returned    — đã hoàn về, chờ kiểm định
```

### 4.2 Bất biến bắt buộc

```text
available + reserved + committed + in_transit + damaged + returned
    = tổng số lượng vật lý

Mọi thành phần ≥ 0
```

Vi phạm bất biến này dẫn tới bán hàng không có, hoặc hàng bị khóa vĩnh viễn.

### 4.3 Sơ đồ chuyển đổi đầy đủ

```text
                    ┌──────────────┐
   Nhập kho ───────►│  Available   │
                    └──────────────┘
                       │        ▲
            Reserve    │        │  ReleaseReservation
                       ▼        │  (hủy checkout hoặc hết hạn)
                    ┌──────────────┐
                    │   Reserved   │
                    └──────────────┘
                       │        ▲
            Commit     │        │  (hiếm — hủy đơn trước khi xuất)
            (đơn xác   ▼        │
             nhận)  ┌──────────────┐
                    │  Committed   │
                    └──────────────┘
                       │
            Ship       │
                       ▼
                    (rời khỏi tồn kho — đã xuất)
                       │
                       │ khách trả hàng
                       ▼
                    ┌──────────────┐
                    │   Returned   │
                    └──────────────┘
                       │
            Kiểm định  │
                       ├──── đạt ────►  Available
                       │
                       └── không đạt ──►  Damaged

    Chuyển kho:
        Available ──► In Transit ──► Available (kho đích)
```

### 4.4 Luồng hoàn hàng — chi tiết

Đây là luồng quan trọng với thời trang (tỷ lệ hoàn cao):

```text
Hàng hoàn về kho
    ↓
Returned (chưa được cộng vào Available)
    ↓
Kiểm định chất lượng
    ├── Còn nguyên tem mác, không lỗi     → Available
    ├── Có vết bẩn nhẹ, xử lý được        → Available (sau xử lý)
    ├── Đã sử dụng, không bán giá gốc     → Available (kênh hàng giảm giá)
    └── Hỏng, bẩn, thiếu phụ kiện         → Damaged
```

**Quy tắc bắt buộc:** hàng hoàn **không bao giờ** tự động cộng vào `Available`. Phải qua kiểm định.

Vi phạm quy tắc này dẫn tới việc bán lại hàng hỏng cho khách khác — thiệt hại uy tín lớn hơn nhiều so với giá trị món hàng.

---

## 5. Xử lý tranh chấp đồng thời

Đây là thách thức kỹ thuật lớn nhất của module này.

### 5.1 Vấn đề

```text
Còn 1 sản phẩm. Hai khách bấm mua cùng lúc.

Nếu xử lý sai:
    Khách A: đọc available=1, kiểm tra OK, ghi available=0
    Khách B: đọc available=1, kiểm tra OK, ghi available=0
    → Bán 2 sản phẩm nhưng chỉ có 1
```

### 5.2 Giải pháp: khóa lạc quan (optimistic locking)

```sql
UPDATE inventory_item
SET quantity_available = quantity_available - $qty,
    quantity_reserved  = quantity_reserved + $qty,
    version = version + 1
WHERE id = $id
  AND version = $expected_version
  AND quantity_available >= $qty;

-- Nếu affected rows = 0 → có xung đột hoặc không đủ hàng → thử lại hoặc báo lỗi
```

**Hai điều kiện trong WHERE là mấu chốt:**

- `version = $expected_version` → phát hiện có người khác vừa sửa
- `quantity_available >= $qty` → **kiểm tra và cập nhật nguyên tử**, không phải đọc rồi ghi

Điều kiện thứ hai một mình đã đủ chống bán quá số lượng. Điều kiện thứ nhất giúp phát hiện xung đột để xử lý đúng.

### 5.3 Vì sao không dùng khóa bi quan

```sql
SELECT ... FOR UPDATE  -- khóa bi quan
```

**Vấn đề:** với live commerce, hàng nghìn người mua cùng một SKU trong vài giây. Khóa bi quan tạo hàng đợi tuần tự — mọi request xếp hàng chờ, độ trễ tăng vọt, kết nối database cạn kiệt.

Khóa lạc quan cho phép xử lý song song, chỉ những request thật sự xung đột mới phải thử lại.

### 5.4 Chiến lược thử lại

```text
Xung đột phiên bản (version mismatch)
    → thử lại tối đa 3 lần với khoảng chờ ngẫu nhiên ngắn
    → nếu vẫn thất bại: trả lỗi tạm thời, đề nghị thử lại

Không đủ hàng (available < qty)
    → KHÔNG thử lại
    → trả lỗi INSUFFICIENT_INVENTORY ngay
```

Phân biệt hai loại lỗi này quan trọng: thử lại khi hết hàng chỉ lãng phí tài nguyên.

### 5.5 Kịch bản live commerce

```text
Yêu cầu: chịu được hàng nghìn request/giây trên MỘT SKU

Chiến lược:
1. Khóa lạc quan (như trên)
2. Hàng đợi ở tầng ứng dụng cho SKU điểm nóng
3. Giới hạn tốc độ theo khách (chống bot)
4. Thà từ chối rõ ràng còn hơn bán rồi hủy

Cân nhắc cho quy mô rất lớn (Phase 4):
   Chia nhỏ tồn kho thành nhiều "ô" để giảm tranh chấp
   → phức tạp hơn, chỉ làm khi đo được cần thiết
```

---

## 6. Cơ chế giữ hàng (Reservation)

### 6.1 Vì sao cần

```text
Khách vào checkout → cần đảm bảo hàng còn khi họ nhập thông tin thanh toán
Nhưng: không được giữ vĩnh viễn nếu khách bỏ ngang
```

### 6.2 Vòng đời

```text
Checkout bắt đầu
    ↓
Reserve(TTL = 15 phút)
    Available → Reserved
    ↓
    ├── Thanh toán thành công trong 15 phút
    │       → Commit: Reserved → Committed
    │
    ├── Khách hủy checkout
    │       → Release: Reserved → Available
    │
    └── Hết 15 phút, không có động tĩnh
            → Expire: Reserved → Available (tự động)
```

### 6.3 Xử lý hết hạn

```text
Tiến trình nền chạy định kỳ (ví dụ mỗi 30 giây):
    - Tìm reservation có expires_at < now và status = ACTIVE
    - Giải phóng
    - Đánh dấu EXPIRED
    - Phát event inventory.reservation_released
```

**Quan trọng:** cơ chế hết hạn phải **đáng tin cậy**. Nếu nó ngừng chạy, hàng bị khóa dần và cuối cùng không bán được gì. Cần giám sát: cảnh báo nếu có reservation quá hạn lâu chưa được xử lý.

### 6.4 Gia hạn

Khi khách đang ở bước thanh toán và cần thêm thời gian (ví dụ đang chuyển khoản ngân hàng), cho phép gia hạn:

```text
ExtendReservation(reservationID, thêm 10 phút)
```

Giới hạn số lần gia hạn để tránh giữ hàng vô hạn.

---

## 7. Dữ liệu sở hữu

```sql
inventory_item          -- tồn kho theo SKU × địa điểm × chủ sở hữu
reservation             -- giữ hàng tạm thời
inventory_movement      -- nhật ký bất biến mọi biến động
stock_location          -- danh mục địa điểm lưu kho
```

### Bảng `inventory_item`

```sql
CREATE TABLE inventory_item (
    id                   UUID PRIMARY KEY,
    sku_id               UUID NOT NULL,
    stock_location_id    UUID NOT NULL,
    inventory_owner_id   UUID NOT NULL,
    quantity_available   INT  NOT NULL DEFAULT 0 CHECK (quantity_available >= 0),
    quantity_reserved    INT  NOT NULL DEFAULT 0 CHECK (quantity_reserved >= 0),
    quantity_committed   INT  NOT NULL DEFAULT 0 CHECK (quantity_committed >= 0),
    quantity_in_transit  INT  NOT NULL DEFAULT 0 CHECK (quantity_in_transit >= 0),
    quantity_damaged     INT  NOT NULL DEFAULT 0 CHECK (quantity_damaged >= 0),
    quantity_returned    INT  NOT NULL DEFAULT 0 CHECK (quantity_returned >= 0),
    production_batch_id  UUID,
    version              BIGINT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (sku_id, stock_location_id, inventory_owner_id)
);

CREATE INDEX idx_inventory_sku ON inventory_item (sku_id);
CREATE INDEX idx_inventory_low_stock ON inventory_item (sku_id)
    WHERE quantity_available <= 10;
```

**Ràng buộc `CHECK` ở tầng database** là lớp bảo vệ cuối cùng. Kể cả khi có lỗi logic ở tầng ứng dụng, database vẫn từ chối số lượng âm.

---

## 8. Interface công khai

```go
type PublicAPI interface {
    // Truy vấn — LUÔN hỗ trợ theo lô để tránh N+1
    GetAvailability(ctx, skuIDs []string, locationID string) (map[string]int, error)
    CheckAvailability(ctx, items []AvailabilityRequest) (*AvailabilityResult, error)
    GetInventoryByOwner(ctx, ownerID string, page Pagination) (*InventoryList, error)

    // Giữ hàng
    Reserve(ctx, req ReserveRequest) (*ReservationResult, error)
    ReleaseReservation(ctx, reservationID string) error
    ExtendReservation(ctx, reservationID string, ttl time.Duration) error

    // Cam kết và xuất
    Commit(ctx, reservationID string) error
    Ship(ctx, req ShipRequest) error

    // Nhập và điều chỉnh
    Receive(ctx, req ReceiveRequest) error
    Adjust(ctx, req AdjustRequest) error
    TransferBetweenLocations(ctx, req TransferRequest) error

    // Hoàn hàng
    ReceiveReturn(ctx, req ReceiveReturnRequest) error
    ProcessReturnInspection(ctx, req InspectionResultRequest) error
}
```

---

## 9. Use case

| Use case | Mô tả | Ai gọi |
|---|---|---|
| `CheckAvailability` | Kiểm tra còn hàng không | cart, checkout |
| `ReserveInventory` | Giữ hàng cho checkout | checkout |
| `CommitInventory` | Cam kết cho đơn | (nghe order.placed) |
| `ReleaseReservation` | Giải phóng | checkout, tiến trình hết hạn |
| `ShipInventory` | Xuất hàng | fulfillment |
| `ReceiveGoods` | Nhập kho | warehouse |
| `AdjustInventory` | Điều chỉnh thủ công (kiểm kê) | admin |
| `TransferInventory` | Chuyển giữa kho | warehouse |
| `ProcessReturn` | Xử lý hàng hoàn | return, quality |
| `ExpireReservations` | Dọn reservation hết hạn | tiến trình nền |

---

## 10. Event

### Phát ra

| Event | Khi nào | Payload chính |
|---|---|---|
| `inventory.reserved` | Giữ hàng thành công | reservation_id, sku_id, quantity |
| `inventory.reservation_released` | Giải phóng | reservation_id, reason |
| `inventory.committed` | Cam kết cho đơn | sku_id, quantity, order_id |
| `inventory.received` | Nhập kho | sku_id, quantity, batch_id |
| **`inventory.depleted`** | **Hết hàng** | sku_id, location_id |
| `inventory.low_stock` | Xuống dưới ngưỡng | sku_id, current, threshold |
| `inventory.adjusted` | Điều chỉnh thủ công | sku_id, delta, reason, performed_by |

**`inventory.depleted` là event chiến lược** — nó là tín hiệu nhu cầu bị bỏ lỡ, đầu vào của bánh đà chuỗi cung ứng. Xem [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 4.2.

### Lắng nghe

| Event | Từ module | Hành động |
|---|---|---|
| `order.placed` | order | Reserved → Committed |
| `order.cancelled` | order | Giải phóng hàng |
| `order.line_cancelled` | order | Giải phóng phần bị hủy |
| `warehouse.goods_received` | warehouse | Tăng Available |
| `return.inspected` | return | Returned → Available hoặc Damaged |

### Những việc KHÔNG đi qua event

Ba thao tác dưới đây được gọi **ĐỒNG BỘ** qua interface công khai, vì bên
gọi cần biết kết quả ngay:

| Thao tác | Ai gọi | Vì sao phải đồng bộ |
|---|---|---|
| `Reserve` | checkout | Không giữ được hàng thì không mở phiên thanh toán |
| `ReleaseReservation` | checkout | Nhả qua event mà event lỗi = hàng khóa vĩnh viễn |
| `ExtendReservation` | checkout | Phiên sống lâu hơn reservation = mất hàng đúng lúc khách trả tiền |

Ngoài ra, `inventory` còn có **tiến trình dọn riêng** (30 giây/lượt) nhả các
reservation quá hạn. Đây là lớp bảo vệ độc lập: kể cả khi module gọi quên
nhả, hàng vẫn được trả về sau khi hết TTL.

---

## 11. Phụ thuộc

```text
Gọi đồng bộ:   (không gọi module nghiệp vụ nào)
Nghe event:    order, checkout, warehouse, return
Được gọi bởi:  cart, checkout, order, fulfillment, warehouse,
               marketplace, supply-chain
```

**Đặc điểm quan trọng:** inventory **không gọi module nào**. Đây là module ở tầng thấp, được nhiều module gọi nhưng không phụ thuộc ai. Điều này làm nó dễ kiểm thử và là ứng viên tách service tương đối rõ ràng về mặt phụ thuộc.

---

## 12. Quy tắc nghiệp vụ quan trọng

| # | Quy tắc | Lý do |
|---|---|---|
| 1 | Tổng các trạng thái = số lượng vật lý | Bất biến cốt lõi |
| 2 | Không trạng thái nào được âm | Chống lỗi logic |
| 3 | Hàng hoàn phải qua kiểm định trước khi Available | Không bán lại hàng hỏng |
| 4 | Mọi biến động ghi vào `inventory_movement` | Truy vết, điều tra sai lệch |
| 5 | Reservation phải có thời hạn | Tránh khóa hàng vĩnh viễn |
| 6 | Kiểm tra và cập nhật phải nguyên tử | Chống bán quá số lượng |
| 7 | Điều chỉnh thủ công phải có lý do và người thực hiện | Kiểm toán |
| 8 | Không tự động cộng hàng khi nhập mà chưa QC | Chất lượng |

---

## 13. Giám sát cần có

| Chỉ báo | Ngưỡng cảnh báo |
|---|---|
| Reservation quá hạn chưa xử lý | > 100 |
| Tỷ lệ xung đột khóa lạc quan | > 5% |
| Số SKU có tồn kho âm | > 0 (nghiêm trọng) |
| Độ lệch khi kiểm kê | > 1% |
| Thời gian xử lý Reserve (p99) | > 100ms |
| Số SKU hết hàng | Theo dõi xu hướng |

Chỉ báo thứ ba phải luôn bằng 0. Bất kỳ giá trị nào khác là lỗi nghiêm trọng.

---

## 14. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Một địa điểm, các trạng thái cơ bản, reservation, khóa lạc quan |
| **Phase 2** | Nhiều địa điểm, chuyển kho, xử lý hàng hoàn |
| **Phase 3** | Truy vết theo lô sản xuất, tích hợp sâu với warehouse |
| **Phase 4** | Tối ưu cho tranh chấp cực cao (live commerce quy mô lớn) |

---

## 15. Tài liệu liên quan

- [../02-domain/entities.md](../02-domain/entities.md) — chi tiết entity
- [../05-data/consistency.md](../05-data/consistency.md) — mô hình nhất quán
- [fulfillment.md](fulfillment.md) — module dùng inventory nhiều nhất
- [warehouse.md](warehouse.md) — thao tác vật lý
