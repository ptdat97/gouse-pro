# Ranh giới module

## 1. Mục đích

Tài liệu này định nghĩa, cho **mỗi module**:

- Trách nhiệm
- Dữ liệu sở hữu
- Interface công khai
- Event phát ra
- Event lắng nghe
- Phụ thuộc được phép

Đây là **hợp đồng**. Thay đổi interface công khai của một module là thay đổi có ảnh hưởng, phải được xem xét kỹ.

---

## 2. Mẫu khai báo chuẩn

Mỗi module được mô tả theo mẫu:

```text
Module:              tên
Bounded Context:     thuộc context nào
Phân loại:           Core | Supporting | Generic
Sở hữu bảng:         danh sách bảng
Interface công khai: các phương thức
Event phát:          danh sách
Event nghe:          danh sách
Phụ thuộc:           module nào, kiểu gì
```

Chi tiết đầy đủ từng module nằm ở [../04-modules/](../04-modules/). Tài liệu này tổng hợp phần ranh giới.

---

## 3. Bảng tổng hợp sở hữu dữ liệu

| Module | Bảng sở hữu |
|---|---|
| `identity` | `user`, `user_credential`, `role`, `permission`, `user_role`, `session` |
| `customer` | `customer`, `customer_address`, `customer_preference`, `customer_consent`, `wishlist` |
| `catalog` | `category`, `brand`, `collection`, `size_chart`, `brand_authorization` |
| `product` | `product`, `variant`, `sku`, `product_attribute`, `product_image` |
| `pricing` | `price_list`, `price_rule`, `price_history` |
| `marketplace` | `offer`, `offer_price_history`, `buy_box_snapshot` |
| `seller` | `seller`, `seller_store`, `seller_document`, `seller_bank_account`, `seller_policy`, `seller_performance` |
| `inventory` | `inventory_item`, `reservation`, `inventory_movement`, `stock_location` |
| `cart` | `cart`, `cart_item` |
| `checkout` | `checkout`, `checkout_line`, `checkout_session` |
| `order` | `order`, `order_line`, `order_address`, `order_status_history` |
| `payment` | `payment`, `payment_intent`, `ledger_entry`, `ledger_line`, `account`, `settlement`, `settlement_line`, `payout`, `refund` |
| `fulfillment` | `fulfillment_order`, `fulfillment_line`, `shipment`, `shipment_tracking` |
| `return` | `return_request`, `return_line`, `return_inspection` |
| `promotion` | `promotion`, `promotion_rule`, `coupon`, `coupon_usage` |
| `creator` | `creator`, `creator_channel`, `creator_bank_account`, `creator_tier` |
| `content` | `content`, `content_media`, `product_tag`, `outfit`, `outfit_item`, `review` |
| `affiliate` | `affiliate_link`, `click`, `attribution`, `commission_record` |
| `campaign` | `campaign`, `campaign_participant`, `campaign_rule`, `campaign_budget` |
| `recommendation` | `recommendation_rule`, `user_affinity` |
| `loyalty` | `loyalty_account`, `point_transaction`, `tier_rule` |
| `supply-chain` | `demand_signal`, `demand_aggregate`, `forecast`, `product_development`, `tech_pack`, `sample`, `production_plan`, `replenishment_suggestion` |
| `procurement` | `supplier`, `supplier_capability`, `supplier_certification`, `supplier_performance`, `purchase_order`, `purchase_order_line`, `supplier_quotation` |
| `manufacturing` | `production_order`, `production_order_line`, `production_batch`, `bill_of_materials` |
| `quality` | `quality_inspection`, `defect_record`, `quality_standard` |
| `warehouse` | `warehouse`, `warehouse_zone`, `goods_receipt`, `pick_list`, `packing_record` |
| `notification` | `notification_template`, `notification_log`, `notification_preference` |
| `analytics` | `event_log`, `metric_snapshot` |

**Quy tắc:** bảng nào không có trong danh sách này thì chưa được tạo. Thêm bảng mới phải cập nhật tài liệu này.

---

## 4. Interface công khai — các module quan trọng

### 4.1 `inventory`

```go
type PublicAPI interface {
    // Truy vấn — hỗ trợ theo lô để tránh N+1
    GetAvailability(ctx, skuIDs []string, locationID string) (map[string]int, error)
    CheckAvailability(ctx, items []AvailabilityRequest) (*AvailabilityResult, error)

    // Giữ hàng
    Reserve(ctx, req ReserveRequest) (*ReservationResult, error)
    ReleaseReservation(ctx, reservationID string) error
    ExtendReservation(ctx, reservationID string, ttl time.Duration) error

    // Cam kết và xuất
    Commit(ctx, reservationID string) error
    Ship(ctx, req ShipRequest) error

    // Nhập kho
    Receive(ctx, req ReceiveRequest) error
    Adjust(ctx, req AdjustRequest) error
}
```

**Lưu ý thiết kế:** `GetAvailability` nhận **danh sách** SKU, không nhận một SKU. Điều này bắt buộc bên gọi phải nghĩ theo lô và tránh vấn đề N+1 ngay từ thiết kế interface.

### 4.2 `marketplace`

```go
type PublicAPI interface {
    // Offer
    GetOffer(ctx, offerID string) (*OfferView, error)
    GetOffersBySKUs(ctx, skuIDs []string) (map[string][]OfferView, error)
    GetBuyBoxOffer(ctx, skuID string) (*OfferView, error)

    // Quy tắc hoa hồng — trả về tỷ lệ để module khác ĐÓNG BĂNG
    GetCommissionRate(ctx, req CommissionRateRequest) (Percentage, error)

    // Kiểm tra quyền bán
    CanSellerCreateOffer(ctx, sellerID, skuID string) (bool, string, error)
}
```

**Lưu ý:** `GetCommissionRate` **chỉ trả về tỷ lệ**. Việc tính số tiền và đóng băng vào đơn là trách nhiệm của `order`. Marketplace không biết về đơn hàng.

### 4.3 `payment`

```go
type PublicAPI interface {
    // Thanh toán của khách
    CreatePaymentIntent(ctx, req PaymentIntentRequest) (*PaymentIntent, error)
    GetPaymentStatus(ctx, paymentID string) (*PaymentStatus, error)

    // Sổ cái — điểm vào DUY NHẤT để ghi tài chính
    RecordLedgerEntry(ctx, req LedgerEntryRequest) (*LedgerEntryResult, error)

    // Truy vấn số dư — KHÔNG module nào khác được tự tính
    GetBalance(ctx, accountType string, ownerID string) (*Balance, error)

    // Hoàn tiền
    IssueRefund(ctx, req RefundRequest) (*Refund, error)
}
```

**Ràng buộc quan trọng:** `GetBalance` là **cách duy nhất** để biết số dư. Không module nào được cộng trừ các bản ghi để tự tính. Đây là nguyên tắc P8.

### 4.4 `order`

```go
type PublicAPI interface {
    PlaceOrder(ctx, cmd PlaceOrderCommand) (*OrderResult, error)
    GetOrderSummary(ctx, orderID string) (*OrderSummary, error)
    GetOrdersByCustomer(ctx, customerID string, page Pagination) (*OrderList, error)
    CancelOrder(ctx, orderID string, reason string) error
    CancelOrderLine(ctx, orderLineID string, quantity int, reason string) error
}
```

**Lưu ý:** không có phương thức `GetOrder` trả về toàn bộ aggregate. Chỉ có `GetOrderSummary` trả DTO chỉ đọc. Điều này ngăn module khác thao tác trực tiếp lên đơn hàng.

### 4.5 `catalog` và `product`

```go
// catalog
type PublicAPI interface {
    GetCategory(ctx, categoryID string) (*CategoryView, error)
    GetCategoryTree(ctx) (*CategoryTree, error)
    GetBrand(ctx, brandID string) (*BrandView, error)
    GetBrandsByIDs(ctx, brandIDs []string) (map[string]BrandView, error)
    GetCollection(ctx, collectionID string) (*CollectionView, error)
    IsBrandProtected(ctx, brandID string) (bool, ProtectionLevel, error)
    GetSizeChart(ctx, sizeChartID string) (*SizeChartView, error)
}

// product
type PublicAPI interface {
    GetProduct(ctx, productID string) (*ProductView, error)
    GetProductsByIDs(ctx, productIDs []string) (map[string]ProductView, error)
    GetSKU(ctx, skuID string) (*SKUView, error)
    GetSKUsByIDs(ctx, skuIDs []string) (map[string]SKUView, error)
    GetVariantsByProduct(ctx, productID string) ([]VariantView, error)
    CreateProductFromDevelopment(ctx, req CreateFromDevelopmentRequest) (*ProductView, error)
}
```

Phương thức cuối cùng là **Anti-Corruption Layer** cho luồng own brand — nhận dữ liệu từ Supply Chain và chuyển thành khái niệm của Catalog. Xem [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md) mục 5.

---

## 5. Bảng tổng hợp Event

### Event phát ra theo module

| Module | Event phát |
|---|---|
| `order` | `order.placed`, `order.paid`, `order.cancelled`, `order.line_cancelled`, `order.completed` |
| `payment` | `payment.captured`, `payment.failed`, `ledger.entry_recorded`, `settlement.created`, `payout.executed`, `refund.issued` |
| `inventory` | `inventory.reserved`, `inventory.reservation_released`, `inventory.committed`, `inventory.received`, `inventory.depleted`, `inventory.low_stock` |
| `fulfillment` | `fulfillment_order.created`, `fulfillment_order.shipped`, `fulfillment_order.delivered`, `fulfillment_order.completed`, `fulfillment_order.delivery_failed` |
| `marketplace` | `offer.created`, `offer.price_changed`, `offer.out_of_stock` |
| `seller` | `seller.applied`, `seller.approved`, `seller.suspended`, `seller.performance_updated` |
| `creator` | `creator.approved`, `creator.suspended` |
| `content` | `content.published`, `content.taken_down` |
| `affiliate` | `affiliate.click_recorded`, `affiliate.conversion_attributed`, `affiliate.attribution_reversed` |
| `return` | `return.requested`, `return.approved`, `return.received`, `return.inspected`, `return.refunded` |
| `supply-chain` | `demand_signal.recorded`, `product_development.approved`, `replenishment.suggested` |
| `manufacturing` | `production_order.created`, `production_batch.completed` |
| `quality` | `quality.approved`, `quality.rejected` |
| `warehouse` | `warehouse.goods_received` |
| `product` | `product.published`, `product.unpublished` |
| `cart` | `cart.item_added`, `cart.abandoned` |
| `checkout` | `checkout.started`, `checkout.expired` |

### Event lắng nghe theo module

| Module | Event nghe | Để làm gì |
|---|---|---|
| `inventory` | `order.placed` | Reserved → Committed |
| | `order.cancelled` | Giải phóng hàng |
| | `checkout.expired` | Giải phóng reservation |
| | `warehouse.goods_received` | Tăng tồn kho |
| | `return.inspected` | Nhập lại hàng bán được |
| `fulfillment` | `order.paid` | Tạo fulfillment order |
| `payment` | `order.placed` | Ghi bút toán doanh thu, hoa hồng |
| | `fulfillment_order.completed` | Chuyển số dư Pending → Available |
| | `return.refunded` | Ghi bút toán hoàn tiền |
| | `affiliate.conversion_attributed` | Ghi hoa hồng creator |
| `affiliate` | `order.placed` | Xác nhận quy kết |
| | `return.refunded` | Đảo ngược hoa hồng |
| `order` | `payment.captured` | Chuyển trạng thái sang Paid |
| | `fulfillment_order.delivered` | Cập nhật trạng thái tổng hợp |
| `marketplace` | `inventory.depleted` | Cập nhật offer sang hết hàng |
| | `seller.suspended` | Ẩn offer |
| `notification` | Nhiều event | Gửi thông báo |
| `analytics` | Hầu hết event | Ghi nhận thống kê |
| `supply-chain` | `inventory.depleted` | Ghi tín hiệu nhu cầu bị bỏ lỡ |
| | `cart.item_added` | Ghi tín hiệu nhu cầu |
| | `order.placed` | Ghi tín hiệu nhu cầu |
| `loyalty` | `order.completed` | Tích điểm |
| `catalog` | `inventory.depleted` | Cập nhật trạng thái hiển thị |

---

## 6. Ma trận trách nhiệm module

Ai chịu trách nhiệm việc gì — tránh trùng lặp sở hữu nghiệp vụ:

| Câu hỏi nghiệp vụ | Module trả lời |
|---|---|
| Sản phẩm này tên gì, mô tả ra sao? | `product` |
| Thuộc thương hiệu nào, bộ sưu tập nào? | `catalog` |
| Ai đang bán, giá bao nhiêu? | `marketplace` |
| Giá gốc, quy tắc giá là gì? | `pricing` |
| Còn hàng không, ở đâu? | `inventory` |
| Có mã giảm giá nào áp dụng được? | `promotion` |
| Khách này là ai? | `customer` |
| Người này có quyền làm việc này không? | `identity` (xác thực) + module sở hữu (phân quyền) |
| Đơn hàng này có gì? | `order` |
| Ai giao đơn này, đến đâu rồi? | `fulfillment` |
| Tiền của ai, bao nhiêu? | `payment` |
| Seller này có được bán không? | `seller` |
| Hoa hồng bao nhiêu phần trăm? | `marketplace` (quy tắc), `order` (đóng băng), `payment` (ghi sổ) |
| Creator nào được tính công đơn này? | `affiliate` |
| Nội dung này có gì? | `content` |
| Nên gợi ý sản phẩm nào? | `recommendation` |
| Cần sản xuất bao nhiêu? | `supply-chain` |
| Lô hàng này đạt chất lượng không? | `quality` |
| Giá vốn thật là bao nhiêu? | `manufacturing` (theo lô) |

**Dòng về hoa hồng** minh họa nguyên tắc: một khái niệm nghiệp vụ có thể đi qua nhiều module, nhưng mỗi module có **vai trò khác nhau và rõ ràng** — định nghĩa quy tắc, áp dụng, ghi sổ. Không có hai module cùng làm một việc.

---

## 7. Kiểm tra chồng lấn trách nhiệm

Dấu hiệu hai module đang tranh giành sở hữu:

| Dấu hiệu | Ví dụ | Cách sửa |
|---|---|---|
| Hai module cùng tính một con số | Cả `order` và `payment` tính hoa hồng | Chỉ một nơi tính, nơi kia dùng lại |
| Hai module cùng lưu một dữ liệu | Cả `seller` và `payment` lưu số dư | Xác định chủ sở hữu duy nhất |
| Cùng một quy tắc ở hai nơi | Kiểm tra hàng tồn ở cả `cart` và `checkout` | Đưa về `inventory` |
| Hai module cùng phát event giống nhau | Cả hai phát "đơn hoàn tất" | Xác định ai là nguồn sự thật |

**Trong tài liệu này đã giải quyết:**

```text
Hoa hồng:
    marketplace  → định nghĩa quy tắc (tỷ lệ nào áp dụng)
    order        → đóng băng vào OrderLine tại thời điểm đặt
    payment      → ghi bút toán vào ledger
    → ba vai trò khác nhau, không trùng

Số dư seller:
    payment      → NGUỒN SỰ THẬT DUY NHẤT
    seller       → chỉ hiển thị, gọi payment.GetBalance()
    → không lưu trùng

Trạng thái hết hàng:
    inventory    → NGUỒN SỰ THẬT (số lượng thật)
    marketplace  → cache trạng thái hiển thị của offer, cập nhật qua event
    → offer.status là dữ liệu dẫn xuất, không phải nguồn sự thật
```

---

## 8. Quy trình thay đổi interface công khai

Interface công khai là hợp đồng. Thay đổi phải theo quy trình:

```text
Thêm phương thức mới:
    → An toàn, không cần quy trình đặc biệt

Thêm tham số tùy chọn:
    → An toàn nếu có giá trị mặc định hợp lý

Đổi tên / xóa phương thức:
    1. Đánh dấu deprecated, giữ hoạt động
    2. Thông báo các module đang dùng
    3. Chờ mọi bên chuyển đổi
    4. Xóa

Đổi ngữ nghĩa (cùng tên, hành vi khác):
    → CẤM. Tạo phương thức mới với tên khác.
```

Quy tắc cuối quan trọng nhất: đổi hành vi mà giữ nguyên tên là cách chắc chắn nhất để tạo ra lỗi khó tìm.

---

## 9. Tài liệu liên quan

- [dependency-rules.md](dependency-rules.md) — quy tắc phụ thuộc
- [modular-monolith.md](modular-monolith.md) — cấu trúc module
- [../04-modules/](../04-modules/) — đặc tả chi tiết từng module
- [../05-data/data-model.md](../05-data/data-model.md) — ma trận sở hữu dữ liệu
