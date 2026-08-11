# Customer API (Storefront)

Base path: `/api/v1/`

---

## 1. Xác thực

```text
Khách vãng lai:  không cần token, dùng session cookie hoặc header X-Session-ID
Khách đăng nhập: Authorization: Bearer <token>
```

**Quan trọng:** khách vãng lai **được phép đặt hàng**. Đây là quyết định giảm rào cản chuyển đổi, đặc biệt với khách đến từ nội dung creator.

---

## 2. Duyệt sản phẩm

### `GET /api/v1/products`

```http
GET /api/v1/products?category_id=cat_01J9X&size=M,L&color=black&price_max=500000&in_stock=true&sort=price:asc&limit=20
```

```json
{
  "data": [
    {
      "id": "prd_01J9XABC",
      "name": "Áo sơ mi linen Oxford",
      "slug": "ao-so-mi-linen-oxford",
      "brand": { "id": "brd_01J9X", "name": "Thương hiệu A" },
      "primary_image_url": "https://.../image.jpg",
      "price_from": { "amount": 299000, "currency": "VND" },
      "compare_at_price": { "amount": 399000, "currency": "VND" },
      "rating": { "average": 4.6, "count": 128 },
      "available_colors": [
        { "name": "Trắng", "hex_code": "#FFFFFF" },
        { "name": "Đen", "hex_code": "#000000" }
      ],
      "available_sizes": ["S", "M", "L", "XL"],
      "offer_count": 3,
      "badges": ["FREE_SHIPPING", "NEW_ARRIVAL"]
    }
  ],
  "pagination": { "next_cursor": "eyJpZCI6...", "has_more": true }
}
```

**Lưu ý thiết kế:**

- `price_from` — giá thấp nhất trong các offer, vì có nhiều nhà bán.
- `available_sizes` — chỉ liệt kê size **còn hàng**, không liệt kê hết mọi size tồn tại. Hiển thị size đã hết là trải nghiệm tệ.
- `offer_count` — cho khách biết có nhiều lựa chọn nhà bán.

### `GET /api/v1/products/{id}`

```json
{
  "id": "prd_01J9XABC",
  "name": "Áo sơ mi linen Oxford",
  "description": "...",
  "brand": { "id": "brd_01J9X", "name": "Thương hiệu A", "logo_url": "..." },
  "collection": { "id": "col_01J9X", "name": "Thu Đông 2026" },
  "material_composition": [
    { "material": "Linen", "percentage": 80 },
    { "material": "Cotton", "percentage": 20 }
  ],
  "care_instructions": "Giặt máy ở 30°C, không dùng chất tẩy",
  "origin_country": "VN",
  "images": [ ... ],
  "variants": [
    {
      "id": "var_01J9X",
      "color": { "name": "Trắng", "hex_code": "#FFFFFF" },
      "images": [ ... ],
      "skus": [
        { "id": "sku_01J9X", "size": { "value": "M", "system": "ALPHA" }, "available": true },
        { "id": "sku_01J9Y", "size": { "value": "L", "system": "ALPHA" }, "available": false }
      ]
    }
  ],
  "size_chart": {
    "id": "szc_01J9X",
    "system": "ALPHA",
    "entries": [
      { "size": "M", "chest_cm": "96-100", "length_cm": 70, "shoulder_cm": 44 }
    ]
  },
  "size_recommendation": { "suggested_size": "L", "reason": "PREVIOUS_PURCHASE" },
  "buy_box_offer": {
    "id": "off_01J9X",
    "seller": { "id": "sel_01J9X", "name": "Cửa hàng ABC", "rating": 4.8 },
    "price": { "amount": 299000, "currency": "VND" },
    "estimated_delivery_days": 2,
    "return_policy_days": 7
  },
  "other_offers_count": 2
}
```

**Ba trường đặc thù thời trang:**

```text
material_composition  — ảnh hưởng quyết định mua, giảm hoàn hàng
size_chart            — số đo thực tế, giảm chọn sai size
size_recommendation   — gợi ý dựa trên lịch sử mua của khách
```

Trường `size_recommendation` chỉ có với khách đã đăng nhập và có lịch sử. Đây là cơ chế giảm trực tiếp tỷ lệ hoàn hàng.

### `GET /api/v1/products/{id}/offers`

```json
{
  "data": [
    {
      "id": "off_01J9X",
      "seller": { "id": "sel_01J9X", "name": "Cửa hàng ABC", "rating": 4.8, "response_time_hours": 4 },
      "price": { "amount": 299000, "currency": "VND" },
      "condition": "NEW",
      "estimated_delivery_days": 2,
      "shipping_fee": { "amount": 30000, "currency": "VND" },
      "return_policy_days": 7,
      "is_buy_box": true
    },
    {
      "id": "off_01J9Y",
      "seller": { "id": "sel_01J9Y", "name": "Shop XYZ", "rating": 4.3, "response_time_hours": 12 },
      "price": { "amount": 289000, "currency": "VND" },
      "estimated_delivery_days": 4,
      "shipping_fee": { "amount": 35000, "currency": "VND" },
      "is_buy_box": false
    }
  ]
}
```

Khách so sánh được **tổng chi phí** (giá + phí ship) và **chất lượng phục vụ**, không chỉ giá.

---

## 3. Giỏ hàng

### `POST /api/v1/cart/items`

```http
POST /api/v1/cart/items
Idempotency-Key: 01J9XABC123
```

```json
{
  "offer_id": "off_01J9X",
  "quantity": 2,
  "source": {
    "content_id": "cnt_01J9X",
    "creator_id": "cre_01J9X"
  }
}
```

Trường `source` là mắt xích quy kết cho creator — ghi nhận ngay ở thời điểm thêm giỏ, không chờ tới lúc mua.

**Response 200:**

```json
{
  "cart": {
    "id": "crt_01J9X",
    "groups": [
      {
        "seller": { "id": "sel_01J9X", "name": "Cửa hàng ABC" },
        "items": [
          {
            "id": "cit_01J9X",
            "offer_id": "off_01J9X",
            "product_name": "Áo sơ mi linen Oxford",
            "variant_description": "Trắng / M",
            "image_url": "...",
            "unit_price": { "amount": 299000, "currency": "VND" },
            "quantity": 2,
            "line_total": { "amount": 598000, "currency": "VND" },
            "availability": "IN_STOCK"
          }
        ],
        "estimated_delivery_days": 2
      }
    ],
    "subtotal": { "amount": 598000, "currency": "VND" },
    "estimated_shipping": { "amount": 30000, "currency": "VND" },
    "total": { "amount": 628000, "currency": "VND" }
  }
}
```

**Giỏ hàng nhóm theo seller** để khách hiểu hàng đến từ đâu và thời gian giao khác nhau.

**Response 422 (không đủ hàng):**

```json
{
  "error": {
    "code": "INSUFFICIENT_INVENTORY",
    "message": "Sản phẩm không đủ số lượng",
    "details": { "offer_id": "off_01J9X", "requested": 2, "available": 1 }
  },
  "request_id": "req_01J9X"
}
```

---

## 4. Thanh toán

### `POST /api/v1/checkout`

```json
{ "cart_id": "crt_01J9X" }
```

```json
{
  "id": "chk_01J9X",
  "expires_at": "2026-08-10T14:38:00Z",
  "lines": [ ... ],
  "subtotal": { "amount": 598000, "currency": "VND" },
  "shipping_fee": { "amount": 30000, "currency": "VND" },
  "discount_amount": { "amount": 0, "currency": "VND" },
  "total": { "amount": 628000, "currency": "VND" }
}
```

**Từ thời điểm này, giá được đóng băng.** Trường `expires_at` cho biết thời hạn giữ hàng.

### `POST /api/v1/checkout/{id}/complete`

```http
POST /api/v1/checkout/chk_01J9X/complete
Idempotency-Key: 01J9XDEF456
```

```json
{ "payment_method": "CARD", "payment_token": "tok_..." }
```

```json
{
  "order": {
    "id": "ord_01J9X",
    "order_number": "FC-2026-08-001234",
    "status": "PAID",
    "total": { "amount": 628000, "currency": "VND" },
    "placed_at": "2026-08-10T14:25:11Z"
  }
}
```

**Response 409 (checkout hết hạn):**

```json
{
  "error": {
    "code": "CHECKOUT_EXPIRED",
    "message": "Phiên thanh toán đã hết hạn, vui lòng thử lại"
  },
  "request_id": "req_01J9X"
}
```

---

## 5. Đơn hàng

### `GET /api/v1/orders/{id}`

```json
{
  "id": "ord_01J9X",
  "order_number": "FC-2026-08-001234",
  "status": "PARTIALLY_SHIPPED",
  "placed_at": "2026-08-10T14:25:11Z",
  "shipping_address": { ... },
  "shipments": [
    {
      "fulfillment_number": "FC-2026-08-001234-A",
      "seller": { "id": "sel_01J9X", "name": "Cửa hàng ABC" },
      "status": "DELIVERED",
      "tracking_number": "VN123456789",
      "delivered_at": "2026-08-12T10:15:00Z",
      "items": [ ... ],
      "return_deadline": "2026-08-19T10:15:00Z"
    },
    {
      "fulfillment_number": "FC-2026-08-001234-B",
      "seller": { "id": "sel_01J9Y", "name": "Shop XYZ" },
      "status": "IN_TRANSIT",
      "tracking_number": "VN987654321",
      "estimated_delivery_date": "2026-08-14",
      "items": [ ... ]
    }
  ],
  "subtotal": { "amount": 598000, "currency": "VND" },
  "shipping_fee": { "amount": 30000, "currency": "VND" },
  "total": { "amount": 628000, "currency": "VND" },
  "can_cancel": false,
  "can_return": true
}
```

**Điểm quan trọng:** khách thấy **một đơn hàng** nhưng nhiều lô giao. Đây là biểu hiện ở tầng API của việc tách `Order` và `FulfillmentOrder`.

Trường `return_deadline` giúp khách biết còn bao lâu để đổi trả — quan trọng với thời trang.

### `POST /api/v1/orders/{id}/returns`

```json
{
  "lines": [
    {
      "order_line_id": "oln_01J9X",
      "quantity": 1,
      "reason_code": "SIZE_TOO_SMALL",
      "reason_detail": "Vai chật hơn bảng size",
      "photo_urls": ["https://..."]
    }
  ]
}
```

`reason_code` **bắt buộc chuẩn hóa** — nó quyết định bên chịu chi phí và là đầu vào cho việc sửa bảng size.

---

## 6. Nội dung và khám phá

### `GET /api/v1/feed`

```json
{
  "data": [
    {
      "type": "OUTFIT",
      "id": "otf_01J9X",
      "title": "Đi làm mùa thu",
      "creator": { "id": "cre_01J9X", "display_name": "Minh Anh", "avatar_url": "..." },
      "is_sponsored": true,
      "media": [ ... ],
      "products": [
        { "id": "prd_01J9X", "name": "Áo sơ mi linen", "price": { "amount": 299000, "currency": "VND" }, "available": true },
        { "id": "prd_01J9Y", "name": "Quần âu", "price": { "amount": 450000, "currency": "VND" }, "available": false,
          "substitutes": [ { "id": "prd_01J9Z", "name": "Quần âu ống suông" } ] }
      ],
      "total_price": { "amount": 749000, "currency": "VND" },
      "engagement": { "views": 12500, "likes": 340, "saves": 89 }
    }
  ],
  "pagination": { "next_cursor": "...", "has_more": true }
}
```

**Ba điểm thiết kế:**

```text
is_sponsored   — tự động gắn, yêu cầu pháp lý
available=false + substitutes — nội dung sống lâu hơn sản phẩm,
                 không để dẫn tới trang lỗi
total_price    — cho phép mua cả bộ, tăng giá trị đơn hàng
```

---

## 7. Tài liệu liên quan

- [api-guidelines.md](api-guidelines.md) — chuẩn kỹ thuật
- [../08-frontend/storefront.md](../08-frontend/storefront.md) — giao diện dùng API này
- [../07-workflows/customer-purchase.md](../07-workflows/customer-purchase.md)
