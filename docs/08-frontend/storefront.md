# Storefront

Ứng dụng mua sắm cho khách hàng. Đây là ứng dụng quan trọng nhất — mọi doanh thu đi qua đây.

---

## 1. Sơ đồ trang

```text
Home                    — trang chủ
Discovery / Feed        — dòng nội dung khám phá
Search                  — tìm kiếm
Category                — duyệt theo danh mục
Product                 — chi tiết sản phẩm
Brand                   — trang thương hiệu
Collection              — trang bộ sưu tập
Seller                  — trang gian hàng
Creator                 — trang creator
Content                 — chi tiết nội dung / outfit
Cart                    — giỏ hàng
Checkout                — thanh toán
Account                 — tài khoản
Orders                  — đơn hàng của tôi
Wishlist                — yêu thích
```

---

## 2. Trang sản phẩm — quan trọng nhất

### Nội dung bắt buộc

```text
Ảnh:
    - Nhiều ảnh, đổi theo màu variant
    - Ảnh chi tiết chất liệu
    - Ảnh trên người mẫu có nêu số đo

Chọn variant:
    - Chọn màu → ảnh thay đổi
    - Chọn size → size hết hàng phải hiển thị rõ là hết,
                  KHÔNG ẩn đi (khách cần biết có size đó)

Thông tin thời trang:
    - Chất liệu (material_composition)
    - BẢNG SIZE với số đo thực tế
    - GỢI Ý SIZE (nếu khách đã đăng nhập và có lịch sử)
    - Hướng dẫn bảo quản
    - Xuất xứ

Mua hàng:
    - Offer thắng buy box hiển thị mặc định
    - Link "Xem N nhà bán khác" nếu có nhiều offer
    - Giá + phí ship + thời gian giao dự kiến
    - Chính sách đổi trả

Nội dung liên quan:
    - Outfit chứa sản phẩm này  ← "complete the look"
    - Nội dung creator có tag sản phẩm này
    - Đánh giá của khách (có ảnh thật)
```

### Ba yếu tố giảm tỷ lệ hoàn hàng

```text
1. Bảng size với SỐ ĐO THỰC TẾ (không chỉ S/M/L)
2. Gợi ý size dựa trên lịch sử mua của khách
3. Ảnh trên người mẫu có nêu chiều cao/số đo
```

Tỷ lệ hoàn hàng là vấn đề kinh tế lớn nhất của thương mại thời trang. Ba yếu tố này tác động trực tiếp tới nó.

### Hiển thị nhiều nhà bán

```text
┌─────────────────────────────────────────┐
│ Áo sơ mi linen Oxford                   │
│                                         │
│ 299.000đ                                │
│ Bán bởi: Cửa hàng ABC ⭐4.8            │
│ Giao trong 2 ngày · Phí ship 30.000đ    │
│ [ Thêm vào giỏ ]                        │
│                                         │
│ ▸ Xem 2 nhà bán khác từ 289.000đ        │
└─────────────────────────────────────────┘
```

Khách so sánh được **tổng chi phí** (giá + ship) và **chất lượng phục vụ**, không chỉ giá.

---

## 3. Feed khám phá

```text
Nội dung hiển thị:
    Trending · New Arrivals · For You
    Collections · Creator Content · Lookbooks · Campaigns
```

### Outfit trong feed

```text
┌─────────────────────────────────────────┐
│ [Ảnh outfit]                            │
│ "Đi làm mùa thu"                        │
│ bởi Minh Anh · Được tài trợ             │
│                                         │
│ Gồm 4 món · Tổng 1.249.000đ             │
│ ├ Áo sơ mi linen      299.000đ  ✓       │
│ ├ Quần âu             450.000đ  Tạm hết │
│ │   → Xem sản phẩm tương tự             │
│ ├ Giày loafer         350.000đ  ✓       │
│ └ Túi tote            150.000đ  ✓       │
│                                         │
│ [ Mua cả bộ ]  [ Chọn từng món ]        │
└─────────────────────────────────────────┘
```

**Ba điểm thiết kế:**

```text
"Được tài trợ"      — hệ thống tự gắn, yêu cầu pháp lý
"Tạm hết" + gợi ý   — nội dung sống lâu hơn sản phẩm,
                      không để dẫn tới trang lỗi
"Mua cả bộ"         — tăng giá trị đơn hàng
```

---

## 4. Giỏ hàng — nhóm theo nhà bán

```text
┌─────────────────────────────────────────┐
│ GIỎ HÀNG                                │
│                                         │
│ ▸ Own Brand              Giao 11/08     │
│   Áo sơ mi linen · Trắng/M · x1         │
│                            299.000đ     │
│                                         │
│ ▸ Cửa hàng ABC           Giao 13/08     │
│   Giày loafer · Nâu/39 · x1             │
│                            650.000đ     │
│                                         │
│ ▸ Shop XYZ               Giao 14/08     │
│   Túi tote · Be · x1                    │
│                            301.000đ     │
│ ─────────────────────────────────────── │
│ Tạm tính              1.250.000đ        │
│ Phí vận chuyển           30.000đ        │
│ Tổng                  1.280.000đ        │
└─────────────────────────────────────────┘
```

Nhóm theo nhà bán giúp khách hiểu hàng đến từ đâu và **thời gian giao khác nhau** — tránh thắc mắc "sao chỉ nhận được một phần".

---

## 5. Checkout

```text
Nguyên tắc thiết kế:
    ✓ Ít bước nhất có thể
    ✓ Khách vãng lai được mua (không bắt đăng ký)
    ✓ Hiển thị rõ thời hạn giữ hàng
    ✓ Thời gian giao riêng cho từng nhóm hàng
    ✓ Tổng tiền LUÔN do backend tính
```

### Hiển thị thời hạn

```text
⏱ Đơn hàng được giữ trong 14:32
```

Cho khách biết có thời hạn — tránh việc quay lại sau 30 phút và bị mất giữ hàng mà không hiểu tại sao.

### Xử lý thanh toán thất bại

```text
Thanh toán thất bại → KHÔNG hủy checkout
→ hiển thị "Thử phương thức khác"
→ giữ nguyên hàng trong thời gian TTL còn lại
```

---

## 6. Trang đơn hàng — nhiều lô giao

```text
┌─────────────────────────────────────────┐
│ Đơn FC-2026-08-001234                   │
│ Đặt ngày 10/08/2026 · 1.280.000đ        │
│                                         │
│ ▸ Lô 1 — Own Brand         ✓ Đã giao    │
│   Giao lúc 10:15 ngày 12/08             │
│   Áo sơ mi linen · Trắng/M              │
│   ⏱ Còn 5 ngày để đổi trả              │
│   [ Yêu cầu đổi/trả ]                   │
│                                         │
│ ▸ Lô 2 — Cửa hàng ABC    🚚 Đang giao   │
│   Dự kiến 13/08 · Mã VN123456789        │
│                                         │
│ ▸ Lô 3 — Shop XYZ        📦 Đang chuẩn bị│
└─────────────────────────────────────────┘
```

Khách thấy **một đơn hàng** nhưng nhiều lô giao — biểu hiện ở giao diện của việc tách `Order`/`FulfillmentOrder`.

Trường "còn N ngày để đổi trả" quan trọng với thời trang — khách cần biết deadline.

---

## 7. Yêu cầu hiệu năng

| Chỉ số | Mục tiêu | Vì sao |
|---|---|---|
| LCP trang sản phẩm | < 2,5s | Chậm → mất khách |
| LCP trang chủ | < 2s | Ấn tượng đầu tiên |
| INP | < 200ms | Cảm giác mượt |
| CLS | < 0,1 | Ảnh nhảy → bấm nhầm |

### Tối ưu ảnh — quan trọng nhất

```text
Thời trang cần ảnh đẹp, nhưng ảnh nặng làm chậm trang.

Giải pháp:
    ✓ AVIF/WebP có dự phòng
    ✓ Nhiều kích thước theo thiết bị
    ✓ Lazy load ảnh ngoài màn hình
    ✓ Placeholder mờ chống nhảy layout
    ✓ Ưu tiên tải ảnh chính (preload)
```

---

## 8. SEO

```text
Bắt buộc:
    ✓ Server render trang sản phẩm, danh mục, thương hiệu
    ✓ Dữ liệu có cấu trúc (Product, Offer, AggregateRating, BreadcrumbList)
    ✓ URL thân thiện: /ao-so-mi-linen-oxford-prd_01J9X
    ✓ Thẻ canonical (chống trùng lặp khi lọc)
    ✓ Alt text cho mọi ảnh
    ✓ Sitemap
```

**Về canonical:** trang danh mục có nhiều bộ lọc sinh ra vô số URL. Không xử lý canonical sẽ bị đánh giá là nội dung trùng lặp.

**Về sản phẩm bị gộp:** URL của product bị gộp phải chuyển hướng 301 sang product chuẩn — giữ giá trị SEO đã tích lũy.

---

## 9. Tài liệu liên quan

- [frontend-architecture.md](frontend-architecture.md)
- [../06-api/customer-api.md](../06-api/customer-api.md)
- [../07-workflows/customer-purchase.md](../07-workflows/customer-purchase.md)
