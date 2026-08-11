# Aggregates

## 1. Aggregate là gì và vì sao quan trọng

Aggregate là **cụm entity được đảm bảo nhất quán trong một giao dịch**. Nó có một entity làm gốc (aggregate root) — cửa ngõ truy cập duy nhất từ bên ngoài.

Ba quy tắc bắt buộc:

```text
1. Một giao dịch database chỉ sửa MỘT aggregate
2. Truy cập từ bên ngoài chỉ qua aggregate root
3. Tham chiếu tới aggregate khác chỉ bằng ĐỊNH DANH, không bằng con trỏ
```

**Vì sao quy tắc 1 quan trọng:** nếu một giao dịch sửa nhiều aggregate, sẽ có tranh chấp khóa và không thể tách service sau này. Nhất quán giữa các aggregate là **nhất quán cuối**, đạt được qua domain event.

**Cách xác định ranh giới aggregate — câu hỏi cần trả lời:**

> "Nếu hai thứ này không nhất quán trong một khoảnh khắc, có gây hậu quả nghiêm trọng không?"

- Có → cùng aggregate
- Không → aggregate riêng, đồng bộ qua event

---

## 2. Danh sách aggregate theo context

### 2.1 Commerce Context

| Aggregate Root | Entity con | Bất biến cần giữ |
|---|---|---|
| `Product` | `Variant`, `ProductAttribute`, `ProductImage` | Mỗi variant có tổ hợp thuộc tính duy nhất |
| `Collection` | `CollectionItem` | Ngày kết thúc sau ngày bắt đầu |
| `Brand` | `BrandAuthorization` | — |
| `Offer` | `OfferPrice` | Giá > 0; một seller chỉ có một offer active/SKU |
| `Cart` | `CartItem` | Số lượng > 0; không trùng offer trong cart |
| `Checkout` | `CheckoutLine`, `CheckoutAddress` | Tổng tiền = Σ dòng + phí − giảm giá |
| `Order` | `OrderLine`, `OrderAddress` | Tổng tiền bất biến sau khi đặt |
| `FulfillmentOrder` | `FulfillmentLine`, `Shipment` | Σ số lượng FO = số lượng Order tương ứng |
| `PromotionRule` | `PromotionCondition` | — |

### 2.2 Marketplace Context

| Aggregate Root | Entity con | Bất biến |
|---|---|---|
| `Seller` | `SellerDocument`, `SellerBankAccount`, `SellerContact` | Seller active phải có tài khoản ngân hàng đã xác minh |
| `SellerStore` | `StoreBanner` | — |
| `SellerPolicy` | `CommissionRule` | Tỷ lệ hoa hồng trong [0, 1] |
| `SellerPerformanceRecord` | — | — |

### 2.3 Inventory Context

| Aggregate Root | Entity con | Bất biến |
|---|---|---|
| `InventoryItem` | `InventoryStateQuantity` | **Σ mọi trạng thái = tổng số lượng vật lý**; mọi số lượng ≥ 0 |
| `Reservation` | — | Có thời hạn hết hiệu lực |
| `StockLocation` | — | — |
| `InventoryMovement` | — | Bất biến sau khi tạo |

### 2.4 Financial Context

| Aggregate Root | Entity con | Bất biến |
|---|---|---|
| `LedgerEntry` | `LedgerLine` | **Σ ghi nợ = Σ ghi có**; bất biến tuyệt đối |
| `Account` | — | — |
| `Settlement` | `SettlementLine` | Tổng = Σ dòng |
| `Payout` | — | Số tiền > 0 |

### 2.5 Growth Context

| Aggregate Root | Entity con | Bất biến |
|---|---|---|
| `Creator` | `CreatorChannel`, `CreatorBankAccount` | — |
| `Content` | `ProductTag`, `ContentMedia` | Content published phải có ≥ 1 media |
| `Outfit` | `OutfitItem` | Ít nhất 2 sản phẩm |
| `AffiliateLink` | — | — |
| `Attribution` | — | Bất biến sau khi tạo |
| `Campaign` | `CampaignParticipant`, `CampaignRule` | Ngày kết thúc sau ngày bắt đầu; ngân sách ≥ 0 |
| `LoyaltyAccount` | `PointTransaction` | Số dư điểm = Σ giao dịch điểm |

### 2.6 Supply Chain Context

| Aggregate Root | Entity con | Bất biến |
|---|---|---|
| `ProductDevelopment` | `TechPack`, `Sample`, `BillOfMaterials` | Không chuyển sang sản xuất khi chưa duyệt mẫu |
| `Supplier` | `SupplierCapability`, `SupplierCertification` | — |
| `PurchaseOrder` | `PurchaseOrderLine` | Số lượng ≥ MOQ của nhà cung cấp |
| `ProductionOrder` | `ProductionOrderLine` | Phải có tech pack đã duyệt |
| `ProductionBatch` | `BatchQuantity` | Giá vốn đơn vị > 0 |
| `QualityInspection` | `DefectRecord` | Cỡ mẫu ≤ số lượng lô |
| `ProductionPlan` | `PlanLine` | Σ phân bổ size = tổng số lượng |
| `DemandSignal` | — | Bất biến sau khi tạo |

### 2.7 Return (thuộc Commerce, phối hợp Supply Chain)

| Aggregate Root | Entity con | Bất biến |
|---|---|---|
| `ReturnRequest` | `ReturnLine` | Số lượng trả ≤ số lượng đã mua |
| `ReturnInspection` | — | — |

---

## 3. Phân tích các quyết định ranh giới khó

### 3.1 Vì sao `Order` và `FulfillmentOrder` là hai aggregate riêng?

Đây là quyết định quan trọng nhất trong mô hình này.

**Lập luận cho việc gộp:** chúng thay đổi cùng nhau, đều thuộc về "đơn hàng".

**Lập luận cho việc tách (đã chọn):**

```text
1. Chủ sở hữu khác nhau
   Order thuộc về khách hàng
   FulfillmentOrder thuộc về seller/kho

2. Vòng đời khác nhau
   Order: Placed → Paid → Completed
   FulfillmentOrder: Pending → Picking → Shipped → Delivered
   Một Order có thể "đang xử lý" trong khi ba FO ở ba trạng thái khác nhau

3. Tần suất thay đổi khác nhau
   Order gần như không đổi sau khi đặt
   FulfillmentOrder thay đổi liên tục theo tiến trình vận chuyển

4. Ràng buộc bảo mật
   Seller được xem FulfillmentOrder của mình
   Seller KHÔNG được xem Order (chứa hàng của seller khác)

5. Tranh chấp ghi
   Nếu gộp, ba seller cập nhật trạng thái đồng thời sẽ tranh chấp
   trên cùng một bản ghi Order
```

Lý do 4 và 5 là quyết định — chúng không thể giải quyết được nếu gộp.

**Nhất quán giữa hai aggregate:**

```text
Order tạo → phát OrderPlaced
    ↓
FulfillmentOrder được tạo (mỗi seller một cái)
    ↓
Mỗi FO hoàn tất → phát FulfillmentCompleted
    ↓
Order lắng nghe, cập nhật trạng thái tổng hợp khi tất cả FO xong
```

Có một khoảng thời gian ngắn Order đã tồn tại mà FO chưa có. Đây là **chấp nhận được** — khách không thấy khác biệt.

### 3.2 Vì sao `Offer` không nằm trong aggregate `Product`?

```text
1. Product có thể có hàng trăm offer từ hàng trăm seller
   → aggregate quá lớn, tải toàn bộ để sửa một offer là lãng phí

2. Seller sửa offer của mình rất thường xuyên (giá, tồn kho)
   → nếu cùng aggregate, mọi seller tranh chấp trên cùng Product

3. Chủ sở hữu khác nhau
   → Product thuộc nền tảng/danh mục chuẩn, Offer thuộc seller

4. Vòng đời khác nhau
   → Product tồn tại lâu dài, Offer đến và đi theo seller
```

Lý do 2 là quyết định. Với sản phẩm phổ biến có 50 seller, gộp chung sẽ tạo điểm nghẽn ghi nghiêm trọng.

### 3.3 Vì sao `InventoryItem` tách khỏi `Offer`?

Thoạt nhìn có vẻ tồn kho thuộc về offer.

```text
Lý do tách:

1. Một SKU có thể có tồn kho ở nhiều địa điểm
   → own brand: kho HN, kho HCM
   → mô hình 1 offer : 1 số lượng không đủ diễn đạt

2. Tồn kho có mô hình trạng thái phức tạp riêng
   → Available/Reserved/Committed/InTransit/Damaged/Returned
   → nhồi vào Offer làm Offer phình to

3. Tồn kho được dùng bởi cả Commerce lẫn Supply Chain
   → nhập kho từ sản xuất cũng làm tăng tồn kho
   → nếu tồn kho nằm trong Offer, Supply Chain phải phụ thuộc Commerce

4. Yêu cầu tranh chấp đồng thời khác biệt
   → giữ tồn kho khi live commerce cần xử lý riêng
```

**Quan hệ:** `Offer` tham chiếu `sku_id` và `seller_id`. `InventoryItem` cũng khóa theo `sku_id` + `stock_location_id` + `inventory_owner_id`. Khi hiển thị offer, hệ thống hỏi Inventory số lượng khả dụng.

### 3.4 Vì sao `LedgerEntry` là aggregate, không phải `Account`?

Cách trực giác: `Account` là aggregate, chứa các bút toán và một số dư.

**Vấn đề với cách đó:**

```text
1. Account của nền tảng sẽ có hàng triệu bút toán
   → aggregate vô hạn, không tải được

2. Mọi giao dịch đều chạm vào Account nền tảng
   → điểm nghẽn ghi nghiêm trọng nhất hệ thống

3. Số dư lưu sẵn trong Account có thể lệch với tổng bút toán
   → vi phạm nguyên tắc nguồn sự thật duy nhất
```

**Cách đã chọn:**

```text
LedgerEntry là aggregate — bất biến, độc lập, ghi được song song
Account chỉ là danh mục (ai sở hữu tài khoản nào)
Balance là kết quả TÍNH TOÁN từ các bút toán
   → có thể lưu bản chụp (snapshot) để tăng tốc,
     nhưng bản chụp luôn kiểm chứng lại được từ bút toán
```

Xem [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md).

### 3.5 Vì sao `Cart` và `Checkout` là hai aggregate?

```text
Cart:
  - sống lâu (nhiều ngày, nhiều phiên)
  - thay đổi tự do
  - KHÔNG giữ tồn kho
  - giá là giá hiện tại, cập nhật động

Checkout:
  - sống ngắn (vài phút)
  - CÓ giữ tồn kho tạm thời
  - giá được ĐÓNG BĂNG
  - có thời hạn hết hiệu lực
```

Đây là hai thứ khác nhau về bản chất. Gộp chung sẽ dẫn tới việc hoặc là giỏ hàng giữ tồn kho (sai — hàng bị khóa vô ích), hoặc là checkout không đóng băng giá (sai — giá đổi giữa chừng thanh toán).

### 3.6 Vì sao `ProductionBatch` tách khỏi `ProductionOrder`?

```text
Một ProductionOrder có thể sinh ra nhiều ProductionBatch:
  - sản xuất chia làm nhiều đợt
  - mỗi đợt có kết quả QC riêng
  - mỗi đợt có thể có giá vốn hơi khác

ProductionBatch còn được tham chiếu bởi:
  - InventoryItem (lô nào đang trong kho)
  - QualityInspection
  - việc truy vết thu hồi

→ Batch có vòng đời riêng, tồn tại lâu hơn Order
```

---

## 4. Sơ đồ aggregate chính — luồng thương mại

```text
   Product ──────┐
      │          │
   Variant       │ (tham chiếu bằng id)
      │          │
     SKU ────────┼──────────┐
      │          │          │
      ▼          ▼          ▼
   Offer    InventoryItem  ProductionBatch
      │          │
      │          │ (kiểm tra khả dụng)
      ▼          │
    Cart ────────┘
      │
      ▼
  Checkout  ──── (giữ tồn kho tạm thời)
      │
      ▼
    Order
      │
      ├──────────┬──────────┐
      ▼          ▼          ▼
    FO-A       FO-B       FO-C
      │          │          │
      ▼          ▼          ▼
  Shipment   Shipment   Shipment
      │
      ▼
  LedgerEntry (nhiều bút toán)
```

**Đường nét liền** = quan hệ trong cùng aggregate hoặc tham chiếu trực tiếp
**Đường qua id** = tham chiếu bằng định danh giữa các aggregate

---

## 5. Ràng buộc bất biến quan trọng nhất

Ba bất biến sau, nếu vi phạm, gây hậu quả nghiêm trọng:

### 5.1 Tồn kho không âm và tổng đúng

```text
InventoryItem:
    available + reserved + committed + in_transit + damaged + returned
    = tổng số lượng vật lý

    mọi thành phần ≥ 0
```

Vi phạm → bán hàng không có, hoặc hàng bị khóa vĩnh viễn.

**Cách bảo vệ:** mọi thay đổi tồn kho đi qua phương thức của aggregate root, có khóa lạc quan (optimistic locking) trên phiên bản. Xem [../05-data/consistency.md](../05-data/consistency.md).

### 5.2 Bút toán cân bằng

```text
LedgerEntry:
    Σ ghi nợ = Σ ghi có
```

Vi phạm → sổ sách sai, không đối soát được.

**Cách bảo vệ:** kiểm tra trong constructor của aggregate. Không tạo được bút toán lệch.

### 5.3 Tổng fulfillment khớp đơn hàng

```text
Với mỗi OrderLine:
    Σ số lượng trong các FulfillmentLine tương ứng = số lượng OrderLine
    (trừ phần đã hủy)
```

Vi phạm → giao thiếu hoặc giao thừa, tiền không khớp.

---

## 6. Kích thước aggregate — cảnh báo

| Dấu hiệu aggregate quá lớn | Hệ quả |
|---|---|
| Có collection không giới hạn số lượng | Không tải nổi |
| Nhiều người dùng sửa đồng thời | Tranh chấp khóa |
| Tải aggregate để đọc một trường nhỏ | Lãng phí |
| Aggregate có > 5–7 loại entity con | Khó hiểu, khó test |

| Dấu hiệu aggregate quá nhỏ | Hệ quả |
|---|---|
| Phải sửa nhiều aggregate trong một thao tác nghiệp vụ | Nhất quán khó đảm bảo |
| Bất biến nghiệp vụ trải rộng nhiều aggregate | Không bảo vệ được |
| Quá nhiều event chỉ để đồng bộ nội bộ | Phức tạp không cần thiết |

**Nguyên tắc thực dụng:** ưu tiên aggregate **nhỏ**. Aggregate nhỏ dễ mở rộng quy mô và dễ tách. Chỉ gộp khi có bất biến nghiệp vụ thật sự bắt buộc phải giữ tức thời.

---

## 7. Tài liệu liên quan

- [entities.md](entities.md) — chi tiết từng entity
- [value-objects.md](value-objects.md) — các value object
- [domain-events.md](domain-events.md) — event đồng bộ giữa aggregate
- [../05-data/consistency.md](../05-data/consistency.md) — mô hình nhất quán
