# Implementation Backlog

**Đây là backlog DUY NHẤT của dự án.** Mọi việc còn phải làm đều nằm ở đây.

**Cập nhật:** 15/08/2026 · **Giai đoạn:** Implementation Completion

| Tài liệu | Vai trò |
|---|---|
| **backlog.md** (file này) | Việc CÒN PHẢI LÀM — nguồn sự thật duy nhất |
| [todo.md](todo.md) | Việc ĐÃ LÀM + bằng chứng kiểm chứng (hồi cứu) |
| [admin-ui-plan.md](admin-ui-plan.md) | Ghi chép quyết định thiết kế lát cắt admin/frontend |

Đừng thêm backlog thứ hai. Việc mới ghi vào đây.

---

## 0. Tình hình hiện tại — con số thật

Đếm từ code ngày 15/08/2026, không phải ước lượng:

```text
Module MVP có logic nghiệp vụ    17/17
Module có tầng HTTP               3/17   (catalog · product · identity)
Operation trong OpenAPI          71
Operation đã cài đặt             10      (14%)
Operation MVP còn lại            46
Operation hoãn (Phase 2/3)       15
```

**Khoảng trống lớn nhất là tầng HTTP.** Domain, application, infrastructure,
PostgreSQL, event bus, test đều đã có. Thứ thiếu là đường nối chúng ra
ngoài:

```text
Application  →  HTTP Handler  →  OpenAPI  →  API thật  →  Next.js
                     ▲
              ĐÂY là chỗ nghẽn
```

---

## 1. Định nghĩa "xong"

Một tính năng CHỈ được đánh dấu xong khi có đủ:

```text
Domain
  ↓
Application
  ↓
Infrastructure
  ↓
API (HTTP handler)
  ↓
OpenAPI (đặc tả khớp cài đặt)
  ↓
Integration test
  ↓
Architecture check (archcheck xanh)
```

Có frontend thì thêm:

```text
API  →  Next.js UI  →  E2E flow
```

**Viết xong domain + application KHÔNG phải là xong.** Đó là lý do 17/17
module "có logic" nhưng chỉ 14% API dùng được.

---

## 2. P0 — Blocking

Việc mà không có nó thì mọi việc khác không làm được.

| # | Việc | Trạng thái |
|---|---|---|
| P0-1 | Middleware xác thực (`Auth`, `RequireRole`) | ✅ xong |
| P0-2 | Middleware `RequireIdempotencyKey` | ✅ xong |
| P0-3 | Phát hành access token (`platform/token`) | ✅ xong |
| P0-4 | Endpoint đăng nhập/làm mới/đăng xuất + `/admin/me` | ✅ xong |
| P0-5 | Audit log (`platform/audit`) + endpoint đọc | ✅ xong |
| P0-6 | **Ranh giới giao dịch cho handler ghi dữ liệu** | ⬜ **chưa làm** |
| P0-7 | Tách wiring khỏi `cmd/api/main.go` | ⬜ chưa làm — xem điều kiện dưới |

### P0-6 là việc P0 duy nhất còn lại

Chưa có handler nào MỞ giao dịch. Catalog và product chỉ đọc; auth tự quản
lý giao dịch bên trong module. Nhưng mọi endpoint ghi ở P1 đều cần:

```text
Mở tx  →  gọi use case  →  ghi audit CÙNG tx  →  commit
```

Không có mẫu chuẩn cho việc này thì mỗi handler sẽ tự nghĩ một kiểu, và
việc "audit ghi cùng giao dịch nghiệp vụ" — thứ `platform/audit` được thiết
kế để bảo đảm — sẽ bị bỏ qua ở chỗ nào đó mà không ai nhận ra.

**Phải làm trước endpoint ghi đầu tiên**, không phải sau.

### P0-7 — tách wiring, nếu và chỉ nếu cần

Số đo hiện tại (15/08/2026):

```text
cmd/api/main.go       479 dòng
func run()            340 dòng
module được wiring     14
```

P1 sẽ thêm khoảng 10 lần đăng ký route module nữa vào chính hàm này. Làm
việc tách **sau** P1 nghĩa là sửa wiring hai lần.

Nếu làm, giữ đúng ba ràng buộc:

```text
cmd/api/main.go  →  internal/platform/bootstrap  →  Modules

✗ KHÔNG dùng DI framework chỉ để giải quyết wiring
✗ KHÔNG tạo trừu tượng hóa không cần thiết
✓ Dependency injection tường minh, giữ mẫu New(Config)
```

Đây là **tái cấu trúc, không phải tính năng** — không tự làm nếu chưa thấy
vướng thật. Ghi ở đây để quyết định có ý thức, không phải để mặc định làm.

---

## 3. P1 — Core MVP (46 operation)

Thứ tự bám theo luồng thương mại, không theo module. Mỗi nhóm chỉ bắt đầu
khi nhóm trước chạy được end-to-end.

### P1.1 — Storefront đọc (2 operation)

| Operation | Module | Ghi chú |
|---|---|---|
| `listProductOffers` | marketplace | Buy box — offer nào hiển thị |
| `search` | product/catalog | Tìm kiếm; **ghi demand signal khi không có kết quả** |

`search` ghi `search_no_result` ngay từ MVP — đây là dữ liệu Phase 3 cần mà
không tạo ngược được (xem [../04-modules/supply-chain.md](../04-modules/supply-chain.md)).

### P1.2 — Tài khoản khách (6 operation)

`getMyProfile` · `updateMyProfile` · `listMyAddresses` · `addMyAddress` ·
`getMyWishlist` · `addWishlistItem` — module `customer`.

### P1.3 — Giỏ hàng và thanh toán (10 operation)

`getCart` · `addCartItem` · `updateCartItem` · `removeCartItem` ·
`startCheckout` · `getCheckout` · `setCheckoutShippingAddress` ·
`setCheckoutShippingMethod` · `applyCheckoutCoupon` · `completeCheckout`

**Ràng buộc bắt buộc:** giữ tồn kho là **đồng bộ** (`inventory.Reserve()`),
không phải event — phải biết còn hàng mới cho đặt.

### P1.4 — Đơn hàng (4 operation)

`listMyOrders` · `placeOrder` · `getOrder` · `cancelOrder`

`placeOrder` bắt buộc `Idempotency-Key` — đã có ràng buộc UNIQUE ở database.

### P1.5 — Seller Center (11 operation)

`applyAsSeller` · `listMyOffers` · `createOffer` · `updateOffer` ·
`updateInventory` · `listMyFulfillmentOrders` · `getMyFulfillmentOrder` ·
`shipFulfillmentOrder` · `getMyBalance` · `getMySettlement` ·
`getMyPerformance`

**Ràng buộc bắt buộc:** mọi truy vấn giới hạn theo `AuthContext.SellerIDs`.
Seller không được thấy dữ liệu seller khác dù biết định danh.

### P1.6 — Admin (11 operation)

`listSellers` · `getSellerDetail` · `approveSeller` · `suspendSeller` ·
`createLedgerAdjustment` · `executePayouts` · `adjustInventory` ·
`getCustomerAsAdmin` · `listAdminOrders` · `getAdminOrderDetail` ·
`cancelAdminOrder`

Bảy thao tác nhạy cảm bắt buộc `reason` — dùng `audit.WriteSensitive`.

**Ba năng lực còn thiếu ở `seller.API`, phải bổ sung:**

```text
ListSellers(ctx, filter)              — chưa có, trang duyệt hồ sơ cần
ApproveSeller  + chính sách hoa hồng  — hiện chỉ nhận (sellerID, approvedBy)
SuspendSeller  + trả effects          — chưa trả offers_hidden
```

### P1.7 — Webhook (2 operation)

`receivePaymentWebhook` · `receiveShippingWebhook` — bắt buộc xác minh chữ
ký HMAC. Không có bước này thì bất kỳ ai cũng gửi được "thanh toán thành
công" giả.

---

## 3b. Bảy luồng bắt buộc chạy được

Đây là **tiêu chí nghiệm thu** của MVP. Chi tiết nghiệp vụ từng luồng ở
[../07-workflows/](../07-workflows/); phần dưới chỉ nêu điều kiện "chạy
được".

| # | Luồng | Điều kiện bắt buộc |
|---|---|---|
| 1 | Admin → Product → Variant → SKU → Publish → Storefront | Sản phẩm chưa duyệt trả **404**, không phải 403 |
| 2 | Seller → Offer → Price → Inventory → Publish → khách thấy | Seller bị đình chỉ thì mọi offer ẩn |
| 3 | Khách → Product → Offer → thêm/sửa/xóa giỏ | Món hết hàng **đánh dấu**, không tự xóa khỏi giỏ |
| 4 | Giỏ → Checkout → tính giá → khuyến mãi → giữ tồn kho → Order | Giữ tồn kho **đồng bộ**; giá **đóng băng** tại thời điểm khách thấy |
| 5 | Order → Payment → Ledger | Idempotency · số nguyên · ranh giới giao dịch · xử lý thất bại |
| 6 | Order → FulfillmentOrder → Line → Shipment → Delivered → Settlement | Tách đơn theo seller; seller không thấy đơn seller khác |
| 7 | Hành vi khách → Domain event → Demand signal | Phát lại event **không** ghi hai lần |

Luồng 7 phải làm ngay từ MVP dù chưa có dự báo hay AI — dữ liệu nhu cầu
không tạo ngược được.

**Trạng thái hiện tại:** cả 7 luồng đã chạy được ở tầng **domain/application**
(có test, có database thật — xem [todo.md](todo.md) mục 5). Chưa luồng nào
chạy được qua **HTTP**. Đó chính là nội dung P1.

---

## 3c. Kết quả rà soát tính toàn vẹn (15/08/2026)

Rà soát trên code thật, không phải đọc tài liệu.

### Tài chính — ĐẠT

```text
Không có float32/float64 nào ở đường tiền
(money · pricing · ledger · commission · settlement · payout)
```

Kiểm chứng bằng cách quét toàn bộ `internal/`, loại trừ analytics và JSON.
Kết quả rỗng. Tiền dùng số nguyên theo đơn vị nhỏ nhất; phần trăm dùng basis
points.

Sổ cái bất biến ở tầng database (trigger, migration 000007). Analytics
**không** phải nguồn sự thật về tiền — nó chỉ nhận event.

### Tồn kho — ĐẠT

Sáu trạng thái trong `inventory/domain/quantities.go`:

```text
available → reserved → committed → in_transit
                    ↘ released (về available)
              damaged · returned
```

Rộng hơn vòng đời tối thiểu bốn trạng thái, vì thời trang có tỷ lệ hoàn
hàng cao. `inventory` là nơi DUY NHẤT sở hữu trạng thái tồn kho — `order`
không tự quản lý.

Khóa lạc quan đã kiểm chứng dưới tải song song: 20 khách mua 1 sản phẩm →
đúng 1 người thắng.

### Không cần sửa gì

Hai mục này **đã đạt yêu cầu**, ghi lại để lần rà soát sau không phải làm
lại từ đầu.

---

## 4. P2 — Integration

| # | Việc | Phụ thuộc |
|---|---|---|
| P2-1 | Demand signal từ event (`add_to_cart`, `order_placed`, `search_no_result`) | P1.3, P1.4 |
| P2-2 | Analytics đọc event, **không phải nguồn sự thật về tiền** | P2-1 |
| P2-3 | Monorepo Next.js + `packages/types` sinh từ OpenAPI | P1 |
| P2-4 | Admin UI — 3 nhóm trang (sellers · audit log · orders) | P1.6 |
| P2-5 | Storefront | P1.1–P1.4 |
| P2-6 | Seller Center | P1.5 |

Chi tiết P2-3, P2-4: xem [admin-ui-plan.md](admin-ui-plan.md).

---

## 5. P3 — Hardening

| # | Việc | Ghi chú |
|---|---|---|
| P3-1 | **Test HTTP cho auth và audit-log** | Hiện chỉ kiểm chứng bằng curl thủ công |
| P3-2 | **Sửa test suite chập chờn** | `order` và `fulfillment` cùng `TRUNCATE "order"`, chạy song song |
| P3-3 | E2E: Product → Offer → Cart → Checkout → Order → Payment → Fulfillment | Sau P1 |
| P3-4 | Rate limit (`429` + `X-RateLimit-*`) | Đặc tả đã khai báo, chưa cài |
| P3-5 | 2FA cho `ADMIN` và `OPS_FINANCE` | **Chặn phát hành Admin UI** |
| P3-6 | Observability: metrics, tracing | |
| P3-7 | Chính sách lưu trữ `audit_log` | Bảng chỉ tăng; chờ có số liệu thật |

P3-1 và P3-2 là **nợ kỹ thuật đã biết**, không phải việc phát sinh — ghi ở
đây để không bị quên.

---

## 6. FUTURE — không làm trong giai đoạn này

15 operation đã có đặc tả nhưng **không cài đặt bây giờ**. Đặc tả giữ
nguyên; chỉ hoãn phần cài đặt.

### Phase 2 — Creator Commerce (12 operation)

```text
FUTURE — Phase 2
```

| Operation | Module chưa tồn tại |
|---|---|
| `applyAsCreator` · `getMyEarnings` · `getMyAnalytics` | `creator` |
| `listMyContent` · `createContent` · `publishContent` · `getContent` · `getOutfit` | `content` |
| `listMyAffiliateLinks` · `createAffiliateLink` | `affiliate` |
| `getFeed` | `content` / `recommendation` |
| `requestReturn` | `return` |

### Phase 2+ (1 operation)

```text
FUTURE — Phase 2+
```

`listProductReviews` — **không có module sở hữu** trong 17 module MVP.

### Phase 3 — Chuỗi cung ứng (2 operation)

```text
FUTURE — Phase 3
```

`createProductionOrder` · `listReplenishmentSuggestions` — module
`supply-chain` hiện chỉ ghi `demand_signal`, chưa có dự báo hay lập kế hoạch.

### Không có đặc tả, không làm

```text
FUTURE      Recommendation ML
FUTURE      Manufacturing Intelligence
FUTURE      Demand Forecasting
FUTURE      Live Commerce
FUTURE      Advanced Attribution
```

Những mục này **chưa có trong docs**. Muốn làm phải viết ADR trước, không tự
thiết kế thêm.

---

## 7. Quy tắc nhận việc mới

Trước khi viết dòng code nào, trả lời:

```text
Việc này làm được bằng kiến trúc hiện có không?

CÓ                  → implement
KHÔNG, thiếu một    → sửa tối thiểu, ghi lý do trong code
  trừu tượng nhỏ
KHÔNG, cần đổi      → viết ADR TRƯỚC
  kiến trúc
```

Nếu lý do chỉ là *"sau này có thể cần"*, *"OSS khác có"*, *"chuẩn hơn"* →
**không làm**.

Và:

> **Tính năng đã có trong docs → xây nó.
> Chưa có trong docs → không tự thiết kế thêm; đề xuất ADR khi thật sự cần.**

---

## 8. Tài liệu liên quan

- [../README.md](../README.md) — Architecture Freeze
- [todo.md](todo.md) — việc đã làm và bằng chứng kiểm chứng
- [../../api/README.md](../../api/README.md) — trạng thái từng operation
- [mvp.md](mvp.md) — phạm vi MVP (đã đóng băng)
- [future-phases.md](future-phases.md) — Phase 2, 3, 4 (FUTURE)
