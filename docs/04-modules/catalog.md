# Module: Catalog

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | **Core** |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý cây danh mục sản phẩm
- Quản lý thương hiệu và mức độ bảo vệ thương hiệu
- Quản lý bộ sưu tập và mùa vụ
- Quản lý bảng size
- Quản lý giấy ủy quyền thương hiệu của seller

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Bản thân sản phẩm, variant, SKU | `product` |
| Giá | `pricing`, `marketplace` |
| Ai bán | `marketplace` |
| Tồn kho | `inventory` |
| Hồ sơ sản phẩm đang phát triển (tech pack, mẫu) | `supply-chain` |

---

## 3. Khái niệm domain

### 3.1 `Brand`

```text
Brand {
    id, name, slug, logo_url, description
    brand_type          OWN | PARTNER | THIRD_PARTY
    protection_level    OPEN | VERIFIED_ONLY | RESTRICTED
    owner_seller_id     (nullable)
    country_of_origin
    status
}
```

**`protection_level` là trường chống hàng giả:**

```text
OPEN           — seller nào cũng tạo offer được
VERIFIED_ONLY  — chỉ seller có ủy quyền còn hiệu lực
RESTRICTED     — chỉ nền tảng hoặc seller được chỉ định
```

### 3.2 `Collection` — khái niệm hạng nhất của thời trang

```text
Collection {
    id, brand_id, name, season, theme
    launch_date
    end_of_season_date
    status  PLANNING | ACTIVE | ENDING | ARCHIVED
}
```

**Vì sao là entity chứ không phải một cái tag:**

| Nếu là tag | Nếu là entity |
|---|---|
| Không có ngân sách sản xuất | Gắn kế hoạch và ngân sách |
| Không có mốc thời gian | Có ngày ra mắt, ngày kết thúc mùa |
| Không đo được sell-through | Đo tỷ lệ bán hết theo bộ sưu tập |
| Không cảnh báo được trễ tiến độ | Cảnh báo khi sản xuất đe dọa ngày ra mắt |

Thời trang bán theo mùa. Một bộ sưu tập bán không hết trước khi hết mùa mất 50–70% giá trị — đây là rủi ro tài chính lớn nhất của own brand. Vì vậy bộ sưu tập là **đơn vị lập kế hoạch và đo lường**, không phải nhãn phân loại.

### 3.3 `SizeChart` — giảm tỷ lệ hoàn hàng

```text
SizeChart {
    id, brand_id, product_type, system
    entries[]  →  { size: "M", chest_cm: "96-100", length_cm: 70, ... }
}
```

Sai size là nguyên nhân hoàn hàng số một trong thời trang. Bảng size có **số đo thực tế** (không chỉ ký hiệu S/M/L) giúp khách chọn đúng.

**Lưu ý:** bảng size khác nhau theo thương hiệu và theo loại sản phẩm. "M" của brand A không bằng "M" của brand B. Vì vậy `SizeChart` gắn với `(brand_id, product_type)`.

### 3.4 `Category`

Cây phân cấp: `Nữ > Áo > Áo sơ mi`.

Dùng cho duyệt tìm và cho việc xác định tỷ lệ hoa hồng theo ngành hàng.

---

## 4. Dữ liệu sở hữu

```sql
category                -- cây danh mục
brand                   -- thương hiệu
collection              -- bộ sưu tập
size_chart              -- bảng size
size_chart_entry
brand_authorization     -- giấy ủy quyền của seller cho thương hiệu
```

```sql
CREATE TABLE brand_authorization (
    id            UUID PRIMARY KEY,
    brand_id      UUID NOT NULL REFERENCES brand(id),
    seller_id     UUID NOT NULL,
    document_url  TEXT NOT NULL,
    valid_from    DATE NOT NULL,
    valid_until   DATE NOT NULL,
    status        TEXT NOT NULL,
    approved_by   TEXT,
    approved_at   TIMESTAMPTZ
);

CREATE INDEX idx_brand_auth ON brand_authorization (brand_id, seller_id)
    WHERE status = 'APPROVED';
```

Trường `valid_until` quan trọng: ủy quyền hết hạn phải tự động chặn việc tạo offer mới.

---

## 5. Interface công khai

```go
type PublicAPI interface {
    GetCategory(ctx, categoryID string) (*CategoryView, error)
    GetCategoryTree(ctx) (*CategoryTree, error)
    GetCategoryPath(ctx, categoryID string) ([]CategoryView, error)

    GetBrand(ctx, brandID string) (*BrandView, error)
    GetBrandsByIDs(ctx, brandIDs []string) (map[string]BrandView, error)
    IsBrandProtected(ctx, brandID string) (bool, ProtectionLevel, error)
    HasValidAuthorization(ctx, brandID, sellerID string) (bool, error)

    GetCollection(ctx, collectionID string) (*CollectionView, error)
    GetActiveCollections(ctx, brandID string) ([]CollectionView, error)

    GetSizeChart(ctx, sizeChartID string) (*SizeChartView, error)
}
```

---

## 6. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `collection.launched` | Bộ sưu tập ra mắt |
| `collection.ending` | Sắp hết mùa (cảnh báo xả hàng) |
| `brand.protection_changed` | Đổi mức bảo vệ thương hiệu |
| `brand_authorization.expired` | Ủy quyền hết hạn |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `inventory.depleted` | inventory | Cập nhật trạng thái hiển thị danh mục |

---

## 7. Phụ thuộc

```text
Gọi đồng bộ:   (không gọi module nghiệp vụ nào)
Nghe event:    inventory
Được gọi bởi:  product, marketplace, pricing, content, recommendation
```

`catalog` ở tầng dữ liệu chính — được nhiều module gọi nhưng gần như không phụ thuộc ai.

---

## 8. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Ủy quyền hết hạn → chặn tạo offer mới trên thương hiệu bảo vệ |
| 2 | Ngày kết thúc bộ sưu tập phải sau ngày ra mắt |
| 3 | Không xóa danh mục còn sản phẩm |
| 4 | Bảng size gắn với (thương hiệu, loại sản phẩm) |
| 5 | Thương hiệu own brand có `brand_type = OWN` |

---

## 9. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Danh mục, thương hiệu, bảng size cơ bản |
| **Phase 2** | Bộ sưu tập, bảo vệ thương hiệu, ủy quyền seller |
| **Phase 3** | Quản lý mùa vụ, cảnh báo tiến độ bộ sưu tập |

---

## 10. Tài liệu liên quan

- [product.md](product.md) — sản phẩm, variant, SKU
- [../02-domain/entities.md](../02-domain/entities.md) — mô hình sản phẩm thời trang
- [../01-business/own-brand.md](../01-business/own-brand.md) — vòng đời bộ sưu tập
