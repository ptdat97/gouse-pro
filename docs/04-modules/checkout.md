# Module: Checkout

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | Supporting |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý phiên thanh toán — bước chuyển từ giỏ hàng thành đơn hàng
- **Đóng băng giá** tại thời điểm bắt đầu checkout
- **Giữ tồn kho** tạm thời
- Thu thập địa chỉ giao hàng, phương thức vận chuyển
- Tính phí vận chuyển và áp dụng khuyến mãi
- Điều phối việc tạo đơn hàng

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Quản lý giỏ hàng | `cart` |
| Tạo bản ghi đơn hàng | `order` |
| Thu tiền | `payment` |
| Quản lý số lượng tồn kho | `inventory` |
| Tạo đơn thực hiện | `fulfillment` |

---

## 3. Vì sao Checkout là aggregate riêng

Đã phân tích tại [../02-domain/aggregates.md](../02-domain/aggregates.md) mục 3.5.

```text
Cart:                          Checkout:
  - sống lâu                     - sống ngắn (15 phút)
  - không giữ tồn kho            - CÓ giữ tồn kho
  - giá cập nhật động            - giá ĐÓNG BĂNG
  - thay đổi tự do               - hạn chế thay đổi
```

Gộp chung sẽ dẫn tới hoặc là giỏ hàng khóa tồn kho vô ích, hoặc là giá thay đổi giữa chừng thanh toán.

---

## 4. Vòng đời phiên checkout

```text
    Khách bấm "Thanh toán"
        ↓
    ┌─────────────────────────────────────────┐
    │ 1. Kiểm tra tồn kho toàn bộ giỏ         │
    │    → thiếu hàng: dừng, báo rõ món nào   │
    └─────────────────────────────────────────┘
        ↓
    ┌─────────────────────────────────────────┐
    │ 2. GIỮ TỒN KHO (TTL 15 phút)            │
    │    inventory.Reserve()                  │
    └─────────────────────────────────────────┘
        ↓
    ┌─────────────────────────────────────────┐
    │ 3. ĐÓNG BĂNG GIÁ                        │
    │    Sao chép giá hiện tại vào CheckoutLine│
    └─────────────────────────────────────────┘
        ↓
    STARTED
        ↓
    Khách nhập địa chỉ → tính phí ship
    Khách chọn phương thức thanh toán
    Khách áp mã giảm giá
        ↓
    PENDING_PAYMENT
        ↓
        ├──→ EXPIRED (quá 15 phút) → giải phóng tồn kho
        │
        ├──→ CANCELLED (khách hủy) → giải phóng tồn kho
        │
        ▼ (thanh toán thành công)
    COMPLETED → tạo Order
```

---

## 5. Đóng băng giá — vì sao quan trọng

```text
Tình huống:
    14:00 — Khách bắt đầu checkout, áo giá 299.000đ
    14:05 — Seller đổi giá thành 350.000đ
    14:10 — Khách hoàn tất thanh toán

Nếu không đóng băng:
    → Khách thấy 299.000đ nhưng bị trừ 350.000đ
    → Khiếu nại, mất niềm tin

Nếu đóng băng:
    → Khách trả đúng 299.000đ như đã thấy
    → Giá mới áp dụng cho lần mua sau
```

Đây là ứng dụng nguyên tắc P9 và là yêu cầu cơ bản về tính minh bạch giá.

---

## 6. Xử lý hết hạn

```text
Reservation có TTL 15 phút. Khi hết hạn:

    1. Tiến trình nền phát hiện phiên quá hạn
    2. NHẢ HÀNG — gọi inventory.ReleaseReservation() ĐỒNG BỘ
    3. Đánh dấu phiên EXPIRED
    4. Phát event checkout.expired (thông báo cho analytics/notification)
    5. Khách quay lại → phải bắt đầu phiên mới
```

**Vì sao nhả hàng ĐỒNG BỘ chứ không qua event (bước 2 trước bước 4):**

```text
Nếu nhả qua event:
    - Có khoảng thời gian phiên đã chết mà hàng vẫn khóa
    - Event thất bại → hàng khóa VĨNH VIỄN, không ai đi tìm
      (phiên đã EXPIRED nên không tiến trình nào quét nó nữa)

Nhả đồng bộ trước khi ghi trạng thái:
    - Ghi thất bại → phiên vẫn EXPIRED ở lượt quét sau, nhả lại vô hại
    - Nhả thất bại → chưa ghi EXPIRED, lượt sau thử lại
```

Thứ tự này là chủ ý: **nhả hàng trước, ghi trạng thái sau**. Sai thứ tự thì
hàng bị khóa cho một phiên mà không tiến trình nào còn đi tìm.

**Lớp bảo vệ thứ hai:** reservation ở `inventory` có TTL riêng và có tiến
trình dọn riêng (30 giây/lượt). Kể cả khi job dọn phiên chết hẳn, hàng vẫn
được nhả đúng hạn — chỉ có trạng thái phiên là hiển thị sai.

**Kiểm tra theo ĐỒNG HỒ, không chỉ theo trạng thái:** tiến trình nền chạy
theo chu kỳ nên luôn có khoảng trống giữa "hết hạn thật" và "được đánh dấu
EXPIRED". Mọi thao tác trên phiên phải kiểm tra `expires_at` so với thời
điểm hiện tại, không chỉ nhìn cột trạng thái.

**Trường hợp cần gia hạn:** khách đang chuyển khoản ngân hàng, cần thêm thời gian.

```text
ExtendCheckout(checkoutID, thêm 10 phút)
→ gọi inventory.ExtendReservation()
→ giới hạn số lần gia hạn (ví dụ tối đa 2 lần)
```

---

## 7. Tính phí vận chuyển cho đơn nhiều nhà bán

```text
Giỏ hàng có hàng từ 3 nguồn khác nhau
    ↓
Với mỗi nguồn (seller/kho):
    - Xác định điểm xuất hàng
    - Gọi fulfillment.EstimateShipping()
    - Nhận về: phí, thời gian dự kiến
    ↓
Tổng hợp:
    Phí ship tổng = Σ phí từng nguồn (hoặc theo chính sách gộp)
    Thời gian giao = hiển thị riêng cho từng nhóm
```

**Quyết định trải nghiệm:** hiển thị thời gian giao **riêng cho từng nhóm hàng**, không gộp thành một con số. Khách cần biết món nào đến trước.

**Chính sách miễn phí vận chuyển — ĐÃ QUYẾT (06/09/2026):** áp dụng trên **tổng đơn**, ngưỡng mặc định **499.000đ**, sửa được lúc chạy qua `checkout.free_shipping_threshold` (ADR-0015). Nền tảng chịu phần chênh lệch.

Ngưỡng xét trên **tiền hàng SAU giảm giá** — số khách thực trả cho hàng, chưa gồm phí ship và thuế. Xét trên subtotal *trước* giảm giá thì một mã giảm 200.000đ biến đơn 400.000đ thành đơn được miễn phí ship, tức nền tảng chịu cả hai khoản cho một đơn nhỏ hơn ngưỡng.

So sánh là `>=`: đơn đúng 499.000đ **được** miễn. Khách nhìn con số quảng cáo rồi mua vừa đúng bằng nó là hành vi phổ biến; để họ trượt vì một đồng là cách chắc chắn để nhận một khiếu nại đúng.

Miễn phí do **mã giảm giá** cấp là một đường khác, không đi qua ngưỡng — cùng kết quả nhưng khác lý do, và hóa đơn cần phân biệt được.

---

## 7b. Thuế

**ĐÃ QUYẾT (07/09/2026):** thuế **một tầng**, áp cho mọi mặt hàng, thuế suất mặc định **8%** và sửa được lúc chạy qua `checkout.tax_rate_bp` (phần vạn: 800 = 8%).

**Giá niêm yết là giá ĐÃ GỒM VAT.** Khách trả đúng con số đã thấy trên trang; thuế được **tách ngược** ra để ghi hóa đơn, không cộng thêm vào tổng.

```text
tiền hàng = subtotal − giảm giá          ĐÃ GỒM VAT
phí ship  = 0 nếu tiền hàng ≥ ngưỡng, ngược lại = phí theo từng nguồn
tổng      = tiền hàng + phí ship         khách trả ĐÚNG con số này
thuế      = phần VAT nằm TRONG tổng, tách ra để ghi hóa đơn
```

Ví dụ, đơn 390.000đ hàng + 30.000đ ship:

```text
Giá niêm yết        390.000đ   ← đã gồm VAT
Phí vận chuyển       30.000đ   ← đã gồm VAT
─────────────────────────────
KHÁCH TRẢ           420.000đ

(trong đó thuế       31.111đ)  ← tách ra, chỉ để ghi hóa đơn
```

**Công thức tách là `tổng × r / (10000 + r)`, KHÔNG phải `tổng × r / 10000`.** Nhầm hai cái cho 33.600đ thay vì 31.111đ trên đơn 420.000đ, và cả hai đều trông hợp lý nếu không đối chiếu ngược:

```text
đúng:  388.889 (chưa thuế) + 31.111 (thuế) = 420.000 ✓
sai:   386.400 (chưa thuế) + 33.600 (thuế) = 420.000 ✗  (386.400 sai)
```

Cài ở `money.ExtractRate`, nhân bằng 128 bit để không tràn — tràn ở đây làm số thuế sai mà tổng vẫn khớp, tức không có gì báo.

**`Total()` KHÔNG cộng `taxAmount`**, ở cả `Checkout` lẫn `Order`. Cộng vào là thu thuế hai lần — một lần đã nằm trong giá, một lần cộng thêm — và tổng vẫn trông hợp lý nên không ai thấy. Hai công thức phải khớp nhau, nếu không con số ở màn hình thanh toán khác con số vào đơn.

**Thuế tách từ CẢ phí vận chuyển.** Vận chuyển là dịch vụ chịu thuế, không phải khoản thu hộ. Bỏ phí ra khỏi phần tách sẽ ghi hóa đơn thiếu phần thuế của dịch vụ vận chuyển.

**Làm tròn NỬA LÊN** (`money.RoundHalfUp`), khác hoa hồng và phí vốn làm tròn xuống. Hai quy tắc cho hai loại số khác nhau; trộn chúng làm đối soát ra hai kết quả cho cùng một đơn.

**Ba con số phụ thuộc nhau theo đúng thứ tự trên**, nên chỉ có MỘT hàm đặt cả phí lẫn thuế (`Checkout.ApDungPhiVaThue`), và mọi đường ghi tiền — chọn cách giao, áp mã, gỡ mã — đều gọi lại nó. Để mỗi đường tự cập nhật một mảnh là cách chắc chắn để ba con số lệch nhau, và lệch ở đây nghĩa là khách bị thu sai tiền.

**Đổi thuế suất KHÔNG sửa lại đơn cũ.** Đơn đã đặt giữ số thuế tại thời điểm đặt — tiền trên đơn là hợp đồng đã đóng băng.

### Còn thiếu

Thuế **một tầng** nghĩa là chưa có: thuế suất theo loại hàng, miễn thuế theo nhóm khách, và câu hỏi marketplace "nền tảng hay nhà bán xuất hóa đơn". Câu cuối quyết định thuế thuộc `order` hay `seller`; hôm nay nó nằm ở `order` vì một tầng thì hai cách cho cùng kết quả.

---

## 8. Dữ liệu sở hữu

```sql
checkout
checkout_line
checkout_session        -- thông tin phiên: địa chỉ, phương thức, mã giảm giá
```

```sql
CREATE TABLE checkout (
    id                UUID PRIMARY KEY,
    cart_id           UUID NOT NULL,
    customer_id       UUID,
    guest_email       TEXT,
    currency          CHAR(3) NOT NULL,
    subtotal          BIGINT NOT NULL,
    shipping_fee      BIGINT NOT NULL DEFAULT 0,
    discount_amount   BIGINT NOT NULL DEFAULT 0,
    tax_amount        BIGINT NOT NULL DEFAULT 0,
    total_amount      BIGINT NOT NULL,
    status            TEXT NOT NULL,
    expires_at        TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_checkout_expiring ON checkout (expires_at)
    WHERE status IN ('STARTED','PENDING_PAYMENT');
```

`checkout_line` lưu **giá đã đóng băng**, khác với `cart_item` lưu tham chiếu tới offer.

---

## 9. Interface công khai

```go
type PublicAPI interface {
    StartCheckout(ctx, req StartCheckoutRequest) (*CheckoutView, error)
    GetCheckout(ctx, checkoutID string) (*CheckoutView, error)

    SetShippingAddress(ctx, checkoutID string, addr Address) (*CheckoutView, error)
    SetShippingMethod(ctx, checkoutID string, methodID string) (*CheckoutView, error)
    ApplyCoupon(ctx, checkoutID string, code string) (*CheckoutView, error)
    RemoveCoupon(ctx, checkoutID string) (*CheckoutView, error)

    ExtendCheckout(ctx, checkoutID string) (*CheckoutView, error)
    CancelCheckout(ctx, checkoutID string) error

    // Hoàn tất — tạo đơn hàng
    CompleteCheckout(ctx, req CompleteRequest) (*OrderResult, error)
}
```

---

## 10. Use case chính: `CompleteCheckout`

```text
Đầu vào: checkout_id, payment_method, idempotency_key

 1. Kiểm tra idempotency_key → nếu đã xử lý, trả kết quả cũ

 2. Kiểm tra checkout còn hiệu lực
    → hết hạn: trả lỗi, yêu cầu làm lại

 3. Xác minh reservation còn hiệu lực

 4. Gọi payment.CreatePaymentIntent()

 5. Chờ kết quả thanh toán
    ├── Thành công → gọi order.PlaceOrder()
    │                 → inventory tự chuyển Reserved → Committed (qua event)
    │                 → checkout chuyển COMPLETED
    │
    └── Thất bại   → giữ nguyên checkout (cho khách thử lại)
                     → nếu hết TTL: giải phóng tồn kho
```

**Điểm quan trọng:** nếu thanh toán thất bại, **không** hủy checkout ngay. Cho khách thử lại phương thức khác trong thời gian TTL còn lại. Hủy ngay là trải nghiệm tệ và làm mất đơn hàng.

---

## 11. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `checkout.started` | Bắt đầu, **đã giữ hàng xong** — event chỉ thông báo, không kích hoạt việc giữ |
| `checkout.expired` | Hết hạn |
| `checkout.cancelled` | Khách hủy |
| `checkout.completed` | Tạo đơn thành công |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `payment.captured` | payment | Hoàn tất checkout |
| `payment.failed` | payment | Ghi nhận, cho phép thử lại |

---

## 12. Phụ thuộc

```text
Gọi đồng bộ:   cart        (lấy nội dung giỏ)
               inventory   (giữ hàng)
               pricing     (giá)
               promotion   (khuyến mãi)
               marketplace (thông tin offer)
               fulfillment (ước tính phí ship)
               payment     (tạo ý định thanh toán)
               order       (tạo đơn)
Nghe event:    payment
```

Checkout là module **điều phối** — gọi nhiều module nhất. Đây là đặc điểm bình thường của tầng điều phối.

---

## 13. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Bắt buộc giữ tồn kho trước khi cho checkout |
| 2 | Giá đóng băng tại thời điểm bắt đầu |
| 3 | Có thời hạn, tự động giải phóng khi hết |
| 4 | Thanh toán thất bại không hủy checkout ngay |
| 5 | Hoàn tất phải idempotent |
| 6 | Khách vãng lai được checkout |
| 7 | Hiển thị thời gian giao riêng theo từng nguồn hàng |

---

## 14. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Checkout cơ bản, giữ hàng, đóng băng giá, một phương thức thanh toán |
| **Phase 2** | Nhiều phương thức, gia hạn, tính phí ship theo nhiều nguồn |
| **Phase 3** | Checkout nhanh (một chạm), lưu phương thức thanh toán |

---

## 15. Tài liệu liên quan

- [cart.md](cart.md) — bước trước
- [order.md](order.md) — bước sau
- [inventory.md](inventory.md) — cơ chế giữ hàng
- [../07-workflows/customer-purchase.md](../07-workflows/customer-purchase.md)
