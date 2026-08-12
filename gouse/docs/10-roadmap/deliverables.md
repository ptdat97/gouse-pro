# Tổng hợp bàn giao

Tài liệu này tổng hợp toàn bộ kết quả và kết luận rà soát tính nhất quán trước khi bắt đầu viết code.

---

## 1. Bản đồ kiến trúc tổng thể

```text
                          NGƯỜI DÙNG
         (Khách · Seller · Creator · Nhân viên · Đối tác)
                              │
                    ┌─────────▼─────────┐
                    │    Next.js UI     │
                    │  (CHỈ TRÌNH BÀY)  │
                    │ Storefront ·      │
                    │ Seller · Creator  │
                    │ · Admin           │
                    └─────────┬─────────┘
                              │  HTTP / REST API
                    ┌─────────▼─────────┐
                    │    GO BACKEND     │
                    │ (Modular Monolith)│
                    │                   │
                    │ interfaces        │
                    │ application       │
                    │ domain            │
                    │ infrastructure    │
                    └─────────┬─────────┘
                  ┌───────────┼───────────┐
                  ▼           ▼           ▼
              Database     Storage   External APIs
```

Chi tiết: [../03-architecture/architecture.md](../03-architecture/architecture.md)

---

## 2. Bản đồ miền

```text
CORE DOMAIN (lợi thế cạnh tranh — đầu tư mạnh nhất)
    Marketplace & Offer · Creator/Content Commerce
    Supply Chain & Demand Intelligence
    Order & Fulfillment Orchestration · Fashion Product Model

SUPPORTING DOMAIN (cần thiết, không tạo khác biệt)
    Cart · Checkout · Pricing · Promotion · Inventory · Return
    Warehouse · Procurement · Quality · Loyalty · Campaign
    Seller Mgmt · Customer · Review

GENERIC DOMAIN (dùng giải pháp có sẵn)
    Identity · Notification · Search Infra · File Storage
    Payment Gateway · Shipping · Audit Log · Analytics Infra
```

Chi tiết: [../02-domain/domain-map.md](../02-domain/domain-map.md)

---

## 3. Bản đồ Bounded Context

```text
COMMERCE      Catalog · Product · Variant · SKU · Offer · Pricing
              Cart · Checkout · Order · FulfillmentOrder · Promotion

MARKETPLACE   Seller · SellerStore · SellerPolicy · Commission
              BuyBox · SellerPerformance

INVENTORY     Inventory · StockLocation · Reservation · Movement

FINANCIAL     Ledger · LedgerEntry · Balance · Settlement · Payout

GROWTH        Creator · Content · Outfit · Affiliate · Attribution
              Campaign · Discovery · Recommendation · Loyalty

SUPPLY CHAIN  ProductDevelopment · TechPack · DemandSignal · Forecast
              ProductionPlan · Supplier · PurchaseOrder
              ProductionOrder · ProductionBatch · Quality · Warehouse

PLATFORM      Identity · Notification · Audit · FileStorage · SearchIndex
```

**Bốn điều chỉnh so với đề xuất ban đầu**, kèm lý do tại [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md) mục 2:

1. `Offer` chuyển từ Marketplace sang **Commerce** — để own brand và marketplace dùng chung một đường
2. Tách **Financial context** riêng — nguồn sự thật duy nhất về tiền
3. `ProductDevelopment` thuộc **Supply Chain**, không thuộc Catalog
4. Logic xếp hạng tìm kiếm chuyển sang **Growth**, chỉ hạ tầng đánh chỉ mục ở Platform

---

## 4. Đồ thị phụ thuộc module

```text
        checkout · fulfillment · supply-chain          ← điều phối
                        │
        cart · order · payment · return                ← giao dịch
        procurement · manufacturing
                        │
        marketplace · seller · creator · content       ← nghiệp vụ
        affiliate · campaign · promotion
        warehouse · quality · loyalty · recommendation
                        │
        catalog · product · pricing                    ← dữ liệu chính
        inventory · customer
                        │
        identity · notification · analytics            ← nền

Phụ thuộc CHỈ đi từ trên xuống. Từ dưới lên: CHỈ qua event.
```

Ma trận chi tiết: [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md)

---

## 5. Sơ đồ quan hệ thực thể

```text
Brand → Collection → Product → Variant → SKU → Offer → Inventory
                                          │
                                          ├→ ProductionBatch (own brand)
                                          │
Cart → Checkout → Order → OrderLine
                    │
                    ├→ FulfillmentOrder → FulfillmentLine → Shipment
                    ├→ LedgerEntry → LedgerLine
                    └→ Attribution → Creator
```

Chi tiết: [../02-domain/entities.md](../02-domain/entities.md)

---

## 6. Kiến trúc API

```text
/api/v1/...              Storefront   — khách hàng
/api/v1/seller/...       Seller Center — CHỈ dữ liệu của mình
/api/v1/creator/...      Creator Center — KHÔNG có dữ liệu cá nhân khách
/api/v1/admin/...        Admin — theo vai trò, có audit
/api/v1/partner/...      Đối tác (Phase 4)
/api/v1/webhooks/...     Nhận webhook
/api/v1/storefront/...   BFF (chỉ khi có vấn đề hiệu năng đo được)
```

Chi tiết: [../06-api/api-domains.md](../06-api/api-domains.md)

---

## 7. Ma trận sở hữu dữ liệu

Mỗi bảng thuộc **đúng một module**. Bảng đầy đủ tại [../03-architecture/module-boundaries.md](../03-architecture/module-boundaries.md) mục 3.

Nguyên tắc bắt buộc:

```text
✓ Module chỉ đọc/ghi bảng của mình
✗ Không JOIN vượt ranh giới module
✗ Không khóa ngoại vượt ranh giới module
✓ Tham chiếu chỉ bằng định danh
```

---

## 8. Ma trận trách nhiệm module

| Câu hỏi nghiệp vụ | Module trả lời |
|---|---|
| Sản phẩm tên gì, mô tả ra sao? | `product` |
| Thuộc thương hiệu, bộ sưu tập nào? | `catalog` |
| Ai bán, giá bao nhiêu? | `marketplace` |
| Còn hàng không, ở đâu? | `inventory` |
| Đơn hàng có gì? | `order` |
| Ai giao, đến đâu rồi? | `fulfillment` |
| **Tiền của ai, bao nhiêu?** | **`payment` (duy nhất)** |
| Hoa hồng bao nhiêu %? | `marketplace` (quy tắc) → `order` (đóng băng) → `payment` (ghi sổ) |
| Creator nào được tính công? | `affiliate` |
| Cần sản xuất bao nhiêu? | `supply-chain` |
| Giá vốn thật? | `manufacturing` (theo lô) |

---

## 9. Sequence diagram quan trọng

| Luồng | Tài liệu |
|---|---|
| Khách mua hàng | [../07-workflows/customer-purchase.md](../07-workflows/customer-purchase.md) |
| Đơn nhiều nhà bán + đối soát | [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) |
| Nội dung → đơn hàng | [../07-workflows/content-commerce.md](../07-workflows/content-commerce.md) |
| Creator affiliate | [../07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md) |
| Đăng ký seller | [../07-workflows/seller-onboarding.md](../07-workflows/seller-onboarding.md) |
| Đăng bán sản phẩm | [../07-workflows/product-publishing.md](../07-workflows/product-publishing.md) |
| Trả hàng | [../07-workflows/return.md](../07-workflows/return.md) |
| Sản phẩm own brand | [../07-workflows/own-brand-product.md](../07-workflows/own-brand-product.md) |
| Đặt sản xuất và QC | [../07-workflows/supplier-production.md](../07-workflows/supplier-production.md) |
| Bổ sung hàng | [../07-workflows/replenishment.md](../07-workflows/replenishment.md) |

---

## 10. Phạm vi theo giai đoạn

```text
MVP (14 module)
    identity · notification · analytics
    customer · catalog · product · pricing · inventory
    marketplace · seller · promotion
    cart · checkout · order · payment · fulfillment
    + ghi demand_signal (dù chưa dùng)

Phase 2 (+7)
    creator · content · affiliate · campaign
    recommendation · return · warehouse

Phase 3 (+5)
    supply-chain · procurement · manufacturing · quality · loyalty

Phase 4 (không thêm module — nâng cấp chiều sâu)
    recommendation ML · demand intelligence · retail media
    live commerce · creator marketplace · kho dữ liệu
```

---

## 11. Chỉ mục ADR

| # | Quyết định |
|---|---|
| [0001](../adr/0001-modular-monolith.md) | Bắt đầu bằng Modular Monolith |
| [0002](../adr/0002-api-first.md) | API First |
| [0003](../adr/0003-go-backend.md) | Go cho backend |
| [0004](../adr/0004-nextjs-frontend.md) | Next.js cho frontend |
| [0005](../adr/0005-module-boundaries.md) | Ranh giới module tường minh |
| [0006](../adr/0006-internal-events.md) | Domain event nội bộ + Outbox |
| [0007](../adr/0007-marketplace-order-model.md) | Offer + tách Order/FulfillmentOrder |
| [0008](../adr/0008-financial-ledger.md) | Sổ cái bất biến |
| [0009](../adr/0009-service-extraction.md) | Hoãn tách service |
| [0010](../adr/0010-database-layer.md) | PostgreSQL + sqlc cho tầng dữ liệu |

### Bổ sung sau nghiên cứu OSS (12/08/2026)

Nghiên cứu 12 dự án OSS xác nhận phần lớn thiết kế và **thay đổi ba điểm**:

| Thay đổi | Nguồn | Tài liệu đã cập nhật |
|---|---|---|
| Link table cho quan hệ nhiều-nhiều vượt module | Medusa | [05-data/data-model.md](../05-data/data-model.md) mục 3.4 |
| `Adjustment` là thực thể hạng nhất | Sylius | [02-domain/entities.md](../02-domain/entities.md) mục 2.10, [04-modules/order.md](../04-modules/order.md) mục 7 |
| Định nghĩa cơ sở tính hoa hồng | Sylius + TikTok Shop | [01-business/monetization.md](../01-business/monetization.md) mục 3.3, [04-modules/affiliate.md](../04-modules/affiliate.md) |

Chi tiết: [11-oss/synthesis.md](../11-oss/synthesis.md).

---

## 12. Kết quả rà soát tính nhất quán

Rà soát theo 11 tiêu chí ở mục 32 của yêu cầu.

### 12.1 Phụ thuộc vòng — **Đạt**

Đồ thị phụ thuộc là DAG năm tầng. Ba trường hợp có nguy cơ vòng đã được giải bằng event:

```text
order ↔ loyalty       → loyalty nghe order.completed
catalog ↔ inventory   → catalog nghe inventory.depleted
seller ↔ payment      → seller gọi payment.GetBalance(), payment không gọi ngược
```

Ngoại lệ có kiểm soát: `identity` nghe `seller.approved` và `creator.approved` để cấp vai trò — chỉ **nghe**, không **gọi**, nên không tạo vòng.

### 12.2 Trùng lặp sở hữu nghiệp vụ — **Đạt**

Ba trường hợp có nguy cơ đã được phân vai rõ:

```text
Hoa hồng:  marketplace (quy tắc) → order (đóng băng) → payment (ghi sổ)
Số dư:     payment là NGUỒN SỰ THẬT DUY NHẤT; seller/creator chỉ hiển thị
Hết hàng:  inventory là nguồn sự thật; offer.status là dữ liệu dẫn xuất
```

### 12.3 Ranh giới aggregate — **Đạt**

Sáu quyết định khó đã được phân tích và ghi lý do tại [../02-domain/aggregates.md](../02-domain/aggregates.md) mục 3: Order/FulfillmentOrder, Offer/Product, Inventory/Offer, LedgerEntry/Account, Cart/Checkout, ProductionBatch/ProductionOrder.

### 12.4 Thuật ngữ nhất quán — **Đạt**

Từ điển tại [../00-overview/business-glossary.md](../00-overview/business-glossary.md), gồm:
- Mục I: từ đa nghĩa theo ngữ cảnh (Product, Customer, Order, Balance, Available)
- Mục J: thuật ngữ bị cấm (Item, Vendor, Stock, User, Shop, Transaction, Status)

### 12.5 Khớp API và domain — **Đạt**

API phản ánh khả năng nghiệp vụ. Điểm kiểm chứng: seller truy cập `FulfillmentOrder` (đúng mô hình domain), không phải `Order`.

### 12.6 Vấn đề đơn hàng marketplace — **Đạt**

Tách Order/FulfillmentOrder hỗ trợ đủ: giao từng phần, hủy từng phần, hoàn từng phần, trả từng phần, đối soát riêng, nhiều điểm xuất, nhiều phương thức vận chuyển.

Quyết định chính sách đã ghi rõ: không thu lại phí ship khi hủy một phần.

### 12.7 Nhất quán tài chính — **Đạt**

```text
✓ Ledger bất biến, có RULE chặn UPDATE/DELETE ở tầng database
✓ Bất biến Σ DEBIT = Σ CREDIT kiểm tra trong constructor
✓ Số dư là kết quả tính, snapshot chỉ là cache kiểm chứng được
✓ Tỷ lệ hoa hồng đóng băng vào OrderLine
✓ Money là số nguyên, có Allocate() để chia không mất đồng nào
✓ Ba chỉ số kiểm tra hàng ngày phải bằng 0
```

### 12.8 Tranh chấp tồn kho — **Đạt**

```text
✓ Khóa lạc quan với điều kiện nguyên tử trong WHERE
✓ Ràng buộc CHECK >= 0 ở tầng database
✓ Phân biệt lỗi xung đột (thử lại) và hết hàng (không thử lại)
✓ Reservation có TTL, tự giải phóng
✓ Kịch bản live commerce đã tính tới từ MVP
```

### 12.9 Lỗ hổng chuỗi cung ứng — **Đạt, có lưu ý**

```text
✓ Ghi demand_signal từ MVP (dữ liệu không tạo ngược được)
✓ Tín hiệu nhu cầu BỊ BỎ LỠ được ghi (stockout, tìm không ra kết quả)
✓ Giá vốn theo LÔ, không theo SKU
✓ Kế hoạch ở mức SKU (bao gồm size)
✓ Hệ thống đề xuất, con người quyết định
```

**Lưu ý:** Phase 3 phụ thuộc vào việc MVP thực sự ghi `demand_signal`. Nếu bỏ qua vì "chưa cần", Phase 3 sẽ chậm gần một năm. Đã đưa vào tiêu chí hoàn thành MVP.

### 12.10 Quy kết creator — **Đạt**

```text
✓ Lưu TOÀN BỘ chuỗi click, không chỉ click được quy kết
  → cho phép đổi mô hình quy kết sau này mà vẫn tính lại được
✓ Attribution bất biến, tỷ lệ đóng băng
✓ Đảo ngược khi hoàn hàng
✓ Hoa hồng chỉ Available sau khi hết hạn đổi trả
✓ cost_bearer ghi rõ trong Campaign
```

### 12.11 Khả năng tách service — **Đạt**

```text
✓ Interface công khai cho mọi module
✓ Không JOIN, không khóa ngoại vượt module
✓ Event contract thiết kế như thể vượt tiến trình
✓ Outbox pattern — đổi bộ phát không sửa module
✓ Định danh ULID/UUID
✓ Distributed tracing từ đầu
✓ Kiểm tra ranh giới tự động trong CI
```

---

## 13. Rủi ro và đánh đổi chính

| # | Rủi ro | Mức độ | Giảm thiểu |
|---|---|---|---|
| 1 | Kỷ luật ranh giới module lỏng dần | **Cao** | Kiểm tra tự động trong CI, thất bại = chặn merge; rà soát mỗi quý |
| 2 | Bỏ qua `demand_signal` ở MVP vì "chưa cần" | **Cao** | Đưa vào tiêu chí hoàn thành MVP |
| 3 | Bỏ qua mô hình Offer vì "phức tạp quá" | **Cao** | Đưa vào tiêu chí hoàn thành MVP |
| 4 | Sai sót tài chính | **Cao** | Ledger bất biến, kiểm tra hàng ngày ba chỉ số = 0 |
| 5 | Rò rỉ dữ liệu giữa các seller | **Cao** | Lọc trong truy vấn, không ở tầng hiển thị |
| 6 | Tồn kho sai do đồng thời | Trung bình | Khóa lạc quan + CHECK ở database |
| 7 | Reservation kẹt do worker ngừng | Trung bình | Giám sát, cảnh báo khi > 100 quá hạn |
| 8 | Frontend truy cập database | Trung bình | Không cấp thông tin kết nối cho Next.js |
| 9 | Chuỗi đảo ngược hoàn hàng thiếu bước | Trung bình | Kiểm tra tự động số bút toán sinh ra |
| 10 | Tách service quá sớm | Trung bình | ADR-0009: bắt buộc số liệu chứng minh |

### Đánh đổi đã chấp nhận

| Chấp nhận | Để đổi lấy |
|---|---|
| Không mở rộng từng module độc lập | Đơn giản vận hành, tốc độ phát triển |
| Nhiều lời gọi thay vì JOIN | Khả năng tách service |
| Nhất quán cuối giữa các module | Giao dịch ngắn, ít tranh chấp |
| Mô hình 5 tầng sản phẩm phức tạp | Hỗ trợ marketplace thật sự |
| Tính số dư chậm hơn | Sổ sách không bao giờ sai |
| Code Go dài dòng | Rõ ràng, dễ bảo trì lâu dài |
| Giỏ hàng không giữ tồn kho | Không khóa hàng vô ích |

---

## 14. Thứ tự triển khai đề xuất

```text
Giai đoạn 1 — Nền tảng
    platform (database, event bus, HTTP, log, trace)
    kernel (Money, ID types)
    identity
    + Thiết lập kiểm tra ranh giới module trong CI  ← LÀM NGAY

Giai đoạn 2 — Danh mục
    catalog → product → pricing

Giai đoạn 3 — Tồn kho và bán hàng
    inventory (đủ 6 trạng thái, khóa lạc quan)
    → marketplace (Offer) → seller (own brand nội bộ)

Giai đoạn 4 — Giao dịch
    cart → checkout (giữ hàng, đóng băng giá)
    → order (đóng băng dữ liệu) → payment (ledger đầy đủ)

Giai đoạn 5 — Thực hiện đơn
    fulfillment (tách đơn) → notification

Giai đoạn 6 — Marketplace hoàn chỉnh
    seller onboarding → đối soát → chi trả

Giai đoạn 7 — Hoàn thiện MVP
    promotion · analytics cơ bản · demand_signal
    customer (wishlist, preference)
```

**Lý do thứ tự:** mỗi giai đoạn xây trên giai đoạn trước theo đúng đồ thị phụ thuộc. Không giai đoạn nào phải chờ module chưa làm.

**Điểm quan trọng ở giai đoạn 1:** thiết lập kiểm tra ranh giới **trước khi** viết module đầu tiên. Thêm sau khi đã có 10 module là dọn dẹp hàng loạt vi phạm.

---

## 15. Kết luận rà soát

```text
✓ 11/11 tiêu chí rà soát đạt
✓ 639 liên kết chéo giữa các tài liệu, không có liên kết hỏng
✓ Không có phụ thuộc vòng trong thiết kế
✓ Không có trùng lặp sở hữu nghiệp vụ
✓ Các quyết định khó đều có ADR ghi lý do và phương án bị loại
```

**Tài liệu đã sẵn sàng để bắt đầu triển khai.**

Ba điều cần giữ kỷ luật nhất trong quá trình code:

```text
1. Kiểm tra ranh giới module trong CI phải có TỪ ĐẦU
   → thêm sau là dọn dẹp hàng loạt vi phạm

2. Bốn thứ phải làm đúng ngay ở MVP:
   Offer · tách Order/FulfillmentOrder · ledger bất biến · demand_signal
   → sửa sau là viết lại

3. Kiểm tra tính toàn vẹn tài chính hàng ngày từ ngày đầu
   → ba chỉ số phải bằng 0, không phải "sai số chấp nhận được"
```

---

## 16. Tài liệu liên quan

- [../README.md](../README.md) — điều hướng toàn bộ tài liệu
- [mvp.md](mvp.md) — phạm vi MVP chi tiết
- [todo.md](todo.md) — tiến độ triển khai thực tế theo 7 giai đoạn ở mục 14
- [../adr/README.md](../adr/README.md) — chỉ mục ADR
