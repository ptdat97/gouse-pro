# Mô hình dữ liệu

## 1. Nguyên tắc nền tảng

| # | Nguyên tắc | Lý do |
|---|---|---|
| 1 | Mỗi bảng thuộc **đúng một module** | Điều kiện để tách service (P5) |
| 2 | Không khóa ngoại vượt ranh giới module | Khóa ngoại cứng ngăn việc tách |
| 3 | Không JOIN vượt ranh giới module **trong đường ghi** | Ràng buộc ranh giới ở tầng truy vấn — xem mục 1.2 |
| 4 | Định danh dùng **ULID có tiền tố loại** | Không lộ quy mô, sắp xếp được theo thời gian, đọc log biết ngay loại |
| 5 | Tiền dùng số nguyên + đơn vị tiền tệ | Tránh sai số dấu chấm động |
| 6 | Không xóa cứng dữ liệu giao dịch | Nghĩa vụ lưu trữ |
| 7 | Bảng quan trọng có `version` cho khóa lạc quan | Chống mất cập nhật |

---

## 1.1. Định danh: ULID có tiền tố, kiểu cột là TEXT

**Quy ước đang dùng trong code** — đây là nguồn sự thật, các đoạn SQL minh
họa ở tài liệu module có thể viết `UUID` cho ngắn gọn:

```sql
id TEXT PRIMARY KEY CHECK (id LIKE 'ord\_%' AND length(id) = 30)
```

```text
ord_01KZXPRRKN1F3JVC6ZE97QH11A
│   └── ULID 26 ký tự, mã hóa Crockford base32
└────── tiền tố 3 ký tự cho biết LOẠI thực thể
```

**Vì sao tiền tố:** đọc log thấy `sel_01K…` là biết ngay đó là seller, không
phải đoán. Và một `order_id` truyền nhầm vào chỗ nhận `seller_id` bị chặn
ngay ở `ids.Parse` chứ không đi tới database.

**Vì sao TEXT chứ không phải kiểu UUID:** ULID có tiền tố không phải UUID
hợp lệ. Đổi lại, ràng buộc `CHECK` ở mỗi bảng cưỡng chế đúng tiền tố và
đúng độ dài — chặt hơn kiểu `UUID` vốn chấp nhận mọi UUID của mọi loại
thực thể.

**Vì sao ULID chứ không phải UUIDv4:** ULID sắp xếp được theo thời gian tạo,
nên chỉ mục B-tree không bị phân mảnh khi ghi. Xem `internal/kernel/ids`.

Định dạng này khớp với mẫu trong đặc tả OpenAPI:
`^[a-z]+_[0-9A-HJKMNP-TV-Z]{26}$`.

---

## 1.2. JOIN vượt module: cấm ở đường GHI, cho phép ở đường ĐỌC

Đây là chỗ dễ hiểu sai nhất của nguyên tắc 3. Nói "không JOIN vượt module
trong mọi trường hợp" sẽ làm kiến trúc đọc trở nên vô lý: một báo cáo doanh
thu theo thương hiệu buộc phải kéo hàng vạn dòng về bộ nhớ ứng dụng rồi tự
gộp.

**Phân biệt hai đường:**

```text
ĐƯỜNG GHI (domain transaction)          ĐƯỜNG ĐỌC (read model / báo cáo)
────────────────────────────────        ─────────────────────────────────
Sửa trạng thái, cưỡng chế bất biến      Chỉ đọc, không quyết định gì
    ↓                                       ↓
KHÔNG chạm bảng của module khác         ĐƯỢC JOIN, view, projection
Gọi API công khai hoặc nghe event       Miễn là CHỈ ĐỌC
```

### Đường ghi — cấm tuyệt đối

```text
✗ Module order UPDATE bảng inventory_item
✗ Module order JOIN order với inventory_item để QUYẾT ĐỊNH có bán được không
✗ Khóa ngoại cứng giữa bảng của hai module
```

Lý do không đổi: quyết định nghiệp vụ dựa trên bảng của module khác nghĩa là
module đó không còn kiểm soát được bất biến của chính nó.

### Đường đọc — được phép, có điều kiện

```text
✓ Truy vấn báo cáo JOIN nhiều bảng của nhiều module
✓ VIEW hoặc MATERIALIZED VIEW phục vụ trang quản trị
✓ Read model dựng sẵn, đồng bộ qua event
✓ Truy vấn phân tích trong tiến trình worker
```

**Bốn điều kiện bắt buộc:**

| # | Điều kiện | Vì sao |
|---|---|---|
| 1 | **CHỈ ĐỌC** — không `INSERT`/`UPDATE`/`DELETE` | Ghi vượt module là phá ranh giới sở hữu |
| 2 | Đặt trong module **báo cáo/analytics**, không nằm trong module nghiệp vụ | Giữ đường ghi của module nghiệp vụ sạch |
| 3 | Kết quả **không quay lại làm đầu vào quyết định** của đường ghi | Nếu quay lại, nó là đường ghi trá hình |
| 4 | Đăng ký tường minh: ghi rõ truy vấn đọc bảng nào của module nào | Khi tách service, đây là danh sách việc phải làm |

**Vì sao nới lỏng có kiểm soát tốt hơn cấm tuyệt đối:** cấm tuyệt đối không
làm biến mất nhu cầu báo cáo — nó chỉ đẩy việc gộp dữ liệu lên tầng ứng
dụng, nơi làm việc đó chậm hơn và không ai nhìn thấy sự phụ thuộc. Một câu
`JOIN` được đăng ký tường minh dễ tìm và dễ sửa hơn một vòng lặp Go gọi ba
module.

**Khi tách service:** mỗi truy vấn đọc vượt module trở thành một việc phải
xử lý — hoặc dựng read model đồng bộ qua event, hoặc gọi API. Điều kiện 4
tồn tại chính vì lúc đó cần biết danh sách này.

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

### 3.4 Link table — quan hệ nhiều-nhiều vượt ranh giới module

Ba mục trên xử lý quan hệ **một-nhiều** (order_line trỏ tới offer). Nhưng còn quan hệ **nhiều-nhiều** vượt module thì bảng trung gian đặt ở đâu?

Ví dụ: một nội dung gắn nhiều sản phẩm; một sản phẩm xuất hiện trong nhiều nội dung. Bảng `product_tag` thuộc module `content` hay `product`?

**Giải pháp: link table** — mẫu lấy từ Medusa (xem [../11-oss/medusa.md](../11-oss/medusa.md)).

```text
Link table là loại bảng RIÊNG với ba quy tắc:

1. CHỈ chứa hai định danh + metadata của CHÍNH QUAN HỆ
   → không chứa dữ liệu thuộc về hai thực thể được liên kết

2. KHÔNG có ràng buộc khóa ngoại vượt module
   → giống mọi tham chiếu vượt module khác

3. Thuộc về module SỞ HỮU Ý NGHĨA của quan hệ
   → "gắn sản phẩm vào nội dung" là khái niệm của content
   → product_tag thuộc module content
```

**Bốn link table trong hệ thống:**

| Link table | Liên kết | Module sở hữu | Metadata của quan hệ |
|---|---|---|---|
| `product_tag` | content ↔ product | `content` | vị trí trên ảnh, giây trong video |
| `outfit_item` | outfit ↔ product | `content` | vai trò (MAIN/COMPLEMENT), sản phẩm thay thế |
| `campaign_participant` | campaign ↔ creator | `campaign` | trạng thái tham gia, ngày duyệt |
| `brand_authorization` | brand ↔ seller | `catalog` | giấy tờ, hạn hiệu lực |

```sql
-- Ví dụ: product_tag thuộc module content
CREATE TABLE content.product_tag (
    id               UUID PRIMARY KEY,
    content_id       UUID NOT NULL REFERENCES content.content(id),  -- ✓ cùng module
    product_id       UUID NOT NULL,     -- ✗ KHÔNG REFERENCES (module product)
    offer_id         UUID,              -- ✗ KHÔNG REFERENCES (module marketplace)
    -- metadata CỦA CHÍNH QUAN HỆ:
    position_x       NUMERIC(4,3),
    position_y       NUMERIC(4,3),
    timestamp_second INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Vì sao quy tắc 1 quan trọng:** nếu `product_tag` chứa `product_name` (để tránh gọi module product), nó trở thành bản sao dữ liệu và sẽ lệch khi tên sản phẩm đổi. Chỉ metadata **của chính quan hệ** — vị trí tag — mới thuộc về link table.

Link table cũng nằm trong phạm vi job đối chiếu ở mục 3.3: phát hiện tag trỏ tới sản phẩm không tồn tại.

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

## 11. Định danh: ULID có tiền tố

> Quy ước đầy đủ ở [mục 1.1](#11-định-danh-ulid-có-tiền-tố-kiểu-cột-là-text).
> Mục này giữ phần lập luận.

```text
Đang dùng: ULID có tiền tố loại, lưu ở cột TEXT

Lý do:
    - Sắp xếp được theo thời gian tạo (tốt cho chỉ mục B-tree)
    - Không lộ quy mô kinh doanh như số tự tăng
    - Tiền tố cho biết LOẠI thực thể ngay khi đọc log
    - Chèn tuần tự → ít phân mảnh chỉ mục hơn UUID ngẫu nhiên
```

**Giả định ban đầu đã sai:** tài liệu này từng ghi "ULID lưu dạng UUID".
Khi triển khai thì thêm tiền tố loại (`ord_`, `sel_`) — thứ khiến chuỗi
không còn là UUID hợp lệ. Đổi lại được hai điều đáng giá hơn: đọc log biết
ngay loại thực thể, và `ids.Parse` chặn được việc truyền nhầm `order_id`
vào chỗ nhận `seller_id`. Cột chuyển sang `TEXT` kèm ràng buộc `CHECK`.

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
| Định danh | `TEXT` + `CHECK` | ULID có tiền tố loại — xem mục 1.1 |
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
