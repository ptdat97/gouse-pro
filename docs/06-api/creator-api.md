# Creator API (Creator Center)

Base path: `/api/v1/creator/`

---

## 1. Xác thực và ranh giới quyền riêng tư

```http
Authorization: Bearer <token>
```

Token chứa `creator_id`. Mọi truy vấn lọc theo định danh này.

### Ràng buộc tuyệt đối

> **Creator KHÔNG BAO GIỜ thấy danh tính khách hàng.**

```text
Creator ĐƯỢC thấy:
    ✓ Số lượt click, số đơn, doanh thu quy kết
    ✓ Hoa hồng của mình
    ✓ Số liệu tổng hợp theo nội dung, theo sản phẩm
    ✓ Phân bố theo thời gian, theo danh mục

Creator KHÔNG thấy:
    ✗ Tên, email, số điện thoại khách hàng
    ✗ Địa chỉ giao hàng
    ✗ Mã đơn hàng cụ thể
    ✗ Lịch sử mua hàng của cá nhân nào
```

**Lý do:** creator không phải bên xử lý dữ liệu cá nhân. Cung cấp dữ liệu khách cho creator là vi phạm quy định bảo vệ dữ liệu ở nhiều thị trường.

**Hệ quả kỹ thuật:** mọi số liệu trả về đều là tổng hợp. Nếu một số liệu có thể suy ngược ra cá nhân (ví dụ "1 đơn hàng hôm nay lúc 14:23"), cần cân nhắc ngưỡng tối thiểu trước khi hiển thị.

---

## 2. Đăng ký làm creator

### `POST /api/v1/creators`

```json
{
  "display_name": "Minh Anh",
  "creator_type": "KOC",
  "bio": "Chia sẻ về thời trang công sở",
  "specialties": ["WORKWEAR", "MINIMALIST"],
  "channels": [
    { "platform": "TIKTOK", "handle": "@minhanh", "follower_count": 45000, "profile_url": "https://..." }
  ],
  "bank_account": { ... }
}
```

```json
{
  "creator": { "id": "cre_01J9X", "status": "PENDING_REVIEW" },
  "next_steps": ["Xác minh kênh mạng xã hội", "Chờ duyệt hồ sơ"]
}
```

---

## 3. Quản lý nội dung

### `POST /api/v1/creator/content`

```json
{
  "content_type": "OUTFIT",
  "title": "Đi làm mùa thu",
  "body": "Bộ phối cho những ngày trời se lạnh...",
  "media": [ { "type": "IMAGE", "url": "https://...", "order": 1 } ],
  "campaign_id": "cmp_01J9X",
  "outfit": {
    "items": [
      { "product_id": "prd_01J9X", "variant_id": "var_01J9X", "role": "MAIN" },
      { "product_id": "prd_01J9Y", "role": "MAIN" },
      { "product_id": "prd_01J9Z", "role": "ACCESSORY" }
    ]
  },
  "product_tags": [
    { "product_id": "prd_01J9X", "position_x": 0.35, "position_y": 0.42 }
  ]
}
```

```json
{
  "content": {
    "id": "cnt_01J9X",
    "status": "PENDING_REVIEW",
    "is_sponsored": true,
    "sponsored_label": "Nội dung được tài trợ"
  }
}
```

**Lưu ý về `is_sponsored`:** hệ thống **tự động** đặt giá trị này khi nội dung thuộc chiến dịch có trả phí. Creator không tự khai và không tắt được.

Đây là nghĩa vụ pháp lý của nền tảng, không thể phụ thuộc vào việc creator có nhớ ghi hay không.

### `POST /api/v1/creator/content/{id}/publish`

```json
{ "content": { "id": "cnt_01J9X", "status": "PUBLISHED", "published_at": "..." } }
```

**Response 422 (sản phẩm không hợp lệ):**

```json
{
  "error": {
    "code": "INVALID_PRODUCT_TAG",
    "message": "Một số sản phẩm không thể gắn thẻ",
    "details": {
      "invalid_products": [
        { "product_id": "prd_01J9Y", "reason": "PRODUCT_UNPUBLISHED" }
      ]
    }
  }
}
```

---

## 4. Affiliate link

### `POST /api/v1/creator/affiliate-links`

```json
{
  "target_type": "CONTENT",
  "target_id": "cnt_01J9X",
  "campaign_id": "cmp_01J9X"
}
```

```json
{
  "id": "afl_01J9X",
  "short_code": "mA7xK2",
  "url": "https://.../r/mA7xK2",
  "created_at": "..."
}
```

---

## 5. Thu nhập

### `GET /api/v1/creator/earnings`

```json
{
  "period": { "from": "2026-08-01", "to": "2026-08-31" },
  "summary": {
    "pending": { "amount": 1250000, "currency": "VND" },
    "available": { "amount": 3480000, "currency": "VND" },
    "paid_this_period": { "amount": 2100000, "currency": "VND" }
  },
  "attributed_gmv": { "amount": 45600000, "currency": "VND" },
  "conversion_count": 152,
  "next_settlement_date": "2026-09-05"
}
```

**Giải thích `pending`:** hoa hồng từ đơn đã giao nhưng chưa hết hạn đổi trả. Chỉ chuyển sang `available` sau khi hết hạn — cơ chế bảo vệ khỏi hoàn hàng.

### `GET /api/v1/creator/analytics`

```json
{
  "period": "LAST_30_DAYS",
  "funnel": {
    "content_views": 125000,
    "product_clicks": 8400,
    "add_to_cart": 1250,
    "conversions": 152
  },
  "rates": {
    "view_to_click": 0.067,
    "click_to_cart": 0.149,
    "cart_to_purchase": 0.122,
    "overall_conversion": 0.0012
  },
  "top_content": [
    {
      "content_id": "cnt_01J9X",
      "title": "Đi làm mùa thu",
      "views": 45000,
      "clicks": 3200,
      "conversions": 68,
      "attributed_gmv": { "amount": 18900000, "currency": "VND" },
      "commission_earned": { "amount": 945000, "currency": "VND" },
      "return_rate": 0.09
    }
  ],
  "top_products": [ ... ]
}
```

**Về `return_rate` theo nội dung:** đây là chỉ số hai chiều.

```text
Với creator: biết nội dung nào gây hiểu nhầm để điều chỉnh cách mô tả
Với nền tảng: phát hiện nội dung mô tả sai lệch
```

Nội dung có tỷ lệ hoàn hàng cao bất thường là dấu hiệu cần rà soát.

**Lưu ý:** mọi số liệu ở đây đều là **tổng hợp**. Không có endpoint nào trả về danh sách đơn hàng hay khách hàng cụ thể.

---

## 6. Chiến dịch

### `GET /api/v1/creator/campaigns`

```json
{
  "data": [
    {
      "id": "cmp_01J9X",
      "name": "Bộ sưu tập Thu Đông 2026",
      "brand": { "id": "brd_01J9X", "name": "Thương hiệu A" },
      "period": { "from": "2026-08-01", "to": "2026-09-30" },
      "fee_structure": "HYBRID",
      "commission_rate": 800,
      "fixed_fee": { "amount": 2000000, "currency": "VND" },
      "requirements": {
        "min_content_count": 3,
        "content_types": ["VIDEO", "OUTFIT"],
        "deadline": "2026-08-20"
      },
      "status": "OPEN_FOR_APPLICATION",
      "eligible": true
    }
  ]
}
```

Trường `fee_structure` phản ánh ba mô hình chi phí đã mô tả ở [../04-modules/campaign.md](../04-modules/campaign.md) — `COMMISSION_ONLY`, `FIXED_FEE`, `HYBRID`.

`commission_rate` là basis points (800 = 8%).

---

## 7. Tài liệu liên quan

- [api-domains.md](api-domains.md) — phân quyền
- [../08-frontend/creator-center.md](../08-frontend/creator-center.md)
- [../04-modules/affiliate.md](../04-modules/affiliate.md)
- [../07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md)
