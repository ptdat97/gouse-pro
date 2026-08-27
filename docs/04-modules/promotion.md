# Module: Promotion

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | Supporting |
| **Giai đoạn** | MVP (cơ bản) |

---

## 1. Trách nhiệm

- Quản lý chương trình khuyến mãi và mã giảm giá
- Kiểm tra điều kiện áp dụng
- Tính số tiền giảm
- Theo dõi số lần sử dụng và ngân sách
- Xác định **bên nào chịu chi phí khuyến mãi**

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Giá gốc | `pricing`, `marketplace` |
| Chiến dịch creator | `campaign` |
| Điểm thưởng | `loyalty` |
| Ghi sổ chi phí khuyến mãi | `payment` |

---

## 3. Ai chịu chi phí khuyến mãi — vấn đề cốt lõi

Đây là điểm phức tạp nhất của khuyến mãi trên marketplace.

```text
Khách dùng mã giảm 50.000đ cho đơn hàng của Seller A.
Ai chịu 50.000đ này?
```

**Ba trường hợp:**

```text
PLATFORM  — nền tảng chịu
            → dùng cho chiến dịch thu hút khách của nền tảng
            → trừ vào chi phí marketing

SELLER    — seller chịu
            → seller tự tạo khuyến mãi cho gian hàng mình
            → trừ vào số tiền seller nhận

SHARED    — chia theo tỷ lệ thỏa thuận
            → dùng cho chiến dịch lớn cần seller cùng tham gia
```

**Hệ quả kiến trúc:**

```text
Promotion {
    ...
    cost_bearer      PLATFORM | SELLER | SHARED
    platform_share   -- nếu SHARED
    seller_share     -- nếu SHARED
}
```

Thông tin này phải được **đóng băng vào OrderLine** để đối soát đúng. Nếu không, không tính được seller thực nhận bao nhiêu.

---

## 4. Các loại khuyến mãi

```text
Mã giảm giá (coupon)         — khách nhập mã
Giảm giá tự động             — tự áp khi đủ điều kiện
Miễn phí vận chuyển          — theo ngưỡng đơn hàng
Mua X tặng Y
Giảm theo combo/outfit       — đặc thù thời trang: mua cả bộ giảm giá
Giảm theo số lượng
```

**Khuyến mãi theo outfit** là loại đặc thù có giá trị cao: khuyến khích khách mua cả bộ thay vì một món, tăng giá trị đơn hàng và giảm chi phí vận chuyển trên mỗi món.

---

## 5. Điều kiện áp dụng

```text
Điều kiện có thể kết hợp:
    - Giá trị đơn tối thiểu
    - Danh mục/thương hiệu/bộ sưu tập cụ thể
    - Seller cụ thể
    - Hạng khách hàng
    - Khách hàng mới lần đầu
    - Khoảng thời gian
    - Số lần dùng tối đa (toàn cục và theo khách)
    - Ngân sách tối đa
```

### Quy tắc chồng lấn

```text
Nhiều khuyến mãi cùng đủ điều kiện → áp dụng cái nào?

Quy tắc:
    1. Khuyến mãi có cờ "không cộng dồn" → chỉ chọn một, ưu tiên lợi cho khách nhất
    2. Khuyến mãi cho phép cộng dồn → áp dụng theo thứ tự ưu tiên
    3. Luôn có giới hạn tổng mức giảm (ví dụ tối đa 50% giá trị đơn)
```

Giới hạn tổng là cơ chế bảo vệ bắt buộc — không có nó, lỗi cấu hình có thể dẫn tới bán hàng với giá gần bằng không.

---

## 6. Dữ liệu sở hữu

```sql
promotion
promotion_rule
promotion_condition
coupon
coupon_usage            -- ai dùng, khi nào, đơn nào
promotion_budget        -- theo dõi ngân sách đã tiêu
```

```sql
CREATE TABLE coupon_usage (
    id            UUID PRIMARY KEY,
    coupon_id     UUID NOT NULL,
    customer_id   UUID,
    order_id      UUID NOT NULL,
    discount_amount BIGINT NOT NULL,
    used_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (coupon_id, order_id)
);

CREATE INDEX idx_coupon_usage_customer ON coupon_usage (coupon_id, customer_id);
```

Ràng buộc duy nhất chặn việc áp cùng một mã hai lần cho một đơn.

---

## 7. Interface công khai

```go
type PublicAPI interface {
    ValidateCoupon(ctx, req ValidateCouponRequest) (*CouponValidation, error)
    CalculateDiscount(ctx, req DiscountRequest) (*DiscountResult, error)
    GetAutoPromotions(ctx, req AutoPromotionRequest) ([]PromotionView, error)

    // Ghi nhận sử dụng — phải idempotent
    RecordUsage(ctx, req RecordUsageRequest) error
    ReleaseUsage(ctx, orderID string) error   // khi hủy đơn
}

type DiscountResult struct {
    TotalDiscount   Money
    Breakdown       []DiscountLine   // giảm cho dòng nào, bao nhiêu
    CostAllocation  []CostAllocation // bên nào chịu bao nhiêu
}
```

`Breakdown` và `CostAllocation` quan trọng: cần biết giảm giá phân bổ vào dòng hàng nào để tính đúng hoa hồng và số tiền hoàn khi trả hàng từng phần.

---

## 8. Phân bổ giảm giá xuống dòng hàng

Vấn đề: mã giảm 50.000đ cho cả đơn, nhưng khách trả lại một món. Hoàn bao nhiêu?

```text
Đơn: 3 món, tổng 500.000đ, giảm 50.000đ (10%)
    Món A: 200.000đ → giảm 20.000đ → thực trả 180.000đ
    Món B: 200.000đ → giảm 20.000đ → thực trả 180.000đ
    Món C: 100.000đ → giảm 10.000đ → thực trả  90.000đ

Khách trả món C → hoàn 90.000đ (không phải 100.000đ)
```

**Quy tắc:** giảm giá phải được **phân bổ theo tỷ lệ** xuống từng dòng hàng và lưu lại. Dùng phép chia `Money.Allocate()` để không mất đồng nào.

Xem [../02-domain/value-objects.md](../02-domain/value-objects.md) mục 2.4.

---

## 9. Event

**Phát ra:** `promotion.applied`, `promotion.budget_exhausted`, `coupon.used`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `order.placed` | order | Ghi nhận sử dụng mã |
| `order.cancelled` | order | Giải phóng lượt sử dụng |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Luôn xác định rõ bên chịu chi phí |
| 2 | Có giới hạn tổng mức giảm |
| 3 | Giảm giá phân bổ theo tỷ lệ xuống từng dòng hàng |
| 4 | Ghi nhận sử dụng phải idempotent |
| 5 | Hủy đơn phải giải phóng lượt sử dụng |
| 6 | Theo dõi ngân sách, tự dừng khi hết |
| 7 | Không cho phép giảm giá âm hoặc vượt giá trị đơn |

---

## 11. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Mã giảm giá cơ bản, miễn phí ship theo ngưỡng |
| **Phase 2** | Khuyến mãi tự động, phân bổ chi phí, khuyến mãi của seller |
| **Phase 3** | Combo/outfit, mua X tặng Y, ngân sách chi tiết |
| **Phase 4** | Khuyến mãi cá nhân hóa |

---

## 12. Trạng thái triển khai (MVP — 14/08/2026)

Mã nguồn: `internal/modules/promotion/`. Migration: `000018_promotion`.
Kiểm chứng: 26 test tích hợp trên PostgreSQL thật, đã kiểm chứng ngược.

**Đã có (đúng phạm vi MVP ở mục 11):** mã giảm giá theo phần trăm và số
tiền cố định · miễn phí ship theo ngưỡng · trần giảm giá · giới hạn lượt
toàn cục và theo khách · ngân sách · phân bổ xuống dòng hàng · phân bổ chi
phí ba kiểu (PLATFORM/SELLER/SHARED) · ghi nhận và giải phóng idempotent.

**Chưa có (Phase 2 trở đi):** khuyến mãi tự động · combo/outfit · mua X
tặng Y · quy tắc chồng lấn · phát event.

### Ba chỗ code KHÁC tài liệu — và vì sao code đúng

**1. Chưa có `promotion_rule`, `promotion_condition`, `promotion_budget`**

Mục 6 liệt kê sáu bảng; code có ba (`promotion`, `coupon`,
`coupon_usage`).

Điều kiện áp dụng ở MVP chỉ có MỘT loại — ngưỡng giá trị đơn — nên nó là
một cột chứ không phải một bảng. Bảng điều kiện có nghĩa khi điều kiện
KẾT HỢP được (danh mục AND thương hiệu OR hạng khách), và đó là Phase 2.

Ngân sách cũng vậy: hai cột `max_budget`/`used_budget` đủ cho một ngân
sách toàn cục. Bảng riêng cần khi ngân sách chia theo ngày hoặc theo
kênh — Phase 3, đúng như mục 11 xếp.

**2. `CalculateDiscount` gộp vào `ValidateCoupon`**

Mục 7 tách hai hàm. Code gộp: tính giảm giá mà chưa kiểm tra điều kiện là
tính một con số KHÔNG ĐƯỢC PHÉP dùng, và tách ra tạo đúng một chỗ để ai đó
gọi nhầm thứ tự.

`GetAutoPromotions` chưa có vì khuyến mãi tự động thuộc Phase 2.

**3. Chưa phát event nào**

Mục 9 liệt kê ba event. Chưa event nào được phát vì chưa có bên nghe.
Chiều ngược lại (nghe `order.placed`, `order.cancelled`) cũng chưa nối —
`RecordUsage` và `ReleaseUsage` đã sẵn sàng và ĐÃ idempotent, chỉ còn
thiếu handler.

### Phát hiện từ kiểm chứng ngược: một lỗi thật, tìm ra trước khi chạy

Test tranh chấp phát hiện bộ đếm chỉ tăng **4 lần** trong khi bảng lượt
sử dụng có **12 hàng**.

```text
Nguyên nhân: RecordUsage ghi lượt vào bảng THÀNH CÔNG, rồi cập nhật bộ
             đếm bằng khóa lạc quan — và trả LỖI khi xung đột.

Hậu quả:     mã giới hạn 100 lượt được dùng vài trăm lần, vì bộ đếm mãi
             không chạm tới giới hạn.
```

Đã sửa bằng `retryUpdate`: đọc lại rồi cộng thêm, tối đa 10 lần. Thử lại
được vì thao tác là ĐỌC-RỒI-CỘNG chứ không phải ghi đè giá trị tính sẵn.
`ReleaseUsage` có đúng lỗi đó và được sửa cùng.

Đây là loại lỗi chỉ lộ ra dưới tải: một mã đang chạy quảng cáo có hàng
trăm người áp trong một giây, còn test tuần tự thì không bao giờ thấy.

### Giới hạn đã biết, có chủ ý

| Giới hạn | Vì sao chấp nhận ở MVP |
|---|---|
| Chưa có quy tắc chồng lấn (mục 5) | MVP áp MỘT mã mỗi đơn. Giới hạn tổng mức giảm — cơ chế bảo vệ bắt buộc — hiện là `MaxDiscountAmount` của từng khuyến mãi. |
| `RecordUsage` không nằm trong MỘT giao dịch với việc đặt đơn | Ghi lượt xong mà đặt đơn hỏng sẽ để lại một lượt bị chiếm. `ReleaseUsage` gỡ được, nhưng cần handler nghe `order.cancelled`. |
| Bộ đếm trên `coupon` không có khóa lạc quan | Nó CHỈ để hiển thị ("mã đã dùng 47 lần"). Giới hạn thật nằm ở `promotion`, và nguồn sự thật là bảng `coupon_usage`. |
| Khách vãng lai không áp được `MaxUsesPerCustomer` | Không có `customer_id` thì không có gì để đếm. Cần rate limit theo thiết bị ở tầng interfaces. |

---

## 13. Tài liệu liên quan

- [pricing.md](pricing.md) — giá gốc
- [campaign.md](campaign.md) — chiến dịch creator
- [../01-business/monetization.md](../01-business/monetization.md)
