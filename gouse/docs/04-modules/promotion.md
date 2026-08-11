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

## 12. Tài liệu liên quan

- [pricing.md](pricing.md) — giá gốc
- [campaign.md](campaign.md) — chiến dịch creator
- [../01-business/monetization.md](../01-business/monetization.md)
