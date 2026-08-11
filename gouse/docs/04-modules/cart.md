# Module: Cart

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | Supporting |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý giỏ hàng của khách (đã đăng nhập và vãng lai)
- Thêm, sửa, xóa món hàng
- Hiển thị giá và tổng tiền hiện tại
- Ghi nhận nguồn giới thiệu (nội dung/creator) cho việc quy kết
- Gộp giỏ khi khách đăng nhập

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| **Giữ tồn kho** | `checkout` (qua `inventory`) |
| Đóng băng giá | `checkout` |
| Tạo đơn hàng | `order` |
| Tính giá gốc | `pricing` |
| Áp dụng khuyến mãi | `promotion` |

---

## 3. Phân biệt Cart và Checkout

Đây là ranh giới quan trọng nhất của module.

| | Cart | Checkout |
|---|---|---|
| Thời gian sống | Nhiều ngày, nhiều phiên | Vài phút |
| **Giữ tồn kho** | **KHÔNG** | **CÓ** |
| Giá | Cập nhật động theo giá hiện tại | **Đóng băng** |
| Thay đổi | Tự do | Hạn chế |
| Hết hạn | Dài (ví dụ 30 ngày) | Ngắn (ví dụ 15 phút) |

### Vì sao giỏ hàng KHÔNG giữ tồn kho

```text
Nếu giỏ hàng giữ tồn kho:
    - Khách thêm vào giỏ rồi bỏ quên 2 tuần → hàng bị khóa 2 tuần
    - Với hàng khan hiếm, vài trăm giỏ hàng bỏ quên = hết hàng ảo
    - Không bán được cho khách thật sự muốn mua

Nếu chỉ checkout giữ:
    - Chỉ khách đang thực sự thanh toán mới khóa hàng
    - Thời gian khóa ngắn, có hết hạn tự động
```

**Hệ quả cần chấp nhận:** khách có thể thêm vào giỏ rồi khi checkout mới biết hết hàng. Đây là đánh đổi đúng — hiển thị "còn hàng" ở giỏ là **thông tin tham khảo**, không phải cam kết.

---

## 4. Giỏ hàng nhiều nhà bán

```text
Cart
├── CartItem (own brand)
├── CartItem (Seller A)
├── CartItem (Seller B)
└── CartItem (Seller A)   ← cùng seller, món khác
```

Giỏ hàng **không** chia theo seller. Việc chia diễn ra ở bước tạo `FulfillmentOrder`.

Nhưng khi **hiển thị**, nên nhóm theo seller để khách hiểu hàng đến từ đâu và thời gian giao khác nhau.

---

## 5. Ghi nhận nguồn giới thiệu

```text
CartItem {
    ...
    source_content_id   (nullable)   ← nội dung nào dẫn tới việc thêm giỏ
    source_creator_id   (nullable)   ← creator nào
    added_at
}
```

Đây là mắt xích của bánh đà creator commerce. Ghi nhận ngay ở thời điểm thêm giỏ (không chỉ ở lúc mua) cho phép:

- Quy kết chính xác hơn khi khách mua sau vài ngày
- Đo được tỷ lệ "thêm giỏ" của từng nội dung, không chỉ tỷ lệ mua
- Phân tích nội dung nào tạo ý định mua nhưng không chốt được

---

## 6. Gộp giỏ khi đăng nhập

```text
Khách vãng lai có giỏ (session_id = X)
    ↓
Đăng nhập, tài khoản đã có giỏ cũ
    ↓
Gộp:
    - Món trùng offer → cộng số lượng (giới hạn theo max_order_quantity)
    - Món không trùng → thêm vào
    - Giữ source_content_id của lần thêm gần nhất
    - Xóa giỏ theo session
```

**Lưu ý:** phải xử lý trường hợp món trong giỏ cũ đã ngừng bán hoặc đổi giá. Không được im lặng bỏ qua — cần thông báo cho khách.

---

## 7. Xử lý món hàng không còn hợp lệ

Giỏ hàng sống lâu nên món trong giỏ có thể thay đổi trạng thái:

```text
Khi khách xem giỏ, kiểm tra từng món:

    Offer đã bị xóa/archive     → đánh dấu UNAVAILABLE, đề xuất thay thế
    Offer hết hàng              → đánh dấu OUT_OF_STOCK, cho phép nhận thông báo
    Giá đã thay đổi             → hiển thị giá mới, thông báo rõ
    Seller bị đình chỉ          → đánh dấu UNAVAILABLE
    Số lượng vượt tồn kho       → giảm về mức khả dụng, thông báo
```

**Nguyên tắc:** không tự động xóa món khỏi giỏ. Đánh dấu và để khách quyết định — xóa im lặng làm khách bối rối.

---

## 8. Dữ liệu sở hữu

```sql
cart
cart_item
```

```sql
CREATE TABLE cart (
    id           UUID PRIMARY KEY,
    customer_id  UUID,
    session_id   TEXT,
    currency     CHAR(3) NOT NULL,
    status       TEXT NOT NULL DEFAULT 'ACTIVE',
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT customer_or_session CHECK (
        customer_id IS NOT NULL OR session_id IS NOT NULL
    )
);

CREATE UNIQUE INDEX idx_cart_customer ON cart (customer_id) WHERE status = 'ACTIVE';
CREATE INDEX idx_cart_abandoned ON cart (updated_at) WHERE status = 'ACTIVE';
```

Chỉ mục cuối phục vụ việc tìm giỏ bị bỏ quên để gửi nhắc nhở.

---

## 9. Interface công khai

```go
type PublicAPI interface {
    GetCart(ctx, cartID string) (*CartView, error)
    GetOrCreateCart(ctx, req GetOrCreateRequest) (*CartView, error)

    AddItem(ctx, req AddItemRequest) (*CartView, error)
    UpdateItemQuantity(ctx, cartItemID string, quantity int) (*CartView, error)
    RemoveItem(ctx, cartItemID string) (*CartView, error)
    ClearCart(ctx, cartID string) error

    MergeCarts(ctx, sessionCartID, customerCartID string) (*CartView, error)
    MarkConverted(ctx, cartID string) error
}
```

---

## 10. Event

### Phát ra

| Event | Khi nào | Bên nghe |
|---|---|---|
| `cart.item_added` | Thêm món | analytics, **supply-chain (tín hiệu nhu cầu)** |
| `cart.item_removed` | Xóa món | analytics |
| `cart.abandoned` | Bỏ quên (sau N giờ không động) | notification, analytics |
| `cart.converted` | Đã thành đơn | analytics |

`cart.item_added` là **tín hiệu nhu cầu mạnh** — mạnh hơn lượt xem nhiều. Xem [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 4.

---

## 11. Phụ thuộc

```text
Gọi đồng bộ:   marketplace (thông tin offer, giá)
               product     (tên sản phẩm, ảnh)
               inventory   (kiểm tra còn hàng — chỉ để hiển thị)
               pricing     (giá hiện tại)
               promotion   (ước tính giảm giá)
Được gọi bởi:  checkout
```

**Lưu ý về hiệu năng:** hiển thị giỏ 10 món cần thông tin từ 5 module. Bắt buộc dùng truy vấn theo lô (`GetProductsByIDs`, `GetOffersBySKUs`), không gọi trong vòng lặp.

---

## 12. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Giỏ hàng KHÔNG giữ tồn kho |
| 2 | Giá trong giỏ là giá hiện tại, cập nhật động |
| 3 | Số lượng > 0 |
| 4 | Tôn trọng min/max_order_quantity của offer |
| 5 | Một khách chỉ có một giỏ ACTIVE |
| 6 | Không tự động xóa món không hợp lệ, chỉ đánh dấu |
| 7 | Ghi nhận nguồn giới thiệu khi thêm món |

---

## 13. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Giỏ cơ bản, nhiều seller, gộp giỏ khi đăng nhập |
| **Phase 2** | Ghi nhận nguồn creator, nhắc giỏ bỏ quên |
| **Phase 3** | Lưu nhiều giỏ, mua lại nhanh |

---

## 14. Tài liệu liên quan

- [checkout.md](checkout.md) — bước tiếp theo
- [inventory.md](inventory.md) — vì sao giỏ không giữ hàng
- [../07-workflows/customer-purchase.md](../07-workflows/customer-purchase.md)
