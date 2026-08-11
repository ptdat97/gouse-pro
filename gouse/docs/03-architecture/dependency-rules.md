# Quy tắc phụ thuộc

## 1. Ba loại phụ thuộc

```text
1. Phụ thuộc TẦNG      — trong một module, tầng nào gọi tầng nào
2. Phụ thuộc MODULE    — module nào được gọi module nào
3. Phụ thuộc HẠ TẦNG   — module gọi platform/kernel
```

Mỗi loại có quy tắc riêng.

---

## 2. Phụ thuộc giữa các tầng

```text
     interfaces
         │ (được gọi)
         ▼
     application
         │ (được gọi)
         ▼
       domain  ◄──────── infrastructure
         │                     ▲
         │ (định nghĩa port)   │ (cài đặt port)
         └─────────────────────┘
```

### Bảng quy tắc

| Từ | Được gọi | Không được gọi |
|---|---|---|
| `interfaces` | `application` | `domain` trực tiếp, `infrastructure` |
| `application` | `domain`, port của domain, public API module khác | `infrastructure` cụ thể, `interfaces` |
| `domain` | Chỉ chính nó và `kernel` | Mọi thứ khác |
| `infrastructure` | `domain` (để cài đặt port) | `application`, `interfaces` |

### Quy tắc quan trọng nhất

> **`domain` không phụ thuộc vào bất cứ thứ gì ngoài `kernel`.**

Kiểm tra: nếu xóa toàn bộ `infrastructure` và `interfaces`, code trong `domain` vẫn phải biên dịch được.

**Vì sao:** đây là điều kiện để logic nghiệp vụ kiểm thử được mà không cần database, không cần HTTP, không cần dịch vụ ngoài. Nếu domain import thư viện database, mọi test domain đều cần database — chậm và giòn.

### Đảo ngược phụ thuộc

```go
// domain/order/repository.go — domain ĐỊNH NGHĨA cái mình cần
package order

type Repository interface {
    FindByID(ctx context.Context, id OrderID) (*Order, error)
    Save(ctx context.Context, o *Order) error
}
```

```go
// infrastructure/postgres_repository.go — infrastructure CÀI ĐẶT
package infrastructure

type PostgresOrderRepository struct {
    db *sql.DB
}

func (r *PostgresOrderRepository) FindByID(ctx context.Context, id order.OrderID) (*order.Order, error) {
    // SQL thật ở đây
}
```

Mũi tên phụ thuộc chỉ **vào trong**: infrastructure biết domain, domain không biết infrastructure.

---

## 3. Ma trận phụ thuộc giữa các module

Ký hiệu:
- `S` = gọi đồng bộ qua interface công khai
- `E` = lắng nghe event (bất đồng bộ)
- `—` = không phụ thuộc
- `✗` = **cấm tuyệt đối**

### 3.1 Tầng nền (không phụ thuộc module nghiệp vụ nào)

| Module | Phụ thuộc |
|---|---|
| `identity` | — |
| `notification` | — (chỉ nhận event) |
| `analytics` | — (chỉ nhận event) |

Ba module này ở **tầng đáy**. Chúng không được gọi module nghiệp vụ nào. Nếu `notification` cần biết tên sản phẩm, thông tin đó phải nằm trong payload event, không phải gọi ngược lại `catalog`.

**Vì sao nghiêm ngặt:** nếu `notification` gọi `catalog`, `order`, `seller`... nó sẽ phụ thuộc vào toàn hệ thống và không tách được.

### 3.2 Tầng danh mục

| Module | catalog | product | pricing | marketplace | inventory |
|---|---|---|---|---|---|
| `catalog` | — | — | — | — | E |
| `product` | S | — | — | — | — |
| `pricing` | S | — | — | — | — |
| `marketplace` | S | S | S | — | E |

**Lưu ý:** `catalog` lắng nghe event từ `inventory` (để biết hết hàng) nhưng **không gọi** `inventory` — đây là quan hệ một chiều qua event, không tạo phụ thuộc vòng.

### 3.3 Tầng giao dịch

| Module | catalog | pricing | inventory | marketplace | promotion | order | payment |
|---|---|---|---|---|---|---|---|
| `cart` | S | S | S | S | S | — | — |
| `checkout` | S | S | S | S | S | S | S |
| `order` | — | — | E | S | — | — | E |
| `payment` | — | — | — | S | — | E | — |
| `fulfillment` | — | — | S | S | — | E | — |

### 3.4 Tầng tăng trưởng

| Module | catalog | order | creator | content | affiliate | campaign |
|---|---|---|---|---|---|---|
| `creator` | — | — | — | — | — | — |
| `content` | S | — | S | — | — | S |
| `affiliate` | — | E | S | S | S | S |
| `campaign` | — | — | S | — | — | — |
| `recommendation` | S | E | — | S | — | — |
| `loyalty` | — | E | — | — | — | — |

### 3.5 Tầng chuỗi cung ứng

| Module | catalog | inventory | supply-chain | procurement | manufacturing | quality | warehouse |
|---|---|---|---|---|---|---|---|
| `supply-chain` | E | S | — | S | S | — | — |
| `procurement` | — | — | — | — | — | — | — |
| `manufacturing` | — | — | — | S | — | S | — |
| `quality` | — | — | — | — | — | — | — |
| `warehouse` | — | S | — | — | E | S | — |
| `return` | — | S | — | — | — | S | — |

---

## 4. Phụ thuộc bị CẤM TUYỆT ĐỐI

Đây là các phụ thuộc mà nếu xuất hiện, kiến trúc đã hỏng:

| Từ | Đến | Vì sao cấm |
|---|---|---|
| `identity` | Bất kỳ module nghiệp vụ nào | Identity là nền tảng, phải độc lập |
| `notification` | Bất kỳ module nghiệp vụ nào | Sẽ phụ thuộc toàn hệ thống |
| `analytics` | Bất kỳ module nghiệp vụ nào (gọi đồng bộ) | Analytics chỉ nhận event |
| `catalog` | `order`, `cart`, `checkout` | Danh mục không được biết về giao dịch |
| `inventory` | `order`, `cart` | Tồn kho không biết ai đang mua |
| `product` | `marketplace`, `seller` | Sản phẩm chuẩn không phụ thuộc nhà bán |
| `payment` | `catalog`, `product` | Thanh toán không cần biết sản phẩm là gì |
| `platform/*` | `modules/*` | Hạ tầng không biết nghiệp vụ |
| `kernel/*` | Bất kỳ thứ gì | Kernel chỉ dùng thư viện chuẩn |
| Bất kỳ module | `domain/` của module khác | Chỉ qua public.go |
| Bất kỳ module | Bảng database của module khác | Nguyên tắc P5 |

### Ví dụ vi phạm điển hình và cách sửa

**Vi phạm 1: `catalog` cần biết sản phẩm bán được bao nhiêu để xếp hạng**

```text
SAI:  catalog gọi order.GetSalesCount(productID)
      → catalog phụ thuộc order → phụ thuộc vòng khi order cần catalog

ĐÚNG: order phát event order.placed
      catalog lắng nghe, tự duy trì bộ đếm doanh số của riêng mình
      → phụ thuộc một chiều qua event
```

**Vi phạm 2: `notification` cần tên sản phẩm để gửi email**

```text
SAI:  notification gọi catalog.GetProduct(id)
      → notification phụ thuộc catalog

ĐÚNG: event order.placed đã chứa product_name (đã đóng băng trong OrderLine)
      → notification chỉ dùng dữ liệu trong payload
```

**Vi phạm 3: `inventory` cần biết đơn hàng nào đang giữ hàng**

```text
SAI:  inventory gọi order.GetOrder(id)

ĐÚNG: Reservation lưu checkout_id / order_id là chuỗi ký tự tham chiếu
      inventory KHÔNG cần biết đơn hàng có gì bên trong
      → chỉ giữ định danh, không giải tham chiếu
```

---

## 5. Phá vỡ phụ thuộc vòng

Khi A cần B và B cần A, có ba cách giải:

### Cách 1: Đảo một chiều thành event (thường dùng nhất)

```text
Trước:
    order  ──S──►  loyalty   (order gọi loyalty để tích điểm)
    loyalty ──S──► order     (loyalty cần biết lịch sử đơn)
    → VÒNG

Sau:
    order  ──phát event──►  order.placed
    loyalty ──lắng nghe──►  tự tích điểm, tự lưu lịch sử cần thiết
    → order KHÔNG biết loyalty tồn tại
```

### Cách 2: Trích xuất phần chung xuống tầng thấp hơn

```text
Trước:
    catalog  ──►  pricing   (cần quy tắc giá)
    pricing  ──►  catalog   (cần thông tin sản phẩm)
    → VÒNG

Sau:
    Nhận ra: pricing chỉ cần category_id và brand_id, không cần cả product
    → truyền vào tham số thay vì gọi ngược
    catalog  ──►  pricing.Calculate(categoryID, brandID, basePrice)
    → pricing không cần gọi ai
```

### Cách 3: Xem lại ranh giới — có thể chúng là một module

```text
Nếu A và B:
    - luôn phải sửa cùng nhau
    - có nhiều event chỉ để đồng bộ với nhau
    - chia sẻ nhiều khái niệm

→ Có thể ranh giới cắt sai chỗ. Gộp lại và xem xét chia theo trục khác.
```

**Nguyên tắc:** phụ thuộc vòng thường là **triệu chứng của ranh giới sai**, không phải vấn đề kỹ thuật cần kỹ thuật giải quyết.

---

## 6. Đồ thị phụ thuộc tổng thể

```text
                    ┌──────────────────────────────┐
                    │      Tầng cao (điều phối)    │
                    │  checkout · fulfillment      │
                    │  supply-chain                │
                    └──────────────────────────────┘
                              │
                    ┌──────────────────────────────┐
                    │      Tầng giao dịch          │
                    │  cart · order · payment      │
                    │  return · procurement        │
                    │  manufacturing               │
                    └──────────────────────────────┘
                              │
                    ┌──────────────────────────────┐
                    │      Tầng nghiệp vụ          │
                    │  marketplace · seller        │
                    │  creator · content ·affiliate│
                    │  campaign · promotion        │
                    │  warehouse · quality         │
                    └──────────────────────────────┘
                              │
                    ┌──────────────────────────────┐
                    │      Tầng dữ liệu chính      │
                    │  catalog · product · pricing │
                    │  inventory · customer        │
                    └──────────────────────────────┘
                              │
                    ┌──────────────────────────────┐
                    │      Tầng nền                │
                    │  identity · notification     │
                    │  analytics                   │
                    └──────────────────────────────┘

Mũi tên phụ thuộc CHỈ đi từ trên xuống dưới.
Từ dưới lên trên: CHỈ qua event.
```

**Kiểm tra nhanh:** nếu vẽ được đồ thị phân tầng như trên mà không có mũi tên đi ngược lên, kiến trúc là DAG hợp lệ.

---

## 7. Quy tắc chọn đồng bộ hay event

| Tình huống | Chọn | Lý do |
|---|---|---|
| Cần dữ liệu để quyết định tiếp | Đồng bộ | Không thể đợi |
| Cần biết thành công/thất bại ngay | Đồng bộ | Phải xử lý lỗi |
| Kiểm tra ràng buộc trước khi cho phép | Đồng bộ | Phải chặn được |
| Thông báo việc đã xảy ra | Event | Không cần đợi |
| Nhiều bên quan tâm | Event | Tránh gọi nhiều nơi |
| Bên nhận có thể chậm | Event | Không làm chậm bên phát |
| Bên nhận có thể lỗi mà không ảnh hưởng | Event | Cách ly lỗi |

**Ví dụ minh họa cả hai trong một luồng:**

```text
Đặt hàng:
    ĐỒNG BỘ: order → inventory.Reserve()
             (phải biết có hàng không mới cho đặt)

    EVENT:   order.placed → notification, analytics, loyalty, affiliate
             (đơn đã đặt rồi, các việc này không được cản trở)
```

---

## 8. Quy tắc với module Platform

```text
platform/ được phép:
    ✓ Cung cấp kết nối database, quản lý giao dịch
    ✓ Cung cấp event bus
    ✓ Cung cấp HTTP server, middleware chung
    ✓ Cung cấp logging, metrics, tracing
    ✓ Xác thực token (biết "ai đang gọi")

platform/ KHÔNG được phép:
    ✗ Import bất kỳ module nghiệp vụ nào
    ✗ Biết về khái niệm nghiệp vụ (order, seller, product)
    ✗ Chứa quyết định phân quyền nghiệp vụ
```

### Ranh giới tinh tế: xác thực vs phân quyền

```text
platform/auth:
    "Token này hợp lệ, thuộc về user_id=123, role=seller"
    → hạ tầng, trung lập domain

module/order:
    "user_id=123 có được xem order này không?"
    → quyết định nghiệp vụ, thuộc module sở hữu dữ liệu
```

Nhầm lẫn hai thứ này dẫn tới việc `platform` phải biết mọi quy tắc nghiệp vụ — vi phạm nguyên tắc P12.

---

## 9. Thực thi tự động trong CI

Kiểm tra bắt buộc, vi phạm làm CI **thất bại**:

```text
Kiểm tra 1: Không có import sâu vào module khác
    modules/*/... chỉ được import modules/*/public.go và các kiểu công khai

Kiểm tra 2: Domain layer sạch
    modules/*/domain/ chỉ import: thư viện chuẩn, kernel/, chính nó

Kiểm tra 3: Platform không biết nghiệp vụ
    platform/... không import modules/...

Kiểm tra 4: Kernel tối thiểu
    kernel/... chỉ import thư viện chuẩn

Kiểm tra 5: Không có chu trình
    Đồ thị phụ thuộc module là DAG

Kiểm tra 6: Sở hữu bảng
    File SQL trong module X chỉ nhắc tới bảng thuộc X

Kiểm tra 7: Không có gói bị cấm
    Không tồn tại thư mục tên common/, utils/, helpers/, services/
```

### Cách cài đặt

```text
Kiểm tra 1–5: phân tích import bằng công cụ phân tích tĩnh Go
Kiểm tra 6:   đối chiếu với file khai báo sở hữu bảng
Kiểm tra 7:   kiểm tra tên thư mục đơn giản
```

**Nguyên tắc:** kiểm tra phải chạy nhanh (< 30 giây) để không ai muốn bỏ qua nó.

---

## 10. Quy trình khi cần phá quy tắc

Đôi khi có lý do chính đáng để vi phạm. Quy trình:

```text
1. Ghi lại lý do trong một ADR
2. Đánh dấu ngoại lệ tường minh trong file cấu hình lint
3. Ghi kèm điều kiện để gỡ bỏ ngoại lệ
4. Rà soát ngoại lệ mỗi quý
```

**Không được phép:** tắt kiểm tra toàn cục, hoặc thêm ngoại lệ mà không ghi lý do. Một ngoại lệ không được ghi chép sẽ trở thành tiền lệ cho mười ngoại lệ sau.

---

## 11. Tài liệu liên quan

- [modular-monolith.md](modular-monolith.md) — cấu trúc module
- [module-boundaries.md](module-boundaries.md) — interface công khai chi tiết
- [../02-domain/domain-events.md](../02-domain/domain-events.md) — event phá vỡ phụ thuộc vòng
- [../adr/0005-module-boundaries.md](../adr/0005-module-boundaries.md) — quyết định về ranh giới
