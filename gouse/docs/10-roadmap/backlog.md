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

### P1.2 — Tài khoản khách (6 operation) — ✅ XONG

`getMyProfile` · `updateMyProfile` · `listMyAddresses` · `addMyAddress` ·
`getMyWishlist` · `addWishlistItem` — module `customer`.

**Mọi endpoint ở đây BẮT BUỘC đăng nhập** — khác hẳn cart và checkout, vốn
phải chạy được cho khách vãng lai. Hồ sơ và sổ địa chỉ không có nghĩa gì nếu
không có tài khoản, và cho khách vãng lai đi qua nghĩa là mọi request ẩn
danh cùng trỏ vào một hồ sơ "định danh rỗng".

**KHÔNG đổi được email qua `updateMyProfile`.** Đổi email là đổi DANH TÍNH,
cần xác minh quyền sở hữu địa chỉ mới — cho đi qua là mở đường chiếm tài
khoản người khác. `DisallowUnknownFields` biến việc này thành `400` thay vì
bỏ qua im lặng.

**Hai khoảng lệch với đặc tả, đã sửa đặc tả:**

1. **`preferences` chưa trả về.** Số đo cơ thể là dữ liệu nhạy cảm, và yêu
   cầu của chính lược đồ là "mã hóa khi lưu". Module chưa có chỗ lưu đã mã
   hóa; lưu dạng thô sẽ VI PHẠM đúng yêu cầu đó. Thiếu một trường tùy chọn
   thì giao diện chịu được, lưu sai cách thì không sửa ngược được. → **P3-14**.

2. **`getMyWishlist` trả `product_id`, không trả `ProductSummary`.** Tên,
   ảnh, giá thuộc module `product`, mà `customer` nằm CÙNG TẦNG với nó — mũi
   tên phụ thuộc chỉ đi từ trên xuống. Trang gọi `listProducts` MỘT lần cho
   cả danh sách; đó cũng là cách ít truy vấn hơn.

Thêm cột `wishlist_item.notify_when_available` (migration 000023): đặc tả
cho khách bật cờ này, nhận rồi vứt đi thì nút bấm là nút giả — và nền tảng
mất thước đo nhu cầu KHÔNG ĐƯỢC ĐÁP ỨNG.

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

### P1.5 — Seller Center (7/11 xong)

**Đã xong: `listMyOffers` · `createOffer` · `updateOffer` ·
`updateInventory` · `listMyFulfillmentOrders` · `getMyFulfillmentOrder` ·
`shipFulfillmentOrder`** — mở khóa **luồng 2 VÀ luồng 6**, hai trong bảy
tiêu chí nghiệm thu MVP. Trước đó seller không đăng bán được gì (mọi offer
đều do seed tạo) và đơn hàng nằm `PENDING` vĩnh viễn.

Luồng 2 kiểm chứng khép kín trên hệ thống thật:

```text
seller lưu trữ offer cũ → tạo offer mới 390.000đ kèm nhập kho 25
→ hàng vào KHO RIÊNG của seller, chủ sở hữu là seller (không phải nền tảng)
→ kiểm kê lại còn 40 → nhật ký ghi ADJUST 15 kèm lý do
→ khách thấy offer mới ở trang sản phẩm
```

#### Kiểm kê nhận con số TUYỆT ĐỐI, và phép trừ nằm TRONG vòng thử lại

Đặc tả nhận `quantity_available` tuyệt đối, còn `inventory.Adjust` nhận
chênh lệch. Tính chênh lệch ở tầng gọi rồi mới ghi là **đọc-rồi-ghi ngoài
vòng khóa lạc quan** — đúng lỗi vừa sửa ở bộ đếm khuyến mãi. Giữa hai bước,
một khách đặt hàng làm số khả dụng đổi, và chênh lệch cũ áp lên số mới cho
ra con số KHÔNG PHẢI cái seller đã đếm.

Đã thêm `inventory.SetAvailable`: phép trừ nằm bên trong vòng thử lại, nên
xung đột làm đọc lại và tính lại.

Kiểm kê ĐÚNG BẰNG số hiện tại thì **không ghi nhật ký** — một dòng "điều
chỉnh 0 đơn vị" làm loãng thứ người ta đọc khi đi tìm hàng thất lạc.

#### `initial_inventory` là bắt buộc, không phải tùy chọn

Seller tạo offer mà không nhập kho thì offer HẾT HÀNG từ giây đầu tiên, và
họ không có đường nào để nhập: `updateInventory` chỉ SỬA bản ghi đã có.

Cổng `InventoryPort` của marketplace CHỈ ĐỌC và giữ nguyên như vậy — module
này không giành lấy quyền tạo tồn kho. Thay vào đó tầng HTTP khai báo một
cổng ghi riêng, `cmd/api` nối với inventory (cùng mẫu với `TokenVerifier`).

Nhập kho thất bại KHÔNG hủy offer: offer đã tồn tại và hợp lệ, chỉ là chưa
có hàng. Hủy nó để "sạch sẽ" là vứt đi thứ seller vừa tạo thành công.

#### Quyền sở hữu offer: quy tắc chuyển xuống tầng application

Ban đầu tôi để nó ở handler. Sai — nó được hỏi từ MỌI đường ghi của seller
(đổi giá, đưa lên bán, lưu trữ), và mỗi nơi tự kiểm lại nghĩa là sớm muộn
một nơi quên. Một nơi quên là đủ để bất kỳ ai hạ giá offer đối thủ về 1đ,
mà định danh offer thì LỘ RA ở trang công khai.

Một sai lầm nữa đã sửa: tôi từng gộp `changedBy` (ai sửa — cho vết kiểm
toán) làm khóa phân quyền. Hai khái niệm khác nhau: quản trị viên cũng sửa
giá được. Test cũ đỏ ngay và chỉ ra điều đó.

---

**Ba operation đơn thực hiện** mở khóa **luồng 6**. Trước đó đơn hàng nằm `PENDING` vĩnh viễn vì không
ai thao tác được.

Kiểm chứng khép kín trên hệ thống thật:

```text
đặt đơn → tách thành FC-2026-08-000002-A
seller thấy: "Áo sơ mi linen Oxford · Trắng / M · SL 1 · 490.000đ"
seller bàn giao: VN987654321 / GHN
trạng thái đơn tự tính lại → SHIPPED
khách tra đơn → thấy đúng mã vận đơn
```

#### Seller phải biết NHẶT GÌ — và điều đó cần dữ liệu mới

Đơn thực hiện trước đây chỉ lưu MÃ dòng hàng. Seller mở ra thấy một danh
sách mã. Với thời trang, tên sản phẩm cũng KHÔNG đủ: cùng một chiếc áo có
năm size nằm ở năm ô kệ khác nhau.

Thiếu nó thì seller phải mở đơn hàng gốc — mà quy tắc bảo mật KHÔNG cho họ
xem đơn gốc (họ sẽ thấy hàng của seller khác, email khách, tổng tiền đơn).

Đã thêm ảnh chụp thông tin nhặt hàng vào `fulfillment_order_line`
(migration 000024), lấy từ payload event `checkout.completed` — event đã
mang sẵn `product_name`, chỉ chưa mang `variant_description` và `unit_price`.
Đã bổ sung cả hai.

Có CẢ `unit_price` lẫn `line_total` là chủ ý: chia `line_total` cho
`quantity` là phép chia số nguyên và nó làm tròn sai với giá không chia hết.

#### Một nút "bàn giao", nhưng máy trạng thái vẫn nguyên vẹn

Đặc tả cho nhà bán ĐÚNG MỘT hành động, còn domain đòi đi qua
`CONFIRMED → PACKED → HANDED_OVER`. `RecordHandOver` đi hết đường hợp lệ
ngắn nhất thay vì nhảy thẳng — nhảy thẳng sẽ phải nới đồ thị chuyển trạng
thái, và khi đó những bước bảo vệ khác (như "đã đóng gói thì không hủy
được") cũng lỏng theo.

`confirmed_at` và `packed_at` ghi bằng thời điểm bàn giao. Đó là thông tin
tốt nhất ta có, và bỏ trống thì chỉ số hiệu suất không tính được.

#### Ranh giới bảo mật nằm ở HAI lớp

Câu SQL (`WHERE seller_id = $1`) và kiểm tra ở tầng application
(`BelongsTo`). Khi viết test tôi phá riêng lớp application thì test VẪN
XANH — vì câu SQL còn giữ. Phải phá cả hai lớp test mới đỏ. Đó là phòng thủ
nhiều lớp hoạt động đúng như thiết kế.

### P1.10 — Địa chỉ giao trên đơn thực hiện — ✅ XONG

Seller biết **nhặt gì** nhưng không biết **gửi đi đâu**. Đã sửa theo đúng
cách của thông tin nhặt hàng: thêm vào payload event `checkout.completed`,
thêm cột (migration 000025), sao chép xuống TỪNG đơn thực hiện lúc tách.

Sao chép xuống từng đơn chứ không lưu một chỗ: một đơn có hàng từ hai nguồn
thì CẢ HAI seller cùng giao tới một nơi, và mỗi người cần phiếu giao riêng.

**KHÔNG kèm email khách.** Chỉ những trường cần để giao: người nhận, số điện
thoại (gọi trước khi giao) và địa chỉ. Email không giúp giao hàng, và mọi
trường thừa là dữ liệu cá nhân trao cho bên thứ ba không cần tới nó. Đơn
thực hiện vẫn lưu `notify_email` cho module notification, nhưng trường đó
KHÔNG ra tới API của seller — đã kiểm chứng trên response thật.

Phiếu giao hàng đầy đủ, chạy thật:

```text
PHIẾU GIAO HÀNG — FC-2026-08-000004-A
  Gửi tới: Khách FO · +84911222333
           9 Hai Bà Trưng, TP. Hồ Chí Minh
  Nhặt:    Áo sơ mi linen Oxford · Trắng / M · SL 1
  Phải trả seller: 490.000đ
```

**Một bài học vận hành khi kiểm chứng:** lần chạy đầu địa chỉ KHÔNG xuống
đơn thực hiện, dù payload event đã có. Nguyên nhân: một tiến trình worker
CŨ (chạy từ trước khi sửa) vẫn còn sống và xử lý event trước. Khi triển khai
thật, thay đổi payload event đòi hỏi worker phải được triển khai TRƯỚC hoặc
cùng lúc với bên phát — nếu không, event mới đi qua bên nhận cũ và dữ liệu
mới bị bỏ im lặng.

### P1.5 — Seller Center: 8 operation còn lại

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

### P1.9 — Đăng ký và đăng nhập cho khách (2 operation) — ✅ XONG

`registerCustomer` (`POST /api/v1/auth/register`) ·
`mergeCartOnLogin` (`POST /api/v1/cart/merge`)

**Vì sao đây không phải việc tự phát sinh:** đăng ký khách hàng có trong
docs từ đầu — `01-business/customer.md` mục 1 (vòng đời Guest → Registered)
và `04-modules/customer.md` mục 5 (gộp danh tính). `identity.Register` đã
tồn tại sẵn ở tầng application; đặc tả chỉ quên khai báo endpoint.

**Module `customer` phục vụ `/api/v1/auth/register`, không phải `identity`.**
Một lần đăng ký sinh ra HAI thứ ở hai module: tài khoản đăng nhập và hồ sơ
mua hàng. identity nằm DƯỚI customer trong đồ thị phụ thuộc nên không gọi
ngược lên được; customer gọi xuống thì hợp lệ.

**Đăng ký KHÔNG trả token** — client gọi `login` ngay sau đó. Phát hành
token là việc của identity; làm ở hai chỗ là nhân bản logic quản lý phiên.

#### Quyết định bảo mật: email đã dùng để đặt hàng vãng lai thì TỪ CHỐI

```text
hồ sơ CÓ user_id      → "đã có tài khoản, vui lòng đăng nhập"
hồ sơ KHÔNG có        → "email đã từng đặt hàng, tra đơn bằng mã + SĐT"
chưa có hồ sơ         → tạo tài khoản
```

Hồ sơ vãng lai chứa lịch sử mua hàng và địa chỉ nhà. Gắn nó vào tài khoản
vừa đăng ký nghĩa là bất kỳ ai biết email người khác đều đọc được những thứ
đó (customer.md mục 5). Gộp chỉ được phép SAU KHI xác minh quyền sở hữu
email — luồng đó chưa dựng, xem P3-15.

Hai thông báo phải PHÂN BIỆT vì chúng dẫn tới hai hành động khác hẳn: trả
chung một câu đẩy nhóm thứ hai đi bấm "quên mật khẩu" cho một tài khoản
không tồn tại.

**Thứ tự các bước có chủ ý:** kiểm tra hồ sơ TRƯỚC khi tạo tài khoản. Đảo
lại thì mỗi lần từ chối để lại một tài khoản mồ côi, và lần đăng ký sau báo
"email đã dùng" vì chính tài khoản đó.

#### Giới hạn tần suất là BẮT BUỘC, không phải tùy chọn

`identity/public.go` ghi rõ: endpoint đăng ký CỐ Ý phân biệt "email đã có
tài khoản" với "chưa dùng", nên nó trả lời được câu hỏi đó — không giới hạn
thì nó là công cụ dò danh sách email. Đã thêm
[`httpserver.RateLimit`](../../internal/platform/httpserver/ratelimit.go),
5 lần / 10 phút / IP.

**Một lỗi bắt được khi chạy thật:** bộ đếm dùng `RemoteAddr` nguyên văn, mà
chuỗi đó kèm CỔNG NGUỒN — đổi theo từng kết nối TCP. Mỗi request thành một
khóa khác và bộ đếm không bao giờ chạm hạn mức: 7 lần đăng ký liên tiếp đều
qua với hạn mức 5. Log sạch, test bằng khóa cố định vẫn xanh, tác dụng thật
bằng không. Đã cắt cổng và thêm test.

**Giới hạn đã biết:** bộ đếm nằm trong BỘ NHỚ, không chia sẻ giữa các tiến
trình — chạy ba bản sao thì kẻ tấn công được gấp ba lượt. Bộ đếm dùng chung
cần Redis; thêm một phụ thuộc hạ tầng cho MVP là cái giá lớn hơn lợi ích.

#### Gộp giỏ khi đăng nhập

`cart.MergeOnLogin` đã tồn tại nhưng CHƯA từng được nối. Không gộp thì khách
thêm hàng lúc chưa đăng nhập, đăng nhập xong thấy giỏ trống — và họ nghĩ hệ
thống mất dữ liệu của mình.

Endpoint KHÔNG nhận tham số: cả hai định danh đã có ở máy chủ (mã khách hàng
từ token, mã phiên từ cookie). Cho client truyền vào là để ai cũng gộp được
giỏ của phiên người khác vào tài khoản mình.

Kiểm chứng trên hệ thống thật: giỏ vãng lai 980.000đ → đăng ký → đăng nhập →
gộp → **cùng mã giỏ, cùng tổng tiền**. Đó là nhánh "đổi chủ", giữ nguyên mọi
nguồn giới thiệu.

### P1.8 — Lô giao cho khách (1 operation) — ✅ XONG

`listOrderShipments` — `GET /api/v1/orders/{order_id}/shipments`, module
`fulfillment` (đã có `GetOrderFulfillments`, chỉ thiếu tầng HTTP).

**Vì sao đây là việc mới chứ không phải phát sinh:** docs mô tả khách thấy
nhiều lô giao với thời gian giao riêng, và `fulfillment.API` đã có sẵn hàm
ghi rõ "dành cho KHÁCH và quản trị viên". Đặc tả chỉ quên khai báo endpoint.

**Quy tắc quyền xem đơn đã được nâng lên DOMAIN** (`Order.ViewableBy`). Nó
được hỏi từ BA nơi — chi tiết đơn, hủy đơn, lô giao — và mỗi nơi tự cài lại
nghĩa là sớm muộn một nơi cài lỏng hơn. Module `fulfillment` HỎI qua cổng
`OrderAccessPort` thay vì cài lại, nên nó vẫn không phụ thuộc module nào.

#### Lỗ hổng bảo mật CÓ SẴN TỪ TRƯỚC, phát hiện khi viết test

Quy tắc cũ kiểm tra theo thứ tự: có `customerID` thì so định danh, không thì
so số điện thoại. Hệ quả: **đơn đã gắn tài khoản vẫn mở được bằng số điện
thoại** — ai biết số của một khách đã đăng ký đều đọc được đơn của họ, kể cả
địa chỉ nhà. Mà số điện thoại thì khách cho đi khắp nơi (giao hàng, hóa đơn,
nhóm chat), khác hẳn mật khẩu.

Quy tắc đúng: **đơn đã có chủ thì CHỈ chủ mở được**; số điện thoại chỉ là
chìa khóa cho đơn vãng lai. Lỗi này tồn tại từ khi viết P1.4 và chỉ lộ ra
khi nâng quy tắc lên domain rồi viết bảng test cho từng tổ hợp.

#### Trả MÃ, không trả object

`seller_id` chứ không phải object nhà bán: tên và đánh giá thuộc module
`seller`, mà `fulfillment` không phụ thuộc module nào.

`order_line_ids` chứ không lặp lại tên và giá: trang chi tiết đơn ĐÃ CÓ dòng
hàng từ `getOrder`, nó chỉ cần biết dòng nào đi trong gói nào. Lặp lại dữ
liệu của module order ở đây nghĩa là hai bản, và chúng lệch nhau khi đơn bị
hủy một phần.

Endpoint nhận CẢ mã đơn lẫn mã hiển thị — khách vãng lai chỉ có mã hiển thị
trong email xác nhận. Cổng phân giải và kiểm quyền trong MỘT bước rồi trả về
mã chuẩn, thay vì bắt bên gọi phân giải lần nữa.

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
| P2-1 | Demand signal từ event (`add_to_cart`, `order_placed`, `search_no_result`) | ✅ xong — xem ghi chú dưới bảng |
| P2-2 | Analytics đọc event, **không phải nguồn sự thật về tiền** | P2-1 |
| P2-3 | Monorepo Next.js + `packages/types` sinh từ OpenAPI | ✅ xong |
| P2-4 | Admin UI — 3 nhóm trang (sellers · audit log · orders) | ✅ xong |
| P2-7 | **CORS** — giao diện ở origin khác gọi được API | ✅ xong |

**P2-1 — ✅ xong, kiểm chứng end-to-end trên hệ thống thật (19/08).**

```text
search_no_result  ✅ demand_signal ghi đúng từ khóa đã tìm
add_to_cart       ✅ ADD_TO_CART, đúng sku, quantity=2
order_placed      ✅ ORDER, đúng sku, quantity=2
```

Kiểm chứng bằng cách đi HẾT luồng mua hàng thật, không phải đọc code:

```text
thêm giỏ → mở phiên (giữ hàng) → địa chỉ → vận chuyển → đặt hàng
    ↓
đơn FC-2026-08-000001 · 1.010.000đ
tồn kho   100 → 98 khả dụng, 2 đã chốt
lô giao   FC-2026-08-000001-A · PENDING
outbox    cart.item_added ✅ · checkout.completed ✅ (đã phát hết)
```

Trước đó việc này bị chặn hai lớp: P3-2 (test xóa sạch database phát triển)
là NGUYÊN NHÂN, P3-13 (dữ liệu mẫu thiếu offer và tồn kho) là HẬU QUẢ. Sửa
cả hai mới chạy thử được.
| P2-5 | Storefront | ✅ luồng mua hàng xong — xem ghi chú dưới bảng |
| P2-6 | Seller Center | ✅ hai màn hình xong — xem ghi chú dưới bảng |

**P2-6 — Trung tâm người bán đã dựng xong (19/08).** `apps/seller` ở cổng
3002, hai màn hình đúng bằng hai câu hỏi nhà bán hỏi mỗi ngày:

```text
/         Việc cần làm    — đơn cần soạn, mỗi đơn một phiếu soạn hàng
/offers   Hàng đang bán   — đăng bán, đổi giá, kiểm kê, ngừng bán
```

**Phiếu soạn hàng gộp ba thứ vào một chỗ:** nhặt gì (kèm mô tả biến thể),
gửi đi đâu, nhận về bao nhiêu. Nhà bán đứng cạnh kệ hàng chứ không ngồi
trước hai màn hình. Thiếu địa chỉ giao thì nút bàn giao bị KHÓA kèm lời
nhắc đừng đoán — giao nhầm địa chỉ đoán ra tốn hơn nhiều so với chờ hỏi.

**Giá và tồn kho tách thành hai ô nhập.** Chúng ở hai module khác nhau dưới
backend, và lẫn lộn là nguồn sai sót phổ biến: đổi giá không làm hàng nhiều
lên, kiểm kê không làm hàng rẻ đi.

**Bốn chỗ đặc tả lệch với API thật**, cả bốn do TypeScript sinh từ đặc tả
bắt được — không chịu biên dịch với handler thật:

1. Ba endpoint offer khai trả `Offer` của trang công khai, thiếu `status`
   mà nhà bán bắt buộc phải thấy. Thêm schema `SellerOffer`.
2. `updateInventory` khai PATCH, handler đăng ký PUT — client sinh đúng
   theo đặc tả sẽ ăn 405. Handler đúng: thân yêu cầu mang toàn bộ trạng
   thái mong muốn.
3. `updated_at` được hứa nhưng không bao giờ trả. Không bắt được bằng kiểu
   vì trường không `required`; chỉ lộ ra khi gọi thật.
4. `http://localhost:3002` thiếu trong danh sách trắng CORS — lỗi thứ ba
   cùng loại, lần thứ ba chỉ hiện ở console trình duyệt.

**Một đính chính về ngữ nghĩa, kiểm chứng bằng đơn thật:** `updated_at` là
lúc con số THAY ĐỔI lần cuối, không phải lúc ĐẾM lần cuối — kiểm kê ra đúng
số đang có là việc không-làm-gì, cố ý, để một lần đếm xác nhận không đẻ ra
bản ghi biến động giả. 37→36 đổi mốc; đếm lại 36 giữ nguyên mốc; 36→37 đổi
mốc. Muốn có "đếm lần cuối" thì phải ghi riêng thời điểm kiểm kê kể cả khi
không lệch — khái niệm khác, chưa có.

Và một lỗi backend nghiêm trọng phát hiện ngay khi thử màn kiểm kê: **P3-18**
(giữ hàng chọn nhầm chủ sở hữu tồn kho).

**P2-5 — luồng mua hàng đã dựng xong (19/08).** `apps/storefront` ở cổng
3001, sáu trang:

```text
/                danh sách sản phẩm
/products/{id}   chọn NHÀ BÁN (offer) → thêm giỏ
/cart            sửa số lượng, xóa — nhóm theo nhà bán
/checkout        mở phiên (giữ hàng 15 phút) → địa chỉ → vận chuyển → đặt
/orders          tra cứu bằng mã đơn + số điện thoại
/orders/{key}    chi tiết đơn
```

**Khách VÃNG LAI là mặc định.** Không trang nào ở đường mua hàng cần đăng
nhập — danh tính đến từ cookie `shopper_session`.

**Hai lỗi CORS bắt được khi chạy thật**, cả hai đều thuộc loại chỉ hiện ở
console trình duyệt còn log máy chủ hoàn toàn sạch:

1. `http://localhost:3001` không nằm trong danh sách trắng mặc định của môi
   trường phát triển — mọi lời gọi của cửa hàng bị chặn.
2. `X-Guest-Phone` thiếu trong `Access-Control-Allow-Headers` — riêng trang
   tra cứu đơn không chạy.

Cả hai đã sửa kèm test.

**Chưa dựng, có lý do:**

- **Trang tài khoản** (hồ sơ, sổ địa chỉ, yêu thích): sáu endpoint đã xong ở
  P1.2, nhưng đặc tả CHƯA có endpoint ĐĂNG KÝ cho khách. `login` hiện phục
  vụ tài khoản nội bộ, nên khách không có đường nào để có tài khoản. Cần
  quyết định sản phẩm trước khi dựng giao diện.
- **Tiến độ giao hàng từng gói**: cần P1.8.
- Tìm kiếm, lọc danh mục/thương hiệu, đánh giá sản phẩm.

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
| P3-2 | **Sửa test suite chập chờn** | ✅ xong — xem ghi chú dưới bảng |
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
| P3-13 | **Dữ liệu mẫu MUA ĐƯỢC**: seed cho offer + tồn kho | ✅ xong — xem ghi chú dưới bảng |
| P3-14 | **Tùy chọn khách hàng** (số đo cơ thể, size ưa thích) | Cần thiết kế lưu trữ MÃ HÓA trước; đặc tả tự yêu cầu điều đó |
| P3-15 | **Xác minh email** → mở đường gộp lịch sử đơn vãng lai | Chặn P1.9: khách từng đặt hàng vãng lai chưa đăng ký được bằng email đó |
| P3-16 | Bộ đếm tần suất DÙNG CHUNG giữa các tiến trình | Bộ đếm hiện nằm trong bộ nhớ; N bản sao = N lần hạn mức |
| P3-17 | SLA cho đơn thực hiện | Đặc tả khai báo `sla_deadline`; domain chưa có khái niệm này |
| P3-18 | **Giữ hàng chọn nhầm CHỦ SỞ HỮU tồn kho** | Nghiêm trọng — kiểm chứng bằng đơn thật (19/08); xem ghi chú dưới bảng |

**P3-18 — giữ hàng chọn nhầm chủ sở hữu tồn kho (phát hiện 19/08, CHƯA sửa).**

Mua qua offer của MỘT nhà bán có thể trừ hàng của NGƯỜI KHÁC.

Kiểm chứng bằng đơn thật, không phải đọc code. SKU
`sku_01M0C73VPF2F4S1J2BAWH8TWK7` có hai bản ghi tồn kho, cả hai đều được tạo
qua đường hợp lệ: nền tảng nhận 100 cái khi nạp dữ liệu mẫu, nhà bán khai 40
cái qua `initial_inventory` lúc tạo offer. Đặt mua 1 cái qua offer ĐANG BÁN
của nhà bán:

    trước:  own_platform 100 khả dụng · nhà bán 40 khả dụng
    sau:    own_platform  99 khả dụng, 1 GIỮ CHỖ · nhà bán 40 khả dụng

Hàng của nền tảng bị giữ cho một đơn của nhà bán. Số 40 của nhà bán không
động đậy.

Nguyên nhân nằm ở `pickStockItem` (checkout/application/service.go): nó lấy
bản ghi ĐẦU TIÊN còn đủ hàng. Cổng `InventoryPort.FindItemsForSKUs` thậm chí
không mang chủ sở hữu về, nên tầng chọn kho KHÔNG CÓ dữ liệu để chọn đúng —
đây là thiếu sót ở hợp đồng giữa hai module, không phải một dòng `if` bị
quên.

Vì sao nghiêm trọng ở đúng chỗ này: chợ tồn tại để NHIỀU nhà bán chào cùng
một SKU. Hai nhà bán cùng bán một mã hàng là trường hợp THƯỜNG, không phải
biên. Khi đó bán được cho B lại trừ kho của A: A hụt hàng vì những đơn họ
chưa bao giờ nhận, còn B giao hàng từ số tồn mà hệ thống tưởng vẫn còn
nguyên. Không bên nào thấy lỗi — hai sổ sách cùng sai và cùng im lặng.

Chưa sửa vì cách sửa là một QUYẾT ĐỊNH THIẾT KẾ, không phải một bản vá:

  - Giữ hàng phải theo chủ sở hữu của offer đang mua. Việc này cần
    `FindItemsForSKUs` trả cả `owner_id`, và checkout truyền chủ sở hữu
    mong muốn xuống.
  - Hàng KÝ GỬI làm mọi thứ phức tạp hơn: hàng của nhà bán nằm trong kho
    nền tảng là hợp lệ và sẽ xảy ra. Mô hình đã tách `stock_location_id`
    với `inventory_owner_id` cho đúng việc này, nên lời giải là lọc theo
    CHỦ SỞ HỮU chứ không phải theo KHO.
  - Còn phải quyết: offer của chính nền tảng thì lấy hàng `own_platform`,
    nhưng nhà bán hết hàng có được mượn kho nền tảng không? Trả lời "có"
    là mở đường cho đúng lỗi đang có, chỉ là có chủ đích.

Trung tâm người bán (P2-6) làm lỗi này lộ ra ngay: nhà bán nhìn số tồn của
mình và thấy nó đứng yên dù đơn vẫn về.

**Lỗi phụ, cùng lượt kiểm chứng:** thiếu `guest_email` khi mở phiên thanh
toán trả về `500 INTERNAL_ERROR` kèm câu "Đã có lỗi xảy ra, vui lòng thử
lại". Đó là lỗi NHẬP LIỆU của người gọi, phải là `422` với thông điệp nói rõ
thiếu gì. Trả 500 vừa giấu nguyên nhân vừa bảo người dùng thử lại một việc
không bao giờ thành công.

**Lỗi MẤT CẬP NHẬT ở khuyến mãi — đã sửa (19/08).**

Phát hiện nhờ một test đồng thời chập chờn: `TestKhoaLacQuanChanMatBoDem` đỏ
khi toàn bộ bộ test chạy song song, xanh khi chạy riêng. Không bỏ qua mà
điều tra — và nó là lỗi thật, tái hiện được dưới tải.

```text
12 lượt áp mã song song
→ bảng coupon_usage: 12 hàng
→ bộ đếm used_count:  11        ← MẤT MỘT
```

**Nguyên nhân:** hàng được ghi ở một giao dịch, bộ đếm cập nhật ở giao dịch
khác bằng ĐỌC-RỒI-GHI với khóa lạc quan, thử lại tối đa 10 lần. Dưới tải,
mười lần trượt liên tiếp là chuyện XẢY RA — và khi đó lượt đã nằm trong
bảng nhưng bộ đếm không tăng. Mã giới hạn 100 lượt sẽ được dùng vài trăm
lần, vì bộ đếm không bao giờ chạm giới hạn.

**Cách sửa:** khóa lạc quan tồn tại để chặn "ghi đè thay đổi của người
khác". Với một phép CỘNG thì không có gì bị ghi đè — hai lượt +1 đồng thời
phải thành +2, và cả hai đều đúng. Thay bằng một câu `UPDATE` cộng dồn tại
chỗ, kèm tính trạng thái CẠN ngay trong cùng câu lệnh.

Trước sửa: đỏ dưới tải. Sau sửa: 32 lượt chạy dưới tải đều xanh.

**Tra sản phẩm theo lô (`GET /api/v1/products?ids=…`) — thêm 19/08.**

Nhiều trang có danh sách MÃ sản phẩm mà không có tên hay ảnh; rõ nhất là
danh sách yêu thích — `customer` nằm cùng tầng với `product` nên chỉ trả
`product_id`. Không có đường này thì trang phải gọi `getProduct` cho từng
mã: danh sách 30 món thành 30 lượt đi-về, đúng vấn đề N+1 mà
`product.GetProductsByIDs` sinh ra để tránh (hàm đó đã có sẵn, chỉ thiếu
tầng HTTP).

Hai bất biến dễ sót, cả hai đều có test:

1. **Giữ đúng thứ tự mã được hỏi.** Bên gọi đã sắp xếp danh sách của họ;
   trả thứ tự khác buộc họ sắp lại, hoặc tệ hơn là họ không nhận ra.
2. **Hàng chưa duyệt không lọt ra**, kể cả khi hỏi đích danh mã. Bộ lọc
   `OnlyVisible` nằm ở truy vấn danh sách, còn đường này đi thẳng vào
   `FindByIDs` và bỏ qua nó.

Test thứ tự dùng SÁU sản phẩm chứ không phải hai: duyệt map trong Go trả
thứ tự ngẫu nhiên, nên với hai phần tử một cài đặt sai vẫn đúng 50% số lần
— test chập chờn thay vì bắt lỗi.

**P3-2 — đã xong (15/08).** Có HAI sự cố, không phải một:

```text
1. Test xóa sạch database phát triển
   `make test-db` cũ chạy `go test ./...` thẳng lên DATABASE_URL.
   Một lần chạy là mất hết dữ liệu mẫu — và im lặng, chỉ lộ ra khi
   mở giao diện thấy trống. Đây là thứ đã chặn việc kiểm chứng P2-1.

2. Test đỏ khi chạy song song
   `order`, `fulfillment`, `checkout` cùng TRUNCATE bảng "order".
   Tái hiện: chạy ba gói cùng lúc → hỏng 3/3 lần, không phải chập chờn.
```

Cách chữa cũ là cờ `-p 1` trong Makefile. Nó chỉ chữa sự cố thứ hai, chỉ khi
người chạy dùng `make test` — `go test ./...` hay `make test-race` vẫn hỏng.

Nay [internal/platform/testdb](../../internal/platform/testdb/testdb.go) cấp
cho **mỗi gói test một database riêng**, sao từ khuôn bằng
`CREATE DATABASE ... TEMPLATE` (nhanh hơn nhiều so với chạy lại 22 migration
cho mỗi gói). Kèm một **hàng rào**: test dừng hẳn nếu `TEST_DATABASE_URL` và
`DATABASE_URL` trỏ cùng một database.

Hệ quả: `-p 1` đã bỏ khỏi Makefile, bộ test chạy song song trở lại.

**P3-13 — đã xong (19/08).** Dữ liệu mẫu trước đây dừng ở SKU và giá, nên
catalog đầy sản phẩm mà giỏ hàng vĩnh viễn rỗng: khách mua OFFER chứ không
mua SKU, và `startCheckout` luôn thất bại "không đủ hàng" nếu không có tồn.

Đã thêm `marketplace.SeedDemo` và `inventory.SeedDemo`. Hai việc phát sinh
khi làm, cả hai đều là khoảng trống có thật chứ không phải chuyện của seed:

1. **Không có đường nào tạo kho hàng.** Test phải chèn `stock_location` bằng
   SQL tay. Đã thêm `domain.StockLocation` + `LocationRepository` +
   `EnsureLocation` (idempotent theo MÃ kho, không theo id).

2. **Thương hiệu own-brand không ai bán được — kể cả nền tảng.** Thương hiệu
   ở mức `RESTRICTED` chỉ cho phép `OwnerSellerID` tạo offer, mà seed để
   trống trường đó. Hàng rào chống hàng giả chặn đúng; thiếu sót nằm ở chỗ
   chưa CHỈ ĐỊNH nhà bán. `catalog.SeedDemo` nay nhận `OwnBrandSellerID`.

P3-1 là **nợ kỹ thuật đã biết**, không phải việc phát sinh — ghi ở đây để
không bị quên.

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
