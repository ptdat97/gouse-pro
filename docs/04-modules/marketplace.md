# Module: Marketplace

| | |
|---|---|
| **Bounded Context** | Marketplace |
| **Phân loại** | **Core** |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý `Offer` — lời chào bán của một seller cho một SKU
- Quyết định buy box (offer nào hiển thị mặc định)
- Định nghĩa quy tắc hoa hồng
- Kiểm soát quyền bán theo thương hiệu được bảo vệ
- Theo dõi cạnh tranh giá giữa các offer

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Hồ sơ, giấy tờ, chính sách seller | `seller` |
| Thông tin sản phẩm | `product`, `catalog` |
| Số lượng tồn kho thật | `inventory` |
| Ghi sổ hoa hồng | `payment` |
| Đóng băng hoa hồng vào đơn | `order` |

**Phân vai về hoa hồng** — ba module, ba vai trò khác nhau:

```text
marketplace  → ĐỊNH NGHĨA quy tắc: "ngành áo, seller loại B, tỷ lệ 10%"
order        → ĐÓNG BĂNG vào OrderLine tại thời điểm đặt hàng
payment      → GHI SỔ vào ledger
```

Không có hai module cùng làm một việc.

---

## 3. Khái niệm domain

### Aggregate: `Offer`

```text
Offer {
    id
    sku_id
    seller_id
    price                   — Money
    compare_at_price
    condition               NEW | USED_LIKE_NEW | USED_GOOD
    handling_time_hours
    min/max_order_quantity
    status  DRAFT | ACTIVE | OUT_OF_STOCK | SUSPENDED | ARCHIVED
    version
}
```

**Bất biến:**
- Một seller chỉ có **một** offer ACTIVE cho một SKU
- `price` > 0
- Nếu brand có `protection_level = VERIFIED_ONLY`, seller phải có ủy quyền còn hiệu lực

### Vì sao Offer thuộc module này chứ không thuộc `product`

Đã phân tích tại [../02-domain/aggregates.md](../02-domain/aggregates.md) mục 3.2. Tóm tắt:

```text
- Một Product có thể có hàng trăm Offer → aggregate quá lớn nếu gộp
- Seller sửa offer rất thường xuyên → tranh chấp ghi nếu gộp
- Chủ sở hữu khác nhau: Product thuộc danh mục chuẩn, Offer thuộc seller
```

### Vì sao Offer không chứa số lượng tồn kho

```text
Offer.status = OUT_OF_STOCK là dữ liệu DẪN XUẤT, cập nhật qua event.
Nguồn sự thật về số lượng là InventoryItem.

Lý do:
    - Một offer có thể được phục vụ từ nhiều kho
    - Tồn kho thay đổi tần suất rất cao, không nên làm bẩn aggregate Offer
    - Tránh hai nơi cùng lưu một sự thật
```

---

## 4. Buy Box

Khi một SKU có nhiều offer, phải chọn một offer hiển thị mặc định.

### Công thức đề xuất

```text
Điểm = w1 × điểm_giá            (giá đã gồm phí ship, thấp hơn = cao điểm)
     + w2 × điểm_thời_gian_giao
     + w3 × điểm_hiệu_suất_seller
     + w4 × điểm_độ_tin_cậy_tồn_kho
     + w5 × điểm_đánh_giá_khách

Ràng buộc bắt buộc:
    - Offer phải còn hàng
    - Seller phải ACTIVE
    - Offer phải ACTIVE
```

### Nguyên tắc thiết kế

**Công thức phải tường minh và công khai với seller.**

Lý do: seller cần hiểu vì sao mình không thắng buy box và làm gì để cải thiện. Một mô hình hộp đen tạo tranh chấp không giải quyết được và cảm giác bất công — dẫn tới seller rời nền tảng.

**Cảnh báo về cạnh tranh giá:** nếu buy box chỉ dựa vào giá thấp nhất, seller đua giảm giá tới mức không bền vững và cắt giảm chất lượng dịch vụ. Trọng số phải cân bằng giá và chất lượng phục vụ.

Đây là ứng dụng nguyên tắc P14: quy tắc đơn giản, giải thích được, để dành chỗ nâng cấp sau.

---

## 5. Kiểm soát thương hiệu được bảo vệ

Rủi ro hàng giả là rủi ro sống còn của marketplace thời trang.

```text
Khi seller tạo offer:

    Lấy brand của SKU
        ↓
    brand.protection_level = ?
        │
        ├── OPEN           → cho phép
        │
        ├── VERIFIED_ONLY  → kiểm tra seller có brand_authorization
        │                     còn hiệu lực không
        │                     → không có: TỪ CHỐI
        │
        └── RESTRICTED     → chỉ seller trong danh sách được chỉ định
```

**Đây là quy tắc domain bắt buộc**, không phải quy trình thủ công bên ngoài hệ thống. Cài đặt trong `CanSellerCreateOffer`.

---

## 6. Dữ liệu sở hữu

```sql
offer                   -- lời chào bán
offer_price_history     -- lịch sử thay đổi giá
buy_box_snapshot        -- ghi nhận offer thắng buy box theo thời điểm
commission_rule         -- quy tắc hoa hồng
```

```sql
CREATE TABLE offer (
    id                   UUID PRIMARY KEY,
    sku_id               UUID NOT NULL,
    seller_id            UUID NOT NULL,
    price_amount         BIGINT NOT NULL CHECK (price_amount > 0),
    price_currency       CHAR(3) NOT NULL,
    compare_at_amount    BIGINT,
    condition            TEXT NOT NULL DEFAULT 'NEW',
    handling_time_hours  INT NOT NULL DEFAULT 24,
    status               TEXT NOT NULL,
    version              BIGINT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Một seller chỉ có một offer ACTIVE cho một SKU
CREATE UNIQUE INDEX idx_offer_unique_active
    ON offer (sku_id, seller_id) WHERE status = 'ACTIVE';

CREATE INDEX idx_offer_sku ON offer (sku_id) WHERE status = 'ACTIVE';
CREATE INDEX idx_offer_seller ON offer (seller_id);
```

Chỉ mục duy nhất có điều kiện thực thi bất biến "một seller một offer active cho một SKU" ở tầng database.

`offer_price_history` cần cho việc phát hiện thao túng giá (tăng giá rồi giảm để giả vờ khuyến mãi) và cho phân tích cạnh tranh.

---

## 7. Interface công khai

```go
type PublicAPI interface {
    // Offer
    GetOffer(ctx, offerID string) (*OfferView, error)
    GetOffersBySKU(ctx, skuID string) ([]OfferView, error)
    GetOffersBySKUs(ctx, skuIDs []string) (map[string][]OfferView, error)
    GetBuyBoxOffer(ctx, skuID string) (*OfferView, error)
    GetBuyBoxOffers(ctx, skuIDs []string) (map[string]OfferView, error)
    GetOffersBySeller(ctx, sellerID string, page Pagination) (*OfferList, error)

    // Hoa hồng — chỉ trả TỶ LỆ, không tính số tiền
    GetCommissionRate(ctx, req CommissionRateRequest) (Percentage, error)

    // Kiểm tra quyền
    CanSellerCreateOffer(ctx, sellerID, skuID string) (bool, string, error)
}

type CommissionRateRequest struct {
    SellerID   string
    CategoryID string
    CampaignID string  // tùy chọn
}
```

Mọi phương thức truy vấn đều có phiên bản **theo lô** để tránh N+1.

---

## 8. Use case

| Use case | Ai gọi |
|---|---|
| `CreateOffer` | Seller Center |
| `UpdateOfferPrice` | Seller Center |
| `SuspendOffer` | Admin, hệ thống (khi seller bị đình chỉ) |
| `CalculateBuyBox` | Nội bộ, chạy khi có thay đổi giá/tồn kho |
| `GetCommissionRate` | order (khi đặt hàng) |
| `ValidateBrandAuthorization` | Nội bộ khi tạo offer |

---

## 9. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `offer.created` | Seller tạo offer |
| `offer.price_changed` | Đổi giá |
| `offer.out_of_stock` | Chuyển sang hết hàng |
| `offer.suspended` | Bị đình chỉ |
| `buy_box.changed` | Offer thắng buy box thay đổi |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `inventory.depleted` | inventory | Chuyển offer sang OUT_OF_STOCK |
| `inventory.received` | inventory | Chuyển offer về ACTIVE nếu đang hết hàng |
| `seller.suspended` | seller | Ẩn toàn bộ offer của seller |
| `seller.reactivated` | seller | Khôi phục offer |
| `product.unpublished` | product | Ẩn offer liên quan |

---

## 10. Phụ thuộc

```text
Gọi đồng bộ:   catalog (kiểm tra brand protection)
               product (kiểm tra SKU tồn tại)
               seller  (kiểm tra trạng thái seller)
Nghe event:    inventory, seller, product
Được gọi bởi:  cart, checkout, order, product (hiển thị)
```

---

## 11. Quy tắc nghiệp vụ quan trọng

| # | Quy tắc |
|---|---|
| 1 | Một seller chỉ có một offer ACTIVE cho một SKU |
| 2 | Giá phải > 0 |
| 3 | Thương hiệu được bảo vệ cần ủy quyền còn hiệu lực |
| 4 | Seller bị đình chỉ → mọi offer ẩn |
| 5 | Lưu lịch sử mọi lần đổi giá |
| 6 | Buy box chỉ chọn offer còn hàng, seller active |
| 7 | Offer không lưu số lượng tồn kho |
| 8 | `GetCommissionRate` chỉ trả tỷ lệ, không tính tiền |

---

## 12. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Offer cơ bản, buy box đơn giản (giá + hiệu suất), hoa hồng theo ngành hàng |
| **Phase 2** | Buy box đầy đủ, hoa hồng theo chiến dịch, kiểm soát thương hiệu |
| **Phase 3** | Phân tích cạnh tranh giá, gợi ý giá cho seller |
| **Phase 4** | Retail media, vị trí được tài trợ |

---

## 13. Tài liệu liên quan

- [../01-business/marketplace.md](../01-business/marketplace.md) — nghiệp vụ marketplace
- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md) — quyết định về Offer
- [seller.md](seller.md) — quản lý nhà bán
