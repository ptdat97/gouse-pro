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
Module có tầng HTTP               9/17   (catalog · product · identity ·
                                        seller · payment · order ·
                                        marketplace · cart · checkout)
Operation trong OpenAPI          71
Operation đã cài đặt             34      (48%)
Operation MVP còn lại            21      (+1 mới: P1.8 lô giao cho khách)
Operation hoãn (Phase 2/3)       16
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
module "có logic" nhưng chỉ 48% API dùng được.

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
| P0-6 | **Ranh giới giao dịch cho thao tác ghi + audit** | ✅ xong |
| P0-7 | Tách wiring khỏi `cmd/api/main.go` | ⬜ chưa làm — xem điều kiện dưới |

### P0-6 — đã xong, và mô tả ban đầu đã SAI

Mô tả cũ giả định handler sẽ mở giao dịch. **Sai.** Khi triển khai mới thấy
codebase đã có mẫu riêng, dùng ở `cart` và `checkout`:

```text
domain           TxFunc func(ctx) error
                 SaveWithAudit(ctx, entity, fn TxFunc) error
                        ↓
infrastructure   mở tx → ghi entity → fn(ctx mang tx) → commit
                 defer Rollback bắt mọi đường thoát
                        ↓
application       định nghĩa PORT (AuditRecorder), không biết database
                        ↓
module root       adapter: TxFrom(ctx) → audit.WriteSensitive(ctx, tx, ...)
```

**Repository sở hữu giao dịch, không phải handler.** Handler chỉ gọi use
case. Đây là mẫu tốt hơn: tầng interfaces không cần biết database tồn tại.

Đã áp mẫu này cho `seller` và chứng minh bằng endpoint thật
(`POST /api/v1/admin/sellers/{id}/suspend`) thay vì dựng một trừu tượng hóa
chưa ai dùng.

**Kiểm chứng bằng cách phá code** — test fail đúng chỗ:

| Bất biến bị phá | Test bắt được |
|---|---|
| `SaveWithAudit` bỏ qua lỗi của `fn` | `TestDinhChiThatBaiKhongDeLaiTrangThaiNuaVoi` |
| Adapter cho phép ghi vết ngoài giao dịch | `TestGhiVetNgoaiGiaoDichBiTuChoi` |

**Kiểm chứng trên server thật:** lý do rác → 400, và seller **vẫn ACTIVE**,
audit **vẫn rỗng**. Không có trạng thái nửa vời.

### Việc còn lại của P0

Chỉ còn P0-7, và nó có điều kiện.

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

## 3. P1 — Core MVP (21 operation còn lại + 1 mới)

Thứ tự bám theo luồng thương mại, không theo module. Mỗi nhóm chỉ bắt đầu
khi nhóm trước chạy được end-to-end.

### P1.1 — Storefront đọc ✅ (2/2 xong)

| Operation | Module | Ghi chú |
|---|---|---|
| `listProductOffers` | marketplace | Buy box tính RIÊNG cho từng SKU |
| `search` | product | Ghi `search_no_result` qua event |

**Quy tắc hiển thị offer** (`domain.Status.IsVisibleToCustomer`): hết hàng
**vẫn hiện**, chỉ `is_sellable: false` — khách cần biết sản phẩm CÓ tổ hợp
màu/size đó để đăng ký nhận thông báo. Ẩn đi thì họ tưởng nền tảng không
bán, và nhu cầu đó không bao giờ được ghi nhận. DRAFT/SUSPENDED/ARCHIVED
thì ẩn.

**Tín hiệu nhu cầu đã chạy end-to-end:**

```text
search không ra kết quả  →  event search.no_result  →  outbox
                         →  worker  →  demand_signal (SEARCH_NO_RESULT)
```

Kiểm chứng trên hệ thống thật: tìm "ao-len-cashmere-co-lo-mua-dong-2027" →
`demand_signal` có đúng từ khóa đó. Đây là **P2-1 phần thứ nhất**.

Dùng event chứ không gọi đồng bộ: "khách tìm không ra kết quả" là sự việc
ĐÃ XẢY RA, và kết quả tìm kiếm không phụ thuộc việc ghi tín hiệu có thành
công hay không (ADR-0006 phần 3). Ghi hỏng KHÔNG làm hỏng tìm kiếm.

### P1.2 — Tài khoản khách (6 operation)

`getMyProfile` · `updateMyProfile` · `listMyAddresses` · `addMyAddress` ·
`getMyWishlist` · `addWishlistItem` — module `customer`.

### P1.3 — Giỏ hàng và thanh toán (10 operation) — ✅ XONG

`getCart` · `addCartItem` · `updateCartItem` · `removeCartItem` ·
`startCheckout` · `getCheckout` · `setCheckoutShippingAddress` ·
`setCheckoutShippingMethod` · `applyCheckoutCoupon` · `completeCheckout`

**Ràng buộc bắt buộc:** giữ tồn kho là **đồng bộ** (`inventory.Reserve()`),
không phải event — phải biết còn hàng mới cho đặt.

**Danh tính người mua** dùng `httpserver.Shopper` chứ không phải
`AuthContext`: khách VÃNG LAI phải mua được. Chuỗi middleware là
`OptionalAuth → ResolveShopper`, nối ở `cmd/api/shopper.go`.

**Ba khoảng trống đã biết, ghi ở đây thay vì giấu:**

1. **Phí vận chuyển là bảng cứng** (`application.shippingRates`):
   STANDARD 30.000đ, EXPRESS 60.000đ, không theo khoảng cách hay số nguồn
   hàng. docs/04-modules/checkout.md §7 quy định phí đến từ
   `fulfillment.EstimateShipping()` — hàm đó chưa tồn tại. Xem P3-8.
2. **`payment_method` được kiểm tra rồi BỎ QUA.** Module order không có
   trường phương thức thanh toán; đơn luôn ở `PENDING_PAYMENT`. Client gửi
   `CARD` sẽ không thấy lỗi nhưng cũng không có gì bị trừ tiền. Xem P3-9.
3. **`offerLookup` (cart/lookup.go) chưa có test.** Nó gọi bốn module thật
   nên cần cả bốn để kiểm chứng. Đây là tình trạng có từ trước, không phải
   mới. Xem P3-10.

### P1.4 — Đơn hàng (4 operation) — ✅ XONG

`listMyOrders` · `placeOrder` · `getOrder` · `cancelOrder`

`placeOrder` bắt buộc `Idempotency-Key` — đã có ràng buộc UNIQUE ở database.

**`placeOrder` do module `checkout` phục vụ, không phải `order`.** Request
nhận `checkout_id` nên phải đọc phiên thanh toán, và `order` không được gọi
`checkout` (checkout đã phụ thuộc order — ADR-0007). Đường dẫn thuộc khái
niệm "đơn hàng", nhưng NĂNG LỰC thuộc checkout; route đi theo năng lực.

**Quyền xem đơn** có hai đường, không hơn:

```text
Đã đăng nhập  → customer_id phải TRÙNG
Vãng lai      → X-Guest-Phone phải TRÙNG số trên đơn
```

"Không phải đơn của bạn" trả **404 chứ không phải 403**: mã hiển thị của đơn
tăng dần (`FC-2026-08-001234`), nên hai mã khác nhau cho "có thật" và "không
có" sẽ đếm được chính xác số đơn nền tảng bán mỗi tháng.

**Ba khoảng trống đã biết:**

1. **`shipments` KHÔNG có trong response chi tiết đơn.** Dữ liệu lô giao
   thuộc `fulfillment`, mà `order` không được gọi (R5). Đã sửa đặc tả: bỏ
   `shipments` khỏi danh sách bắt buộc và thêm `lines`. Endpoint lô giao cho
   khách CHƯA TỒN TẠI — xem P1.8.
2. **`refund` KHÔNG có trong response hủy đơn.** Số tiền hoàn thuộc
   `payment`. Đoán một con số tệ hơn không trả gì: khách đọc "hoàn trong 3
   ngày" rồi chờ, trong khi không gì cam kết con số đó.
3. **Lọc `status` và phân trang làm ở tầng HTTP**, không trong truy vấn —
   xem P3-11 và P3-12.

### P1.5 — Seller Center (11 operation)

`applyAsSeller` · `listMyOffers` · `createOffer` · `updateOffer` ·
`updateInventory` · `listMyFulfillmentOrders` · `getMyFulfillmentOrder` ·
`shipFulfillmentOrder` · `getMyBalance` · `getMySettlement` ·
`getMyPerformance`

**Ràng buộc bắt buộc:** mọi truy vấn giới hạn theo `AuthContext.SellerIDs`.
Seller không được thấy dữ liệu seller khác dù biết định danh.

### P1.6 — Admin (8/10 xong)

✅ **Trang duyệt hồ sơ seller**: `listSellers` · `getSellerDetail` ·
`approveSeller` · `suspendSeller`

✅ **Tài chính**: `createLedgerAdjustment` — endpoint nhạy cảm nhất hệ
thống, ba lớp bảo vệ nguyên tử với nhau (cân bằng · lý do · vết kiểm toán).

✅ **Đơn hàng (hỗ trợ khách)**: `listAdminOrders` · `getAdminOrderDetail` ·
`cancelAdminOrder` — xem chi tiết đơn **ghi vết việc ĐỌC**, vì response chứa
tên người nhận, số điện thoại và địa chỉ.

⬜ Còn lại (2): `adjustInventory` · `getCustomerAsAdmin`

`executePayouts` đã chuyển sang **FUTURE** — xem mục 6.

**Ba trang của Admin UI giờ đã có đủ API thật:** sellers · audit log ·
orders. Đây là điều kiện để bắt đầu P2-3/P2-4.

Bảy thao tác nhạy cảm bắt buộc `reason` — dùng `audit.WriteSensitive` theo
mẫu đã có ở `seller`.

**Còn thiếu ở `seller.API`:**

```text
✅ ListSellers(ctx, filter)               — đã thêm
✅ SuspendSeller + audit trong giao dịch  — đã thêm
✅ ApproveSeller + hoa hồng + audit       — đã thêm
✅ GetSellerDetail                        — handler dùng thẳng application
```

### P1.7 — Webhook (2 operation)

`receivePaymentWebhook` · `receiveShippingWebhook` — bắt buộc xác minh chữ
ký HMAC. Không có bước này thì bất kỳ ai cũng gửi được "thanh toán thành
công" giả.

### P1.8 — Lô giao cho khách (1 operation) — MỚI

`listOrderShipments` — `GET /api/v1/orders/{order_id}/shipments`, module
`fulfillment` (đã có `GetOrderFulfillments`, chỉ thiếu tầng HTTP).

**Vì sao đây là việc mới chứ không phải phát sinh:** docs mô tả khách thấy
nhiều lô giao với thời gian giao riêng, và `fulfillment.API` đã có sẵn hàm
ghi rõ "dành cho KHÁCH và quản trị viên". Đặc tả chỉ quên khai báo endpoint.
Trang chi tiết đơn của storefront (P2-5) cần nó.

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

## 3d. Đặc tả hứa dữ liệu liên module — BA chỗ, cùng một nguyên nhân

Phát hiện khi triển khai. Không phải ba lỗi riêng lẻ mà là **một kiểu lỗi
lặp lại ba lần** khi viết đặc tả: mô tả những gì MÀN HÌNH cần, rồi khai báo
nguyên xi thành response của MỘT endpoint.

```text
suspendSeller.effects.offers_hidden       seller  →  marketplace   ✗ vòng
listAdminOrders.fulfillment_count         order   →  fulfillment   ✗ vòng
getAdminOrderDetail.fulfillment_orders[]  order   →  fulfillment   ✗ vòng
getAdminOrderDetail.customer{}            order   →  customer      ✗ chưa nối
```

Ba quan hệ đầu tạo **phụ thuộc vòng** — archcheck R5 chặn, và ADR-0007 đã
quyết định chiều phụ thuộc là `fulfillment → order`, không ngược lại.

**Nguyên tắc rút ra:**

> Việc ghép dữ liệu là của **TRANG**, không phải của **ENDPOINT**.
> Admin UI gọi nhiều endpoint rồi ghép lại.

Đó cũng là cách đúng về mặt dữ liệu: trạng thái lô giao và số offer bị ẩn
đều được cập nhật **bất đồng bộ qua event**, nên một con số trả về đồng bộ
tại thời điểm gọi sẽ sai ngay khi trả.

Đặc tả đã sửa cho khớp. Chi tiết trường hợp đầu:

Đặc tả `api/paths/admin.yaml#/seller_suspend` khai báo:

```yaml
effects:
  offers_hidden: 142
  pending_fulfillment_orders: 8
```

**Hai con số này không tồn tại tại thời điểm trả lời.**

```text
Đình chỉ seller  →  phát event  →  marketplace nghe  →  ẩn offer
                                        ▲
                              BẤT ĐỒNG BỘ, xảy ra SAU khi response đã trả
```

Và seller **không được phép** gọi marketplace để đếm: marketplace đã phụ
thuộc seller, chiều ngược lại tạo phụ thuộc vòng — archcheck R5 chặn.

Cài đặt hiện tại trả `effects.note` (quy tắc "đơn đang xử lý KHÔNG bị hủy"),
bỏ hai con số. **Không bịa dữ liệu chưa tồn tại.**

Ba phương án, chọn khi có nhu cầu thật từ giao diện:

```text
A. Bỏ hai trường khỏi đặc tả       — trung thực nhất, sửa 1 file
B. Đổi thành số ƯỚC TÍNH trước khi ẩn (đếm offer đang hiển thị)
   → vẫn cần điểm gộp gọi cả hai module
C. Giao diện gọi riêng marketplace sau khi đình chỉ
   → đúng bản chất bất đồng bộ, nhưng thêm một vòng gọi
```

Nghiêng về **A**. Chọn B hoặc C là quyết định kiến trúc → cần ADR.

---

## 4. P2 — Integration

| # | Việc | Phụ thuộc |
|---|---|---|
| P2-1 | Demand signal từ event (`add_to_cart`, `order_placed`, `search_no_result`) | Đường đã thông (P1.3, P1.4 xong) — xem ghi chú dưới |
| P2-2 | Analytics đọc event, **không phải nguồn sự thật về tiền** | P2-1 |
| P2-3 | Monorepo Next.js + `packages/types` sinh từ OpenAPI | ✅ xong |
| P2-4 | Admin UI — 3 nhóm trang (sellers · audit log · orders) | ✅ xong |
| P2-7 | **CORS** — giao diện ở origin khác gọi được API | ✅ xong |

**P2-1 — trạng thái CHÍNH XÁC.** Ba mảnh đã có và đã nối:

```text
search_no_result  ✅ kiểm chứng end-to-end trên hệ thống thật
add_to_cart       ⬜ handler đã đăng ký, cart đã phát event,
                     CHƯA quan sát được tín hiệu vào bảng
order_placed      ⬜ như trên
```

Hai dòng cuối chưa đánh dấu xong vì **chưa chạy thử được**: database phát
triển không còn offer/SKU hợp lệ nào (test đã TRUNCATE — chính là P3-2), nên
không thêm được món vào giỏ để sinh tín hiệu. Sửa P3-2 trước, rồi kiểm chứng
lại. Đánh dấu xong dựa trên "code trông đúng" là đúng thứ mục 1 cấm.
| P2-5 | Storefront | P1.1–P1.4 |
| P2-6 | Seller Center | P1.5 |

Chi tiết P2-3, P2-4: xem [admin-ui-plan.md](admin-ui-plan.md).

### Frontend nằm ở REPO RIÊNG

```text
gouse-pro/
├── gouse/       — Go backend, đặc tả OpenAPI, tài liệu
└── gouse-web/   — monorepo Next.js
```

Tách repo giữ ranh giới "Next.js không chạm database" thành ranh giới VẬT
LÝ chứ không chỉ là quy ước. Cái giá: `gouse-web` phụ thuộc đường dẫn
`../gouse/api/openapi.yaml` khi sinh kiểu — chấp nhận được vì đặc tả là
nguồn sự thật duy nhất, sao chép sang sẽ tạo bản thứ hai lệch nhau.

Dùng **npm workspaces**, không phải pnpm + Turborepo như kế hoạch ban đầu:
pnpm chưa cài trên máy, và npm 10 đã có workspaces sẵn. Với 1 app + 4
package, Turborepo chưa giải quyết vấn đề nào có thật.

### P2-7 — CORS, phát hiện khi nối hai đầu

Backend **chưa có CORS**, nên trình duyệt chặn mọi lời gọi từ `:3000` sang
`:8080`. Build xanh, typecheck xanh, nhưng chạy thật là chết — thứ chỉ lộ
ra khi thực sự nối hai tiến trình.

`docs/09-operations/security.md` đã quy định "CORS chặt — chỉ domain của
mình", nên đây là việc đã thiết kế, chưa cài.

Bất biến quan trọng nhất: **không bao giờ `*` kèm credentials**. Response
mang cookie phiên, nên cho phép mọi origin nghĩa là bất kỳ trang web nào
cũng gọi được API dưới danh nghĩa người đang đăng nhập. Danh sách rỗng =
chặn tất cả, và ở production không có giá trị mặc định.

---

## 5. P3 — Hardening

| # | Việc | Ghi chú |
|---|---|---|
| P3-1 | **Test HTTP cho auth và audit-log** | Hiện chỉ kiểm chứng bằng curl thủ công |
| P3-2 | **Sửa test suite chập chờn** | `order` và `fulfillment` cùng `TRUNCATE "order"`, chạy song song |
| P3-3 | E2E: Product → Offer → Cart → Checkout → Order → Payment → Fulfillment | Sau P1 |
| P3-4 | Rate limit (`429` + `X-RateLimit-*`) | Đặc tả đã khai báo, chưa cài |
| P3-5 | 2FA cho `ADMIN` và `OPS_FINANCE` | Tăng cường SAU phát hành — chủ dự án đã gỡ khỏi điều kiện chặn (15/08) |
| P3-6 | Observability: metrics, tracing | |
| P3-7 | Chính sách lưu trữ `audit_log` | Bảng chỉ tăng; chờ có số liệu thật |
| P3-8 | **Phí vận chuyển thật** thay bảng cứng trong checkout | Cần `fulfillment.EstimateShipping()` (checkout.md §7) |
| P3-9 | **Nối `payment_method` vào đơn hàng** | Hiện được kiểm tra rồi bỏ qua; đơn luôn `PENDING_PAYMENT` |
| P3-10 | Test cho `cart/lookup.go` (`offerLookup`) | Cần cả bốn module thật; có từ trước P1.3 |
| P3-11 | Lọc `status` của `listMyOrders` trong TRUY VẤN | Hiện lọc sau khi đọc: một trang có thể trả ít hơn `limit` |
| P3-12 | Phân trang theo KHÓA thay vì offset | `next_cursor` hiện là offset mã hóa; đơn mới xen vào có thể làm lặp bản ghi |

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

### Phase 2 — Chi trả tự động (1 operation)

```text
FUTURE — Phase 2
```

`executePayouts` — đặc tả nhận `settlement_ids`, nhưng **`settlement` chưa
có bảng nào trong 40 migration**. `docs/04-modules/payment.md` phân định:

```text
MVP      → Thanh toán một cổng, ledger đầy đủ, ĐỐI SOÁT THỦ CÔNG
Phase 2  → Đối soát tự động, PAYOUT TỰ ĐỘNG, hoàn tiền
```

Phát hiện khi triển khai nhóm finance. Sổ cái đã ghi được payout
(`RecordPayout`), thứ thiếu là khái niệm *kỳ đối soát* gom nhiều đơn lại —
đó mới là việc của Phase 2.

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
