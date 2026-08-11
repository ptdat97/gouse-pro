# Design System

## 1. Mục đích và phạm vi

Hệ thống thiết kế dùng chung cho bốn ứng dụng, đảm bảo:

```text
✓ Nhất quán trải nghiệm
✓ Không viết lại component cơ bản bốn lần
✓ Thay đổi thương hiệu ở một chỗ
✓ Khả năng tiếp cận được đảm bảo ở tầng component
```

**Phạm vi:** chỉ chứa thành phần trình bày. **Không** chứa logic nghiệp vụ.

```text
✓ Component `PriceDisplay` — định dạng và hiển thị số tiền
✗ Component `PriceCalculator` — tính giá (đó là việc của backend)
```

---

## 2. Cấu trúc

```text
packages/
├── design-tokens/     — màu, khoảng cách, typography, bo góc
├── ui/                — component cơ bản dùng chung
└── ui-commerce/       — component đặc thù thương mại
```

### Vì sao tách `ui` và `ui-commerce`

```text
ui/            — Button, Input, Modal, Table, Toast
               → dùng ở cả bốn ứng dụng

ui-commerce/   — ProductCard, PriceDisplay, SizeSelector,
                 ColorSwatch, OutfitCard, SellerBadge
               → chủ yếu storefront, một phần seller center
               → admin gần như không dùng
```

Tách ra để admin không phải tải bundle chứa component thương mại.

---

## 3. Design tokens

```text
Màu:
    Thương hiệu (primary, secondary)
    Ngữ nghĩa (success, warning, danger, info)
    Trung tính (nền, viền, chữ)

Typography:
    Font chữ, cỡ chữ, độ đậm, chiều cao dòng

Khoảng cách:
    Thang đo nhất quán (4, 8, 12, 16, 24, 32, 48, 64)

Khác:
    Bo góc, đổ bóng, thời gian chuyển động, breakpoint
```

**Nguyên tắc:** component **không bao giờ** dùng giá trị màu hay khoảng cách trực tiếp. Luôn qua token.

```text
❌ color: #1A73E8
✅ color: var(--color-primary)
```

Lý do: đổi màu thương hiệu là sửa một file token, không phải tìm kiếm khắp codebase.

---

## 4. Component đặc thù thời trang

Đây là phần khác biệt so với design system thương mại điện tử thông thường.

### 4.1 `SizeSelector`

```text
┌─────────────────────────────────────┐
│ Kích cỡ          [Bảng size]        │
│                                     │
│  S    M    L    XL   XXL            │
│ [ ]  [✓]  [ ]  [✗]  [✗]            │
│                                     │
│ 💡 Gợi ý: L (dựa trên lần mua trước)│
└─────────────────────────────────────┘
```

**Quy tắc thiết kế:**

```text
✓ Size hết hàng vẫn HIỂN THỊ, gạch chéo — không ẩn
  → khách cần biết sản phẩm có size đó, để đăng ký nhận thông báo

✓ Link "Bảng size" luôn nằm cạnh — không giấu trong tab
  → sai size là nguyên nhân hoàn hàng số một

✓ Gợi ý size hiển thị rõ lý do
  → tạo niềm tin, khác với gợi ý không giải thích
```

### 4.2 `ColorSwatch`

```text
Hiển thị ô màu thật (hex_code), không chỉ tên màu.
Có tooltip tên màu đầy đủ.
Chọn màu → đổi bộ ảnh sản phẩm.
```

Với thời trang, khách chọn theo màu nhìn thấy, không theo tên. "Xanh navy" và "Xanh than" khó phân biệt bằng chữ.

### 4.3 `SizeChartTable`

```text
┌──────────────────────────────────────────┐
│ BẢNG SIZE — Thương hiệu A · Áo           │
│                                          │
│ Size │ Ngực (cm) │ Dài (cm) │ Vai (cm)   │
│ S    │  92–96    │    68    │    42      │
│ M    │  96–100   │    70    │    44      │
│ L    │ 100–104   │    72    │    46      │
│                                          │
│ ℹ Bảng size khác nhau theo thương hiệu   │
└──────────────────────────────────────────┘
```

Hiển thị **số đo thực tế**, không chỉ ký hiệu. Ghi chú nhắc khách rằng size khác nhau giữa các thương hiệu — thông tin quan trọng trên marketplace nhiều thương hiệu.

### 4.4 `OutfitCard`

```text
Hiển thị:
    - Ảnh outfit
    - Danh sách sản phẩm với trạng thái còn hàng
    - Tổng giá cả bộ
    - Nút "Mua cả bộ" và "Chọn từng món"
    - Sản phẩm hết hàng → hiển thị gợi ý thay thế
```

### 4.5 `PriceDisplay`

```text
Nhận: { amount, currency }
Hiển thị: định dạng theo locale

Biến thể:
    - Giá thường
    - Giá có gạch ngang (compare_at_price)
    - Khoảng giá ("từ 299.000đ" khi nhiều offer)
```

**Quan trọng:** component này chỉ **định dạng**, không tính toán. Không có logic "nếu giảm giá thì...". Backend đã tính sẵn.

### 4.6 `SellerBadge`

```text
Hiển thị: tên seller, đánh giá, thời gian giao dự kiến

Với own brand: hiển thị khác biệt (huy hiệu chính hãng)
```

---

## 5. Component trạng thái

```text
OrderStatusBadge        — trạng thái đơn hàng
FulfillmentStatusBadge  — trạng thái lô giao
StockBadge              — còn hàng / sắp hết / hết hàng
SponsoredLabel          — nhãn "Được tài trợ"
```

### `SponsoredLabel` — yêu cầu pháp lý

```text
✓ Luôn hiển thị khi is_sponsored = true
✓ Vị trí dễ thấy, không giấu
✓ Không cho phép tắt qua props
```

Component được thiết kế **không có cách nào ẩn nhãn** — vì đây là nghĩa vụ pháp lý, không phải tùy chọn thiết kế.

---

## 6. Xử lý ảnh

Ảnh là yếu tố quyết định trong thời trang, đồng thời là nguyên nhân chính làm chậm trang.

```text
Component `ProductImage`:
    ✓ Định dạng hiện đại (AVIF/WebP) có dự phòng
    ✓ Nhiều kích thước theo thiết bị
    ✓ Lazy load ảnh ngoài màn hình
    ✓ Placeholder mờ chống nhảy layout (CLS)
    ✓ Alt text bắt buộc (SEO + khả năng tiếp cận)
    ✓ Ảnh chính được ưu tiên tải (preload)
```

**Ràng buộc:** component từ chối render nếu thiếu `alt`. Buộc lập trình viên phải điền — không để trống mặc định.

---

## 7. Khả năng tiếp cận

Đảm bảo ở tầng component để không phụ thuộc vào việc từng lập trình viên có nhớ hay không:

```text
✓ Mọi input có nhãn liên kết
✓ Điều hướng bàn phím đầy đủ
✓ Trạng thái focus rõ ràng
✓ Độ tương phản màu đạt chuẩn (kiểm tra ở tầng token)
✓ Thông báo lỗi liên kết với input tương ứng
✓ Modal bẫy focus và đóng bằng Escape
```

---

## 8. Responsive

```text
Thiết kế mobile-first.

Lý do: phần lớn lưu lượng thương mại thời trang đến từ di động,
       đặc biệt lưu lượng từ nội dung creator (mạng xã hội).
```

Breakpoint được định nghĩa trong design-tokens, không hardcode trong component.

---

## 9. Nguyên tắc phát triển component

| Nguyên tắc | Lý do |
|---|---|
| Không chứa logic nghiệp vụ | P4 — nghiệp vụ ở backend |
| Không gọi API trực tiếp | Component nhận dữ liệu qua props |
| Không dùng giá trị màu/khoảng cách trực tiếp | Luôn qua token |
| Khả năng tiếp cận là mặc định, không phải tùy chọn | Không thể quên |
| Chấp nhận lặp lại đến lần thứ ba mới trừu tượng hóa | P16 |

Quy tắc thứ hai quan trọng: component gọi API sẽ khó test, khó tái sử dụng, và làm mờ ranh giới giữa trình bày và lấy dữ liệu.

---

## 10. Tài liệu liên quan

- [frontend-architecture.md](frontend-architecture.md)
- [storefront.md](storefront.md) — nơi dùng nhiều component thời trang nhất
- [../00-overview/principles.md](../00-overview/principles.md) — P4, P16
