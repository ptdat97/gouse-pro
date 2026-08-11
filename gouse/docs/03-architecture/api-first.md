# API First

## 1. API First nghĩa là gì

> Hợp đồng API được thiết kế và thống nhất **trước** khi viết code cài đặt hoặc code giao diện.

```text
Cách thông thường:              Cách API First:
─────────────────               ───────────────
Thiết kế màn hình               Xác định use case nghiệp vụ
    ↓                               ↓
Viết backend cho màn hình đó    Thiết kế hợp đồng API
    ↓                               ↓
API mang hình dạng màn hình     Frontend và backend làm song song
    ↓                               ↓
Màn hình khác cần → API mới     Mặt tiền mới dùng lại API có sẵn
```

---

## 2. Vì sao bắt buộc với nền tảng này

Nền tảng phục vụ **sáu loại người dùng khác nhau**:

```text
Storefront       — khách hàng mua sắm
Seller Center    — nhà bán quản lý gian hàng
Creator Center   — creator quản lý nội dung và thu nhập
Admin            — nhân viên vận hành
Mobile App       — ứng dụng di động (tương lai)
Partner API      — đối tác tích hợp (tương lai)
```

Nếu API sinh ra từ màn hình:

```text
- API "lấy dữ liệu trang chủ" chỉ dùng được cho trang chủ web
- App di động cần trang chủ khác → phải viết API mới
- Đối tác muốn lấy danh mục → không có API phù hợp
- Cùng một logic nghiệp vụ được viết lại nhiều lần
```

Xem nguyên tắc P3 tại [../00-overview/principles.md](../00-overview/principles.md) và [ADR-0002](../adr/0002-api-first.md).

---

## 3. Quy trình phát triển API First

```text
1. Xác định use case nghiệp vụ
   "Khách thêm sản phẩm vào giỏ"
        ↓
2. Thiết kế hợp đồng API
   POST /api/v1/cart/items
   Request/response schema, mã lỗi
        ↓
3. Viết đặc tả OpenAPI
   Đưa vào /api/openapi.yaml
        ↓
4. Rà soát hợp đồng
   Backend + frontend + người hiểu nghiệp vụ cùng xem
        ↓
5. Sinh mã và server giả lập
   Frontend dùng server giả lập, không phải chờ backend
        ↓
6. Cài đặt song song
   Backend viết logic thật
   Frontend viết giao diện
        ↓
7. Kiểm thử hợp đồng
   Đảm bảo cài đặt khớp đặc tả
```

**Lợi ích lớn nhất:** bước 5–6 chạy **song song**. Frontend không phải chờ backend xong.

---

## 4. Nguyên tắc thiết kế API

### 4.1 API phản ánh khả năng nghiệp vụ, không phản ánh màn hình

```text
SAI (theo màn hình):
    GET /api/v1/homepage
    GET /api/v1/product-detail-page/{id}
    GET /api/v1/seller-dashboard

ĐÚNG (theo khả năng):
    GET /api/v1/products?featured=true&limit=12
    GET /api/v1/products/{id}
    GET /api/v1/products/{id}/offers
    GET /api/v1/sellers/{id}/metrics
```

### 4.2 Ngoại lệ có kiểm soát: endpoint tổng hợp (BFF)

Nguyên tắc trên có một chi phí thật: trang sản phẩm cần 5 lệnh gọi (sản phẩm, offer, tồn kho, đánh giá, nội dung liên quan) — chậm trên mạng di động.

**Giải pháp có kiểm soát:**

```text
Cho phép endpoint tổng hợp KHI:
  ✓ Có vấn đề hiệu năng đo được (không phải phỏng đoán)
  ✓ Đặt trong nhóm riêng: /api/v1/storefront/...
  ✓ Chỉ TỔNG HỢP, không chứa logic nghiệp vụ mới
  ✓ API thành phần vẫn tồn tại và dùng được độc lập

Ví dụ:
    GET /api/v1/storefront/product-page/{id}
    → gọi nội bộ 5 API, gộp kết quả, trả một lần
```

**Ranh giới:** endpoint tổng hợp là **tối ưu truyền tải**, không phải nơi đặt logic. Nếu nó bắt đầu tính giá hay quyết định trạng thái, nó đã sai.

### 4.3 Bốn nhóm API theo đối tượng

```text
/api/v1/...                  — Storefront (khách hàng)
/api/v1/seller/...           — Seller Center
/api/v1/creator/...          — Creator Center
/api/v1/admin/...            — Admin
/api/v1/partner/...          — Đối tác (tương lai)
/api/v1/webhooks/...         — Nhận webhook từ bên ngoài
```

**Vì sao tách:** mỗi nhóm có mô hình phân quyền, giới hạn tốc độ và mức độ chi tiết dữ liệu khác nhau. Gộp chung sẽ tạo rủi ro rò rỉ dữ liệu.

Ví dụ cụ thể: `GET /api/v1/orders/{id}` (khách xem đơn của mình) và `GET /api/v1/seller/fulfillment-orders/{id}` (seller xem phần của mình) trả về **dữ liệu khác nhau cho cùng một đơn hàng**. Seller không được thấy hàng của seller khác trong đơn đó.

---

## 5. Những gì phải định nghĩa trước khi viết code

Đây là danh sách bắt buộc, phải thống nhất trước khi có endpoint đầu tiên:

| Hạng mục | Quyết định |
|---|---|
| Xác thực | Cơ chế token, thời hạn, làm mới |
| Phân quyền | Mô hình vai trò và quyền |
| Phiên bản | Đặt trong đường dẫn: `/api/v1/` |
| Phân trang | Con trỏ (cursor) cho danh sách lớn, offset cho nhỏ |
| Lọc | Cú pháp tham số truy vấn |
| Sắp xếp | Cú pháp `sort=field:asc` |
| Định dạng lỗi | Cấu trúc thống nhất, mã lỗi có ý nghĩa |
| Idempotency | Header `Idempotency-Key` cho mọi lệnh ghi |
| Giới hạn tốc độ | Ngưỡng theo nhóm API và theo người dùng |
| Truy vết | Header `X-Request-ID`, truyền qua toàn hệ thống |
| Đa ngôn ngữ | Header `Accept-Language` |
| Tiền tệ | Luôn trả kèm đơn vị, không giả định |

Chi tiết đầy đủ: [../06-api/api-guidelines.md](../06-api/api-guidelines.md).

---

## 6. Hợp đồng API là hợp đồng thật

### Quy tắc thay đổi

```text
Thay đổi TƯƠNG THÍCH NGƯỢC (được phép trong cùng phiên bản):
  ✓ Thêm endpoint mới
  ✓ Thêm trường tùy chọn vào request
  ✓ Thêm trường vào response
  ✓ Thêm giá trị mới vào enum (client phải xử lý giá trị lạ)
  ✓ Nới lỏng ràng buộc kiểm tra

Thay đổi PHÁ VỠ (cần phiên bản mới):
  ✗ Xóa endpoint hoặc trường
  ✗ Đổi tên trường
  ✗ Đổi kiểu dữ liệu
  ✗ Thêm trường bắt buộc vào request
  ✗ Đổi ý nghĩa của trường
  ✗ Đổi mã HTTP trả về
  ✗ Siết chặt ràng buộc kiểm tra
```

### Quy trình khi cần thay đổi phá vỡ

```text
1. Tạo /api/v2/ cho endpoint bị ảnh hưởng
2. Giữ v1 hoạt động (thời gian cam kết, ví dụ 6 tháng)
3. Đánh dấu deprecated trong đặc tả và header response
4. Theo dõi ai còn dùng v1
5. Thông báo trước khi ngừng
6. Ngừng v1
```

**Lưu ý:** không nhất thiết nâng phiên bản toàn bộ API. Có thể chỉ có `/api/v2/orders` trong khi phần còn lại vẫn v1.

---

## 7. OpenAPI là nguồn sự thật

```text
/api/openapi.yaml   ← nguồn sự thật duy nhất về hợp đồng API
```

Từ file này sinh ra:

```text
├── Tài liệu API (cho nội bộ và đối tác)
├── Kiểu TypeScript cho Next.js
├── Client SDK
├── Server giả lập cho frontend phát triển song song
└── Kiểm thử hợp đồng trong CI
```

**Quy tắc:** đặc tả phải được cập nhật **cùng pull request** với thay đổi code. Không chấp nhận "cập nhật tài liệu sau".

**Kiểm tra tự động:** CI so sánh đặc tả với cài đặt thật, thất bại nếu lệch.

---

## 8. Frontend không bao giờ truy cập database

Nhắc lại nguyên tắc P4, vì đây là nơi dễ vi phạm nhất khi dùng Next.js.

```text
Next.js server components / API routes CÓ THỂ truy cập database về mặt kỹ thuật.
ĐIỀU NÀY BỊ CẤM.
```

### Vì sao cấm

| Lý do | Giải thích |
|---|---|
| Logic trùng lặp | Quy tắc nghiệp vụ sẽ dần rò rỉ vào frontend |
| Không dùng lại được | App di động không chạy được code Next.js |
| Bảo mật | Thông tin kết nối database ở tầng gần người dùng |
| Không kiểm soát được | Không đi qua phân quyền, giới hạn tốc độ, ghi log của backend |
| Không quan sát được | Truy vấn không xuất hiện trong hệ thống giám sát backend |

### Điều Next.js ĐƯỢC làm

```text
✓ Gọi API backend (từ server component hoặc client)
✓ Cache response ở tầng Next.js
✓ Tổng hợp nhiều lời gọi API để render
✓ Quản lý trạng thái giao diện
✓ Kiểm tra hợp lệ form (để trải nghiệm tốt — backend vẫn kiểm lại)
✓ Hiển thị/ẩn phần tử theo vai trò (chỉ là giao diện, không phải bảo mật)
```

Xem [../08-frontend/frontend-architecture.md](../08-frontend/frontend-architecture.md).

---

## 9. Idempotency là bắt buộc

Mọi endpoint thay đổi trạng thái phải hỗ trợ:

```http
POST /api/v1/orders
Idempotency-Key: 01J9XABC...
Content-Type: application/json
```

**Hành vi:**

```text
Lần gọi đầu   → xử lý, lưu kết quả gắn với key
Gọi lại cùng key, cùng nội dung  → trả kết quả đã lưu, KHÔNG xử lý lại
Gọi lại cùng key, khác nội dung  → trả lỗi 409 Conflict
Key hết hạn (ví dụ 24 giờ)       → xử lý như request mới
```

**Vì sao bắt buộc:** mạng không tin cậy. Khách bấm nút hai lần. Ứng dụng thử lại khi timeout. Không có idempotency thì khách bị trừ tiền hai lần.

Xem [../05-data/idempotency.md](../05-data/idempotency.md).

---

## 10. Ví dụ hợp đồng API hoàn chỉnh

```yaml
POST /api/v1/cart/items

Headers:
  Authorization: Bearer <token>        # tùy chọn — khách vãng lai dùng session
  Idempotency-Key: <ulid>              # bắt buộc
  X-Request-ID: <ulid>                 # tùy chọn, sinh tự động nếu thiếu

Request:
  {
    "offer_id": "off_01J9X...",
    "quantity": 2,
    "source": {                         # để quy kết cho creator
      "content_id": "cnt_01J9X...",
      "creator_id": "cre_01J9X..."
    }
  }

Response 200:
  {
    "cart": {
      "id": "crt_01J9X...",
      "items": [
        {
          "id": "cit_01J9X...",
          "offer_id": "off_01J9X...",
          "product_name": "Áo sơ mi linen Oxford",
          "variant_description": "Trắng / M",
          "unit_price": { "amount": 299000, "currency": "VND" },
          "quantity": 2,
          "line_total": { "amount": 598000, "currency": "VND" },
          "seller": { "id": "sel_01J9X...", "name": "Cửa hàng ABC" },
          "availability": "IN_STOCK"
        }
      ],
      "subtotal": { "amount": 598000, "currency": "VND" },
      "estimated_shipping": { "amount": 30000, "currency": "VND" },
      "total": { "amount": 628000, "currency": "VND" }
    }
  }

Response 409 (hết hàng):
  {
    "error": {
      "code": "INSUFFICIENT_INVENTORY",
      "message": "Sản phẩm không đủ số lượng",
      "details": {
        "offer_id": "off_01J9X...",
        "requested": 2,
        "available": 1
      }
    },
    "request_id": "req_01J9X..."
  }
```

**Quan sát quan trọng:**

- Mọi số tiền có **cả giá trị và đơn vị tiền tệ** — không giả định VND.
- Tổng tiền do **backend tính**, frontend chỉ hiển thị.
- Lỗi có **mã máy đọc được** (`INSUFFICIENT_INVENTORY`), không chỉ thông báo tiếng người.
- Chi tiết lỗi đủ để frontend hiển thị thông tin hữu ích ("chỉ còn 1 sản phẩm").
- Có `request_id` để đối chiếu khi hỗ trợ khách hàng.

---

## 11. Tài liệu liên quan

- [../06-api/api-guidelines.md](../06-api/api-guidelines.md) — chuẩn chi tiết
- [../06-api/api-domains.md](../06-api/api-domains.md) — phân nhóm API
- [../08-frontend/frontend-architecture.md](../08-frontend/frontend-architecture.md) — cách Next.js dùng API
- [../adr/0002-api-first.md](../adr/0002-api-first.md) — quyết định kiến trúc
