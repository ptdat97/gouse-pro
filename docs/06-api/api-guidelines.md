# Chuẩn API

## 1. Nguyên tắc chung

```text
1. REST trên HTTP, dữ liệu JSON
2. API phản ánh khả năng nghiệp vụ, không phản ánh màn hình
3. Hợp đồng định nghĩa trước, cài đặt sau
4. OpenAPI là nguồn sự thật duy nhất
5. Mọi số tiền đều kèm đơn vị tiền tệ
6. Mọi lệnh ghi đều idempotent
```

---

## 2. Cấu trúc đường dẫn

```text
/api/v1/...                  — Storefront (khách hàng)
/api/v1/seller/...           — Seller Center
/api/v1/creator/...          — Creator Center
/api/v1/admin/...            — Admin
/api/v1/partner/...          — Đối tác (Phase 4)
/api/v1/webhooks/...         — Nhận webhook từ bên ngoài
/api/v1/storefront/...       — Endpoint tổng hợp (BFF, có kiểm soát)
```

**Vì sao tách theo đối tượng:** mỗi nhóm có mô hình phân quyền, giới hạn tốc độ và mức chi tiết dữ liệu khác nhau. Gộp chung tạo rủi ro rò rỉ dữ liệu.

Ví dụ cụ thể: `GET /api/v1/orders/{id}` và `GET /api/v1/seller/fulfillment-orders/{id}` trả **dữ liệu khác nhau cho cùng một đơn** — seller không được thấy hàng của seller khác trong đơn đó.

---

## 3. Quy ước đặt tên

```text
Tài nguyên:  danh từ số nhiều, kebab-case
             /products, /fulfillment-orders, /affiliate-links

Định danh:   /products/{product_id}

Quan hệ:     /products/{id}/offers
             /orders/{id}/fulfillment-orders

Hành động không phải CRUD:  động từ sau tài nguyên
             POST /orders/{id}/cancel
             POST /returns/{id}/approve
             POST /checkouts/{id}/complete

Trường JSON: snake_case
             { "product_name": "...", "created_at": "..." }
```

**Về hành động:** không cố ép mọi thứ thành CRUD. `POST /orders/{id}/cancel` rõ ràng hơn nhiều so với `PATCH /orders/{id}` với body `{"status": "CANCELLED"}` — vì hủy đơn có quy tắc nghiệp vụ riêng, không phải đơn thuần đổi một trường.

---

## 4. Phương thức HTTP

| Method | Dùng cho | Idempotent |
|---|---|---|
| `GET` | Đọc | Có (tự nhiên) |
| `POST` | Tạo mới, hành động | **Cần Idempotency-Key** |
| `PUT` | Thay thế toàn bộ | Có (tự nhiên) |
| `PATCH` | Cập nhật một phần | **Cần Idempotency-Key** |
| `DELETE` | Xóa | Có (tự nhiên) |

---

## 5. Mã trạng thái HTTP

```text
200 OK                    — thành công, có nội dung
201 Created               — tạo mới thành công
202 Accepted              — nhận, xử lý bất đồng bộ
204 No Content            — thành công, không có nội dung

400 Bad Request           — dữ liệu không hợp lệ
401 Unauthorized          — chưa xác thực
403 Forbidden             — đã xác thực nhưng không có quyền
404 Not Found             — không tìm thấy
409 Conflict              — xung đột (idempotency, trạng thái không cho phép)
422 Unprocessable Entity  — hợp lệ về định dạng, sai về nghiệp vụ
429 Too Many Requests     — vượt giới hạn tốc độ

500 Internal Server Error — lỗi hệ thống
503 Service Unavailable   — tạm thời không phục vụ được
```

### Phân biệt 400, 422, 409

```text
400 — "quantity phải là số"           → sai định dạng
422 — "quantity vượt tồn kho"          → đúng định dạng, sai nghiệp vụ
409 — "đơn đã giao, không hủy được"   → xung đột trạng thái
```

### Phân biệt 401 và 403

```text
401 — chưa đăng nhập hoặc token hết hạn  → client nên đăng nhập lại
403 — đã đăng nhập nhưng không đủ quyền  → đăng nhập lại vô ích
```

---

## 6. Định dạng lỗi thống nhất

```json
{
  "error": {
    "code": "INSUFFICIENT_INVENTORY",
    "message": "Sản phẩm không đủ số lượng",
    "details": {
      "offer_id": "off_01J9X...",
      "requested": 5,
      "available": 2
    },
    "field_errors": [
      { "field": "quantity", "code": "EXCEEDS_AVAILABLE", "message": "Chỉ còn 2 sản phẩm" }
    ]
  },
  "request_id": "req_01J9X..."
}
```

### Quy tắc

```text
1. `code` là mã MÁY ĐỌC ĐƯỢC, ổn định, SCREAMING_SNAKE_CASE
   → client dùng để xử lý, không parse `message`

2. `message` cho người đọc, có thể đổi, có thể đa ngôn ngữ

3. `details` chứa thông tin đủ để client hiển thị hữu ích
   → "chỉ còn 2 sản phẩm" tốt hơn "hết hàng"

4. `field_errors` cho lỗi kiểm tra dữ liệu form

5. `request_id` LUÔN có → dùng khi hỗ trợ khách hàng
```

### Danh mục mã lỗi chính

```text
VALIDATION_FAILED           INSUFFICIENT_INVENTORY
UNAUTHORIZED                OFFER_NOT_AVAILABLE
FORBIDDEN                   CHECKOUT_EXPIRED
NOT_FOUND                   PAYMENT_FAILED
CONFLICT                    ORDER_NOT_CANCELLABLE
IDEMPOTENCY_KEY_REUSED      RETURN_WINDOW_EXPIRED
RATE_LIMIT_EXCEEDED         SELLER_NOT_AUTHORIZED
INTERNAL_ERROR              BRAND_PROTECTED
```

---

## 7. Phân trang

### Con trỏ (cursor) — mặc định cho danh sách lớn

```http
GET /api/v1/products?limit=20&cursor=eyJpZCI6...
```

```json
{
  "data": [ ... ],
  "pagination": {
    "next_cursor": "eyJpZCI6...",
    "has_more": true
  }
}
```

**Vì sao dùng con trỏ:**

```text
Offset có vấn đề:
    - Trang 500 phải quét qua 10.000 bản ghi → chậm
    - Dữ liệu thay đổi giữa các trang → bản ghi bị lặp hoặc bị nhảy

Con trỏ:
    - Hiệu năng ổn định bất kể trang nào
    - Không lặp/nhảy khi có bản ghi mới
```

### Offset — chỉ cho danh sách nhỏ, cần nhảy trang

```http
GET /api/v1/admin/sellers?page=3&per_page=50
```

Dùng khi người dùng cần nhảy tới trang cụ thể (giao diện admin).

---

## 8. Lọc và sắp xếp

```http
GET /api/v1/products
    ?category_id=cat_01J9X
    &brand_id=brd_01J9X
    &price_min=100000
    &price_max=500000
    &size=M,L
    &color=black,white
    &in_stock=true
    &sort=price:asc
    &limit=20
```

```text
Quy tắc:
    - Nhiều giá trị: phân tách bằng dấu phẩy
    - Khoảng: hậu tố _min, _max
    - Sắp xếp: field:asc | field:desc, nhiều trường phân tách dấu phẩy
    - Chỉ hỗ trợ lọc/sắp xếp có chỉ mục — không cho lọc tùy ý
```

**Điểm quan trọng:** danh sách trường được phép lọc và sắp xếp phải **giới hạn tường minh**. Cho phép lọc tùy ý dẫn tới truy vấn quét toàn bảng và là lỗ hổng gây quá tải.

---

## 9. Định dạng dữ liệu

### Tiền — luôn có đơn vị

```json
{
  "unit_price": { "amount": 299000, "currency": "VND" }
}
```

**Không bao giờ** trả về số trần `299000` — client không biết là đồng, nghìn đồng, hay xu.

`amount` là số nguyên theo đơn vị nhỏ nhất của tiền tệ.

### Thời gian — ISO 8601, UTC

```json
{ "placed_at": "2026-08-10T14:23:11Z" }
```

Luôn UTC. Client tự chuyển sang múi giờ hiển thị.

### Định danh — chuỗi có tiền tố

```json
{ "order_id": "ord_01J9XABC...", "sku_id": "sku_01J9XDEF..." }
```

Tiền tố giúp gỡ lỗi nhanh: nhìn `off_...` biết ngay đó là offer, không phải order.

### Enum — chuỗi viết hoa

```json
{ "status": "PENDING_PAYMENT" }
```

**Yêu cầu với client:** phải xử lý được giá trị enum **chưa biết** (khi server thêm trạng thái mới). Không được crash.

---

## 10. Idempotency

```http
POST /api/v1/orders
Idempotency-Key: 01J9XABC123DEF456
```

**Bắt buộc** cho mọi `POST` và `PATCH` thay đổi trạng thái.

Chi tiết hành vi: [../05-data/idempotency.md](../05-data/idempotency.md).

---

## 11. Giới hạn tốc độ

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1754835791
Retry-After: 30
```

```text
Ngưỡng đề xuất theo nhóm:

Không xác thực (duyệt web)     — theo IP, ngưỡng thấp
Khách đã đăng nhập             — theo user, ngưỡng vừa
Seller Center                  — theo seller
Creator Center                 — theo creator
Admin                          — ngưỡng cao
Đối tác                        — theo hợp đồng

Ngưỡng riêng, chặt hơn cho:
    - Đăng nhập (chống dò mật khẩu)
    - Tạo đơn hàng (chống bot)
    - Tìm kiếm (chống quét dữ liệu)
```

---

## 12. Header chuẩn

### Request

```http
Authorization: Bearer <token>
Idempotency-Key: <ulid>          # cho lệnh ghi
X-Request-ID: <ulid>             # tùy chọn, sinh nếu thiếu
Accept-Language: vi-VN
Content-Type: application/json
```

### Response

```http
Content-Type: application/json
X-Request-ID: <ulid>             # LUÔN trả về
X-RateLimit-*: ...
Deprecation: true                # nếu endpoint sắp ngừng
Sunset: Wed, 01 Jan 2027 00:00:00 GMT
```

---

## 13. Phiên bản

```text
Phiên bản trong đường dẫn: /api/v1/

Thay đổi TƯƠNG THÍCH NGƯỢC (cùng phiên bản):
    ✓ Thêm endpoint
    ✓ Thêm trường tùy chọn vào request
    ✓ Thêm trường vào response
    ✓ Thêm giá trị enum
    ✓ Nới lỏng kiểm tra

Thay đổi PHÁ VỠ (cần phiên bản mới):
    ✗ Xóa/đổi tên trường
    ✗ Đổi kiểu dữ liệu
    ✗ Thêm trường bắt buộc
    ✗ Đổi ý nghĩa trường
    ✗ Đổi mã HTTP
    ✗ Siết chặt kiểm tra
```

**Lưu ý:** không cần nâng phiên bản toàn bộ API. Có thể chỉ `/api/v2/orders` trong khi phần còn lại vẫn v1.

---

## 14. Endpoint tổng hợp (BFF) — ngoại lệ có kiểm soát

Trang sản phẩm cần dữ liệu từ 5 module. Gọi 5 lần trên mạng di động là chậm.

```text
Cho phép endpoint tổng hợp KHI:
    ✓ Có vấn đề hiệu năng ĐO ĐƯỢC (không phải phỏng đoán)
    ✓ Đặt trong /api/v1/storefront/...
    ✓ CHỈ tổng hợp, không có logic nghiệp vụ mới
    ✓ API thành phần vẫn tồn tại và dùng độc lập được
```

```http
GET /api/v1/storefront/product-page/{product_id}
```

**Ranh giới:** đây là tối ưu truyền tải, không phải nơi đặt logic. Nếu nó bắt đầu tính giá hay quyết định trạng thái, nó đã sai.

---

## 15. OpenAPI

```text
/api/openapi.yaml   ← nguồn sự thật duy nhất (ĐÃ CÓ)

Cấu trúc:
    api/
    ├── openapi.yaml           — file gốc
    ├── components/            — kiểu cơ bản, schema domain
    └── paths/                 — tách theo nhóm API

Sinh ra từ đó:
    - Tài liệu API
    - Kiểu TypeScript cho Next.js
    - Client SDK
    - Server giả lập cho frontend làm song song
    - Kiểm thử hợp đồng trong CI
```

Xem [../../gouse/api/README.md](../../gouse/api/README.md) để biết lệnh lint, bundle và sinh kiểu.

**Quy tắc:** cập nhật đặc tả **cùng pull request** với thay đổi code. CI so sánh đặc tả với cài đặt, thất bại nếu lệch.

---

## 16. Bảo mật API

| Yêu cầu | Cách thực hiện |
|---|---|
| Luôn HTTPS | Bắt buộc, chuyển hướng HTTP |
| Kiểm tra quyền ở backend | Không dựa vào việc frontend ẩn nút |
| Lọc dữ liệu theo chủ thể | Mọi truy vấn thêm điều kiện theo user/seller |
| Không lộ thông tin trong lỗi | Không trả stack trace, không lộ cấu trúc nội bộ |
| Kiểm tra kích thước request | Chống tấn công bằng payload lớn |
| Kiểm tra kiểu và định dạng | Chống chèn mã |

**Quy tắc quan trọng nhất:** với mọi endpoint trả dữ liệu, câu hỏi đầu tiên phải là *"người gọi này được xem những bản ghi nào?"* — và điều kiện lọc phải nằm trong truy vấn, không phải trong tầng hiển thị.

Xem [../09-operations/security.md](../09-operations/security.md).

---

## 17. Tài liệu liên quan

- [api-domains.md](api-domains.md) — phân nhóm API chi tiết
- [customer-api.md](customer-api.md), [seller-api.md](seller-api.md), [creator-api.md](creator-api.md), [admin-api.md](admin-api.md)
- [webhook.md](webhook.md)
- [../03-architecture/api-first.md](../03-architecture/api-first.md)
