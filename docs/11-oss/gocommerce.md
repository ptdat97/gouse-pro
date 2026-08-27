# GoCommerce (Netlify)

| | |
|---|---|
| Repository | `github.com/netlify/gocommerce` |
| License | MIT |
| Sao / Fork | 1.617 / 216 |
| Cập nhật cuối | 2025-07-11 (bảo trì cầm chừng) |
| Vai trò | Tham chiếu **API headless, thuế/VAT, coupon** |

---

## 1. GoCommerce là gì

API thương mại headless nhỏ gọn, sinh ra để phục vụ trang tĩnh JAMstack. Triết lý: backend thương mại tối giản, frontend hoàn toàn tách rời.

Phạm vi hẹp: order, payment, coupon, thuế, tiền tệ. **Không có** marketplace, không có tồn kho nhiều kho, không có creator.

---

## Năng lực: Kiến trúc headless triệt để

### Cách OSS làm

Backend chỉ cung cấp API JSON. Không template, không session server-side, không giao diện quản trị. Frontend là trang tĩnh gọi API.

### Điểm mạnh

Chứng minh mô hình **frontend hoàn toàn không sở hữu logic nghiệp vụ** hoạt động được trong thực tế.

Điều này xác nhận nguyên tắc P4 của chúng ta: Next.js chỉ trình bày, mọi tính toán ở backend.

### Điểm yếu

Phạm vi quá hẹp cho nhu cầu của chúng ta. Không có gì để học về marketplace hay chuỗi cung ứng.

### Adopt

**Ranh giới nghiêm ngặt giữa API và frontend** — đã có trong [ADR-0002](../adr/0002-api-first.md) và [ADR-0004](../adr/0004-nextjs-frontend.md).

GoCommerce là bằng chứng thực tế cho quyết định này, không phải nguồn ý tưởng mới.

---

## Năng lực: Mô hình thuế

### Cách OSS làm

Thuế tính theo **quốc gia + loại sản phẩm**. Hỗ trợ giá đã gồm thuế (gross) và chưa gồm thuế (net) — khác biệt giữa thị trường EU và Mỹ.

### Điểm mạnh

Nhận diện đúng: thuế không phải một con số cố định mà phụ thuộc **nơi bán, nơi giao, loại hàng**.

### Yêu cầu của chúng ta

Thị trường chính là Việt Nam — VAT đơn giản hơn EU. Nhưng nếu mở rộng quốc tế, mô hình thuế sẽ phức tạp nhanh.

### Adapt

**Thuế là một loại `Adjustment`, không phải trường cố định trên đơn hàng.**

Ý tưởng này lấy từ Sylius (xem [sylius.md](sylius.md)) nhưng GoCommerce xác nhận nhu cầu: thuế phải tách khỏi giá để tính lại được khi đổi nơi giao.

```text
Sai:  order.tax_amount = 30000
Đúng: Adjustment{ type: TAX, amount: 30000, source: "VAT_VN_10", line_id: ... }
```

Lợi ích: hoàn tiền từng phần tính đúng phần thuế tương ứng.

### Quyết định cuối

```text
✓ Thuế là Adjustment gắn vào dòng hàng
✓ MVP: một mức VAT duy nhất, nhưng mô hình sẵn sàng cho nhiều mức
✗ Không làm net/gross hai chiều ở MVP
```

---

## Năng lực: Coupon

### Cách OSS làm

Coupon có mã, phần trăm hoặc số tiền cố định, điều kiện áp dụng, thời hạn.

### Điểm yếu

Không có khái niệm **ai chịu chi phí**. Với cửa hàng một chủ thì không cần; với marketplace thì bắt buộc.

### Yêu cầu của chúng ta

Khách dùng mã giảm 50.000đ cho đơn của Seller A — ai chịu 50.000đ này?

```text
PLATFORM  nền tảng chịu (chiến dịch thu hút khách)
SELLER    seller chịu (khuyến mãi gian hàng)
SHARED    chia theo tỷ lệ thỏa thuận
```

### Adapt

Lấy cấu trúc coupon cơ bản, **bổ sung `cost_bearer`** — đã có trong [04-modules/promotion.md](../04-modules/promotion.md) mục 3.

### Quyết định cuối

```text
✓ Coupon phải có cost_bearer — đóng băng vào OrderLine
✓ Giảm giá phân bổ theo tỷ lệ xuống từng dòng hàng
```

---

## 2. Tổng kết GoCommerce

| Hạng mục | Quyết định |
|---|---|
| Ranh giới API/frontend nghiêm ngặt | **ADOPT** — xác nhận P4 |
| Thuế là Adjustment | **ADAPT** |
| Coupon cơ bản | **ADAPT** — bổ sung `cost_bearer` |
| Phạm vi hẹp (không marketplace) | Không áp dụng |
| Dùng làm nền | **REJECT** — quá hẹp |

**Nhận xét cuối:** GoCommerce chủ yếu **xác nhận** các quyết định đã có, đóng góp ít ý tưởng mới. Giá trị lớn nhất là bằng chứng rằng headless triệt để khả thi.

---

## 3. Tài liệu liên quan

- [../adr/0002-api-first.md](../adr/0002-api-first.md)
- [../04-modules/promotion.md](../04-modules/promotion.md)
- [sylius.md](sylius.md) — mô hình Adjustment đầy đủ hơn
