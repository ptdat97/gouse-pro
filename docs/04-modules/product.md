# Module: Product

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | **Core** |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý `Product`, `Variant`, `SKU`
- Quản lý thuộc tính, hình ảnh sản phẩm
- Quy trình duyệt và xuất bản sản phẩm
- Tiếp nhận sản phẩm own brand từ Supply Chain (qua ACL)

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Thương hiệu, danh mục, bộ sưu tập | `catalog` |
| Giá | `pricing`, `marketplace` |
| Ai bán, offer | `marketplace` |
| Tồn kho | `inventory` |
| Tech pack, mẫu, giá vốn | `supply-chain` |

---

## 3. Chuỗi Product → Variant → SKU

```text
Product        "Áo sơ mi linen Oxford"
   │            — cái khách nhìn thấy: một trang, một bộ ảnh, một mô tả
   │
Variant        "Màu Trắng, Size M"
   │            — tổ hợp thuộc tính; ảnh thay đổi theo màu
   │
SKU            "SM-LIN-OXF-WHT-M"
                — đơn vị lưu kho, mã vạch, đơn vị nhà máy sản xuất
```

### Vì sao cần cả ba tầng

| Câu hỏi thực tế | Tầng trả lời |
|---|---|
| "Áo này có màu gì?" | Variant |
| "Màu trắng còn size nào?" | SKU + Inventory |
| "Khách xem trang nào?" | Product |
| "Nhà máy sản xuất cái gì?" | SKU |
| "Mã vạch quét ra cái gì?" | SKU |

**SKU là định danh hàng hóa chung**, không thuộc về seller nào. Đây là cái cho phép biết "ba seller đang bán cùng một món hàng" — nền tảng của mô hình Offer.

---

## 4. Trường đặc thù thời trang

```text
Product {
    ...
    care_instructions       — hướng dẫn giặt là
    material_composition    — "80% cotton, 20% linen"
    origin_country
    product_type            TOP | BOTTOM | DRESS | OUTERWEAR | SHOES | BAG | ACCESSORY
    gender_target           MEN | WOMEN | UNISEX | KIDS
    size_chart_id           — bảng size áp dụng
}
```

Các trường này **không phải tùy chọn** — chúng ảnh hưởng trực tiếp tỷ lệ hoàn hàng:

```text
Không có material_composition  → khách mua nhầm chất liệu, không vừa ý
Không có size_chart_id         → khách chọn sai size
Không có care_instructions     → khách giặt hỏng, khiếu nại
```

---

## 5. Anti-Corruption Layer với Supply Chain

Đây là điểm tích hợp quan trọng cho luồng own brand.

```text
Supply Chain Context              Catalog/Product Context
────────────────────              ───────────────────────
ProductDevelopment                Product
  - concept                         - tên hiển thị
  - tech pack                       - mô tả marketing
  - bill of materials               - hình ảnh
  - costing (giá vốn)               - thuộc tính tìm kiếm
  - sample                          - variant, SKU
  - supplier
        │
        │  khi mẫu được duyệt
        ▼
   event product_development.approved
        │
        ▼
   ACL: CreateProductFromDevelopment()
        │
        │  CHỈ lấy: tên, loại, thuộc tính, danh sách SKU
        │  BỎ QUA: tech pack, giá vốn, nhà cung cấp
        ▼
   Product (trạng thái DRAFT)
```

**Vì sao cần ACL:** Catalog **không được biết** về tech pack, giá vốn, nhà cung cấp. Nếu những khái niệm này rò rỉ sang, mô hình catalog bị ô nhiễm bởi khái niệm sản xuất và hai context bị ghép chặt.

Xem [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md) mục 5.

---

## 6. Quy trình xuất bản sản phẩm

```text
    DRAFT
      │  (seller/nhân viên hoàn thiện thông tin)
      ▼
    PENDING_REVIEW
      │
      ├── Kiểm tra tự động:
      │     - đủ ảnh, đủ mô tả
      │     - có bảng size
      │     - thuộc thương hiệu được phép
      │     - không dùng từ ngữ cấm
      │
      ├── Kiểm tra thủ công (với thương hiệu bảo vệ / seller mới)
      │
      ├──→ REJECTED (có lý do cụ thể)
      │
      ▼
    ACTIVE
      │
      ├──→ INACTIVE (tạm ngừng bán)
      └──→ ARCHIVED (ngừng vĩnh viễn)
```

**Lưu ý:** sản phẩm `ACTIVE` chưa chắc bán được — còn cần có ít nhất một `Offer` active và còn hàng. Đây là hai khái niệm riêng.

---

## 7. Chống trùng lặp sản phẩm

Vấn đề: nhiều seller đăng cùng một hàng nhưng tạo product mới thay vì gắn offer vào product có sẵn.

```text
Cơ chế:
1. Đối sánh khi đăng bán
   → gợi ý sản phẩm đã có dựa trên mã sản phẩm, tên, thương hiệu

2. Danh mục chuẩn hóa
   → với thương hiệu lớn, nền tảng tạo sẵn product chuẩn
   → seller chỉ tạo offer, không tạo product

3. Quy trình gộp
   → admin gộp product trùng, chuyển offer sang product chuẩn
```

**Yêu cầu của quy trình gộp:** phải giữ được lịch sử — đơn hàng cũ vẫn trỏ đúng, đánh giá phải được chuyển theo. Không được xóa product bị gộp, chỉ đánh dấu và chuyển hướng.

---

## 8. Dữ liệu sở hữu

```sql
product
variant
sku
product_attribute
product_image
product_merge_log       -- lịch sử gộp sản phẩm trùng
```

```sql
CREATE TABLE sku (
    id            UUID PRIMARY KEY,
    variant_id    UUID NOT NULL REFERENCES variant(id),
    sku_code      TEXT NOT NULL UNIQUE,
    barcode       TEXT,
    weight_gram   INT,
    length_mm     INT,
    width_mm      INT,
    height_mm     INT,
    status        TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Kích thước và trọng lượng cần cho tính phí vận chuyển và xếp kho — không phải thông tin phụ.

---

## 9. Interface công khai

```go
type PublicAPI interface {
    GetProduct(ctx, productID string) (*ProductView, error)
    GetProductsByIDs(ctx, productIDs []string) (map[string]ProductView, error)
    SearchProducts(ctx, req SearchRequest) (*ProductSearchResult, error)

    GetVariant(ctx, variantID string) (*VariantView, error)
    GetVariantsByProduct(ctx, productID string) ([]VariantView, error)

    GetSKU(ctx, skuID string) (*SKUView, error)
    GetSKUsByIDs(ctx, skuIDs []string) (map[string]SKUView, error)
    GetSKUsByProduct(ctx, productID string) ([]SKUView, error)

    // ACL cho own brand
    CreateProductFromDevelopment(ctx, req CreateFromDevelopmentRequest) (*ProductView, error)
}
```

---

## 10. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `product.created` | Tạo sản phẩm |
| `product.published` | Xuất bản |
| `product.unpublished` | Ngừng bán |
| `product.merged` | Gộp sản phẩm trùng |
| `sku.created` | Tạo SKU mới |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `product_development.approved` | supply-chain | Tạo Product qua ACL |
| `inventory.depleted` | inventory | Cập nhật trạng thái hiển thị |

---

## 11. Phụ thuộc

```text
Gọi đồng bộ:   catalog (thương hiệu, danh mục, bảng size)
Nghe event:    supply-chain, inventory
Được gọi bởi:  marketplace, cart, checkout, content, recommendation
```

---

## 12. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | `sku_code` duy nhất toàn hệ thống |
| 2 | Không trùng tổ hợp thuộc tính trong một Product |
| 3 | Product ACTIVE phải có đủ ảnh và mô tả |
| 4 | Sản phẩm thời trang phải có bảng size |
| 5 | Không xóa product đã có đơn hàng |
| 6 | Gộp sản phẩm phải giữ lịch sử |
| 7 | ACL không để khái niệm sản xuất rò rỉ vào catalog |

---

## 13. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Product/Variant/SKU, xuất bản cơ bản |
| **Phase 2** | Chống trùng lặp, quy trình duyệt đầy đủ, gộp sản phẩm |
| **Phase 3** | ACL với Supply Chain cho own brand |

---

## 14. Tài liệu liên quan

- [catalog.md](catalog.md) — thương hiệu, danh mục
- [marketplace.md](marketplace.md) — offer
- [../02-domain/entities.md](../02-domain/entities.md) — mô hình sản phẩm chi tiết
