# Mô hình dữ liệu

## 1. Nguyên tắc nền tảng

| # | Nguyên tắc | Lý do |
|---|---|---|
| 1 | Mỗi bảng thuộc **đúng một module** | Điều kiện để tách service (P5) |
| 2 | Không khóa ngoại vượt ranh giới module | Khóa ngoại cứng ngăn việc tách |
| 3 | Không JOIN vượt ranh giới module | Ràng buộc ranh giới ở tầng truy vấn |
| 4 | Định danh dùng UUID/ULID | Không lộ quy mô, dễ tách |
| 5 | Tiền dùng số nguyên + đơn vị tiền tệ | Tránh sai số dấu chấm động |
| 6 | Không xóa cứng dữ liệu giao dịch | Nghĩa vụ lưu trữ |
| 7 | Bảng quan trọng có `version` cho khóa lạc quan | Chống mất cập nhật |

---

## 2. Ma trận sở hữu dữ liệu

Xem bảng đầy đủ tại [../03-architecture/module-boundaries.md](../03-architecture/module-boundaries.md) mục 3.

**Quy tắc bổ sung:** bảng nào không có trong bảng đó thì **chưa được tạo**. Thêm bảng mới phải cập nhật tài liệu ranh giới module trong cùng pull request.

---

## 3. Tham chiếu giữa các module

### 3.1 Cách làm đúng

```sql
-- Bảng order_line thuộc module order
CREATE TABLE order_line (
    id         UUID PRIMARY KEY,
    order_id   UUID NOT NULL REFERENCES "order"(id),  -- ✓ cùng module
    offer_id   UUID NOT NULL,     -- ✗ KHÔNG có REFERENCES (module marketplace)
    sku_id     UUID NOT NULL,     -- ✗ KHÔNG có REFERENCES (module product)
    seller_id  UUID NOT NULL,     -- ✗ KHÔNG có REFERENCES (module seller)
    ...
);
```

**Trong cùng module:** dùng khóa ngoại bình thường.
**Vượt module:** chỉ lưu định danh, **không** khai báo khóa ngoại.

### 3.2 Vì sao không dùng khóa ngoại vượt module

```text
Nếu order_line có FOREIGN KEY tới offer:
    - Không thể tách module marketplace ra service riêng
    - Xóa/sửa offer bị chặn bởi ràng buộc từ module khác
    - Migration phải điều phối giữa hai module
    - Ranh giới module chỉ tồn tại trên giấy
```

### 3.3 Đánh đổi phải chấp nhận

Không có khóa ngoại nghĩa là database không đảm bảo tính toàn vẹn tham chiếu.

**Cách bù đắp:**

```text
1. Kiểm tra ở tầng ứng dụng khi tạo
   → gọi marketplace.GetOffer() xác nhận tồn tại

2. Job đối chiếu định kỳ
   → tìm order_line trỏ tới offer_id không tồn tại
   → cảnh báo, không tự sửa

3. Không xóa cứng
   → offer bị archive, không bị DELETE
   → tham chiếu cũ vẫn phân giải được
```

Điểm 3 là quan trọng nhất: nếu không bao giờ xóa cứng, vấn đề tham chiếu treo gần như không xảy ra.

---

## 4. Sơ đồ quan hệ tổng thể

```text
CATALOG                    PRODUCT                  MARKETPLACE
┌──────────┐              ┌──────────┐             ┌──────────┐
│ brand    │◄─ brand_id ──│ product  │             │  offer   │
│ category │◄─ cat_id ────│          │             │          │
│collection│◄─ col_id ────│          │             │          │
│size_chart│◄─ sc_id  ────│          │             └────┬─────┘
└──────────┘              └────┬─────┘                  │
                               │                        │ sku_id
                          ┌────▼─────┐                  │ seller_id
                          │ variant  │                  │
                          └────┬─────┘                  │
                               │                        │
                          ┌────▼─────┐◄─────────────────┘
                          │   sku    │
                          └────┬─────┘
                               │ sku_id
        ┌──────────────────────┼──────────────────────┐
        ▼                      ▼                      ▼
   INVENTORY               CART                  MANUFACTURING
┌──────────────┐       ┌──────────┐            ┌────────────────┐
│inventory_item│       │cart_item │            │production_batch│
│ reservation  │       └────┬─────┘            └────────────────┘
│  movement    │            │
└──────────────┘            ▼
                       CHECKOUT
                    ┌──────────────┐
                    │checkout_line │
                    └──────┬───────┘
                           ▼
                        ORDER
                    ┌──────────────┐
                    │   "order"    │
                    │  order_line  │
                    └──────┬───────┘
                           │ order_id
              ┌────────────┼────────────┐
              ▼            ▼            ▼
       FULFILLMENT     PAYMENT      AFFILIATE
   ┌───────────────┐ ┌──────────┐ ┌────────────┐
   │fulfillment_ord│ │ledger_ent│ │attribution │
   │fulfillment_lin│ │ledger_lin│ └────────────┘
   │   shipment    │ │settlement│
   └───────────────┘ └──────────┘
```

**Đường nét liền trong cùng module** = khóa ngoại thật.
**Đường vượt module** = chỉ lưu định danh.

---

## 5. Quy ước đặt tên

```text
Bảng:        số ít, snake_case          → order, order_line, inventory_item
Cột:         snake_case                 → created_at, total_amount
Khóa chính:  id                         → luôn là UUID
Khóa ngoại:  <entity>_id                → order_id, sku_id
Thời gian:   <verb>_at                  → created_at, shipped_at, deleted_at
Boolean:     is_/has_                   → is_sponsored, has_variant
Tiền:        <name>_amount + <name>_currency
Enum:        lưu dạng TEXT, không dùng số
```

### Vì sao enum lưu dạng TEXT

```sql
-- SAI
status INT  -- 1 = PENDING, 2 = PAID, ...

-- ĐÚNG
status TEXT NOT NULL CHECK (status IN ('PENDING','PAID','SHIPPED',...))
```

Lý do: đọc dữ liệu trực tiếp hiểu ngay; thêm trạng thái mới không phải nhớ số nào chưa dùng; ít rủi ro nhầm lẫn khi đổi thứ tự.

---

## 6. Lưu trữ Value Object

Value object được **nhúng** vào bảng của entity chứa nó:

```sql
-- Money → hai cột
price_amount    BIGINT NOT NULL,
price_currency  CHAR(3) NOT NULL,

-- Address → nhiều cột có tiền tố
shipping_recipient_name  TEXT NOT NULL,
shipping_phone           TEXT NOT NULL,
shipping_street_address  TEXT NOT NULL,
shipping_ward            TEXT,
shipping_district        TEXT,
shipping_province        TEXT NOT NULL,
shipping_country_code    CHAR(2) NOT NULL,

-- Percentage → basis points
commission_rate  INT NOT NULL,  -- 1000 = 10.00%
```

**Không** tạo bảng riêng cho value object, trừ khi nó được dùng lại nhiều nơi và có định danh (khi đó nó là entity, ví dụ `size_chart`).

---

## 7. Dữ liệu bất biến

Ba loại bảng **chỉ ghi thêm**, không sửa không xóa:

```sql
ledger_entry, ledger_line     -- sổ cái tài chính
inventory_movement            -- nhật ký biến động tồn kho
attribution                   -- quy kết creator
point_transaction             -- giao dịch điểm thưởng
event_outbox                  -- hàng đợi event
demand_signal                 -- tín hiệu nhu cầu
```

**Bảo vệ ở tầng database:**

```sql
CREATE RULE ledger_entry_no_update AS ON UPDATE TO ledger_entry DO INSTEAD NOTHING;
CREATE RULE ledger_entry_no_delete AS ON DELETE TO ledger_entry DO INSTEAD NOTHING;
```

Đây là lớp bảo vệ cuối cùng — kể cả khi có lỗi code hoặc thao tác thủ công nhầm.

---

## 8. Xóa mềm vs xóa cứng

| Loại dữ liệu | Cách xóa | Lý do |
|---|---|---|
| Đơn hàng, bút toán, quy kết | **Không bao giờ xóa** | Nghĩa vụ lưu trữ, kiểm toán |
| Sản phẩm, offer đã có đơn | Xóa mềm (`status = ARCHIVED`) | Tham chiếu cũ phải phân giải được |
| Sản phẩm nháp chưa từng bán | Xóa cứng được | Không ai tham chiếu |
| Giỏ hàng cũ | Xóa cứng sau thời hạn | Dữ liệu tạm |
| Dữ liệu cá nhân khi khách yêu cầu | Ẩn danh hóa, không xóa bản ghi | Giữ được dữ liệu giao dịch |
| Log sự kiện | Xóa theo chính sách lưu trữ | Khối lượng lớn |

**Về ẩn danh hóa:** thay thông tin định danh bằng giá trị vô danh, giữ nguyên `customer_id` và dữ liệu giao dịch. Xem [audit.md](audit.md).

---

## 9. Chiến lược đánh chỉ mục

### Nguyên tắc

```text
1. Tạo chỉ mục cho truy vấn THẬT SỰ dùng, không tạo phòng hờ
2. Chỉ mục có điều kiện (partial) cho tập con thường truy vấn
3. Chỉ mục ghép theo đúng thứ tự cột trong WHERE
4. Theo dõi chỉ mục không được dùng, xóa đi
```

### Ví dụ chỉ mục có điều kiện

```sql
-- Chỉ đánh chỉ mục đơn đang xử lý (chiếm phần nhỏ)
CREATE INDEX idx_order_active ON "order" (status, placed_at)
    WHERE status NOT IN ('COMPLETED','CANCELLED');

-- Chỉ đánh chỉ mục offer đang bán
CREATE INDEX idx_offer_active ON offer (sku_id)
    WHERE status = 'ACTIVE';

-- Chỉ đánh chỉ mục hàng sắp hết
CREATE INDEX idx_inventory_low ON inventory_item (sku_id)
    WHERE quantity_available <= 10;
```

Chỉ mục có điều kiện nhỏ hơn nhiều và nhanh hơn khi truy vấn tập con.

---

## 10. Bảng có khối lượng lớn

Bốn bảng sẽ lớn nhanh nhất và cần chiến lược riêng:

| Bảng | Ước tính | Chiến lược |
|---|---|---|
| `click` | Rất lớn | Phân vùng theo ngày, giữ chi tiết 90 ngày |
| `event_log` | Rất lớn | Phân vùng theo ngày, chính sách lưu trữ |
| `demand_signal` | Lớn | Phân vùng, tổng hợp rồi nén dữ liệu cũ |
| `inventory_movement` | Lớn | Phân vùng theo tháng, giữ lâu (cần cho kiểm toán) |

```sql
CREATE TABLE click (
    id          UUID NOT NULL,
    clicked_at  TIMESTAMPTZ NOT NULL,
    ...
) PARTITION BY RANGE (clicked_at);

CREATE TABLE click_2026_08 PARTITION OF click
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
```

**Lưu ý:** phân vùng chỉ cần khi bảng thật sự lớn. Không phân vùng từ đầu cho bảng nhỏ — thêm phức tạp không cần thiết.

---

## 11. Định danh: UUID hay ULID

```text
Khuyến nghị: ULID cho hầu hết bảng

Lý do:
    - Sắp xếp được theo thời gian tạo (tốt cho chỉ mục B-tree)
    - Không lộ quy mô kinh doanh như số tự tăng
    - Tương thích định dạng UUID
    - Chèn tuần tự → ít phân mảnh chỉ mục hơn UUID ngẫu nhiên
```

**Vì sao không dùng số tự tăng:**

```text
- Lộ quy mô: đơn #1547 cho biết nền tảng mới có 1547 đơn
- Khó tách service: hai service sinh id trùng
- Dễ dò: đoán được id của bản ghi khác
```

**Ngoại lệ:** `order_number` hiển thị cho khách nên có định dạng dễ đọc, dễ đọc qua điện thoại:

```text
FC-2026-08-001234
```

Đây là mã hiển thị, khác với `id` nội bộ.

---

## 12. Kiểu dữ liệu

| Loại | Kiểu PostgreSQL | Ghi chú |
|---|---|---|
| Định danh | `UUID` | ULID lưu dạng UUID |
| Tiền | `BIGINT` + `CHAR(3)` | **Không bao giờ dùng FLOAT** |
| Số lượng | `INT` | Có `CHECK >= 0` |
| Phần trăm | `INT` | Basis points |
| Thời gian | `TIMESTAMPTZ` | Luôn có múi giờ |
| Ngày | `DATE` | Khi không cần giờ |
| Trạng thái | `TEXT` + `CHECK` | Không dùng số |
| Văn bản dài | `TEXT` | Không giới hạn độ dài tùy tiện |
| Dữ liệu linh hoạt | `JSONB` | Dùng hạn chế, xem mục 13 |
| Mảng | `TEXT[]` | Cho danh sách đơn giản |

---

## 13. Khi nào dùng JSONB

```text
NÊN dùng:
    ✓ Thuộc tính sản phẩm khác nhau theo loại
    ✓ Metadata của event
    ✓ Cấu hình linh hoạt
    ✓ Payload lưu trữ từ hệ thống ngoài

KHÔNG nên dùng:
    ✗ Dữ liệu cần truy vấn/lọc thường xuyên
    ✗ Dữ liệu có cấu trúc ổn định
    ✗ Bất cứ thứ gì liên quan tiền
    ✗ Thay thế cho việc thiết kế schema đúng
```

**Cảnh báo:** JSONB dễ trở thành nơi nhét mọi thứ để tránh phải suy nghĩ về schema. Nếu một trường trong JSONB được truy vấn thường xuyên, nó nên là cột thật.

---

## 14. Migration

```text
Nguyên tắc:
    1. Mọi thay đổi schema qua file migration, không sửa tay
    2. Migration phải tương thích ngược trong thời gian triển khai
    3. Thêm cột: có giá trị mặc định hoặc cho phép NULL
    4. Xóa cột: hai bước — ngừng dùng, sau đó mới xóa
    5. Đổi tên: thêm cột mới, sao chép, ngừng dùng cột cũ, xóa
    6. Không migration nào khóa bảng lớn quá lâu
```

### Quy trình ba bước cho thay đổi phá vỡ

```text
Bước 1 — Mở rộng:  thêm cấu trúc mới, code ghi cả cũ và mới
Bước 2 — Di trú:   chuyển dữ liệu, code đọc từ mới
Bước 3 — Thu hẹp:  xóa cấu trúc cũ
```

Mỗi bước là một lần triển khai riêng, có thể quay lui.

---

## 15. Tài liệu liên quan

- [database.md](database.md) — lựa chọn và cấu hình database
- [consistency.md](consistency.md) — mô hình nhất quán
- [audit.md](audit.md) — nhật ký kiểm toán
- [../03-architecture/module-boundaries.md](../03-architecture/module-boundaries.md) — ma trận sở hữu
