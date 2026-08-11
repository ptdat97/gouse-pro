# Seller API (Seller Center)

Base path: `/api/v1/seller/`

---

## 1. Xác thực và phạm vi

```http
Authorization: Bearer <token>
```

Token chứa `seller_id`. **Mọi truy vấn tự động lọc theo `seller_id` này ở tầng repository**, không phải ở tầng hiển thị.

### Ràng buộc bảo mật quan trọng nhất

> **Seller không bao giờ thấy dữ liệu của seller khác, kể cả gián tiếp.**

Bao gồm cả việc suy ngược từ báo cáo tổng hợp. Ví dụ: không cung cấp "thị phần của bạn trong danh mục" nếu danh mục chỉ có hai seller — vì seller kia suy ra được doanh số của đối thủ.

Vi phạm ràng buộc này làm mất niềm tin toàn bộ nhà bán.

---

## 2. Đăng ký làm seller

### `POST /api/v1/sellers`

Endpoint này nằm ở nhóm storefront (người dùng thường đăng ký), không phải nhóm seller.

```json
{
  "seller_type": "BUSINESS",
  "business_name": "Công ty ABC",
  "tax_id": "0123456789",
  "contact_email": "contact@abc.vn",
  "contact_phone": "+84901234567",
  "documents": [
    { "type": "BUSINESS_LICENSE", "file_url": "https://..." }
  ],
  "bank_account": {
    "bank_code": "...",
    "account_number": "...",
    "account_holder": "CONG TY ABC"
  }
}
```

```json
{
  "seller": { "id": "sel_01J9X", "status": "PENDING_REVIEW", "applied_at": "..." },
  "next_steps": ["Chờ duyệt hồ sơ (1–3 ngày làm việc)", "Xác minh tài khoản ngân hàng"]
}
```

---

## 3. Quản lý sản phẩm và offer

### `POST /api/v1/seller/offers`

```http
POST /api/v1/seller/offers
Idempotency-Key: 01J9X...
```

```json
{
  "sku_id": "sku_01J9X",
  "price": { "amount": 299000, "currency": "VND" },
  "compare_at_price": { "amount": 399000, "currency": "VND" },
  "condition": "NEW",
  "handling_time_hours": 24,
  "initial_inventory": { "stock_location_id": "loc_01J9X", "quantity": 50 }
}
```

**Response 403 (thương hiệu được bảo vệ):**

```json
{
  "error": {
    "code": "BRAND_PROTECTED",
    "message": "Thương hiệu này yêu cầu giấy ủy quyền",
    "details": {
      "brand_id": "brd_01J9X",
      "protection_level": "VERIFIED_ONLY",
      "required_action": "UPLOAD_AUTHORIZATION"
    }
  },
  "request_id": "req_01J9X"
}
```

Đây là cơ chế chống hàng giả — kiểm tra ở tầng domain, không phải quy trình thủ công.

**Response 422 (giá ngoài khung):**

```json
{
  "error": {
    "code": "PRICE_OUT_OF_RANGE",
    "message": "Giá nằm ngoài khung cho phép",
    "details": { "min_price": 150000, "max_price": 600000, "submitted": 50000 }
  }
}
```

Khung giá chống hai rủi ro: bán phá giá và lỗi nhập liệu (thiếu số 0).

### `PATCH /api/v1/seller/offers/{id}`

```json
{ "price": { "amount": 279000, "currency": "VND" } }
```

Mọi thay đổi giá được ghi vào `offer_price_history` — dùng để phát hiện thao túng giá (tăng rồi giảm giả vờ khuyến mãi).

---

## 4. Quản lý tồn kho

### `PATCH /api/v1/seller/inventory/{sku_id}`

```json
{
  "stock_location_id": "loc_01J9X",
  "quantity_available": 45,
  "reason": "Kiểm kê thực tế"
}
```

**Lưu ý:** đây là **đặt giá trị tuyệt đối**, không phải cộng trừ. Lý do: idempotent tự nhiên — gọi lại không làm sai lệch.

Trường `reason` bắt buộc và được ghi vào `inventory_movement`.

---

## 5. Xử lý đơn hàng — điểm khác biệt quan trọng nhất

### `GET /api/v1/seller/fulfillment-orders`

Seller thấy **FulfillmentOrder**, không phải **Order**.

```json
{
  "data": [
    {
      "id": "ful_01J9X",
      "fulfillment_number": "FC-2026-08-001234-A",
      "status": "PENDING",
      "created_at": "2026-08-10T14:25:11Z",
      "sla_deadline": "2026-08-11T14:25:11Z",
      "items": [
        {
          "sku_id": "sku_01J9X",
          "sku_code": "SM-LIN-OXF-WHT-M",
          "product_name": "Áo sơ mi linen Oxford",
          "variant_description": "Trắng / M",
          "quantity": 2,
          "unit_price": { "amount": 299000, "currency": "VND" }
        }
      ],
      "shipping_address": {
        "recipient_name": "Nguyễn Văn A",
        "phone": "+84901234567",
        "street_address": "...",
        "ward": "...", "district": "...", "province": "..."
      },
      "shipping_method": "STANDARD",
      "seller_payout_estimate": { "amount": 538200, "currency": "VND" }
    }
  ]
}
```

### Điều seller KHÔNG thấy

```text
✗ order_id của đơn tổng
✗ Tổng tiền của cả đơn hàng
✗ Các FulfillmentOrder khác trong cùng đơn
✗ Tên các seller khác
✗ Lịch sử mua hàng của khách
✗ Email khách (chỉ cần tên, số điện thoại để giao hàng)
```

Đây là lý do kỹ thuật thứ hai cho việc tách `Order`/`FulfillmentOrder`, bên cạnh lý do về tranh chấp ghi. Xem [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md).

**Về `seller_payout_estimate`:** ước tính số tiền seller nhận sau khi trừ hoa hồng và phí — minh bạch ngay từ lúc nhận đơn, tránh tranh chấp về sau.

### `POST /api/v1/seller/fulfillment-orders/{id}/ship`

```json
{
  "tracking_number": "VN123456789",
  "shipping_provider": "PROVIDER_A",
  "shipped_at": "2026-08-11T09:30:00Z"
}
```

---

## 6. Tài chính

### `GET /api/v1/seller/balance`

```json
{
  "currency": "VND",
  "pending": { "amount": 2450000, "currency": "VND" },
  "available": { "amount": 8320000, "currency": "VND" },
  "processing": { "amount": 0, "currency": "VND" },
  "on_hold": { "amount": 0, "currency": "VND" },
  "reserve_held": { "amount": 500000, "currency": "VND" },
  "next_settlement_date": "2026-08-13"
}
```

**Giải thích cho seller:**

```text
pending       — đơn đã giao, đang trong thời hạn đổi trả
available     — sẵn sàng chi trả kỳ tới
processing    — đang chuyển tiền
on_hold       — bị giữ (tranh chấp)
reserve_held  — giữ lại theo chính sách bảo đảm
```

Hiển thị đủ năm trạng thái giúp seller hiểu vì sao chưa nhận được tiền — giảm khiếu nại.

### `GET /api/v1/seller/settlements/{id}`

```json
{
  "id": "stl_01J9X",
  "period": { "from": "2026-08-01", "to": "2026-08-07" },
  "status": "CONFIRMED",
  "summary": {
    "gross_sales": { "amount": 12500000, "currency": "VND" },
    "platform_commission": { "amount": -1250000, "currency": "VND" },
    "payment_fee": { "amount": -187500, "currency": "VND" },
    "fulfillment_fee": { "amount": -300000, "currency": "VND" },
    "creator_commission": { "amount": -180000, "currency": "VND" },
    "refunds": { "amount": -890000, "currency": "VND" },
    "adjustments": { "amount": 0, "currency": "VND" },
    "net_payout": { "amount": 9692500, "currency": "VND" }
  },
  "lines_url": "/api/v1/seller/settlements/stl_01J9X/lines"
}
```

**Yêu cầu minh bạch:** seller phải xem được **từng dòng** cấu thành số tiền, không chỉ tổng. Đối soát không minh bạch là nguyên nhân tranh chấp lớn nhất giữa nền tảng và nhà bán.

---

## 7. Hiệu suất

### `GET /api/v1/seller/performance`

```json
{
  "period": "LAST_30_DAYS",
  "metrics": [
    { "name": "cancellation_rate", "value": 0.021, "threshold": 0.03, "status": "GOOD" },
    { "name": "on_time_shipping_rate", "value": 0.94, "threshold": 0.95, "status": "WARNING" },
    { "name": "return_rate_description", "value": 0.018, "threshold": 0.05, "status": "GOOD" },
    { "name": "average_rating", "value": 4.6, "threshold": 4.0, "status": "GOOD" },
    { "name": "inventory_accuracy", "value": 0.97, "threshold": 0.95, "status": "GOOD" }
  ],
  "impact": {
    "buy_box_win_rate": 0.62,
    "message": "Cải thiện tỷ lệ giao đúng hạn sẽ tăng cơ hội thắng buy box"
  }
}
```

**Nguyên tắc P14 áp dụng:** chỉ số và ngưỡng **công khai, tường minh**. Seller hiểu được mình đang ở đâu và cần làm gì. Mô hình hộp đen tạo tranh chấp không giải quyết được.

---

## 8. Tài liệu liên quan

- [api-domains.md](api-domains.md) — phân quyền
- [../08-frontend/seller-center.md](../08-frontend/seller-center.md)
- [../04-modules/seller.md](../04-modules/seller.md)
- [../07-workflows/seller-onboarding.md](../07-workflows/seller-onboarding.md)
