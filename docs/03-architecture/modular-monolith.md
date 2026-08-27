# Modular Monolith

## 1. Định nghĩa

Modular monolith là một ứng dụng **triển khai như một khối duy nhất**, nhưng bên trong được chia thành các module có **ranh giới nghiêm ngặt như thể chúng là service riêng**.

```text
Monolith truyền thống          Modular Monolith           Microservices
─────────────────────          ─────────────────          ─────────────
Một tiến trình                 Một tiến trình             Nhiều tiến trình
Không có ranh giới             Ranh giới nghiêm ngặt      Ranh giới vật lý
Gọi hàm tự do                  Chỉ qua interface công khai Gọi qua mạng
Database chung, join tự do     Database chung, mỗi bảng   Database riêng
                               thuộc một module
Dễ viết, khó bảo trì           Kỷ luật cao, dễ tiến hóa   Phức tạp vận hành
```

**Điểm mấu chốt:** modular monolith có **kỷ luật của microservices** nhưng **chi phí vận hành của monolith**.

---

## 2. Vì sao chọn cách này

Xem đầy đủ tại [ADR-0001](../adr/0001-modular-monolith.md). Tóm tắt:

| Lý do | Giải thích |
|---|---|
| Ranh giới domain chưa ổn định | Sửa ranh giới trong monolith là refactor; giữa các service là dự án di trú |
| Giao dịch đơn giản | Đặt hàng chạm nhiều aggregate — giao dịch phân tán rất khó |
| Đội ngũ nhỏ | Microservices cần nhiều nguồn lực vận hành |
| Gỡ lỗi dễ | Một stack trace, không phải truy vết qua nhiều dịch vụ |
| Triển khai đơn giản | Một artifact, một phiên bản |

**Nhưng:** kỷ luật ranh giới phải nghiêm ngặt **ngay từ ngày đầu**. Nếu không, sau 2 năm nó trở thành monolith rối và không tách được.

---

## 3. Cấu trúc một module

Mỗi module có cấu trúc bốn tầng giống nhau:

```text
/internal/modules/order/
    │
    ├── public.go              ← INTERFACE CÔNG KHAI (điểm vào duy nhất)
    │
    ├── domain/
    │   ├── order.go           — aggregate root
    │   ├── order_line.go      — entity
    │   ├── status.go          — value object, enum
    │   ├── events.go          — domain event
    │   ├── errors.go          — lỗi domain
    │   └── repository.go      — PORT (interface, không phải cài đặt)
    │
    ├── application/
    │   ├── place_order.go     — use case
    │   ├── cancel_order.go
    │   ├── complete_order.go
    │   ├── queries.go         — truy vấn đọc
    │   └── handlers.go        — xử lý event từ module khác
    │
    ├── infrastructure/
    │   ├── postgres_repository.go   — cài đặt Repository
    │   ├── mappers.go               — chuyển đổi domain ↔ bảng
    │   └── queries.sql
    │
    └── interfaces/
        ├── http_handler.go    — HTTP endpoint
        ├── dto.go             — request/response
        └── routes.go
```

### Vì sao tất cả module dùng chung cấu trúc

```text
1. Người mới đọc module nào cũng biết tìm gì ở đâu
2. Công cụ kiểm tra ranh giới hoạt động thống nhất
3. Việc tách module thành service sau này có quy trình chuẩn
```

**Ngoại lệ được phép:** module rất đơn giản (ví dụ `notification`) có thể bỏ tầng `domain` nếu không có logic nghiệp vụ thật sự. Không bắt buộc tạo tầng rỗng cho đủ hình thức.

---

## 4. Interface công khai của module

Đây là cơ chế thực thi ranh giới quan trọng nhất.

```go
// internal/modules/inventory/public.go
package inventory

// PublicAPI là CÁCH DUY NHẤT module khác tương tác với inventory.
type PublicAPI interface {
    // Truy vấn
    GetAvailableQuantity(ctx context.Context, skuID, locationID string) (int, error)
    CheckAvailability(ctx context.Context, items []AvailabilityRequest) (*AvailabilityResult, error)

    // Lệnh
    Reserve(ctx context.Context, req ReserveRequest) (*Reservation, error)
    ReleaseReservation(ctx context.Context, reservationID string) error
    Commit(ctx context.Context, reservationID string) error
}

// DTO công khai — KHÔNG phải domain object
type ReserveRequest struct {
    CheckoutID string
    Items      []ReserveItem
    TTL        time.Duration
}

type ReserveItem struct {
    SKUID      string
    LocationID string
    Quantity   int
}
```

### Quy tắc bắt buộc

```text
1. Chỉ public.go được export ra ngoài module
   → domain/, application/, infrastructure/, interfaces/ là nội bộ

2. Interface công khai KHÔNG trả về domain object
   → trả DTO riêng
   → lý do: thay đổi nội bộ domain không phá module khác

3. Interface công khai KHÔNG nhận domain object của module khác
   → chỉ nhận kiểu nguyên thủy và DTO của chính mình

4. Không truyền con trỏ tới aggregate qua ranh giới module
   → aggregate chỉ được sửa bởi module sở hữu nó
```

### Vì sao không trả domain object

```go
// SAI
func (a *api) GetOrder(id string) (*domain.Order, error)
// → module khác có thể gọi order.Cancel() — phá vỡ tính đóng gói
// → đổi cấu trúc domain.Order phá vỡ mọi module dùng nó

// ĐÚNG
func (a *api) GetOrderSummary(id string) (*OrderSummary, error)
// → OrderSummary là DTO chỉ đọc, ổn định
```

---

## 5. Xử lý code dùng chung

Đây là nơi modular monolith hay thất bại. Nguyên tắc P12 cấm `common/`, `utils/`, `helpers/`, `services/` làm bãi rác.

### Ba loại code dùng chung được phép

```text
┌─────────────────────────────────────────────────────────────┐
│ 1. SHARED KERNEL  (/internal/kernel)                        │
│                                                             │
│ Khái niệm domain mà MỌI module đều hiểu giống nhau          │
│                                                             │
│ Được phép:  Money, Percentage, Quantity, ID types,          │
│             kiểu lỗi domain chuẩn                           │
│                                                             │
│ Ràng buộc:  - Rất nhỏ, thay đổi rất hiếm                    │
│             - Thay đổi phải được mọi đội đồng ý             │
│             - KHÔNG chứa logic đặc thù module nào           │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ 2. PLATFORM  (/internal/platform)                           │
│                                                             │
│ Hạ tầng kỹ thuật TRUNG LẬP với domain                       │
│                                                             │
│ Được phép:  kết nối database, quản lý giao dịch,            │
│             event bus, HTTP server, logging, metrics,       │
│             tracing, cache client, xác thực token           │
│                                                             │
│ Ràng buộc:  - KHÔNG biết gì về nghiệp vụ                    │
│             - Nếu nó nhắc tới "order" hay "seller"          │
│               → đặt sai chỗ                                 │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ 3. GENERIC TECHNICAL  (/pkg hoặc thư viện ngoài)            │
│                                                             │
│ Tiện ích kỹ thuật thuần túy                                 │
│                                                             │
│ Được phép:  chuyển đổi chuỗi, xử lý thời gian,              │
│             mã hóa, sinh ID                                 │
│                                                             │
│ Ràng buộc:  - Không có trạng thái                           │
│             - Dùng được ở bất kỳ dự án nào                  │
│             - Ưu tiên dùng thư viện có sẵn hơn tự viết      │
└─────────────────────────────────────────────────────────────┘
```

### Bài kiểm tra khi định thêm code dùng chung

```text
Câu hỏi 1: Nó có nhắc tới khái niệm nghiệp vụ cụ thể không?
    Có  → KHÔNG đặt vào platform. Nó thuộc về một module.

Câu hỏi 2: Nếu tách module thành service, code này đi đâu?
    Đi theo một module cụ thể  → đặt vào module đó
    Cần nhân bản ở mọi service → có thể là kernel/platform
    Không rõ                    → chưa đủ hiểu, đừng vội trừu tượng hóa

Câu hỏi 3: Đã có bao nhiêu chỗ cần nó?
    1–2 chỗ  → để nguyên, chấp nhận lặp lại (nguyên tắc P16)
    3+ chỗ   → cân nhắc trích xuất
```

### Ví dụ phân loại

| Code | Đặt ở đâu | Vì sao |
|---|---|---|
| Kiểu `Money` | kernel | Mọi module dùng, khái niệm chung |
| Hàm tính hoa hồng | module marketplace | Logic nghiệp vụ cụ thể |
| Bọc kết nối database | platform | Trung lập domain |
| Kiểm tra định dạng email | pkg | Tiện ích thuần |
| "Kiểm tra seller có active không" | module seller | Nghiệp vụ của seller |
| Middleware xác thực JWT | platform | Trung lập domain |
| "Người dùng này được xem đơn này không" | module order | Quyết định nghiệp vụ |

Hai dòng cuối minh họa ranh giới tinh tế: **xác thực** (bạn là ai) là hạ tầng; **phân quyền nghiệp vụ** (bạn được làm gì với dữ liệu này) thuộc module sở hữu dữ liệu.

---

## 6. Sở hữu dữ liệu

**Nguyên tắc P5:** mỗi bảng thuộc về đúng một module.

```text
Module order sở hữu:      order, order_line, order_address
Module inventory sở hữu:  inventory_item, reservation, inventory_movement
Module seller sở hữu:     seller, seller_document, seller_policy
```

### Quy tắc truy cập

```text
ĐƯỢC:
  ✓ Module order đọc/ghi bảng order
  ✓ Module order gọi inventory.PublicAPI để biết tồn kho
  ✓ Module order lắng nghe event từ inventory

KHÔNG ĐƯỢC:
  ✗ Module order SELECT trực tiếp từ bảng inventory_item
  ✗ Module order JOIN order với inventory_item
  ✗ Module order UPDATE bảng của module khác
  ✗ Khóa ngoại cứng giữa bảng của hai module
```

### Vì sao cấm JOIN qua ranh giới module — và cấm ở đâu

Lệnh cấm áp dụng cho **đường ghi**, không phải mọi truy vấn. Phân biệt này
quan trọng: xem [../05-data/data-model.md](../05-data/data-model.md) mục 1.2.

```text
ĐƯỜNG GHI (quyết định nghiệp vụ)     → CẤM JOIN vượt module
ĐƯỜNG ĐỌC (báo cáo, phân tích)       → ĐƯỢC, nếu chỉ đọc và đăng ký tường minh
```

Với đường ghi:

```text
Nếu cho phép JOIN:
    - Không biết ai đang phụ thuộc vào cấu trúc bảng nào
    - Đổi schema của inventory phá vỡ module không liên quan
    - Tách inventory thành service → mọi JOIN đều gãy
    - Ranh giới module chỉ tồn tại trên giấy

Nếu cấm JOIN:
    - Phụ thuộc tường minh qua interface
    - Đổi schema nội bộ an toàn
    - Tách service chỉ là thay cài đặt interface
```

Với đường đọc, lập luận trên **không áp dụng**: một truy vấn báo cáo không
cưỡng chế bất biến nào, nên nó không thể làm hỏng bất biến của module khác.
Rủi ro duy nhất là gãy khi tách service — xử lý bằng cách đăng ký tường minh
danh sách truy vấn đọc vượt module.

Cấm tuyệt đối không làm biến mất nhu cầu báo cáo; nó chỉ đẩy việc gộp dữ
liệu lên tầng ứng dụng, nơi làm chậm hơn và không ai nhìn thấy sự phụ thuộc.

### Đánh đổi cần chấp nhận

Cấm JOIN có chi phí thật: một số truy vấn cần nhiều lần gọi thay vì một câu SQL.

**Cách xử lý:**

```text
1. Đa số trường hợp: chấp nhận, hiệu năng vẫn đủ tốt
   (gọi hàm trong tiến trình rất nhanh)

2. Trang danh sách cần dữ liệu nhiều module:
   → dùng batch API (lấy nhiều id một lần), không gọi trong vòng lặp

3. Báo cáo/analytics cần join phức tạp:
   → ĐƯỢC dùng JOIN / view / read model, với bốn điều kiện ở
     05-data/data-model.md mục 1.2 (chỉ đọc · nằm ở module báo cáo ·
     không quay lại làm đầu vào đường ghi · đăng ký tường minh)
   → đây là chỗ CQRS có lý do chính đáng
```

**Cảnh báo về vấn đề N+1:** nếu module order cần thông tin của 50 sản phẩm, phải gọi `catalog.GetProductsByIDs([50 ids])` một lần, không gọi `GetProduct(id)` 50 lần. Interface công khai phải thiết kế hỗ trợ truy vấn theo lô ngay từ đầu.

---

## 7. Ranh giới giao dịch

```text
Quy tắc: một giao dịch database chỉ sửa dữ liệu của MỘT module.
```

**Vì sao:** giao dịch trải nhiều module tạo ghép nối chặt và ngăn việc tách service.

**Cách xử lý khi nghiệp vụ cần nhiều module:**

```text
Ví dụ: đặt hàng cần tạo Order VÀ cam kết tồn kho

Cách SAI:
    BEGIN
      INSERT INTO "order" ...
      UPDATE inventory_item ...
    COMMIT

Cách ĐÚNG:
    1. Gọi inventory.Reserve() — giao dịch riêng của inventory
    2. BEGIN (giao dịch của order)
         INSERT INTO "order" ...
         INSERT INTO event_outbox (order.placed) ...
       COMMIT
    3. Inventory nghe order.placed → chuyển Reserved thành Committed
       (giao dịch riêng của inventory)
```

**Xử lý thất bại:** nếu bước 2 thất bại sau khi bước 1 thành công, reservation sẽ **tự hết hạn** sau TTL. Đây là mẫu bù trừ đơn giản và hiệu quả — không cần giao dịch phân tán.

Xem [../05-data/consistency.md](../05-data/consistency.md).

---

## 8. Thực thi ranh giới bằng công cụ

Kỷ luật con người không đủ. Phải có kiểm tra tự động trong CI.

### 8.1 Dùng `internal/` của Go

```text
/internal/modules/order/domain/     ← Go chặn import từ ngoài internal/
```

Điều này chặn được code bên ngoài dự án, nhưng **không** chặn module này import module khác.

### 8.2 Kiểm tra phụ thuộc tùy chỉnh

Cần một công cụ trong CI kiểm tra:

```text
Quy tắc kiểm tra:
  1. modules/X/... chỉ được import modules/Y/public.go, không import sâu hơn
  2. modules/X/domain/ không được import modules/Y/... (bất kỳ)
  3. platform/ không được import modules/...
  4. kernel/ không được import gì ngoài thư viện chuẩn
  5. Không có chu trình trong đồ thị phụ thuộc module
```

Có thể cài đặt bằng cách phân tích cây cú pháp Go hoặc dùng công cụ lint kiến trúc có sẵn.

**Quan trọng:** vi phạm phải làm CI thất bại, không phải cảnh báo. Cảnh báo sẽ bị bỏ qua.

### 8.3 Kiểm tra sở hữu bảng

```text
Duy trì một file khai báo: bảng nào thuộc module nào
CI kiểm tra: file SQL trong module X chỉ nhắc tới bảng của X
```

Cách đơn giản: đặt tiền tố tên bảng theo module, hoặc dùng schema riêng cho mỗi module trong PostgreSQL.

---

## 9. Kiểm thử

Cấu trúc module cho phép chiến lược kiểm thử rõ ràng:

| Loại | Phạm vi | Cần gì |
|---|---|---|
| Unit test domain | Aggregate, value object, quy tắc nghiệp vụ | Không cần gì — chạy trong bộ nhớ |
| Unit test application | Use case với repository giả lập | Bản giả lập |
| Integration test | Module + database thật | Database test |
| Contract test | Interface công khai của module | Kiểm tra hợp đồng không đổi |
| End-to-end test | Nhiều module qua HTTP | Toàn hệ thống |

**Điểm quan trọng:** domain layer không phụ thuộc gì nên **kiểm thử quy tắc nghiệp vụ cực nhanh**. Đây là lợi ích lớn của việc tách tầng — có thể chạy hàng nghìn test domain trong vài giây.

**Contract test** đặc biệt quan trọng cho tương lai: nó đảm bảo interface công khai của module ổn định, giúp việc tách service sau này an toàn.

---

## 10. Dấu hiệu kiến trúc đang xuống cấp

Cần theo dõi và xử lý ngay khi thấy:

| Dấu hiệu | Nghĩa là | Xử lý |
|---|---|---|
| Interface công khai của một module có > 30 phương thức | Module quá lớn | Cân nhắc tách |
| Module A và B luôn phải sửa cùng nhau | Ranh giới sai | Xem lại, có thể gộp hoặc chia lại |
| Có nhiều truy vấn "chỉ đọc chút xíu" từ bảng module khác | Kỷ luật đang lỏng | Thêm vào interface công khai |
| `kernel/` ngày càng phình | Đang thành bãi rác | Rà soát, đẩy về module |
| Nhiều event chỉ để đồng bộ giữa hai module | Hai module đó có thể là một | Xem lại ranh giới |
| Phải sửa 5 module cho một tính năng | Ranh giới cắt sai chỗ | Xem lại bounded context |

**Nguyên tắc:** rà soát đồ thị phụ thuộc mỗi quý. Kiến trúc module không tự duy trì — nó cần chăm sóc chủ động.

---

## 11. Tài liệu liên quan

- [dependency-rules.md](dependency-rules.md) — ma trận phụ thuộc chi tiết
- [module-boundaries.md](module-boundaries.md) — interface công khai từng module
- [evolution-to-services.md](evolution-to-services.md) — khi nào và cách tách service
- [../adr/0001-modular-monolith.md](../adr/0001-modular-monolith.md) — quyết định kiến trúc
- [../adr/0005-module-boundaries.md](../adr/0005-module-boundaries.md) — quyết định về ranh giới
