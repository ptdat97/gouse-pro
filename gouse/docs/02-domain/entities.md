# Entities

## 1. Mô hình sản phẩm thời trang — trung tâm của hệ thống

Đây là mô hình quan trọng nhất. Làm sai chỗ này thì mọi thứ phía sau đều sai.

### 1.1 Chuỗi phân cấp

```text
Brand
  │
Collection            (bộ sưu tập, gắn mùa)
  │
Product               "Áo sơ mi linen Oxford"
  │                    — cái khách nhìn thấy, có mô tả, ảnh
  │
Variant               "Màu Trắng, Size M"
  │                    — tổ hợp thuộc tính
  │
SKU                   "SM-LIN-OXF-WHT-M"
  │                    — đơn vị lưu kho, mã định danh vật lý
  │
Offer                 "Seller A bán 299.000đ"
  │                    — lời chào bán của MỘT nhà bán
  │
Inventory             "Kho HN: 12 cái khả dụng"
                       — hàng thật ở đâu, còn bao nhiêu
```

### 1.2 Vì sao cần đủ 5 tầng?

Câu hỏi hợp lý: sao không gộp Product/Variant/SKU làm một?

```text
Product     — cần riêng vì: một trang sản phẩm, một bộ ảnh, một mô tả
              khách xem "Áo sơ mi linen" chứ không xem "Áo sơ mi linen trắng size M"

Variant     — cần riêng vì: khách chọn màu rồi chọn size
              ảnh thay đổi theo màu, không theo size
              cần biết màu nào còn size nào

SKU         — cần riêng vì: đơn vị đếm kho, đơn vị mã vạch
              đơn vị mà nhà máy sản xuất, kho lưu, người giao cầm

Offer       — cần riêng vì: nhiều seller bán cùng SKU, giá khác nhau
              (đã phân tích tại 01-business/marketplace.md)

Inventory   — cần riêng vì: một SKU ở nhiều kho, nhiều trạng thái
```

**Kiểm chứng bằng câu hỏi thực tế:**

| Câu hỏi | Tầng trả lời |
|---|---|
| "Áo này có màu gì?" | Variant |
| "Màu trắng còn size nào?" | SKU + Inventory |
| "Ai bán rẻ nhất?" | Offer |
| "Kho nào xuất hàng?" | Inventory |
| "Lô sản xuất nào?" | Inventory → ProductionBatch |

---

## 2. Entity chi tiết — Commerce Context

### 2.1 Brand

```text
Brand {
    id
    name
    slug
    logo_url
    description
    brand_type          OWN | PARTNER | THIRD_PARTY
    protection_level    OPEN | VERIFIED_ONLY | RESTRICTED
    owner_seller_id     (nullable — brand của seller)
    country_of_origin
    status              ACTIVE | INACTIVE
}
```

**`protection_level` — trường quan trọng cho chống hàng giả:**

```text
OPEN           — seller nào cũng tạo offer được
VERIFIED_ONLY  — chỉ seller có giấy ủy quyền còn hiệu lực
RESTRICTED     — chỉ nền tảng hoặc seller được chỉ định
```

Kiểm tra này là **quy tắc domain bắt buộc** trong `Offer`, không phải quy trình thủ công. Xem [../01-business/marketplace.md](../01-business/marketplace.md) mục 4.1.

### 2.2 Collection

```text
Collection {
    id
    brand_id
    name                "Thu Đông 2026"
    season              FW2026
    theme
    launch_date
    end_of_season_date
    status              PLANNING | ACTIVE | ENDING | ARCHIVED
}
```

**Vì sao là entity riêng, không phải tag:** bộ sưu tập có ngân sách sản xuất, mốc thời gian, chỉ số sell-through riêng. Nó là đơn vị lập kế hoạch và đo lường trong thời trang. Xem [../01-business/own-brand.md](../01-business/own-brand.md) mục 4.

### 2.3 Product

```text
Product {
    id
    brand_id
    collection_id       (nullable)
    name
    slug
    description
    care_instructions           ← đặc thù thời trang: hướng dẫn giặt là
    material_composition        ← "80% cotton, 20% linen"
    origin_country
    category_id
    product_type        TOP | BOTTOM | DRESS | OUTERWEAR | SHOES | BAG | ACCESSORY
    gender_target       MEN | WOMEN | UNISEX | KIDS
    size_chart_id               ← bảng size áp dụng
    status              DRAFT | PENDING_REVIEW | ACTIVE | INACTIVE | ARCHIVED
    published_at
    created_by_seller_id (nullable — null nghĩa là danh mục chuẩn của nền tảng)
}
```

**Các trường đặc thù thời trang** (`care_instructions`, `material_composition`, `size_chart_id`) không phải tùy chọn — chúng trực tiếp ảnh hưởng tỷ lệ hoàn hàng. Khách không biết chất liệu sẽ mua nhầm; không có bảng size sẽ chọn sai size.

### 2.4 Variant

```text
Variant {
    id
    product_id
    color               (value object)
    size                (value object)
    other_attributes    (map — ví dụ: kiểu ống, độ dài tay)
    images[]            ← ảnh riêng theo màu
    display_order
    status              ACTIVE | INACTIVE
}
```

**Ràng buộc:** trong một Product, không được có hai Variant cùng tổ hợp thuộc tính.

### 2.5 SKU

```text
SKU {
    id
    variant_id
    sku_code            "SM-LIN-OXF-WHT-M"  — duy nhất toàn hệ thống
    barcode             (EAN/UPC nếu có)
    weight_gram                 ← cần cho tính phí vận chuyển
    dimensions                  ← cần cho tính phí và xếp kho
    status              ACTIVE | DISCONTINUED
}
```

**Lưu ý:** SKU là **định danh hàng hóa chung**, không thuộc về seller nào. Đây là cái cho phép biết "ba seller này đang bán cùng một món hàng".

### 2.6 Offer — entity trung tâm của marketplace

```text
Offer {
    id
    sku_id
    seller_id
    price                       ← Money value object
    compare_at_price            ← giá gốc để hiển thị giảm giá
    currency
    condition           NEW | USED_LIKE_NEW | USED_GOOD
    handling_time_hours         ← thời gian seller cần để chuẩn bị hàng
    return_policy_id
    min_order_quantity
    max_order_quantity
    status              DRAFT | ACTIVE | OUT_OF_STOCK | SUSPENDED | ARCHIVED
    published_at
    version                     ← cho khóa lạc quan
}
```

**Ràng buộc bất biến:**
- Một seller chỉ có **một** offer đang hoạt động cho một SKU.
- `price` > 0.
- Nếu brand có `protection_level = VERIFIED_ONLY`, seller phải có ủy quyền còn hiệu lực.

**Lưu ý về tồn kho:** Offer **không** chứa số lượng tồn. Số lượng nằm ở `InventoryItem`. Offer chỉ có trạng thái `OUT_OF_STOCK` được cập nhật qua event từ Inventory.

Lý do: một offer có thể được phục vụ từ nhiều kho; và tồn kho thay đổi với tần suất rất cao, không nên làm bẩn aggregate Offer.

### 2.7 Cart và CartItem

```text
Cart {
    id
    customer_id         (nullable — cho khách vãng lai)
    session_id          (cho khách chưa đăng nhập)
    currency
    status              ACTIVE | CONVERTED | ABANDONED
    expires_at
}

CartItem {
    id
    cart_id
    offer_id            ← trỏ tới Offer, KHÔNG phải SKU
    quantity
    added_at
    source_content_id   (nullable) ← nội dung nào dẫn tới việc thêm giỏ
    source_creator_id   (nullable) ← phục vụ quy kết
}
```

**Vì sao `CartItem` trỏ tới `offer_id` chứ không phải `sku_id`:** khách chọn mua từ một nhà bán cụ thể với giá cụ thể. Nếu chỉ lưu SKU, không biết mua của ai.

**Hai trường `source_*`** là mắt xích của bánh đà — chúng cho phép quy kết doanh thu về nội dung/creator ngay từ hành vi thêm giỏ.

### 2.8 Order và OrderLine

```text
Order {
    id
    order_number                ← mã khách nhìn thấy: "FC-2026-08-001234"
    customer_id         (nullable)
    guest_email                 ← cho khách vãng lai
    guest_phone
    shipping_address    (đóng băng — value object)
    billing_address     (đóng băng)
    currency
    subtotal
    shipping_fee
    discount_amount
    tax_amount
    total_amount
    status              PENDING_PAYMENT | PAID | PROCESSING | PARTIALLY_SHIPPED
                        | SHIPPED | PARTIALLY_DELIVERED | DELIVERED
                        | COMPLETED | CANCELLED | PARTIALLY_CANCELLED
    placed_at
    completed_at
    idempotency_key             ← chống tạo đơn trùng
}

OrderLine {
    id
    order_id
    offer_id
    sku_id                      ← sao chép để truy vấn không cần join
    seller_id                   ← sao chép
    product_name                ← ĐÓNG BĂNG: tên tại thời điểm mua
    variant_description         ← ĐÓNG BĂNG: "Trắng / M"
    unit_price                  ← ĐÓNG BĂNG
    quantity
    line_total
    commission_rate             ← ĐÓNG BĂNG (nguyên tắc P9)
    commission_amount           ← ĐÓNG BĂNG
    attributed_creator_id (nullable)
    creator_commission_rate     ← ĐÓNG BĂNG
    status              ACTIVE | CANCELLED | RETURNED
}
```

**Vì sao đóng băng nhiều trường như vậy:**

| Trường | Nếu không đóng băng |
|---|---|
| `product_name` | Seller đổi tên sản phẩm → hóa đơn cũ hiển thị sai |
| `unit_price` | Giá đổi → tổng tiền đơn cũ không khớp |
| `commission_rate` | Đổi chính sách → đối soát tháng trước ra số khác |
| `variant_description` | Sửa variant → không biết khách đã mua size nào |

Đây là ứng dụng trực tiếp của nguyên tắc P9. Xem [../00-overview/principles.md](../00-overview/principles.md).

### 2.9 FulfillmentOrder

```text
FulfillmentOrder {
    id
    fulfillment_number          ← "FC-2026-08-001234-A"
    order_id
    seller_id
    stock_location_id
    fulfillment_type    PLATFORM | SELLER | PLATFORM_SERVICE
    shipping_address    (sao chép từ Order)
    shipping_method
    shipping_provider
    tracking_number
    status              PENDING | ALLOCATED | PICKING | PACKED | HANDED_OVER
                        | IN_TRANSIT | DELIVERED | DELIVERY_FAILED
                        | RETURNED_TO_SENDER | COMPLETED | CANCELLED
    estimated_delivery_date
    shipped_at
    delivered_at
    completed_at                ← sau khi hết hạn đổi trả
}

FulfillmentLine {
    id
    fulfillment_order_id
    order_line_id
    sku_id
    quantity
    production_batch_id (nullable) ← truy vết lô, cho own brand
}
```

Trường `production_batch_id` là mắt xích cho phép tính COGS chính xác và truy vết thu hồi. Xem [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 8.

### 2.10 Adjustment — mọi khoản cộng/trừ là thực thể

Mẫu lấy từ Sylius (xem [../11-oss/sylius.md](../11-oss/sylius.md)), bổ sung `cost_bearer` cho marketplace.

```text
OrderLineAdjustment {
    id
    order_line_id
    type          PROMOTION | TAX | SHIPPING | COMMISSION | FEE | MANUAL
    label         nhãn hiển thị: "Giảm giá THUDONG20"
    amount        Money — ÂM là giảm, DƯƠNG là tăng
    source_type   PROMOTION | TAX_RULE | SHIPPING_METHOD | COMMISSION_RULE
    source_id     định danh nguồn gốc
    cost_bearer   PLATFORM | SELLER | SHARED    ← bổ sung cho marketplace
    created_at
}
```

**Vì sao là thực thể chứ không phải trường trên `Order`:**

| Nếu là trường (`discount_amount`) | Nếu là thực thể |
|---|---|
| Không biết giảm giá từ đâu ra | Truy vết được tới quy tắc cụ thể |
| Không biết ai chịu chi phí | `cost_bearer` rõ ràng |
| Hoàn từng phần tính sai | Adjustment gắn từng dòng → tính đúng |
| Tính lại giỏ dễ sót/trùng | Xóa hết rồi tính lại, an toàn |
| Không giải thích được cho khách | Liệt kê được từng khoản |

**Ví dụ giải quyết vấn đề hoàn tiền từng phần:**

```text
Đơn 3 món, tổng 500.000đ, giảm 50.000đ (10%)

Không có Adjustment:
    order.discount_amount = 50000
    → khách trả món C (100.000đ), hoàn bao nhiêu?
    → phải tính lại tỷ lệ, dễ sai

Có Adjustment (phân bổ khi đặt hàng):
    OrderLine A → Adjustment{PROMOTION, −20.000, SELLER}
    OrderLine B → Adjustment{PROMOTION, −20.000, SELLER}
    OrderLine C → Adjustment{PROMOTION, −10.000, SELLER}
    → khách trả món C, hoàn 100.000 − 10.000 = 90.000đ
    → đọc trực tiếp, không tính lại
```

Đây là cơ chế mà [../07-workflows/return.md](../07-workflows/return.md) mục 5 yêu cầu nhưng trước đây chưa định nghĩa.

**Bất biến:**

```text
OrderLine.line_total = unit_price × quantity + Σ Adjustment.amount
```

**Phân bổ giảm giá cấp đơn xuống dòng hàng** dùng `Money.Allocate()` theo tỷ lệ `line_total` — không mất đồng nào.

---

## 3. Entity — Inventory Context

```text
InventoryItem {
    id
    sku_id
    stock_location_id
    inventory_owner_id          ← PLATFORM hoặc seller_id
    quantity_available
    quantity_reserved
    quantity_committed
    quantity_in_transit
    quantity_damaged
    quantity_returned
    production_batch_id (nullable)
    version                     ← khóa lạc quan
    updated_at
}
```

**Ba trường khóa** (`sku_id`, `stock_location_id`, `inventory_owner_id`) là điều cho phép mô hình hóa cả ba mô hình fulfillment bằng một cấu trúc. Xem [../01-business/fulfillment.md](../01-business/fulfillment.md) mục 2.3.

```text
Reservation {
    id
    inventory_item_id
    checkout_id
    quantity
    expires_at                  ← tự động giải phóng khi hết hạn
    status              ACTIVE | CONVERTED | EXPIRED | RELEASED
}

InventoryMovement {
    id
    inventory_item_id
    movement_type       RECEIVE | RESERVE | COMMIT | SHIP | RETURN
                        | ADJUST | TRANSFER | DAMAGE
    quantity
    from_state
    to_state
    reference_type              ← ORDER | PRODUCTION_BATCH | RETURN | MANUAL
    reference_id
    reason
    performed_by
    created_at                  ← BẤT BIẾN sau khi tạo
}
```

`InventoryMovement` là sổ nhật ký bất biến — cho phép tái dựng lại trạng thái tồn kho tại bất kỳ thời điểm nào và điều tra khi có sai lệch.

---

## 4. Entity — Financial Context

```text
Account {
    id
    account_type        PLATFORM_REVENUE | PLATFORM_CASH | SELLER_PAYABLE
                        | CREATOR_PAYABLE | CUSTOMER_REFUND_PAYABLE
                        | SUPPLIER_PAYABLE | COGS | FEE_EXPENSE
    owner_type          PLATFORM | SELLER | CREATOR | CUSTOMER | SUPPLIER
    owner_id            (nullable)
    currency
}

LedgerEntry {
    id
    entry_type          ORDER_REVENUE | COMMISSION | CREATOR_COMMISSION
                        | REFUND | PAYOUT | ADJUSTMENT | FEE | COGS
    reference_type              ← ORDER | SETTLEMENT | RETURN | MANUAL
    reference_id
    description
    idempotency_key             ← chống ghi trùng
    created_at                  ← BẤT BIẾN
    created_by
}

LedgerLine {
    id
    ledger_entry_id
    account_id
    direction           DEBIT | CREDIT
    amount
    currency
}
```

**Bất biến bắt buộc:** trong mỗi `LedgerEntry`, Σ DEBIT = Σ CREDIT.

Ví dụ bút toán cho một đơn marketplace 300.000đ:

```text
LedgerEntry: ORDER_REVENUE, ref=Order#1000

  DEBIT   PLATFORM_CASH                    300.000
  CREDIT  SELLER_PAYABLE (seller A)        250.500
  CREDIT  PLATFORM_REVENUE (hoa hồng)       30.000
  CREDIT  CREATOR_PAYABLE (creator X)       15.000
  CREDIT  FEE_EXPENSE (phí PSP)              4.500
  ─────────────────────────────────────────────────
  Σ DEBIT = 300.000 = Σ CREDIT ✓
```

---

## 5. Entity — Growth Context

```text
Creator {
    id
    user_id
    display_name
    creator_type        KOC | KOL | INFLUENCER | STYLIST | CONTENT_PARTNER
    bio
    avatar_url
    specialties[]               ← phong cách, ngành hàng sở trường
    status              APPLIED | PENDING_REVIEW | APPROVED | ACTIVE
                        | SUSPENDED | TERMINATED
    approved_at
}

Content {
    id
    creator_id          (nullable — null nghĩa là nội dung của nền tảng)
    content_type        VIDEO | IMAGE | LOOKBOOK | ARTICLE | OUTFIT | LIVE
    title
    body
    media[]
    campaign_id         (nullable)
    is_sponsored                ← tự động set nếu thuộc campaign có trả phí
    status              DRAFT | PENDING_REVIEW | PUBLISHED | TAKEN_DOWN | ARCHIVED
    published_at
}

ProductTag {
    id
    content_id
    product_id
    offer_id            (nullable — có thể tag sản phẩm chung)
    position_x                  ← vị trí trên ảnh
    position_y
    timestamp_second    (nullable) ← vị trí trong video
}

Outfit {
    id
    content_id          (nullable)
    creator_id          (nullable)
    name                "Đi làm mùa thu"
    description
    style_tags[]
    total_price                 ← tính từ các item
    status
}

OutfitItem {
    id
    outfit_id
    product_id
    variant_id          (nullable — gợi ý màu/size cụ thể)
    role                MAIN | COMPLEMENT | ACCESSORY
    display_order
    substitute_product_ids[]    ← sản phẩm thay thế khi hết hàng
}
```

Trường `substitute_product_ids` giải quyết vấn đề đã nêu ở [../01-business/content-commerce.md](../01-business/content-commerce.md) mục 3 — nội dung sống lâu hơn sản phẩm.

```text
AffiliateLink {
    id
    creator_id
    target_type         PRODUCT | CONTENT | COLLECTION | STORE
    target_id
    campaign_id         (nullable)
    short_code                  ← mã ngắn trong URL
    created_at
}

Click {
    id
    affiliate_link_id
    creator_id
    session_id
    customer_id         (nullable)
    ip_hash                     ← đã ẩn danh hóa
    device_fingerprint
    referrer
    clicked_at
}

Attribution {
    id
    order_id
    order_line_id
    creator_id
    click_id
    attribution_model   LAST_CLICK | FIRST_CLICK | WEIGHTED
    attribution_weight          ← 1.0 với last click
    commission_rate             ← ĐÓNG BĂNG
    commission_amount
    status              PENDING | CONFIRMED | REVERSED
    created_at                  ← BẤT BIẾN
}
```

**Lưu ý quan trọng:** lưu **toàn bộ chuỗi Click**, không chỉ click được quy kết. Lý do đã nêu tại [../01-business/creator.md](../01-business/creator.md) mục 4b — để có thể tính lại theo mô hình quy kết khác trong tương lai.

---

## 6. Entity — Supply Chain Context

```text
ProductDevelopment {
    id
    concept_name
    collection_id
    target_category
    target_cost                 ← giá vốn mục tiêu
    target_retail_price
    target_margin
    demand_signal_ref           ← tín hiệu nhu cầu nào dẫn tới ý tưởng này
    designer_id
    status              CONCEPT | DESIGN | TECH_PACK | COSTING | SAMPLING
                        | SAMPLE_APPROVED | PLANNING | IN_PRODUCTION
                        | LAUNCHED | CANCELLED
    catalog_product_id  (nullable) ← liên kết sau khi tạo trong Catalog
}

TechPack {
    id
    product_development_id
    version
    specifications              ← thông số kỹ thuật
    measurements                ← bảng số đo theo size
    construction_details        ← quy cách may
    material_requirements
    attachments[]
    approved_at
    approved_by
}

Sample {
    id
    product_development_id
    tech_pack_version
    supplier_id
    sample_round                ← vòng thứ mấy
    received_at
    evaluation_result   APPROVED | REJECTED | APPROVED_WITH_CHANGES
    feedback
    photos[]
}

ProductionOrder {
    id
    product_development_id
    supplier_id
    tech_pack_id
    total_quantity
    unit_cost_agreed
    total_cost
    currency
    order_date
    expected_delivery_date
    status              DRAFT | SENT | CONFIRMED | MATERIAL_SOURCING
                        | IN_PRODUCTION | QC_PENDING | QC_PASSED
                        | SHIPPED | RECEIVED | CANCELLED
}

ProductionOrderLine {
    id
    production_order_id
    sku_id
    size                        ← phân bổ theo size
    quantity
}

ProductionBatch {
    id
    production_order_id
    sku_id
    quantity_produced
    quantity_passed_qc
    quantity_rejected
    unit_cost                   ← GIÁ VỐN CỦA LÔ NÀY
    production_date
    supplier_id
    material_batch_refs[]       ← truy vết nguyên liệu
    certificates[]
    status              PRODUCED | QC_PENDING | QC_PASSED | QC_FAILED | RECEIVED
}

DemandSignal {
    id
    signal_type         VIEW | SEARCH | SEARCH_NO_RESULT | CLICK | ADD_TO_CART
                        | WISHLIST | ORDER | STOCKOUT | RETURN | NOTIFY_REQUEST
    sku_id              (nullable)
    product_id          (nullable)
    category_id         (nullable)
    search_term         (nullable)
    quantity
    occurred_at
    metadata
}
```

`DemandSignal` với `signal_type = SEARCH_NO_RESULT` và `STOCKOUT` là hai loại tín hiệu đo **nhu cầu không được đáp ứng** — đã phân tích tại [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 4.2.

---

## 7. Quy tắc chung cho mọi entity

| Quy tắc | Lý do |
|---|---|
| Định danh dùng UUID hoặc ULID, không dùng số tự tăng | Tránh lộ quy mô kinh doanh; dễ tách service |
| Mọi entity có `created_at`, `updated_at` | Truy vết, gỡ lỗi |
| Entity quan trọng có `version` cho khóa lạc quan | Chống mất cập nhật khi đồng thời |
| Không xóa cứng entity có liên quan giao dịch | Nghĩa vụ lưu trữ, truy vết |
| Tham chiếu qua ranh giới aggregate chỉ dùng id | Cho phép tách service |
| Trạng thái dùng kiểu liệt kê tường minh, không dùng số | Đọc được, an toàn khi đổi |
| Tiền dùng Money value object, không dùng float | Tránh sai số làm tròn |

Quy tắc cuối cùng là bắt buộc tuyệt đối. Xem [value-objects.md](value-objects.md).

---

## 8. Tài liệu liên quan

- [aggregates.md](aggregates.md) — ranh giới giao dịch
- [value-objects.md](value-objects.md) — Money, Address, Size, Color
- [domain-events.md](domain-events.md) — sự kiện phát ra khi entity thay đổi
- [../05-data/data-model.md](../05-data/data-model.md) — ánh xạ xuống database
