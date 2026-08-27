# Bounded Contexts

## 1. Bounded Context là gì và vì sao cần

Bounded context là ranh giới trong đó **một mô hình domain có nghĩa nhất quán**.

Ví dụ cụ thể: từ "Product".

```text
Trong Catalog Context:
    Product = thứ khách nhìn thấy
    Có: tên hiển thị, mô tả marketing, hình ảnh, danh mục, thuộc tính tìm kiếm

Trong Supply Chain Context:
    Product = thứ được sản xuất
    Có: tech pack, định mức nguyên liệu, giá vốn, nhà cung cấp, quy cách may
```

Cố gắng nhồi cả hai vào **một** lớp `Product` duy nhất sẽ tạo ra một entity khổng lồ với 80 trường, mà 60 trường luôn null tùy hoàn cảnh. Đây là dấu hiệu kinh điển của việc thiếu bounded context.

**Quy tắc:** cùng một từ ở hai context là **hai mô hình khác nhau**, liên kết bằng định danh, không bằng khóa ngoại trực tiếp.

---

## 2. Đánh giá lại cấu trúc đề xuất

Tài liệu yêu cầu đề xuất bounded context theo năm nhóm (Commerce, Marketplace, Growth, Supply Chain, Platform), đồng thời yêu cầu **đánh giá và điều chỉnh dựa trên trách nhiệm miền** thay vì áp dụng máy móc.

Sau khi rà soát, tôi giữ lại phần lớn cấu trúc nhưng **thay đổi bốn điểm**, mỗi điểm có lý do.

### Thay đổi 1: `Offer` chuyển từ Marketplace sang Commerce

**Đề xuất ban đầu:** `Offer` nằm trong Marketplace context.

**Vấn đề:** nếu Offer thuộc Marketplace, thì đơn hàng own brand (không qua marketplace) sẽ không có Offer — dẫn tới hai đường đi khác nhau cho cùng một việc bán hàng. Đây chính là điều [../01-business/own-brand.md](../01-business/own-brand.md) muốn tránh.

**Quyết định:** `Offer` thuộc **Commerce context**. Mọi thứ bán được đều có Offer, kể cả own brand. Marketplace context quản lý **seller-specific concerns** (chính sách seller, buy box, hoa hồng), không quản lý bản thân Offer.

### Thay đổi 2: Tách `Ledger` khỏi `Payment`

**Đề xuất ban đầu:** Payment nằm trong Commerce; Settlement nằm trong Marketplace.

**Vấn đề:** đặt như vậy làm sổ sách tài chính bị chia đôi. Hoa hồng creator (thuộc Growth), hoa hồng seller (thuộc Marketplace), doanh thu own brand (thuộc Commerce) — cả ba đều là bút toán tài chính. Nếu ba context tự ghi sổ riêng, không có nguồn sự thật duy nhất về tiền.

**Quyết định:** tạo **Financial context** riêng, sở hữu Ledger, Settlement, Payout cho **mọi** bên. Payment gateway integration vẫn thuộc Commerce (thu tiền khách), nhưng việc ghi sổ tập trung tại Financial.

Đây là ứng dụng của nguyên tắc P8 — không tính số dư bằng phép cộng rải rác.

### Thay đổi 3: Tách `Product Development` khỏi `Catalog`

**Đề xuất ban đầu:** không nói rõ sản phẩm own brand ở giai đoạn phát triển thuộc đâu.

**Vấn đề:** đã phân tích tại [../01-business/own-brand.md](../01-business/own-brand.md) mục 3 — nếu để concept/tech pack/sample trong Catalog, Catalog sẽ phụ thuộc vào Supply Chain.

**Quyết định:** `ProductDevelopment` thuộc **Supply Chain context**. `CatalogProduct` được tạo qua event khi mẫu được duyệt.

### Thay đổi 4: `Search` chuyển từ Platform sang Discovery

**Đề xuất ban đầu:** Search nằm trong Platform (cùng Identity, Notification, Audit).

**Vấn đề:** Platform là nơi chứa năng lực kỹ thuật trung lập. Nhưng logic xếp hạng tìm kiếm — ưu tiên hàng còn size, trộn nội dung creator, tính hiệu suất seller — là **quyết định nghiệp vụ**, không trung lập.

**Quyết định:** tách làm hai. Hạ tầng đánh chỉ mục thuộc Platform; logic xếp hạng và khám phá thuộc **Discovery context** trong nhóm Growth.

---

## 3. Bản đồ bounded context cuối cùng

```text
┌────────────────────────────────────────────────────────────────────┐
│  COMMERCE CONTEXT                                                  │
│  Trách nhiệm: bán hàng cho khách                                   │
│                                                                    │
│  Catalog · Product · Variant · SKU · Offer · Pricing               │
│  Cart · Checkout · Order · FulfillmentOrder · Payment · Promotion  │
└────────────────────────────────────────────────────────────────────┘
              │                                    │
              │ OrderPlaced                        │ OfferCreated
              ▼                                    ▼
┌──────────────────────────────┐   ┌───────────────────────────────┐
│  MARKETPLACE CONTEXT         │   │  INVENTORY CONTEXT            │
│  Trách nhiệm: quản lý nhà bán│   │  Trách nhiệm: hàng ở đâu,     │
│                              │   │  còn bao nhiêu                │
│  Seller · SellerStore        │   │                               │
│  SellerPolicy · Commission   │   │  Inventory · StockLocation    │
│  BuyBox · SellerPerformance  │   │  Reservation · Movement       │
└──────────────────────────────┘   └───────────────────────────────┘
              │                                    │
              │ CommissionCalculated               │ InventoryReserved
              ▼                                    ▼
┌────────────────────────────────────────────────────────────────────┐
│  FINANCIAL CONTEXT                                                 │
│  Trách nhiệm: nguồn sự thật duy nhất về tiền                       │
│                                                                    │
│  Ledger · LedgerEntry · Balance · Settlement · Payout              │
│  Refund · Adjustment · Fee                                         │
└────────────────────────────────────────────────────────────────────┘
              ▲                                    ▲
              │ CommissionEarned                   │ PurchaseOrderApproved
              │                                    │
┌──────────────────────────────┐   ┌───────────────────────────────┐
│  GROWTH CONTEXT              │   │  SUPPLY CHAIN CONTEXT         │
│  Trách nhiệm: tạo nhu cầu    │   │  Trách nhiệm: có hàng để bán  │
│                              │   │                               │
│  Creator · Content · Outfit  │   │  ProductDevelopment · TechPack│
│  Affiliate · Attribution     │   │  DemandSignal · Forecast      │
│  Campaign · Discovery        │   │  ProductionPlan · Supplier    │
│  Recommendation · Loyalty    │   │  PurchaseOrder·ProductionOrder│
│                              │   │  ProductionBatch · Quality    │
└──────────────────────────────┘   │  Warehouse · Replenishment    │
                                   └───────────────────────────────┘

┌────────────────────────────────────────────────────────────────────┐
│  PLATFORM CONTEXT                                                  │
│  Trách nhiệm: năng lực kỹ thuật trung lập với domain               │
│                                                                    │
│  Identity · Authorization · Notification · Audit                   │
│  FileStorage · SearchIndex · EventBus · Analytics Infra            │
└────────────────────────────────────────────────────────────────────┘
```

---

## 4. Trách nhiệm từng context

### 4.1 Commerce Context

**Câu hỏi trả lời:** khách mua được hàng không, với giá bao nhiêu, đơn đi tới đâu?

| Sở hữu | Không sở hữu |
|---|---|
| Thông tin sản phẩm hiển thị | Thông tin sản xuất |
| Offer, giá | Tồn kho thực tế |
| Giỏ hàng, checkout | Hồ sơ seller |
| Đơn hàng, fulfillment order | Sổ cái tài chính |
| Khuyến mãi | Nội dung creator |

**Ranh giới quan trọng:** Commerce **hỏi** Inventory về tồn kho, không tự quản lý. Commerce **thông báo** cho Financial về giao dịch, không tự ghi sổ.

### 4.2 Marketplace Context

**Câu hỏi trả lời:** ai được bán, theo điều kiện nào, hiệu suất ra sao?

| Sở hữu | Không sở hữu |
|---|---|
| Hồ sơ seller | Bản thân Offer |
| Chính sách, hoa hồng | Đơn hàng |
| Quy tắc buy box | Tiền của seller |
| Chỉ số hiệu suất | Tồn kho seller |

**Lưu ý:** Marketplace **cung cấp quy tắc** (tỷ lệ hoa hồng nào áp dụng), Commerce **áp dụng quy tắc** khi tạo đơn, Financial **ghi sổ** kết quả.

### 4.3 Inventory Context

**Câu hỏi trả lời:** hàng ở đâu, còn bao nhiêu, đang ở trạng thái nào?

Tách riêng khỏi Commerce vì: tồn kho có mô hình trạng thái phức tạp riêng, có yêu cầu xử lý tranh chấp đồng thời cao, và được dùng bởi cả Commerce lẫn Supply Chain.

### 4.4 Financial Context

**Câu hỏi trả lời:** tiền của ai, bao nhiêu, khi nào được rút?

Đây là context có **ràng buộc nhất quán nghiêm ngặt nhất**. Mọi thay đổi đều là bút toán bất biến.

**Nguyên tắc:** không context nào khác được tự tính số dư. Muốn biết seller còn bao nhiêu tiền — hỏi Financial.

### 4.5 Growth Context

**Câu hỏi trả lời:** làm sao khách biết đến và muốn mua?

Bao gồm creator, nội dung, quy kết, chiến dịch, khám phá, gợi ý, khách hàng thân thiết.

**Lý do gộp:** các thành phần này chia sẻ chung mục tiêu và dữ liệu hành vi. Tách nhỏ hơn ở giai đoạn đầu sẽ tạo nhiều ranh giới phải đồng bộ mà chưa có lợi ích rõ ràng.

**Có thể tách sau:** nếu Discovery/Recommendation phát triển thành hệ thống lớn với yêu cầu tính toán riêng, tách thành context độc lập. Xem [../03-architecture/evolution-to-services.md](../03-architecture/evolution-to-services.md).

### 4.6 Supply Chain Context

**Câu hỏi trả lời:** làm sao có hàng đúng thứ, đúng lượng, đúng lúc?

Đây là context lớn nhất và phức tạp nhất, bao gồm cả phần thông minh (tín hiệu nhu cầu, kế hoạch) lẫn phần thực thi (mua hàng, sản xuất, kiểm định, kho).

**Cân nhắc tách:** context này có thể tách thành hai — `Demand Intelligence` (tín hiệu, dự báo, kế hoạch) và `Supply Execution` (mua, sản xuất, QC, kho). Nhưng ở giai đoạn đầu giữ chung vì chúng thay đổi cùng nhau và chưa có đủ quy mô để tách.

### 4.7 Platform Context

**Câu hỏi trả lời:** hạ tầng kỹ thuật nào mọi context đều cần?

**Ràng buộc nghiêm ngặt:** Platform **không được chứa logic nghiệp vụ**. Nếu một thứ trong Platform bắt đầu biết về "seller" hay "đơn hàng", nó đã đặt sai chỗ.

Đây là cách phòng ngừa vấn đề đã cảnh báo ở nguyên tắc P12 — Platform là nơi dễ trở thành bãi rác phụ thuộc nhất.

---

## 5. Quan hệ giữa các context (Context Map)

Dùng thuật ngữ chuẩn của Domain-Driven Design:

| Từ | Đến | Kiểu quan hệ | Giải thích |
|---|---|---|---|
| Commerce | Inventory | Customer/Supplier | Commerce cần tồn kho, Inventory phục vụ |
| Commerce | Marketplace | Customer/Supplier | Commerce cần quy tắc hoa hồng |
| Commerce | Financial | Published Language | Commerce phát event, Financial ghi sổ |
| Growth | Commerce | Customer/Supplier | Growth cần dữ liệu sản phẩm, đơn hàng |
| Growth | Financial | Published Language | Hoa hồng creator ghi vào ledger |
| Supply Chain | Inventory | Customer/Supplier | Nhập kho làm tăng tồn |
| Supply Chain | Commerce | **Anti-Corruption Layer** | ProductDevelopment → CatalogProduct |
| Supply Chain | Growth | Customer/Supplier | Tín hiệu nhu cầu từ hành vi |
| Mọi context | Platform | Shared Kernel (hạn chế) | Chỉ hạ tầng, không logic |

### Vì sao Supply Chain → Commerce cần Anti-Corruption Layer

Đây là ranh giới có **mô hình khác nhau nhất**:

```text
Supply Chain nói:                 Commerce nghe:
"ProductDevelopment #42            "CatalogProduct mới
 đã duyệt mẫu, tech pack v3,        cần tạo, tên tạm,
 giá vốn dự kiến 120k,              chưa có ảnh,
 nhà cung cấp XYZ,                  chưa publish"
 kế hoạch 500 chiếc"
```

Commerce **không cần biết** tech pack, giá vốn, nhà cung cấp. Nếu để những khái niệm này rò rỉ sang Commerce, mô hình Catalog sẽ bị ô nhiễm bởi khái niệm sản xuất.

ACL là lớp dịch: nhận event từ Supply Chain, chuyển thành khái niệm của Commerce, bỏ đi phần không liên quan.

---

## 6. Ánh xạ giữa các context

Khi cùng một thực thể xuất hiện ở nhiều context:

| Thực thể thực tế | Commerce | Supply Chain | Financial |
|---|---|---|---|
| Một mẫu áo | `CatalogProduct` | `ProductDevelopment` | — |
| Một đơn vị hàng | `SKU` | `SKU` + `ProductionBatch` | — |
| Một nhà bán | `seller_id` (tham chiếu) | — | `AccountHolder` |
| Một đơn hàng | `Order` | — | `LedgerEntry` nhiều dòng |
| Một creator | — | — | `AccountHolder` |

**Quy tắc liên kết:** dùng **định danh chung** (`sku_id`, `seller_id`), không dùng khóa ngoại có ràng buộc cứng giữa các context. Lý do: khóa ngoại cứng ngăn việc tách service sau này.

Xem [../05-data/data-model.md](../05-data/data-model.md).

---

## 7. Quy tắc giao tiếp giữa context

```text
Được phép:
  ✓ Gọi interface công khai của context khác (đồng bộ)
  ✓ Lắng nghe domain event của context khác (bất đồng bộ)
  ✓ Tham chiếu bằng định danh

Không được phép:
  ✗ Truy vấn trực tiếp bảng dữ liệu của context khác
  ✗ JOIN dữ liệu qua ranh giới context
  ✗ Chia sẻ entity giữa hai context
  ✗ Giao dịch database trải rộng hai context
  ✗ Phụ thuộc vòng giữa hai context
```

**Chọn đồng bộ hay bất đồng bộ:**

| Dùng gọi đồng bộ khi | Dùng event khi |
|---|---|
| Cần kết quả ngay để quyết định | Chỉ cần thông báo việc đã xảy ra |
| Ví dụ: kiểm tra tồn kho trước khi cho đặt hàng | Ví dụ: đơn đã đặt → ghi sổ, gửi thông báo |
| Người gọi biết mình cần gì | Người phát không cần biết ai nghe |

Xem [../adr/0006-internal-events.md](../adr/0006-internal-events.md).

---

## 8. Từ Context tới Module

Bounded context là khái niệm thiết kế. Module là đơn vị tổ chức code.

```text
Commerce Context      → catalog, product, pricing, cart, checkout,
                        order, payment (gateway), promotion, fulfillment

Marketplace Context   → marketplace, seller

Inventory Context     → inventory

Financial Context     → payment (ledger phần)

Growth Context        → creator, content, affiliate, campaign,
                        recommendation, loyalty

Supply Chain Context  → supply-chain, procurement, manufacturing,
                        quality, warehouse, return (phần QC hàng hoàn)

Platform Context      → identity, notification, analytics
```

**Lưu ý:** một context có thể gồm nhiều module, nhưng một module **không** được thuộc hai context. Nếu thấy module nào có vẻ thuộc hai context, đó là dấu hiệu module đó cần tách.

Chi tiết từng module: [../04-modules/](../04-modules/).

---

## 9. Tài liệu liên quan

- [domain-map.md](domain-map.md) — phân loại core/supporting/generic
- [aggregates.md](aggregates.md) — aggregate trong từng context
- [domain-events.md](domain-events.md) — event vượt ranh giới context
- [../03-architecture/module-boundaries.md](../03-architecture/module-boundaries.md) — ranh giới ở tầng code
