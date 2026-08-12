# Vendure

| | |
|---|---|
| Repository | `github.com/vendurehq/vendure` |
| License | **GPLv3** (Community) hoặc giấy phép thương mại |
| Sao / Fork | 8.319 / 1.461 |
| Ngôn ngữ | TypeScript |
| Cập nhật cuối | 2026-08-11 (tích cực) |
| Vai trò | Tham chiếu **Channel, plugin, event** — **CHỈ ĐỌC Ý TƯỞNG** |

---

## 1. Cảnh báo license — nghiêm trọng

**Vendure Community Edition là GPLv3.**

```text
GPLv3 là copyleft mạnh:
  ✗ KHÔNG sao chép code vào sản phẩm độc quyền
  ✗ KHÔNG sao chép cấu trúc file/lớp một cách trực tiếp
  ✗ Nếu dùng, TOÀN BỘ sản phẩm phải mở nguồn theo GPLv3

Được phép:
  ✓ Đọc để hiểu ý tưởng
  ✓ Học khái niệm kiến trúc rồi tự cài đặt từ đầu
```

**Ý tưởng và kiến trúc không bị bảo hộ bản quyền — chỉ mã nguồn cụ thể mới bị.** Học "Channel dùng để phân tách nhiều gian hàng" là hợp pháp; sao chép lớp `ChannelService` của họ thì không.

Mọi ghi chép trong tài liệu này là **mô tả khái niệm bằng lời**, không trích code.

---

## Năng lực: Channel — và vì sao nó KHÔNG phải mô hình marketplace của chúng ta

Đây là phân tích quan trọng nhất của file này.

### Cách OSS làm

`Channel` là môi trường bán hàng độc lập trong một hệ thống. Nhiều thực thể "channel-aware": Product, ProductVariant, Order, Customer, Collection, Asset, Promotion, ShippingMethod, PaymentMethod, StockLocation.

Giá **không** nằm trên Product. `ProductVariant` có quan hệ một-nhiều với `ProductVariantPrice`; mỗi channel mà variant được gán vào cần ít nhất một bản ghi giá.

Mỗi Channel gán cho **một** `Seller`. Marketplace được xây bằng cách: mỗi seller một channel.

Client chọn channel bằng header `vendure-token`.

### Điểm mạnh

- Giải quyết tốt đa thị trường, đa tiền tệ, đa vùng
- Tách giá khỏi sản phẩm — đúng hướng
- Một hệ thống phục vụ nhiều mặt tiền

### Điểm yếu với bài toán của chúng ta

**Mô hình "một channel một seller" giải quyết bài toán KHÁC với bài toán của chúng ta.**

```text
Vendure (channel-per-seller):
    Seller A có gian hàng riêng, danh mục riêng
    Seller B có gian hàng riêng, danh mục riêng
    → Khách vào gian hàng A hoặc gian hàng B
    → Giống mô hình "nhiều cửa hàng độc lập trên một nền tảng"

Chúng ta (offer-per-seller):
    MỘT trang sản phẩm "Áo thun cotton mã 450251"
    ├── Seller A → 299.000đ, còn 12, giao 2 ngày
    ├── Seller B → 289.000đ, còn 3,  giao 4 ngày
    └── Seller C → 310.000đ, còn 50, giao 1 ngày
    → Khách so sánh và chọn nhà bán
    → Cần buy box, cần cạnh tranh giá
```

### Vì sao khác biệt này quyết định

Nếu dùng mô hình channel-per-seller, ba năng lực cốt lõi của chúng ta **không làm được**:

```text
1. So sánh giá giữa các nhà bán trên cùng một trang
   → channel tách biệt, khách không thấy được cùng lúc

2. Buy box — chọn offer mặc định theo giá + chất lượng phục vụ
   → không có khái niệm "nhiều lựa chọn cho cùng một SKU"

3. Gộp nhiều seller vào MỘT giỏ hàng và MỘT đơn
   → Vendure: order thuộc về một channel
   → chúng ta: một Order, nhiều FulfillmentOrder theo seller
```

Điểm thứ ba là điều kiện tiên quyết cho trải nghiệm mua sắm của chúng ta — khách mua áo own brand + giày Seller A + túi Seller B trong một lần thanh toán.

### Yêu cầu của chúng ta

Xem [ADR-0007](../adr/0007-marketplace-order-model.md): `Offer` tách khỏi `Product`, một `Order` chứa nhiều `FulfillmentOrder`.

### Adopt

**Tách giá khỏi sản phẩm.** Vendure đúng ở điểm này — giá không thuộc về Product mà thuộc về "ngữ cảnh bán".

Với Vendure, ngữ cảnh là Channel. Với chúng ta, ngữ cảnh là **Offer** (seller + SKU).

### Adapt

**Dùng Channel cho việc nó giỏi, không dùng cho marketplace.**

Nếu sau này mở rộng đa quốc gia hoặc đa tiền tệ, khái niệm channel/market sẽ hữu ích cho:

```text
Giá theo thị trường
Tiền tệ theo thị trường
Danh mục khả dụng theo vùng
Thuế theo vùng
```

Nhưng đó là **Phase 4**, và không liên quan tới marketplace.

### Reject

```text
✗ Channel-per-seller làm mô hình marketplace
✗ Order thuộc về một channel duy nhất
```

### Quyết định cuối

```text
✓ Giữ mô hình Offer — ADR-0007 đúng, không sửa
✓ Ghi nhận Channel là phương án cho đa thị trường (Phase 4)
✗ REJECT channel-per-seller cho marketplace
```

---

## Năng lực: Event bus và plugin

### Cách OSS làm

Event bus nội bộ; plugin đăng ký handler cho event. Plugin có thể mở rộng entity (custom field), thêm API, thêm chiến lược.

Dùng nhiều **strategy pattern** cho các điểm tùy biến.

### Điểm mạnh

Hệ sinh thái mở rộng phong phú mà không sửa code lõi.

### Điểm yếu với chúng ta

**Plugin system là chi phí kiến trúc rất lớn:**

```text
Phải định nghĩa và duy trì điểm mở rộng ổn định
Phải quản lý phiên bản API plugin
Phải xử lý xung đột giữa các plugin
Phải kiểm soát bảo mật code bên thứ ba
```

Chi phí này chỉ đáng khi bạn **bán framework** cho người khác dùng. Chúng ta xây nền tảng cho **chính mình**.

### Adopt

**Event bus nội bộ** — đã có trong [ADR-0006](../adr/0006-internal-events.md) với outbox pattern.

**Strategy pattern cho điểm tùy biến** — chúng ta gọi là port/adapter, cùng ý tưởng: `PaymentGateway`, `ShippingProvider`, `RecommendationEngine`.

### Reject

**Plugin system.** Nguyên tắc P15: mỗi thứ đưa vào phải giải thích được vì sao cần cho **chính** nghiệp vụ này. Chúng ta không có nhu cầu bên thứ ba mở rộng.

### Quyết định cuối

```text
✓ Event bus — đã có
✓ Port/adapter cho điểm tùy biến — đã có
✗ REJECT plugin system
```

---

## Năng lực: Order state machine

### Cách OSS làm

Đơn hàng đi qua các trạng thái xác định, chuyển đổi được kiểm soát. Có thể tùy biến bằng cách thêm trạng thái.

### Yêu cầu của chúng ta

Chúng ta có `Order` với trạng thái tổng hợp **suy ra** từ các `FulfillmentOrder`.

### Adapt

Sylius có mô hình state machine tốt hơn (tách theo mối quan tâm) — xem [sylius.md](sylius.md).

Điều lấy từ Vendure: **trạng thái đơn phải kiểm soát chuyển đổi**, không cho phép gán tùy tiện. Đã có trong thiết kế.

---

## 2. Tổng kết Vendure

| Hạng mục | Quyết định |
|---|---|
| Tách giá khỏi sản phẩm | **ADOPT** — qua Offer |
| Channel cho đa thị trường | **ADAPT** — Phase 4 |
| **Channel-per-seller làm marketplace** | **REJECT** — không giải được bài toán của chúng ta |
| Event bus | **ADOPT** — đã có |
| Strategy pattern | **ADOPT** — qua port/adapter |
| Plugin system | **REJECT** — chi phí không tương xứng |
| Sao chép code | **CẤM** — GPLv3 |

**Nhận xét cuối:** Vendure có kiến trúc tốt cho bài toán **đa gian hàng độc lập**. Bài toán của chúng ta là **nhiều nhà bán cạnh tranh trên cùng một sản phẩm** — khác về bản chất.

Đây là ví dụ điển hình vì sao nguyên tắc "OSS không được định đoạt domain" quan trọng: nếu chọn Vendure làm nền, chúng ta sẽ bị ép vào mô hình marketplace sai và phát hiện quá muộn.

---

## 3. Tài liệu liên quan

- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md)
- [../01-business/marketplace.md](../01-business/marketplace.md) mục 2
- [saleor.md](saleor.md) — mô hình channel listing tinh tế hơn
