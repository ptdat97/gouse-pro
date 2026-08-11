# ADR-0004: Next.js cho frontend

**Trạng thái:** Accepted

---

## Context

Cần chọn công nghệ frontend cho bốn ứng dụng có yêu cầu rất khác nhau:

```text
Storefront      — SEO cực quan trọng, tốc độ tải ảnh hưởng doanh số,
                  lưu lượng lớn, chủ yếu di động
Seller Center   — sau đăng nhập, không cần SEO, nhiều biểu mẫu và bảng
Creator Center  — sau đăng nhập, nhiều media
Admin           — sau đăng nhập, nhiều bảng dữ liệu, ít người dùng
```

Yêu cầu đặc thù thương mại thời trang:

```text
- Nhiều ảnh chất lượng cao mỗi sản phẩm (3–8 ảnh)
- Ảnh là yếu tố quyết định mua hàng
- Nhưng ảnh nặng làm chậm trang → giảm chuyển đổi
```

---

## Decision

**Dùng Next.js cho cả bốn ứng dụng**, trong một monorepo, với vai trò **chỉ trình bày**.

```text
apps/
├── storefront/       — SSG/ISR + SSR, tối ưu SEO và tốc độ
├── seller-center/    — chủ yếu CSR
├── creator-center/   — chủ yếu CSR
└── admin/            — chủ yếu CSR

packages/
├── api-client/       — sinh từ OpenAPI
├── ui/               — component cơ bản
├── ui-commerce/      — component thương mại
└── design-tokens/
```

---

## Alternatives

### A. Next.js — **được chọn**

```text
Ưu:
    + Server render + tĩnh hóa → SEO tốt cho storefront
    + Tối ưu ảnh sẵn có (định dạng hiện đại, nhiều kích thước,
      lazy load, placeholder) → RẤT quan trọng với thời trang
    + Chia nhỏ bundle tự động
    + Hệ sinh thái React lớn
    + Một công nghệ cho cả bốn ứng dụng → chia sẻ component

Nhược:
    − Server component CÓ THỂ truy cập database → rủi ro vi phạm ranh giới
    − Phức tạp hơn ứng dụng React thuần
    − Cần hạ tầng chạy Node.js (không chỉ file tĩnh)
```

**Ưu điểm quyết định:** tối ưu ảnh có sẵn. Với thương mại thời trang, đây không phải tiện ích nhỏ — nó là yếu tố ảnh hưởng trực tiếp tới cả trải nghiệm lẫn tốc độ.

### B. React thuần (SPA) — **bị loại**

```text
Ưu:
    + Đơn giản hơn
    + Triển khai file tĩnh
    + Ranh giới rõ ràng: không thể truy cập database

Nhược (quyết định):
    − SEO kém — trang sản phẩm không được đánh chỉ mục tốt
    − Tải ban đầu chậm
    − Phải tự xây tối ưu ảnh
```

**Lý do loại:** SEO là kênh thu hút khách quan trọng cho thương mại. Trang sản phẩm không được đánh chỉ mục nghĩa là mất lưu lượng tự nhiên.

### C. Astro / công nghệ tĩnh hóa mạnh — **bị loại**

```text
Ưu:
    + Rất nhanh, ít JavaScript
    + SEO tuyệt vời

Nhược (quyết định):
    − Kém phù hợp với ứng dụng có nhiều tương tác (checkout, admin)
    − Phải dùng công nghệ khác cho seller/creator/admin
    → hai công nghệ, hai design system, chi phí bảo trì kép
```

### D. Tách công nghệ theo ứng dụng — **bị loại**

```text
Ví dụ: Astro cho storefront, React SPA cho admin

Nhược:
    − Hai hệ thống component
    − Hai quy trình build và triển khai
    − Khó chia sẻ design token
    − Chi phí học tập kép cho lập trình viên
```

---

## Decision kèm theo: Frontend KHÔNG truy cập database

Đây là ràng buộc quan trọng nhất và cần nhấn mạnh vì Next.js **có khả năng kỹ thuật** để vi phạm.

```text
Next.js server component và API route CÓ THỂ truy cập database.
ĐIỀU NÀY BỊ CẤM.
```

### Vì sao cấm

| Lý do | Giải thích |
|---|---|
| Logic trùng lặp | Quy tắc nghiệp vụ dần rò rỉ vào frontend |
| Không dùng lại | App di động không chạy được code Next.js |
| Bảo mật | Thông tin kết nối database ở tầng gần người dùng |
| Không kiểm soát | Bỏ qua phân quyền, giới hạn tốc độ, ghi log backend |
| Không quan sát được | Truy vấn không xuất hiện trong giám sát backend |

### Ví dụ vi phạm tinh vi

```typescript
// ❌ SAI — tính toán ở frontend, dù chỉ là phép cộng
const total = items.reduce((sum, i) => sum + i.price * i.qty, 0);

// ✅ ĐÚNG — backend tính, frontend hiển thị
const { total } = await api.getCart(cartId);
```

Ngay cả phép cộng đơn giản cũng không làm ở frontend — vì quy tắc giảm giá sẽ phức tạp dần, và khi đó hai nơi tính ra hai kết quả khác nhau.

### Thực thi

```text
✓ Không cấu hình thông tin kết nối database cho ứng dụng Next.js
  → về mặt vật lý không kết nối được
✓ Kiểm tra trong CI: không import thư viện database trong apps/
✓ Rà soát code
```

Biện pháp đầu tiên hiệu quả nhất — không có thông tin kết nối thì không thể vi phạm.

---

## Consequences

### Tích cực

```text
✓ SEO tốt cho storefront
✓ Tối ưu ảnh sẵn có — quan trọng với thời trang
✓ Một công nghệ, chia sẻ component và design token
✓ Chiến lược render linh hoạt theo từng trang
```

### Tiêu cực

```text
− Rủi ro vi phạm ranh giới (đã có biện pháp thực thi)
− Cần hạ tầng chạy Node.js
− Phức tạp hơn SPA thuần
```

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Rủi ro frontend truy cập database | SEO, tối ưu ảnh |
| Hạ tầng phức tạp hơn file tĩnh | Server render, ISR |
| Một công nghệ cho mọi ứng dụng | Chia sẻ component, một quy trình build |

---

## Tài liệu liên quan

- [../08-frontend/frontend-architecture.md](../08-frontend/frontend-architecture.md)
- [ADR-0002](0002-api-first.md) — API First là điều kiện tiên quyết
- [../00-overview/principles.md](../00-overview/principles.md) — P4
