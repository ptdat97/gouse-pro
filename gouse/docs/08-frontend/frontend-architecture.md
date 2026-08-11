# Kiến trúc Frontend

## 1. Vai trò của Next.js

```text
Next.js = TẦNG TRÌNH BÀY

Làm:      hiển thị, điều hướng, tổng hợp dữ liệu để render,
          trạng thái giao diện, kiểm tra form (trải nghiệm)

KHÔNG làm: logic nghiệp vụ, tính giá, quyết định trạng thái,
          truy cập database, phân quyền thật
```

Đây là nguyên tắc P4 tại [../00-overview/principles.md](../00-overview/principles.md).

---

## 2. Ranh giới nghiêm ngặt nhất

> **Next.js KHÔNG BAO GIỜ truy cập database.**

Next.js server components và API routes **có thể** truy cập database về mặt kỹ thuật. **Điều này bị cấm.**

### Vì sao cấm

| Lý do | Giải thích |
|---|---|
| Logic trùng lặp | Quy tắc nghiệp vụ sẽ dần rò rỉ vào frontend |
| Không dùng lại được | App di động không chạy được code Next.js |
| Bảo mật | Thông tin kết nối database ở tầng gần người dùng |
| Không kiểm soát | Không qua phân quyền, giới hạn tốc độ, ghi log của backend |
| Không quan sát được | Truy vấn không xuất hiện trong giám sát backend |

### Ví dụ cụ thể

```typescript
// ❌ SAI — tính toán ở frontend
const total = items.reduce((sum, i) => sum + i.price * i.qty, 0);
const discount = total > 500000 ? total * 0.1 : 0;
const finalTotal = total - discount + shippingFee;

// ✅ ĐÚNG — hiển thị số backend trả về
const { subtotal, discount, shippingFee, total } = await api.getCart(cartId);
```

Ngay cả phép cộng đơn giản cũng không làm ở frontend — vì quy tắc giảm giá sẽ phức tạp dần, và khi đó hai nơi tính ra hai kết quả khác nhau.

---

## 3. Bốn ứng dụng

```text
apps/
├── storefront/       — khách hàng mua sắm
├── seller-center/    — nhà bán quản lý gian hàng
├── creator-center/   — creator quản lý nội dung và thu nhập
└── admin/            — nhân viên vận hành
```

### Vì sao tách bốn ứng dụng, không gộp một

| Yếu tố | Storefront | Seller/Creator/Admin |
|---|---|---|
| Người dùng | Công chúng, rất đông | Nội bộ, ít |
| Yêu cầu tốc độ tải | Cực cao (ảnh hưởng doanh số) | Vừa phải |
| SEO | Rất quan trọng | Không cần |
| Render | Server-side, tĩnh hóa | Client-side chủ yếu |
| Kích thước bundle | Phải nhỏ | Có thể lớn hơn |
| Tần suất thay đổi | Cao | Vừa |

Gộp chung sẽ khiến storefront phải tải cả code của admin — làm chậm trang bán hàng, ảnh hưởng trực tiếp doanh số.

---

## 4. Cấu trúc monorepo

```text
/apps
    /storefront
    /seller-center
    /creator-center
    /admin

/packages
    /api-client        — sinh từ OpenAPI, dùng chung
    /ui                — component dùng chung (button, input, modal)
    /design-tokens     — màu, khoảng cách, typography
    /utils             — tiện ích thuần (định dạng ngày, tiền)
    /types             — kiểu TypeScript sinh từ OpenAPI
```

### Quy tắc packages

```text
✓ packages/ chỉ chứa code KHÔNG có logic nghiệp vụ
✗ KHÔNG có "tính giá", "kiểm tra seller có được bán không"
✓ Định dạng tiền để hiển thị: được (thuần trình bày)
✗ Tính tổng tiền: không (nghiệp vụ)
```

---

## 5. Chiến lược render

```text
Trang tĩnh (SSG/ISR):
    Trang chủ, trang danh mục, trang thương hiệu
    → nội dung ít đổi, cần SEO và tốc độ

Server render (SSR):
    Trang sản phẩm, kết quả tìm kiếm
    → cần SEO, nội dung động (giá, tồn kho)

Client render (CSR):
    Giỏ hàng, checkout, tài khoản
    → cá nhân hóa, không cần SEO

Seller/Creator/Admin:
    Chủ yếu CSR
    → sau đăng nhập, không cần SEO
```

### Lưu ý về dữ liệu thay đổi nhanh

```text
Trang sản phẩm render server nhưng:
    - GIÁ và TỒN KHO có thể đã cũ
    - Phải fetch lại ở client hoặc dùng thời gian cache rất ngắn
    - Khi thêm giỏ, backend LUÔN kiểm tra lại
```

Hiển thị "còn hàng" là thông tin tham khảo, không phải cam kết. Cam kết chỉ có ở bước giữ tồn kho trong checkout.

---

## 6. Gọi API

```text
packages/api-client sinh từ /api/openapi.yaml

Lợi ích:
    ✓ Kiểu TypeScript tự động, khớp với backend
    ✓ Backend đổi hợp đồng → biên dịch frontend lỗi ngay
    ✓ Không viết tay kiểu dữ liệu
```

### Xử lý lỗi thống nhất

```typescript
// Mọi lỗi API có cấu trúc giống nhau
try {
  await api.addToCart({ offer_id, quantity });
} catch (e) {
  if (isApiError(e)) {
    switch (e.code) {
      case 'INSUFFICIENT_INVENTORY':
        // dùng e.details.available để hiển thị hữu ích
        showError(`Chỉ còn ${e.details.available} sản phẩm`);
        break;
      case 'OFFER_NOT_AVAILABLE':
        showError('Sản phẩm không còn bán');
        break;
      default:
        showError(e.message);
    }
  }
}
```

**Nguyên tắc:** xử lý theo `code` (máy đọc được), không parse `message` (có thể đổi, có thể đa ngôn ngữ).

---

## 7. Quản lý trạng thái

```text
Trạng thái server (dữ liệu từ API):
    → thư viện quản lý fetch/cache (React Query hoặc tương đương)
    → tự động cache, làm mới, thử lại

Trạng thái giao diện (modal mở, tab đang chọn):
    → state cục bộ của component

Trạng thái toàn cục (thông tin đăng nhập, giỏ hàng tóm tắt):
    → context, giữ tối thiểu
```

**Nguyên tắc:** không sao chép dữ liệu server vào store toàn cục rồi tự đồng bộ. Đó là nguồn gốc của lỗi dữ liệu cũ.

---

## 8. Xác thực ở frontend

```text
Lưu token:
    Access token  → bộ nhớ (không localStorage — chống XSS)
    Refresh token → httpOnly cookie

Hiển thị theo vai trò:
    → CHỈ để trải nghiệm, KHÔNG phải bảo mật
    → backend LUÔN kiểm tra lại
```

### Ví dụ

```typescript
// Ẩn nút xóa nếu không có quyền — CHỈ LÀ GIAO DIỆN
{user.can('product.delete') && <DeleteButton />}

// Backend VẪN phải kiểm tra khi nhận request xóa
// Người dùng có thể gọi API trực tiếp, bỏ qua giao diện
```

---

## 9. Hiệu năng — quan trọng với storefront

| Chỉ số | Mục tiêu |
|---|---|
| Largest Contentful Paint | < 2,5s |
| Interaction to Next Paint | < 200ms |
| Cumulative Layout Shift | < 0,1 |
| Kích thước JS ban đầu | < 200KB nén |

### Đặc thù thương mại thời trang: ảnh

```text
Ảnh là yếu tố quyết định trong thời trang:
    - Nhiều ảnh mỗi sản phẩm (3–8 ảnh)
    - Ảnh chất lượng cao (khách cần thấy chi tiết chất liệu)
    - Ảnh thay đổi theo màu variant

Yêu cầu:
    ✓ Định dạng hiện đại (AVIF/WebP) có dự phòng
    ✓ Kích thước phù hợp thiết bị (responsive)
    ✓ Tải chậm (lazy) ảnh ngoài màn hình
    ✓ Ảnh mờ placeholder tránh nhảy layout
    ✓ Ưu tiên tải ảnh chính của sản phẩm
```

Ảnh nặng làm chậm trang → giảm tỷ lệ chuyển đổi. Đây là đánh đổi trực tiếp giữa chất lượng hình ảnh và tốc độ.

---

## 10. Đa ngôn ngữ và tiền tệ

```text
Ngôn ngữ:
    - Chuỗi giao diện: file dịch ở frontend
    - Nội dung (tên sản phẩm, mô tả): backend trả về theo Accept-Language

Tiền tệ:
    - Backend LUÔN trả { amount, currency }
    - Frontend chỉ ĐỊNH DẠNG hiển thị
    - KHÔNG BAO GIỜ quy đổi tiền tệ ở frontend
```

Quy đổi tiền tệ là nghiệp vụ (cần tỷ giá tại thời điểm giao dịch), không phải trình bày.

---

## 11. Khả năng tiếp cận (accessibility)

```text
Bắt buộc tối thiểu:
    ✓ HTML ngữ nghĩa
    ✓ Điều hướng bằng bàn phím
    ✓ Nhãn cho mọi input
    ✓ Alt text cho ảnh sản phẩm
    ✓ Độ tương phản màu đạt chuẩn
    ✓ Trạng thái focus rõ ràng
```

Với thương mại thời trang, `alt` cho ảnh sản phẩm còn có lợi ích SEO trực tiếp.

---

## 12. Tài liệu liên quan

- [storefront.md](storefront.md), [seller-center.md](seller-center.md), [creator-center.md](creator-center.md), [admin.md](admin.md)
- [design-system.md](design-system.md)
- [../03-architecture/api-first.md](../03-architecture/api-first.md)
- [../adr/0004-nextjs-frontend.md](../adr/0004-nextjs-frontend.md)
