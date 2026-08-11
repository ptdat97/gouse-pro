# Kiến trúc tổng thể

## 1. Sơ đồ tổng thể

```text
                          NGƯỜI DÙNG
         (Khách · Seller · Creator · Nhân viên · Đối tác)
                              │
                              ▼
    ┌─────────────────────────────────────────────────────┐
    │                    Next.js UI                       │
    │              (CHỈ TRÌNH BÀY)                        │
    │                                                     │
    │  Storefront · Seller Center · Creator Center· Admin │
    └─────────────────────────────────────────────────────┘
                              │
                        HTTP / REST API
                     (JSON, có xác thực)
                              │
                              ▼
    ┌─────────────────────────────────────────────────────┐
    │                   GO BACKEND                        │
    │                (Modular Monolith)                   │
    │                                                     │
    │  ┌───────────────────────────────────────────────┐  │
    │  │  Interfaces Layer                             │  │
    │  │  HTTP handler · middleware · DTO · validation │  │
    │  └───────────────────────────────────────────────┘  │
    │  ┌───────────────────────────────────────────────┐  │
    │  │  Application Layer                            │  │
    │  │  Use case · điều phối · giao dịch             │  │
    │  └───────────────────────────────────────────────┘  │
    │  ┌───────────────────────────────────────────────┐  │
    │  │  Domain Layer                                 │  │
    │  │  Aggregate · Entity · Value Object            │  │
    │  │  Domain service · Domain event · Port         │  │
    │  └───────────────────────────────────────────────┘  │
    │  ┌───────────────────────────────────────────────┐  │
    │  │  Infrastructure Layer                         │  │
    │  │  Repository · Event bus · Adapter bên ngoài   │  │
    │  └───────────────────────────────────────────────┘  │
    │                                                     │
    └─────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ┌──────────┐   ┌───────────┐   ┌──────────────┐
        │ Database │   │  Storage  │   │External APIs │
        │PostgreSQL│   │  (ảnh,    │   │ PSP · Ship · │
        │          │   │   video)  │   │ Email · SMS  │
        └──────────┘   └───────────┘   └──────────────┘
```

---

## 2. Phân định trách nhiệm giữa Next.js và Go

Đây là ranh giới quan trọng nhất của toàn kiến trúc.

| Trách nhiệm | Next.js | Go Backend |
|---|---|---|
| Hiển thị giao diện | Có | Không |
| Điều hướng | Có | Không |
| Kiểm tra hợp lệ (trải nghiệm) | Có | Có (bắt buộc) |
| Kiểm tra hợp lệ (bảo mật) | Không | Có |
| **Tính giá, giảm giá, phí** | **Không** | **Có** |
| **Quyết định trạng thái đơn** | **Không** | **Có** |
| **Kiểm tra tồn kho** | **Không** | **Có** |
| **Tính hoa hồng** | **Không** | **Có** |
| **Phân quyền** | Hiển thị/ẩn UI | **Quyết định thật** |
| Truy cập database | **Không bao giờ** | Có |
| Gọi dịch vụ bên ngoài | Không | Có |
| Tổng hợp dữ liệu để hiển thị | Có | Có (BFF endpoint) |
| Cache phía trình duyệt | Có | Không |

### Vì sao ranh giới này nghiêm ngặt

```text
1. Có nhiều mặt tiền
   Storefront web, app di động, API đối tác, seller center
   → logic ở frontend = logic phải viết lại 4 lần, và sẽ phân kỳ

2. Bảo mật
   Mọi thứ chạy ở trình duyệt đều có thể bị sửa
   → "frontend không hiển thị nút xóa" không phải phân quyền

3. Tính đúng đắn tài chính
   Giá và hoa hồng tính ở client có thể bị thao túng
   → mọi con số tiền phải do backend tính và trả về

4. Khả năng tiến hóa
   Đổi giao diện không được ảnh hưởng nghiệp vụ
```

### Ví dụ cụ thể về ranh giới

**Sai:**
```typescript
// Trong Next.js
const total = items.reduce((sum, i) => sum + i.price * i.qty, 0);
const discount = total > 500000 ? total * 0.1 : 0;
const finalTotal = total - discount + shippingFee;
```

**Đúng:**
```typescript
// Next.js chỉ hiển thị số backend trả về
const { subtotal, discount, shippingFee, total } = await api.getCart(cartId);
```

Ngay cả phép cộng đơn giản cũng không làm ở frontend — vì quy tắc giảm giá sẽ phức tạp dần, và khi đó hai nơi sẽ tính ra hai kết quả khác nhau.

---

## 3. Bốn tầng trong Go backend

### 3.1 Domain Layer — trung tâm

```text
Chứa:
  - Aggregate, Entity, Value Object
  - Domain service (logic không thuộc riêng entity nào)
  - Domain event
  - Port (interface do domain định nghĩa)
  - Quy tắc nghiệp vụ và bất biến

KHÔNG chứa:
  - Câu lệnh SQL
  - Chi tiết HTTP
  - Cấu trúc JSON
  - Thư viện bên ngoài (trừ thư viện chuẩn và tiện ích thuần)
```

**Quy tắc quan trọng nhất:** domain layer **không phụ thuộc vào bất cứ tầng nào khác**. Đây là điều kiện để logic nghiệp vụ kiểm thử được mà không cần database.

```go
// domain/order/order.go
package order

type Order struct {
    id     OrderID
    lines  []OrderLine
    status Status
    total  Money
}

// Quy tắc nghiệp vụ nằm ở đây
func (o *Order) Cancel(reason CancelReason) error {
    if o.status == StatusShipped || o.status == StatusDelivered {
        return ErrCannotCancelShippedOrder
    }
    o.status = StatusCancelled
    o.recordEvent(OrderCancelled{OrderID: o.id, Reason: reason})
    return nil
}

// Port — domain định nghĩa cái mình cần
type Repository interface {
    FindByID(ctx context.Context, id OrderID) (*Order, error)
    Save(ctx context.Context, o *Order) error
}
```

### 3.2 Application Layer — điều phối

```text
Chứa:
  - Use case (một thao tác nghiệp vụ)
  - Điều phối nhiều aggregate
  - Quản lý ranh giới giao dịch
  - Phát event
  - Gọi module khác qua interface công khai

KHÔNG chứa:
  - Quy tắc nghiệp vụ (thuộc domain)
  - Chi tiết HTTP
  - SQL
```

```go
// application/order/place_order.go
package order

type PlaceOrderUseCase struct {
    orders     domain.Repository
    inventory  inventory.PublicAPI    // interface của module khác
    pricing    pricing.PublicAPI
    events     EventPublisher
    tx         TransactionManager
}

func (uc *PlaceOrderUseCase) Execute(ctx context.Context, cmd PlaceOrderCommand) (*PlaceOrderResult, error) {
    // 1. Kiểm tra idempotency
    // 2. Gọi module khác lấy thông tin cần thiết
    // 3. Tạo aggregate qua domain logic
    // 4. Lưu + ghi outbox trong MỘT giao dịch
    // 5. Trả kết quả
}
```

### 3.3 Infrastructure Layer — cài đặt kỹ thuật

```text
Chứa:
  - Cài đặt Repository (SQL thật)
  - Adapter cho dịch vụ bên ngoài (PSP, vận chuyển, email)
  - Cài đặt Event bus
  - Cache
  - Cài đặt các Port do domain định nghĩa
```

**Nguyên tắc đảo ngược phụ thuộc:**

```text
Domain định nghĩa:     PaymentGateway interface
Infrastructure cài:    StripeAdapter, VNPayAdapter

→ Domain KHÔNG biết tên nhà cung cấp nào
→ Đổi nhà cung cấp = thêm adapter, không sửa domain
```

Đây là nguyên tắc P13 tại [../00-overview/principles.md](../00-overview/principles.md).

### 3.4 Interfaces Layer — cổng vào

```text
Chứa:
  - HTTP handler
  - Middleware (xác thực, ghi log, rate limit)
  - DTO (cấu trúc request/response)
  - Chuyển đổi DTO ↔ domain object
  - Xử lý lỗi thành mã HTTP

KHÔNG chứa:
  - Quy tắc nghiệp vụ
  - Truy cập database trực tiếp
```

**Quy tắc:** DTO **không** được là domain object. Nếu trả thẳng aggregate ra JSON, mọi thay đổi nội bộ domain sẽ phá vỡ API.

---

## 4. Luồng một request đi qua các tầng

```text
POST /api/v1/orders
        │
        ▼
┌───────────────────────────────────────────┐
│ Interfaces                                │
│  1. Middleware: xác thực, rate limit      │
│  2. Parse JSON → DTO                      │
│  3. Kiểm tra định dạng                    │
│  4. DTO → Command                         │
└───────────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────────┐
│ Application (PlaceOrderUseCase)           │
│  5. Kiểm tra idempotency key              │
│  6. Gọi inventory.Reserve() (module khác) │
│  7. Gọi pricing.Calculate()               │
│  8. Bắt đầu giao dịch                     │
└───────────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────────┐
│ Domain                                    │
│  9. Order.New(...) — kiểm tra bất biến    │
│ 10. Sinh domain event                     │
└───────────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────────┐
│ Infrastructure                            │
│ 11. Lưu order vào database                │
│ 12. Ghi event vào outbox (cùng giao dịch) │
│ 13. Commit                                │
└───────────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────────┐
│ Interfaces                                │
│ 14. Kết quả → DTO response                │
│ 15. Trả 201 Created + JSON                │
└───────────────────────────────────────────┘
        │
        ▼ (bất đồng bộ, sau khi commit)
┌───────────────────────────────────────────┐
│ Outbox publisher                          │
│ 16. Đọc outbox → phát order.placed        │
│ 17. Các module khác xử lý                 │
└───────────────────────────────────────────┘
```

---

## 5. Cấu trúc thư mục Go

```text
/cmd
    /api                    — điểm khởi chạy HTTP server
    /worker                 — xử lý tác vụ nền, outbox publisher
    /migrate                — chạy migration

/internal
    /modules
        /order
            /domain         — aggregate, entity, VO, port, event
            /application    — use case
            /infrastructure — repository, adapter
            /interfaces     — HTTP handler, DTO
            public.go       — INTERFACE CÔNG KHAI cho module khác
        /inventory
            (cấu trúc tương tự)
        /catalog
        ...

    /platform               — hạ tầng trung lập với domain
        /database           — kết nối, giao dịch, migration
        /eventbus           — cài đặt event bus, outbox
        /httpserver         — cấu hình server, middleware chung
        /observability      — log, metric, trace
        /auth               — xác thực token (không phải phân quyền nghiệp vụ)

    /kernel                 — shared kernel (RẤT hạn chế)
        /types              — Money, ID, Percentage
        /errors             — kiểu lỗi chuẩn

/pkg                        — thư viện dùng lại được bên ngoài (nếu có)

/migrations                 — file SQL migration
/api                        — đặc tả OpenAPI
/docs                       — tài liệu này
```

### Quy tắc thư mục

| Quy tắc | Lý do |
|---|---|
| `internal/` để Go chặn import từ ngoài | Bảo vệ ranh giới ở mức trình biên dịch |
| Mỗi module có `public.go` | Điểm vào duy nhất cho module khác |
| Không có thư mục `common/`, `utils/`, `services/` | Nguyên tắc P12 |
| `platform/` chỉ chứa thứ trung lập domain | Nếu nó biết về "đơn hàng" thì đặt sai chỗ |
| `kernel/` cực kỳ hạn chế | Mọi thứ ở đây là phụ thuộc của toàn hệ thống |

---

## 6. Một tiến trình, nhiều vai trò

```text
cmd/api        — phục vụ HTTP request
cmd/worker     — xử lý outbox, tác vụ định kỳ, job nền

Cả hai dùng CHUNG code module
Khác nhau ở chỗ khởi chạy cái gì
```

**Vì sao tách tiến trình:** tác vụ nền nặng (tổng hợp tín hiệu nhu cầu, tạo báo cáo) không được làm chậm request của khách. Tách tiến trình cho phép mở rộng quy mô độc lập.

**Lưu ý:** đây **không phải** microservices. Chúng vẫn dùng chung codebase, chung database, triển khai cùng phiên bản.

---

## 7. Những gì cố ý KHÔNG có trong kiến trúc này

Theo nguyên tắc P15 — mỗi thứ đưa vào phải giải thích được vì sao cần cho nghiệp vụ này.

| Không dùng | Vì sao |
|---|---|
| Microservices | Ranh giới domain còn thay đổi; xem [ADR-0001](../adr/0001-modular-monolith.md) |
| Kubernetes (giai đoạn đầu) | Quy mô chưa cần; tăng độ phức tạp vận hành |
| Message broker riêng | Event trong tiến trình đủ dùng; xem [ADR-0006](../adr/0006-internal-events.md) |
| CQRS toàn hệ thống | Chỉ dùng ở chỗ có lý do rõ (báo cáo, tìm kiếm) |
| Event Sourcing toàn hệ thống | Chỉ ledger và inventory movement dùng mô hình append-only |
| GraphQL | REST đủ; GraphQL thêm phức tạp về cache và phân quyền |
| Service mesh | Không có nhiều service để mesh |
| Nhiều loại database | Một PostgreSQL đủ cho MVP; thêm khi có lý do đo được |

**Lưu ý về CQRS:** không áp dụng toàn hệ thống, nhưng **có** tách mô hình đọc/ghi ở những chỗ cụ thể:

```text
Tìm kiếm sản phẩm    → chỉ mục tìm kiếm riêng, đồng bộ qua event
Báo cáo tài chính    → bảng tổng hợp, tính lại được từ ledger
Dashboard seller     → view tổng hợp, cập nhật định kỳ
```

Đây là áp dụng có chọn lọc, không phải mẫu kiến trúc bắt buộc.

---

## 8. Điểm mở rộng đã chuẩn bị sẵn

Kiến trúc chuẩn bị sẵn các điểm sau để tiến hóa mà không phải thiết kế lại:

| Điểm | Chuẩn bị bằng cách | Cho phép sau này |
|---|---|---|
| Tách service | Ranh giới module nghiêm ngặt, event contract | Trích xuất module thành service |
| Đổi cổng thanh toán | Port `PaymentGateway` | Thêm/đổi PSP |
| Đổi đơn vị vận chuyển | Port `ShippingProvider` | Nhiều đối tác song song |
| Thêm gợi ý bằng ML | Port `RecommendationEngine` | Thay cài đặt quy tắc |
| Message broker thật | Outbox pattern | Đổi bộ phát, không sửa module |
| Nhiều kho | `stock_location_id` từ đầu | Mở rộng địa điểm |
| Đa tiền tệ | `Money` có currency | Bán quốc tế |
| Nhiều own brand | Own brand là seller nội bộ | Thêm thương hiệu |

---

## 9. Tài liệu liên quan

- [modular-monolith.md](modular-monolith.md) — chi tiết cấu trúc module
- [dependency-rules.md](dependency-rules.md) — quy tắc phụ thuộc bắt buộc
- [module-boundaries.md](module-boundaries.md) — ranh giới và interface công khai
- [api-first.md](api-first.md) — nguyên tắc API
- [evolution-to-services.md](evolution-to-services.md) — chiến lược tách service
