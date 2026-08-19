# Đặc tả OpenAPI

## Nguồn sự thật duy nhất

`openapi.yaml` là **nguồn sự thật duy nhất** về hợp đồng API.

Đặc tả phải được cập nhật **trong cùng pull request** với thay đổi code. CI so sánh đặc tả với cài đặt và **thất bại** nếu lệch.

---

## Cấu trúc

```text
api/
├── openapi.yaml           ← file gốc, tham chiếu tới các file bên dưới
├── components/
│   ├── common.yaml        — kiểu cơ bản, tham số, header, response lỗi
│   └── schemas.yaml       — schema domain (sản phẩm, đơn hàng, tài chính…)
└── paths/
    ├── storefront.yaml    — /api/v1/...            (khách hàng)
    ├── cart-checkout.yaml — /api/v1/cart, /checkout
    ├── orders.yaml        — /api/v1/orders
    ├── account.yaml       — /api/v1/me
    ├── seller.yaml        — /api/v1/seller/...     (nhà bán)
    ├── creator.yaml       — /api/v1/creator/...    (creator)
    ├── admin.yaml         — /api/v1/admin/...      (quản trị)
    └── webhooks.yaml      — /api/v1/webhooks/...   (nhận webhook)
```

**Vì sao tách nhiều file:** một file đơn ~6.000 dòng rất khó rà soát trong pull request và dễ xung đột khi nhiều người sửa cùng lúc. Tách theo nhóm API trùng với ranh giới phân quyền.

---

## Lệnh thường dùng

```bash
# Kiểm tra tính hợp lệ
npx @redocly/cli lint openapi.yaml

# Gộp thành một file (cho công cụ không hỗ trợ $ref nhiều file)
npx @redocly/cli bundle openapi.yaml -o dist/openapi.bundled.yaml

# Sinh kiểu TypeScript cho Next.js
#
# Frontend nằm ở REPO ANH EM ../../gouse-web (không chung repo này).
# Chạy từ bên đó: npm run types
npx openapi-typescript openapi.yaml -o ../../gouse-web/packages/types/openapi.d.ts

# Xem tài liệu tương tác
npx @redocly/cli preview-docs openapi.yaml
```

---

## Quy ước bắt buộc

### Tiền — luôn kèm đơn vị

```yaml
unit_price:
  amount: 299000      # SỐ NGUYÊN theo đơn vị nhỏ nhất (VND: đồng)
  currency: VND
```

Không bao giờ trả số trần. Không bao giờ dùng số thực — sai số tích lũy làm độ lệch đối soát khác 0.

### Idempotency — bắt buộc cho mọi lệnh ghi

```http
POST /api/v1/orders
Idempotency-Key: 01J9XABC123DEF456GHJKMNPQR
```

Key gắn với **ý định của người dùng**, không phải lần gọi mạng. Mọi lần thử lại dùng cùng key.

### Phần trăm — basis points

`1000` = 10,00%. Tránh số thực.

### Định danh — ULID có tiền tố

```text
ord_01J9XABC123DEF456GHJKMNPQR   (26 ký tự Crockford base32, không có I/L/O/U)
```

Tiền tố giúp gỡ lỗi nhanh — nhìn `off_` biết ngay là offer.

### Enum — client phải chịu được giá trị lạ

Server có thể thêm giá trị enum mới trong cùng phiên bản API (thay đổi tương thích ngược). Client phải rơi vào nhánh mặc định, không crash.

---

## Quy tắc thay đổi

```text
TƯƠNG THÍCH NGƯỢC (được phép trong cùng phiên bản):
  ✓ Thêm endpoint
  ✓ Thêm trường tùy chọn vào request
  ✓ Thêm trường vào response
  ✓ Thêm giá trị enum
  ✓ Nới lỏng kiểm tra

PHÁ VỠ (cần /api/v2/):
  ✗ Xóa / đổi tên trường
  ✗ Đổi kiểu dữ liệu
  ✗ Thêm trường bắt buộc vào request
  ✗ Đổi ý nghĩa trường
  ✗ Đổi mã HTTP trả về
  ✗ Siết chặt kiểm tra
```

Không cần nâng phiên bản toàn bộ API — có thể chỉ `/api/v2/orders` trong khi phần còn lại vẫn v1.

---

## Ranh giới phân quyền — không được vi phạm

| Nhóm | Phạm vi dữ liệu |
|---|---|
| Storefront | Dữ liệu của chính khách hàng |
| Seller | **Chỉ** gian hàng của mình. Thấy `FulfillmentOrder`, **không** thấy `Order` |
| Creator | Số liệu **tổng hợp**. **Không bao giờ** thấy danh tính khách hàng |
| Admin | Theo vai trò. Truy cập dữ liệu cá nhân **đều ghi audit** |

Phạm vi được áp dụng **trong truy vấn ở tầng repository**, không phải ở tầng hiển thị — quên lọc một lần là rò rỉ dữ liệu.

---

## Trạng thái hiện tại

```text
71 operation
0 lỗi lint
4 cảnh báo (server URL mẫu và license — cấu hình khi triển khai thật)
532 tham chiếu $ref, tất cả phân giải được
Sinh kiểu TypeScript: thành công
```

---

## Trạng thái cài đặt từng operation

**Đặc tả không phải tài liệu — nó là hợp đồng phải có code phía sau.** Bảng
này theo dõi khoảng cách giữa hai thứ đó.

Cập nhật: 15/08/2026.

### Bốn mức

| Mức | Nghĩa |
|---|---|
| `DESIGNED` | Có trong đặc tả, **chưa có handler** |
| `IMPLEMENTED` | Handler chạy được, chưa có test tự động ở tầng HTTP |
| `TESTED` | Có integration test ở tầng HTTP |
| `INTEGRATED` | Frontend thật đang gọi |

### Tổng hợp

```text
TESTED         23      catalog · product · cart · checkout · orders (khách) ·
                       account (hồ sơ · địa chỉ · yêu thích) · đăng ký
IMPLEMENTED    19      auth · audit-log · admin sellers · ledger · orders ·
                       offers · search · checkout (3 đường cần phiên thật) ·
                       placeOrder
INTEGRATED      3      Admin UI đang gọi: listSellers · listAdminOrders ·
                       listAuditLog (+ các thao tác kèm theo)
DESIGNED       31      (15 MVP còn lại + 16 hoãn Phase 2/3)
                ──
                73
```

### Đã cài đặt (42)

| Operation | Đường dẫn | Mức |
|---|---|---|
| `getBrand` | `GET /api/v1/brands/{brand_id}` | TESTED |
| `getCollection` | `GET /api/v1/collections/{collection_id}` | TESTED |
| `getCategoryTree` | `GET /api/v1/categories` | TESTED |
| `getProduct` | `GET /api/v1/products/{product_id}` | TESTED |
| `listProducts` | `GET /api/v1/products` | TESTED |
| `login` | `POST /api/v1/auth/login` | IMPLEMENTED |
| `refreshSession` | `POST /api/v1/auth/refresh` | IMPLEMENTED |
| `logout` | `POST /api/v1/auth/logout` | IMPLEMENTED |
| `getAdminMe` | `GET /api/v1/admin/me` | IMPLEMENTED |
| `listAuditLog` | `GET /api/v1/admin/audit-log` | IMPLEMENTED |
| `listSellers` | `GET /api/v1/admin/sellers` | IMPLEMENTED |
| `getSellerDetail` | `GET /api/v1/admin/sellers/{id}` | IMPLEMENTED |
| `approveSeller` | `POST /api/v1/admin/sellers/{id}/approve` | IMPLEMENTED |
| `suspendSeller` | `POST /api/v1/admin/sellers/{id}/suspend` | IMPLEMENTED |
| `createLedgerAdjustment` | `POST /api/v1/admin/ledger/adjustments` | IMPLEMENTED |
| `listAdminOrders` | `GET /api/v1/admin/orders` | IMPLEMENTED |
| `getAdminOrderDetail` | `GET /api/v1/admin/orders/{id}` | IMPLEMENTED |
| `cancelAdminOrder` | `POST /api/v1/admin/orders/{id}/cancel` | IMPLEMENTED |
| `listProductOffers` | `GET /api/v1/products/{id}/offers` | IMPLEMENTED |
| `search` | `GET /api/v1/search` | IMPLEMENTED |
| `getCart` | `GET /api/v1/cart` | TESTED |
| `addCartItem` | `POST /api/v1/cart/items` | TESTED |
| `updateCartItem` | `PATCH /api/v1/cart/items/{cart_item_id}` | TESTED |
| `removeCartItem` | `DELETE /api/v1/cart/items/{cart_item_id}` | TESTED |
| `startCheckout` | `POST /api/v1/checkout` | TESTED |
| `getCheckout` | `GET /api/v1/checkout/{checkout_id}` | IMPLEMENTED |
| `setCheckoutShippingAddress` | `PATCH /api/v1/checkout/{id}/shipping-address` | IMPLEMENTED |
| `setCheckoutShippingMethod` | `PATCH /api/v1/checkout/{id}/shipping-method` | TESTED |
| `applyCheckoutCoupon` | `POST /api/v1/checkout/{id}/coupon` | IMPLEMENTED |
| `completeCheckout` | `POST /api/v1/checkout/{id}/complete` | TESTED |

**Về mức của ba operation checkout còn ở `IMPLEMENTED`:** chúng cần một
phiên thanh toán CÓ THẬT, nghĩa là cần inventory giữ được hàng — thuộc test
E2E (P3-3).

| `listMyOrders` | `GET /api/v1/orders` | TESTED |
| `getOrder` | `GET /api/v1/orders/{order_id}` | TESTED |
| `cancelOrder` | `POST /api/v1/orders/{order_id}/cancel` | TESTED |
| `placeOrder` | `POST /api/v1/orders` | IMPLEMENTED |

**`placeOrder` do module `checkout` phục vụ**, không phải `order`: nó phải
đọc phiên thanh toán, mà order không được gọi checkout (ADR-0007). Nó ở mức
`IMPLEMENTED` vì đường thành công cần một phiên có thật — cùng lý do với ba
operation checkout ở trên.

| `getMyProfile` | `GET /api/v1/me` | TESTED |
| `updateMyProfile` | `PATCH /api/v1/me` | TESTED |
| `listMyAddresses` | `GET /api/v1/me/addresses` | TESTED |
| `addMyAddress` | `POST /api/v1/me/addresses` | TESTED |
| `getMyWishlist` | `GET /api/v1/me/wishlist` | TESTED |
| `addWishlistItem` | `POST /api/v1/me/wishlist` | TESTED |

**Mọi endpoint tài khoản BẮT BUỘC đăng nhập.** Khách vãng lai nhận `401`,
không phải hồ sơ rỗng — khác hẳn cart và checkout, vốn phải chạy được cho
khách chưa có tài khoản.

| `registerCustomer` | `POST /api/v1/auth/register` | TESTED |
| `mergeCartOnLogin` | `POST /api/v1/cart/merge` | TESTED |

**`registerCustomer` do module `customer` phục vụ**, không phải `identity`:
một lần đăng ký sinh ra tài khoản đăng nhập VÀ hồ sơ mua hàng, mà identity
nằm dưới customer trong đồ thị phụ thuộc. Nó **không trả token** — client
gọi `login` ngay sau đó.

**Đây là hai operation KHÔNG có trong đặc tả ban đầu.** Tính năng thì có
trong docs (`01-business/customer.md` mục 1), chỉ endpoint là thiếu.

**Về mức của nhóm checkout:** `TESTED` ở đây nghĩa là có test HTTP cho những
gì handler CHẶN — quyền sở hữu giỏ, phương thức thanh toán, phương thức vận
chuyển, `Idempotency-Key`. Đường đi THÀNH CÔNG của `completeCheckout` cần cả
inventory, marketplace và order chạy thật; nó thuộc test E2E (P3-3), chưa
làm.

Mười lăm operation `IMPLEMENTED` đã được kiểm chứng bằng **chạy server thật**,
nhưng chưa có test tự động ở **tầng HTTP** — nợ kỹ thuật P3-1 trong
[backlog](../docs/10-roadmap/backlog.md). Riêng `suspendSeller` có nghiệp vụ
được phủ bởi test module chạy trên PostgreSQL thật (ranh giới giao dịch,
kiểm tra lý do, ghi vết kiểm toán); phần chưa phủ tự động là lớp HTTP:
phân quyền, hình dạng JSON, ánh xạ lỗi.

**Đặc tả đã được sửa cho khớp kiến trúc — ba chỗ.** Cả ba đều là cùng một
kiểu lỗi: đặc tả hứa dữ liệu TỔNG HỢP TỪ NHIỀU MODULE mà ranh giới phụ
thuộc không cho phép. Chi tiết ở
[backlog](../docs/10-roadmap/backlog.md) mục 3d.

```text
suspendSeller.effects.offers_hidden       → seller không gọi được marketplace
listAdminOrders.fulfillment_count         → order không gọi được fulfillment
getAdminOrderDetail.fulfillment_orders[]  → cùng lý do (ADR-0007, R5)
getAdminOrderDetail.customer{}            → thay bằng customer_id
```

Việc ghép dữ liệu là của **TRANG**, không phải của **ENDPOINT**: Admin UI
gọi nhiều endpoint rồi ghép lại.

### MVP còn lại (35) — `DESIGNED`

| Nhóm | Số | Operation |
|---|---|---|
| Account | 6 | `getMyProfile` · `updateMyProfile` · `listMyAddresses` · `addMyAddress` · `getMyWishlist` · `addWishlistItem` |
| Cart & Checkout | 10 | `getCart` · `addCartItem` · `updateCartItem` · `removeCartItem` · `startCheckout` · `getCheckout` · `setCheckoutShippingAddress` · `setCheckoutShippingMethod` · `applyCheckoutCoupon` · `completeCheckout` |
| Orders | 4 | `listMyOrders` · `placeOrder` · `getOrder` · `cancelOrder` |
| Seller | 11 | `applyAsSeller` · `listMyOffers` · `createOffer` · `updateOffer` · `updateInventory` · `listMyFulfillmentOrders` · `getMyFulfillmentOrder` · `shipFulfillmentOrder` · `getMyBalance` · `getMySettlement` · `getMyPerformance` |
| Admin | 2 | `adjustInventory` · `getCustomerAsAdmin` |
| Webhooks | 2 | `receivePaymentWebhook` · `receiveShippingWebhook` |

### Hoãn (16) — `FUTURE`

Đặc tả **giữ nguyên**; chỉ hoãn phần cài đặt.

| Giai đoạn | Số | Operation |
|---|---|---|
| Phase 2 | 12 | `applyAsCreator` · `getMyEarnings` · `getMyAnalytics` · `listMyContent` · `createContent` · `publishContent` · `getContent` · `getOutfit` · `listMyAffiliateLinks` · `createAffiliateLink` · `getFeed` · `requestReturn` |
| Phase 2 | 1 | `executePayouts` — `settlement` **chưa có bảng nào**; payment.md ghi "payout tự động → Phase 2". MVP là **đối soát thủ công** |
| Phase 2+ | 1 | `listProductReviews` — **không có module sở hữu** trong 17 module MVP |
| Phase 3 | 2 | `createProductionOrder` · `listReplenishmentSuggestions` |

### Quy tắc thêm operation mới

Chỉ thêm khi phục vụ một trong bốn thứ sau:

```text
✓ MVP          ✓ seller/admin workflow
✓ frontend thật ✓ integration test
```

Không thêm vì *"sau này có thể cần"*.

---

## Tài liệu liên quan

- Chuẩn API chi tiết: [../docs/06-api/api-guidelines.md](../docs/06-api/api-guidelines.md)
- Phân nhóm và phân quyền: [../docs/06-api/api-domains.md](../docs/06-api/api-domains.md)
- Nguyên tắc API First: [../docs/03-architecture/api-first.md](../docs/03-architecture/api-first.md)
- Idempotency: [../docs/05-data/idempotency.md](../docs/05-data/idempotency.md)
- Quyết định kiến trúc: [../docs/adr/0002-api-first.md](../docs/adr/0002-api-first.md)
