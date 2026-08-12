# Flamingo Commerce

| | |
|---|---|
| Repository | `github.com/i-love-flamingo/flamingo-commerce` |
| License | MIT |
| Sao / Fork | 591 / 95 |
| Cập nhật cuối | 2026-08-11 (đang hoạt động) |
| Trạng thái | Beta — API có thể đổi |
| Vai trò | **Tham chiếu kiến trúc Go chính** |

---

## 1. Vì sao đây là tham chiếu chính

Flamingo là dự án Go duy nhất trong nhóm nghiên cứu áp dụng **Domain-Driven Design + Ports & Adapters** một cách nhất quán cho thương mại điện tử. Nó chứng minh rằng kiến trúc chúng ta chọn (modular monolith, domain layer sạch, port/adapter) **hoạt động được trong Go** ở quy mô thương mại thật.

Các bounded context của Flamingo:

```text
price      giá trị tiền, tính toán, làm tròn, chia
product    mô hình sản phẩm nhiều loại
category   cây danh mục
cart       giỏ hàng nhiều điểm giao, nhiều giao dịch thanh toán
checkout   luồng thanh toán
payment    trừu tượng thanh toán
order      đơn hàng lịch sử
customer   khách hàng
search     tìm kiếm có bộ lọc
```

---

## Năng lực: Mô hình Price và chia tiền

### Cách OSS làm

`Price` là value object bất biến gồm `amount` (big.Float) và `currency`. Các phép toán: `Add`, `Sub`, `Multiply`, `Divided`, `Discounted`, `Inverse`, `TaxFromNet`, `TaxFromGross`.

Bốn chế độ làm tròn: `Floor`, `Ceil`, `HalfUp` (mặc định), `HalfDown`. `GetPayable()` áp dụng độ chính xác theo tiền tệ.

Phương thức quan trọng nhất — `SplitInPayables(count)`:

```text
12.46 EUR chia 6 phần → 2.07 + 2.07 + 2.08 + 2.08 + 2.08 + 2.08
                        (tổng đúng bằng 12.46, không mất xu nào)
```

Số âm được xử lý bằng cách đảo dấu, chia, rồi đảo lại.

### Điểm mạnh

- Bất biến, so sánh bằng giá trị — đúng bản chất value object
- Chia tiền **không mất đồng nào** — bảo toàn tổng tuyệt đối
- Chế độ làm tròn tường minh, không ẩn giấu
- Tách `Charge` (khoản tiền có phân loại) khỏi `Price` (số tiền thuần)

### Điểm yếu

Dùng `big.Float` thay vì số nguyên. Điều này chính xác về mặt toán học nhưng:

- Chậm hơn số nguyên đáng kể
- Phải gọi `GetPayable()` mới ra số tiền thật — dễ quên
- Khó lưu vào database (phải chuyển đổi)
- Vẫn cần biết "độ chính xác của tiền tệ" ở tầng nào

### Yêu cầu của chúng ta

Độ lệch đối soát phải bằng **0**. Nền tảng giữ tiền hộ seller và creator; sai một đồng làm sổ cái không cân và vi phạm bất biến `Σ DEBIT = Σ CREDIT`.

### Adopt

**Thuật toán chia tiền bảo toàn tổng.** Đây là mẫu đã kiểm chứng và chúng ta đã cài đặt trong `Money.Allocate()`:

```go
// internal/kernel/money/allocate.go
func (m Money) Allocate(ratios []int64) ([]Money, error)
```

Test `TestAllocateNeverLosesMoney` kiểm chứng bất biến tương tự.

**Khái niệm `Charge`** — khoản tiền có **phân loại** và **tham chiếu**. Chúng ta chưa có khái niệm này và nó lấp một khoảng trống thật: khi một đơn hàng có hoa hồng nền tảng, hoa hồng creator, phí PSP, phí fulfillment — mỗi khoản cần biết "loại gì, cho ai".

### Adapt

**Dùng số nguyên thay `big.Float`.** Chúng ta đã chọn `int64` theo đơn vị nhỏ nhất:

```go
type Money struct {
    amount   int64      // 299000 = 299.000đ
    currency Currency
}
```

Lý do khác Flamingo: chúng ta ưu tiên **lưu trữ trực tiếp vào database** và **hiệu năng ổn định**, chấp nhận đánh đổi là không biểu diễn được phân số nhỏ hơn đơn vị nhỏ nhất (điều mà thương mại không cần).

### Reject

Không lấy `TaxFromNet`/`TaxFromGross` ở giai đoạn này — mô hình thuế Việt Nam đơn giản hơn châu Âu, và thêm hai chiều tính thuế làm phức tạp không cần thiết ở MVP.

### Quyết định cuối

```text
✓ Đã cài Money.Allocate() theo mẫu SplitInPayables
✓ Bổ sung khái niệm Charge vào module payment (Phase 2)
✗ Không dùng big.Float
✗ Hoãn mô hình thuế hai chiều
```

---

## Năng lực: Ports & Adapters

### Cách OSS làm

Domain định nghĩa **interface (port)**; hạ tầng cài đặt **adapter**. Mỗi module có adapter giả (`fake`) cung cấp dữ liệu test — cho phép chạy toàn bộ ứng dụng mà không cần hệ thống ngoài.

Ví dụ: `product.ProductService` là port; `fake.ProductService` và adapter thật cùng cài đặt nó.

### Điểm mạnh

- Domain kiểm thử được mà không cần database
- Đổi nhà cung cấp = thêm adapter, không sửa domain
- Adapter giả làm việc phát triển frontend độc lập với backend
- Ranh giới rõ ràng giữa "cái gì" (domain) và "làm thế nào" (hạ tầng)

### Điểm yếu

Nhiều tầng gián tiếp. Với module đơn giản, port/adapter là chi phí thừa — mỗi thao tác phải đi qua ba file.

### Yêu cầu của chúng ta

Nguyên tắc P13: năng lực thay thế được (thanh toán, vận chuyển, tìm kiếm, gợi ý) phải nằm sau interface do domain định nghĩa.

### Adopt

**Mẫu port/adapter cho mọi năng lực thay thế được.** Đã có trong thiết kế:

```go
// domain định nghĩa
type PaymentGateway interface { ... }
type ShippingProvider interface { ... }
type RecommendationEngine interface { ... }
```

**Adapter giả cho test.** Đây là điểm chúng ta chưa làm và nên làm: mỗi port có một cài đặt in-memory để test tầng application mà không cần database.

### Adapt

Không áp dụng port/adapter **cho mọi thứ**. Chỉ áp dụng khi:

```text
✓ Năng lực sẽ đổi nhà cung cấp (PSP, vận chuyển)
✓ Năng lực cần thay cài đặt theo giai đoạn (gợi ý: quy tắc → ML)
✗ Repository nội bộ đơn giản — dùng interface nhưng không cần adapter giả riêng
```

Đây là ứng dụng nguyên tắc P16: chấp nhận lặp lại đến lần thứ ba mới trừu tượng hóa.

### Quyết định cuối

```text
✓ Port/adapter cho PSP, vận chuyển, tìm kiếm, gợi ý, thông báo, lưu trữ file
✓ Thêm adapter giả in-memory cho test tầng application
✗ Không port/adapter hóa mọi repository
```

---

## Năng lực: Cart nhiều điểm giao, nhiều giao dịch thanh toán

### Cách OSS làm

Một `Cart` chứa nhiều `Delivery`, mỗi delivery có địa chỉ và phương thức giao riêng. Một đơn có thể có nhiều `PaymentTransaction` — ví dụ trả một phần bằng thẻ quà tặng, phần còn lại bằng thẻ tín dụng.

### Điểm mạnh

Đây là mô hình **hiếm** trong OSS. Đa số nền tảng giả định một đơn = một địa chỉ = một lần thanh toán.

Nó giải quyết được: giao hàng đến nhiều địa chỉ, tách hàng theo nguồn, thanh toán kết hợp nhiều phương thức.

### Điểm yếu

Phức tạp đáng kể cho trường hợp thông thường (một địa chỉ, một thẻ).

### Yêu cầu của chúng ta

Đơn hàng của chúng ta có hàng từ **nhiều nguồn** (own brand + nhiều seller), không thể đóng chung gói. Đây chính là lý do tách `Order`/`FulfillmentOrder`.

### Adopt

**Ý tưởng nhiều lô giao trong một đơn** — đã có trong thiết kế qua `FulfillmentOrder`.

**Ý tưởng nhiều giao dịch thanh toán cho một đơn** — chúng ta cần cho: thanh toán một phần bằng điểm thưởng, hoặc trả lại một phần khi hủy từng phần.

### Adapt

Flamingo chia theo **địa chỉ giao**; chúng ta chia theo **nguồn hàng (seller/kho)**.

```text
Flamingo:  Cart → Delivery (theo địa chỉ)
Chúng ta:  Order → FulfillmentOrder (theo seller + kho)
```

Lý do khác: bài toán của chúng ta là marketplace (nhiều bên bán), không phải giao nhiều địa chỉ.

### Quyết định cuối

```text
✓ Giữ mô hình Order/FulfillmentOrder hiện có — đã đúng hướng
✓ Bổ sung: một Order có thể có nhiều Payment (Phase 2)
✗ Không chia theo địa chỉ giao ở MVP — chưa có nhu cầu
```

---

## Năng lực: Chiến lược kiểm thử bằng adapter giả

### Cách OSS làm

Mỗi module có package `fake` cài đặt đầy đủ các port với dữ liệu mẫu. Chạy `flamingo serve` với cấu hình fake → có ngay một cửa hàng hoạt động, không cần database.

### Điểm mạnh

- Frontend phát triển song song, không chờ backend
- Test tích hợp chạy nhanh, không cần hạ tầng
- Người mới chạy được dự án trong vài phút

### Yêu cầu của chúng ta

Hiện tại `cmd/api` chạy được nhưng chưa có module nghiệp vụ nào. Khi thêm module đầu tiên (`catalog`), cần quyết định: có làm adapter giả không?

### Adopt

**Có.** Đây là mẫu đáng lấy vì nó giải quyết đúng vấn đề chúng ta đang gặp: máy phát triển chưa có Docker/PostgreSQL, nhưng vẫn cần kiểm chứng mô hình domain.

Kế hoạch: mỗi module có `infrastructure/inmemory/` cài đặt repository trong bộ nhớ.

```text
Lợi ích tức thì:
  ✓ Kiểm chứng mô hình domain trước khi có database
  ✓ Test tầng application nhanh
  ✓ Chạy demo không cần hạ tầng
```

### Quyết định cuối

```text
✓ Mỗi module có repository in-memory cho test và phát triển
✓ Cấu hình chọn adapter: STORAGE=memory | postgres
✗ Không làm "fake data generator" đầy đủ như Flamingo — quá tốn công
```

---

## 2. Tổng kết Flamingo

| Hạng mục | Quyết định |
|---|---|
| Thuật toán chia tiền | **ADOPT** — đã cài `Money.Allocate()` |
| Khái niệm `Charge` | **ADOPT** — bổ sung Phase 2 |
| Ports & Adapters | **ADOPT** — cho năng lực thay thế được |
| Adapter giả cho test | **ADOPT** — repository in-memory |
| Nhiều lô giao / đơn | **ADOPT** — đã có qua `FulfillmentOrder` |
| Nhiều giao dịch thanh toán | **ADAPT** — Phase 2 |
| `big.Float` cho tiền | **REJECT** — dùng `int64` |
| Mô hình thuế hai chiều | **REJECT** ở MVP |
| Cấu trúc DI container của Flamingo | **REJECT** — quá nặng, dùng khởi tạo tường minh |

**Nhận xét cuối:** Flamingo là bằng chứng mạnh nhất rằng kiến trúc chúng ta chọn khả thi trong Go. Nhưng nó là **framework** — nó muốn bạn xây trên nó. Chúng ta lấy **mẫu thiết kế**, không lấy framework.

---

## 3. Tài liệu liên quan

- [../02-domain/value-objects.md](../02-domain/value-objects.md) — Money của chúng ta
- [../00-overview/principles.md](../00-overview/principles.md) — P13, P16
- [synthesis.md](synthesis.md)
