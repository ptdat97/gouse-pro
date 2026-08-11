# Phân nhóm API

## 1. Sáu nhóm API

```text
┌──────────────────────────────────────────────────────────────┐
│ /api/v1/...              STOREFRONT                          │
│ Đối tượng: khách hàng (đăng nhập và vãng lai)                │
│ Phân quyền: dữ liệu của chính mình                           │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ /api/v1/seller/...       SELLER CENTER                       │
│ Đối tượng: nhà bán                                           │
│ Phân quyền: CHỈ dữ liệu của gian hàng mình                   │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ /api/v1/creator/...      CREATOR CENTER                      │
│ Đối tượng: creator                                           │
│ Phân quyền: nội dung và thu nhập của mình; KHÔNG có dữ liệu  │
│             cá nhân khách hàng                               │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ /api/v1/admin/...        ADMIN                               │
│ Đối tượng: nhân viên nền tảng                                │
│ Phân quyền: theo vai trò; mọi truy cập dữ liệu cá nhân       │
│             đều được ghi audit                               │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ /api/v1/partner/...      PARTNER (Phase 4)                   │
│ Đối tượng: đối tác tích hợp                                  │
│ Phân quyền: theo hợp đồng và phạm vi cấp                     │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ /api/v1/webhooks/...     WEBHOOK IN                          │
│ Đối tượng: dịch vụ bên ngoài gọi vào                         │
│ Phân quyền: xác minh chữ ký                                  │
└──────────────────────────────────────────────────────────────┘
```

---

## 2. Vì sao tách theo đối tượng, không theo tài nguyên

Cùng một khái niệm nghiệp vụ nhưng **dữ liệu trả về khác nhau** theo người gọi:

### Ví dụ: một đơn hàng có hàng của ba seller

```text
GET /api/v1/orders/{id}                      (khách hàng)
    → toàn bộ đơn: 3 nhóm hàng, tổng tiền, địa chỉ giao
    → thấy tên cả ba seller

GET /api/v1/seller/fulfillment-orders/{id}   (Seller A)
    → CHỈ phần hàng của Seller A
    → thấy địa chỉ giao (cần để gửi hàng)
    → KHÔNG thấy hàng của Seller B, C
    → KHÔNG thấy tổng tiền đơn

GET /api/v1/admin/orders/{id}                (nhân viên)
    → toàn bộ, kèm thông tin nội bộ:
      bút toán, lịch sử trạng thái, ghi chú vận hành
```

Nếu dùng chung một endpoint và lọc theo vai trò, rủi ro rò rỉ dữ liệu rất cao — chỉ cần quên một điều kiện lọc là seller thấy dữ liệu của đối thủ.

---

## 3. Bảng phân quyền theo nhóm

| Tài nguyên | Storefront | Seller | Creator | Admin |
|---|---|---|---|---|
| Sản phẩm (đọc) | Công khai | Của mình + công khai | Công khai | Tất cả |
| Sản phẩm (ghi) | — | Của mình | — | Tất cả |
| Offer (ghi) | — | Của mình | — | Tất cả |
| Tồn kho | Chỉ trạng thái còn/hết | Của mình | — | Tất cả |
| Giỏ hàng | Của mình | — | — | Đọc (hỗ trợ) |
| Đơn hàng | Của mình | **Chỉ FO của mình** | — | Tất cả |
| Khách hàng | Của mình | **Chỉ thông tin giao hàng trên đơn của mình** | **Không** | Tất cả (có audit) |
| Tài chính | Của mình | Số dư của mình | Thu nhập của mình | Tất cả |
| Nội dung | Công khai | — | Của mình | Tất cả |
| Quy kết | — | — | Số liệu tổng hợp | Tất cả |
| Chuỗi cung ứng | — | — | — | Theo vai trò |

Hai ô in đậm là ranh giới bảo mật quan trọng nhất:

```text
Seller KHÔNG thấy dữ liệu seller khác
    → vi phạm = mất niềm tin toàn bộ nhà bán

Creator KHÔNG thấy danh tính khách hàng
    → creator không phải bên xử lý dữ liệu cá nhân
```

---

## 4. Mô hình phân quyền

```text
Tầng 1 — Xác thực (platform/auth)
    "Token hợp lệ, user_id=123, role=SELLER_OWNER, seller_id=sel_01J9X"

Tầng 2 — Kiểm tra vai trò (middleware)
    "Endpoint /api/v1/seller/* yêu cầu vai trò SELLER_*"

Tầng 3 — Phạm vi dữ liệu (module sở hữu dữ liệu)  ← QUAN TRỌNG NHẤT
    "Truy vấn thêm điều kiện WHERE seller_id = $ctx.seller_id"
```

**Tầng 3 là nơi bảo mật thật sự.** Tầng 1 và 2 chỉ chặn người không có quyền vào khu vực; tầng 3 chặn người có quyền xem dữ liệu của người khác.

### Ví dụ cài đặt

```go
// SAI — lọc ở tầng hiển thị
orders := repo.FindAll(ctx)
return filterBySeller(orders, sellerID)   // dữ liệu đã rời database

// ĐÚNG — lọc trong truy vấn
orders := repo.FindBySeller(ctx, sellerID)
```

Cách sai vừa chậm vừa nguy hiểm — chỉ cần quên gọi hàm lọc một lần là rò rỉ.

---

## 5. Danh sách endpoint chính

### 5.1 Storefront

```text
Duyệt và tìm kiếm
GET    /api/v1/products
GET    /api/v1/products/{id}
GET    /api/v1/products/{id}/offers
GET    /api/v1/products/{id}/reviews
GET    /api/v1/categories
GET    /api/v1/brands/{id}
GET    /api/v1/collections/{id}
GET    /api/v1/sellers/{id}
GET    /api/v1/search

Nội dung
GET    /api/v1/content
GET    /api/v1/content/{id}
GET    /api/v1/outfits/{id}
GET    /api/v1/creators/{id}
GET    /api/v1/feed

Giỏ hàng
GET    /api/v1/cart
POST   /api/v1/cart/items
PATCH  /api/v1/cart/items/{id}
DELETE /api/v1/cart/items/{id}

Thanh toán
POST   /api/v1/checkout
GET    /api/v1/checkout/{id}
PATCH  /api/v1/checkout/{id}/shipping-address
PATCH  /api/v1/checkout/{id}/shipping-method
POST   /api/v1/checkout/{id}/coupon
POST   /api/v1/checkout/{id}/complete

Đơn hàng
POST   /api/v1/orders
GET    /api/v1/orders
GET    /api/v1/orders/{id}
POST   /api/v1/orders/{id}/cancel
POST   /api/v1/orders/{id}/returns

Tài khoản
GET    /api/v1/me
PATCH  /api/v1/me
GET    /api/v1/me/addresses
POST   /api/v1/me/addresses
GET    /api/v1/me/wishlist
POST   /api/v1/me/wishlist/items
GET    /api/v1/me/loyalty
```

### 5.2 Seller Center

```text
POST   /api/v1/sellers                          — đăng ký làm seller
GET    /api/v1/seller/profile
GET    /api/v1/seller/dashboard

GET    /api/v1/seller/products
POST   /api/v1/seller/products
PATCH  /api/v1/seller/products/{id}

GET    /api/v1/seller/offers
POST   /api/v1/seller/offers
PATCH  /api/v1/seller/offers/{id}

GET    /api/v1/seller/inventory
PATCH  /api/v1/seller/inventory/{sku_id}

GET    /api/v1/seller/fulfillment-orders
GET    /api/v1/seller/fulfillment-orders/{id}
POST   /api/v1/seller/fulfillment-orders/{id}/confirm
POST   /api/v1/seller/fulfillment-orders/{id}/pack
POST   /api/v1/seller/fulfillment-orders/{id}/ship

GET    /api/v1/seller/returns
POST   /api/v1/seller/returns/{id}/approve

GET    /api/v1/seller/balance
GET    /api/v1/seller/settlements
GET    /api/v1/seller/settlements/{id}

GET    /api/v1/seller/performance
GET    /api/v1/seller/analytics
```

### 5.3 Creator Center

```text
POST   /api/v1/creators                         — đăng ký làm creator
GET    /api/v1/creator/profile
PATCH  /api/v1/creator/profile

GET    /api/v1/creator/content
POST   /api/v1/creator/content
PATCH  /api/v1/creator/content/{id}
POST   /api/v1/creator/content/{id}/publish

POST   /api/v1/creator/outfits
POST   /api/v1/creator/content/{id}/product-tags

POST   /api/v1/creator/affiliate-links
GET    /api/v1/creator/affiliate-links

GET    /api/v1/creator/campaigns
POST   /api/v1/creator/campaigns/{id}/join

GET    /api/v1/creator/earnings
GET    /api/v1/creator/settlements
GET    /api/v1/creator/analytics
```

### 5.4 Admin

```text
GET    /api/v1/admin/customers
GET    /api/v1/admin/customers/{id}             — ghi audit

GET    /api/v1/admin/sellers
POST   /api/v1/admin/sellers/{id}/approve
POST   /api/v1/admin/sellers/{id}/suspend

GET    /api/v1/admin/creators
POST   /api/v1/admin/creators/{id}/approve

GET    /api/v1/admin/products
POST   /api/v1/admin/products/{id}/approve
POST   /api/v1/admin/products/merge

GET    /api/v1/admin/orders
GET    /api/v1/admin/orders/{id}
POST   /api/v1/admin/orders/{id}/cancel

GET    /api/v1/admin/content
POST   /api/v1/admin/content/{id}/take-down

GET    /api/v1/admin/ledger
POST   /api/v1/admin/ledger/adjustments        — bắt buộc có lý do
GET    /api/v1/admin/settlements
POST   /api/v1/admin/payouts

GET    /api/v1/admin/inventory
POST   /api/v1/admin/inventory/adjustments     — bắt buộc có lý do

GET    /api/v1/admin/suppliers
POST   /api/v1/admin/production-orders
GET    /api/v1/admin/production-orders/{id}
POST   /api/v1/admin/quality-inspections
GET    /api/v1/admin/replenishment-suggestions

GET    /api/v1/admin/analytics
GET    /api/v1/admin/audit-log
```

---

## 6. Endpoint tổng hợp (BFF)

```text
GET /api/v1/storefront/home
GET /api/v1/storefront/product-page/{id}
GET /api/v1/storefront/checkout-page/{id}
```

Chỉ tạo khi có vấn đề hiệu năng **đo được**. Chỉ tổng hợp, không chứa logic nghiệp vụ mới.

---

## 7. Nguyên tắc khi thêm endpoint mới

```text
Câu hỏi trước khi thêm:

1. Đây là khả năng nghiệp vụ hay là một màn hình cụ thể?
   → màn hình cụ thể: xem lại, có thể dùng API có sẵn

2. Người gọi được xem những bản ghi nào?
   → phải trả lời được TRƯỚC khi viết code

3. Nó thuộc nhóm nào?
   → nếu thuộc hai nhóm, thường là hai endpoint khác nhau

4. Có làm thay đổi trạng thái không?
   → cần Idempotency-Key

5. Có trả về dữ liệu cá nhân không?
   → cần ghi audit nếu người gọi không phải chủ sở hữu
```

---

## 8. Tài liệu liên quan

- [api-guidelines.md](api-guidelines.md) — chuẩn kỹ thuật
- [customer-api.md](customer-api.md), [seller-api.md](seller-api.md), [creator-api.md](creator-api.md), [admin-api.md](admin-api.md)
- [../09-operations/security.md](../09-operations/security.md)
