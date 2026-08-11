# Module: Loyalty

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | Supporting |
| **Giai đoạn** | Phase 3 |

---

## 1. Trách nhiệm

- Quản lý tài khoản điểm thưởng
- Tích và tiêu điểm
- Quản lý hạng thành viên
- Quyền lợi theo hạng

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Hồ sơ khách hàng | `customer` |
| Mã giảm giá | `promotion` |
| Ghi sổ giá trị điểm | `payment` (nếu điểm quy đổi thành tiền) |

---

## 3. Mô hình điểm

```text
LoyaltyAccount {
    customer_id
    tier                 -- hạng hiện tại
    points_balance       -- TÍNH TỪ point_transaction, không lưu riêng
    tier_progress        -- tiến độ lên hạng
    tier_valid_until     -- hạng có thời hạn
}

PointTransaction {
    id
    account_id
    type          EARN | REDEEM | EXPIRE | ADJUST | REVERSE
    points        -- dương hoặc âm
    reference_type, reference_id
    expires_at    -- điểm tích có hạn dùng
    created_at    ← BẤT BIẾN
}
```

**Nguyên tắc giống ledger:** số dư điểm là **kết quả tính** từ các giao dịch điểm, không phải trường được cập nhật. Lý do tương tự [payment.md](payment.md) mục 5 — tránh lệch và tránh điểm nghẽn ghi.

---

## 4. Vòng đời điểm

```text
Đơn hàng COMPLETED (không phải PLACED)
    ↓
Tích điểm
    ↓
    ├── Dùng để giảm giá đơn sau
    ├── Đổi quyền lợi (miễn phí ship, ưu tiên)
    └── Hết hạn sau N tháng không dùng
```

**Vì sao tích điểm ở `COMPLETED` chứ không phải `PLACED`:**

```text
Nếu tích ngay khi đặt hàng:
    → khách hoàn hàng, đã tiêu điểm rồi
    → phải thu hồi điểm âm, phức tạp và gây khó chịu

Nếu tích khi hoàn tất (hết hạn đổi trả):
    → an toàn, tránh phần lớn trường hợp phải thu hồi
```

---

## 5. Hạng thành viên

```text
Member → Silver → Gold → VIP

Điều kiện lên hạng: chi tiêu tích lũy trong 12 tháng gần nhất
Hạng có thời hạn: xét lại định kỳ
```

**Quyền lợi theo hạng** (ví dụ):

| Hạng | Quyền lợi |
|---|---|
| Member | Tích điểm |
| Silver | + Miễn phí ship từ ngưỡng thấp hơn |
| Gold | + Ưu đãi sinh nhật, đổi trả kéo dài |
| VIP | + Sớm tiếp cận bộ sưu tập, ưu tiên hỗ trợ |

Quyền lợi "sớm tiếp cận bộ sưu tập" đặc biệt phù hợp với thời trang — tạo cảm giác đặc quyền mà không tốn chi phí trực tiếp.

---

## 6. Xử lý hoàn hàng

```text
Đơn bị hoàn sau khi đã tích điểm
    ↓
Ghi PointTransaction loại REVERSE (điểm âm)
    ↓
    ├── Số dư còn đủ → trừ bình thường
    └── Đã tiêu hết  → số dư âm, trừ vào lần tích sau
```

**Không** xóa giao dịch điểm cũ — ghi giao dịch đảo ngược, giống nguyên tắc ledger.

---

## 7. Dữ liệu sở hữu

```sql
loyalty_account
point_transaction       -- BẤT BIẾN
tier_rule
tier_benefit
tier_history            -- lịch sử thay đổi hạng
```

---

## 8. Interface công khai

```go
type PublicAPI interface {
    GetAccount(ctx, customerID string) (*LoyaltyAccount, error)
    GetPointBalance(ctx, customerID string) (int, error)
    GetTier(ctx, customerID string) (*TierView, error)
    GetTierBenefits(ctx, tier string) ([]Benefit, error)

    EarnPoints(ctx, req EarnRequest) error      // idempotent
    RedeemPoints(ctx, req RedeemRequest) error  // idempotent
    ReversePoints(ctx, req ReverseRequest) error
}
```

---

## 9. Event

**Phát ra:** `loyalty.points_earned`, `loyalty.points_redeemed`, `loyalty.points_expired`, `loyalty.tier_changed`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `order.completed` | order | Tích điểm |
| `return.refunded` | return | Đảo ngược điểm |
| `customer.registered` | customer | Tạo tài khoản điểm |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Số dư điểm là kết quả tính từ giao dịch |
| 2 | Giao dịch điểm bất biến, sửa bằng giao dịch mới |
| 3 | Tích điểm khi đơn COMPLETED, không phải PLACED |
| 4 | Tích/tiêu điểm phải idempotent |
| 5 | Điểm có hạn sử dụng |
| 6 | Hạng có thời hạn, xét lại định kỳ |

---

## 11. Tài liệu liên quan

- [customer.md](customer.md), [promotion.md](promotion.md)
- [payment.md](payment.md) — mô hình bất biến tương tự
