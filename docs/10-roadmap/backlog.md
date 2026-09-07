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

Đếm từ code ngày **20/08/2026**, không phải ước lượng:

```text
Module MVP có logic nghiệp vụ    17/17
Module có tầng HTTP              12/17   (thiếu: analytics · notification ·
                                         pricing · promotion · supplychain —
                                         cả năm phục vụ module khác qua Go,
                                         không cần đường HTTP riêng ở MVP)
Thao tác trong OpenAPI           75
Thao tác đã có route             51      (68%)
Thao tác chưa cài                24      (20 thuộc Phase 2/3 — xem mục 6)

Migration                        25
Test Go                          740
Test trình duyệt (Playwright)     5
Test đơn vị TypeScript           10
```

**Tầng HTTP KHÔNG còn là chỗ nghẽn.** Đó là tình hình của tháng 8 đầu; giờ
mọi module thương mại đều có đường ra ngoài, ba giao diện đều gọi được, và
bảy luồng nghiệm thu MVP đều chạy.

### Đổi phase: từ "dựng commerce core" sang PRODUCTION HARDENING

Commerce core đã dựng xong và có test. Việc còn lại **không phải thêm tính
năng** mà là chứng minh những gì đã có chịu được điều kiện thật: đồng thời,
lỗi, thử lại, và kẻ tấn công.

Ưu tiên trong phase này, theo đúng thứ tự:

```text
Correctness > Consistency > Security > Reliability > Performance > Feature velocity
```

Danh sách việc ở **mục 2 (P0 — Production Hardening)**. Đánh số P1/P2/P3
của các mục sau là LỊCH SỬ, giữ nguyên vì mã P3-18, P3-19… được chú thích
trong code tham chiếu tới; đừng đánh số lại.

**Không mở rộng miền trong phase này.** Danh sách cấm ở mục 6.

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

## 2. P0 — Production Hardening `[PHASE HIỆN TẠI]`

Việc mà không có nó thì không được đưa lên production.

**Điều kiện kết thúc phase** — chỉ chuyển phase khi chuỗi dưới đây chạy E2E
ổn định trên PostgreSQL thật, với **cả sáu kịch bản**:

```text
Product → SKU → Offer → Seller/Own Brand → Inventory → Cart → Checkout
        → Order → Payment → Fulfillment → Shipment → Event/Outbox
        → Demand Signal

kịch bản:  own brand · 1 seller · nhiều seller · đơn trộn
           · thực hiện từng phần · lỗi + thử lại · thanh toán đồng thời
```

và mọi bất biến về **ownership, authorization, inventory, transaction,
idempotency** đều có test hồi quy tự động.

---

### PH-34 — Hủy đơn ghi đè tiến độ giao hàng `[XONG 27/08]`

Tìm ra khi quét CÓ HỆ THỐNG hình dạng `FindByID` + `Update` trên đường
ghi — cùng hình dạng đã gây PH-31 và PH-32.

Bảng `order` không có cột `version`, mà HAI tiến trình cùng sửa nó:

```text
khách bấm Hủy       → API     → CancelOrder
nhà bán bàn giao    → worker  → ApplyFulfillmentProgress
```

Đọc-sửa-ghi ở hai giao dịch rời. Ai ghi sau thắng.

Tái hiện xác định (`internal/app/api_donhang_dua_test.go`), dùng cổng tiêm
đồng hồ của module order: dừng lượt hủy đúng giữa lúc đọc và lúc ghi, cho
lượt bàn giao chạy trọn, rồi thả. Đơn **đã SHIPPED bị ghi thành
CANCELLED**, và lệnh hủy báo THÀNH CÔNG. Hàng đã rời kho mà hệ thống tin
là đã hủy — tiền hoàn cho một lô hàng khách vẫn nhận được.

Sửa: `order.version` + ràng buộc phiên bản ở `Update` (migration 000030),
`ErrVersionConflict` → HTTP 409. Khóa LẠC QUAN vì xung đột hiếm, đúng quy
tắc 2 của [ADR-0013](../adr/0013-write-transaction-boundary.md).

**Kéo theo — một bẫy đã sập HAI lần:** `withLines` dựng lại Order từ đầu
sau khi nạp dòng hàng, và danh sách trường phải chép tay. Quên `Version`
làm mọi lần chuyển trạng thái TIẾP THEO thất bại; quên `SourceCheckoutID`
làm bất biến "một phiên một đơn" mất chỗ dựa. Trình biên dịch không nhắc,
vì thiếu trường chỉ là giá trị rỗng hợp lệ.

Đã thêm `internal/modules/order/vong_doc_ghi_test.go` so từng trường bằng
phản chiếu, nên danh sách tự dài ra khi domain thêm trường. Ghi rõ giới
hạn của nó ngay trong bài: phép so bằng phản chiếu KHÔNG bắt được trường
có giá trị rỗng — `Version` của đơn mới bằng 0 — nên có thêm một khẳng
định riêng sau một lần cập nhật.

---

### PH-39 — Lọc danh mục theo giá và còn hàng `[CHẶN: cần read model]`

Đặc tả `listProducts` khai sáu bộ lọc. Bốn cái đã cài (27/08): `size`,
`color` (nhóm màu), `gender_target`, `product_type` — tất cả đều là dữ
liệu của module `product`, nên lọc được bằng một truy vấn.

Hai cái còn lại KHÔNG cài được như vậy:

```text
price_min / price_max   giá nằm ở module marketplace (offer)
in_stock                tồn kho nằm ở module inventory
```

Lọc theo chúng cần JOIN xuyên module — thứ kiến trúc cấm, và cấm có lý do:
module nào sở hữu bảng nào là ranh giới giữ cho chúng thay đổi độc lập.

**Vì sao không lách bằng cách lọc ở tầng ứng dụng**

Trang lấy 50 sản phẩm rồi tự loại những cái hết hàng sẽ trả về ít hơn 50,
và trang 2 bỏ sót. Phân trang chỉ đúng khi việc lọc nằm TRONG truy vấn.

**Hướng đề xuất: read model cho danh mục.**

Một bảng chiếu `product_search` do event cập nhật, mang sẵn giá thấp nhất
và cờ còn hàng:

```text
offer.price_changed        → cập nhật min_price
inventory.stock_changed    → cập nhật in_stock
product.published/archived → thêm/xóa dòng
```

Đây là thay đổi kiến trúc thật (thêm một nguồn dữ liệu phải giữ đồng bộ),
nên dừng ở mức đề xuất theo đúng quy tắc. Cần quyết trước: chấp nhận độ
trễ bao lâu giữa lúc giá đổi và lúc bộ lọc thấy, và ai đối chiếu khi bảng
chiếu lệch.

---

### PH-38 — Số dư nhà bán: Đang chờ → Rút được `[XONG 27/08]`

Nhà bán kiếm được tiền nhưng không có cách nào NHÌN THẤY, và không có ranh
giới nào giữa "tiền đã ghi nhận" với "tiền rút được".

Chi trả từ khoản còn trong hạn đổi trả là cách mất tiền chắc chắn nhất:
khách trả hàng sau khi nhà bán đã rút thì đòi bằng gì.

**Hai tài khoản thay vì một cột trạng thái**

```text
SELLER_PAYABLE    đơn đã giao, CÒN trong hạn đổi trả
SELLER_AVAILABLE  hết hạn đổi trả, sẵn sàng chi trả
```

Số dư là KẾT QUẢ TÍNH từ sổ cái ([ADR-0008](../adr/0008-financial-ledger.md)
quyết định 3), nên "chuyển trạng thái tiền" phải là một BÚT TOÁN — tổng nợ
phải trả không đổi, tiền chỉ đổi chỗ, và mọi lần đổi chỗ đều để lại dấu vết.

Sự kiện `fulfillment_order.completed` (mới) mang sẵn số tiền phải trả, nên
payment không phải gọi ngược fulfillment.

`GET /api/v1/seller/balance` trả năm trường theo đặc tả, nhưng chỉ
`pending` và `available` có thật. Ba trường còn lại — `processing`,
`on_hold`, `reserve_held` — trả 0, và 0 là con số ĐÚNG: chưa có luồng chi
trả, chưa có cơ chế giữ tiền khi tranh chấp, chưa có chính sách reserve.

**Còn lại của đường tiền:**

| Việc | Ghi chú |
|---|---|
| ~~Bảng `settlement` + job tổng hợp theo kỳ~~ | **XONG 27/08** — job chạy mỗi giờ, mỗi bút toán chỉ vào được một đợt |
| ~~`GET /api/v1/seller/settlements/{id}`~~ | **XONG 27/08** — kèm danh sách đợt |
| ~~Số dư âm~~ | **XONG 27/08** — phần âm của tài khoản đang chờ bị trừ khỏi số thực chi; đợt không có số dương thì không xác nhận được |
| **`POST /api/v1/admin/payouts`** | Thao tác NGUY HIỂM NHẤT: chuyển tiền thật ra ngoài. Cần 2FA và tích hợp ngân hàng — CHƯA có cái nào |
| Chặn chuyển sang rút được khi đang có yêu cầu trả hàng | Hiện xử lý bằng cách trừ phần âm ở đối soát, đúng nhưng gián tiếp. Chặn từ đầu thì sạch hơn — cần fulfillment hỏi returns |

---

### PH-37 — Trả hàng `[XONG phần lõi 27/08]`

Module `returns` (thư mục `returns` vì `return` là từ khóa Go). Xin trả →
nhà bán duyệt/từ chối → nhận hàng → hoàn tiền, tất cả qua HTTP.

**Phát hiện quan trọng khi làm:** nguy cơ HOÀN THỪA đã sẵn sàng nổ.

```text
POST /checkout/{id}/coupon   route có, module promotion CHƯA nối
                             → trả "chưa sẵn sàng"        (ĐÃ NỐI 27/08)
promotion.AllocateDiscount   tồn tại, KHÔNG ai gọi        (ĐÃ SỬA 27/08)
```

**Trạng thái 27/08:** cả hai đã xong, và xong ĐÚNG THỨ TỰ — phân bổ trước,
nối promotion sau. Nối trước thì mọi đơn có mã giảm giá tạo ra trong
khoảng giữa sẽ không trả hàng tự động được.

**Đính chính:** mô tả đầu tiên của mục này nói nguy cơ "đã sẵn sàng nổ".
Không đúng — module `promotion` chưa được nối vào ứng dụng, nên hôm nay
không tạo được đơn có giảm giá qua HTTP. Nguy cơ là TIỀM ẨN, và nó sẽ
thành thật đúng vào lúc ai đó nối promotion vào. Việc phân bổ đã làm xong
trước thời điểm đó.

Comment của chính `AllocateDiscount` cảnh báo: *"Không lưu lại thì nền
tảng hoàn nhiều hơn đã thu."* Nên một đơn có mã giảm giá sẽ có
`discount_amount > 0` mà mọi dòng đều không mang khoản điều chỉnh — hoàn
theo giá dòng là hoàn thừa đúng bằng phần giảm.

`TinhTienHoan` vì thế TỪ CHỐI khi thấy giảm giá cấp đơn chưa phân bổ, và
trả 409 kèm thông điệp nói thẳng để người vận hành xử lý tay.

**Việc còn lại của luồng trả hàng:**

| Việc | Ghi chú |
|---|---|
| ~~Nối `AllocateDiscount` vào checkout~~ | **XONG 27/08** — phân bổ ở `CompleteCheckout`, đóng băng thành khoản điều chỉnh của dòng hàng |
| ~~Nối module `promotion` vào `internal/app`~~ | **XONG 27/08** — mã giảm giá chạy được từ đầu tới cuối, kể cả trả hàng |
| ~~Kiểm định chất lượng → Available~~ | **XONG 27/08** — `POST /seller/returns/{id}/inspect`, đạt vào Available, loại vào Damaged |
| **Đối chiếu dòng "đã kiểm" với tồn kho** | Kiểm định lưu trạng thái TRƯỚC rồi mới ghi tồn kho. Lượt ghi tồn kho hỏng để lại dòng đã kiểm mà hàng còn kẹt ở Returned — không có gì tự phát hiện. Cần một truy vấn đối chiếu định kỳ |
| Giải phóng lượt dùng mã giảm giá | `promotion.ReleaseUsage` đã có, chưa nối |
| Thu hồi điểm thưởng | Chưa có module loyalty |
| Đảo hoa hồng creator | Chưa có module creator |
| Phí vận chuyển chiều về | `LyDo.LoiCuaNguoiBan()` đã phân loại được ai chịu; chưa dùng |

---

### PH-36 — Webhook thanh toán `[XONG 06/09]`

Webhook **vận chuyển** đã cài xong (27/08). Webhook **thanh toán** thì
không, và lý do đáng ghi lại.

`api/paths/webhooks.yaml` quy định ba lớp bảo vệ cho webhook thanh toán:

```text
1. Xác minh chữ ký HMAC          → cài được, đã có httpserver.KiemChuKyHMAC
2. Idempotency theo event_id     → cài được, đã có bảng webhook_event
3. ĐỐI CHIẾU SỐ TIỀN với         → KHÔNG cài được
   payment_intent trong hệ thống
```

Không có bảng `payment_intent`, và không có `PaymentIntent` nào trong mã
nguồn — đã kiểm cả schema lẫn code. Module `payment` là một SỔ CÁI, không
phải phần tích hợp cổng thanh toán.

**Vì sao không cài hai lớp trước rồi bổ sung lớp ba sau:** như thế là dựng
đúng nửa nguy hiểm. Một endpoint nhận "payment.succeeded" mà không đối
chiếu số tiền sẽ ghi nhận doanh thu theo con số bên ngoài gửi vào. Chữ ký
đúng chỉ chứng minh thông điệp đến từ nhà cung cấp, không chứng minh nội
dung khớp với thứ khách đã trả — lỗi tích hợp phía họ, hoặc một khóa bị
lộ, đều thành tiền ghi sai.

**Cần trước:** mô hình `payment_intent` — tạo khi khách chọn phương thức
trả trước, giữ số tiền đã đóng băng, và là thứ webhook đối chiếu vào. Đó
là việc lớn hơn bản thân webhook.

---

**ĐÃ LÀM 06/09 — quyết định ở [ADR-0017](../adr/0017-payment-intent.md).**

`payment_intent` giữ số tiền hệ thống CHỜ THU, tạo đồng bộ ngay sau
`PlaceOrder` cho đơn TRẢ TRƯỚC. Đồng bộ chứ không qua event: webhook có
thể tới trước khi worker vét outbox, và khi ấy nó không tìm thấy gì để đối
chiếu — tiền về mà hệ thống trả 404.

**COD KHÔNG có intent**, có chủ ý. Không có cuộc trao đổi nào với cổng
thanh toán nên không có gì để đối chiếu, và hàng nghìn intent không bao
giờ được thu sẽ làm mọi cảnh báo dựa trên tồn đọng thành vô dụng. Quy tắc
này chỉ viết được nhờ P3-9 (06/09) — trước đó hệ thống không biết đơn nào
là COD.

**`Order.MarkPaid` hết là mã chết.** Nó tồn tại đủ ba tầng từ lâu mà không
ai gọi; webhook chính là bên gọi nó được viết cho.

**Kiểm chứng bằng cách phá, hai lần, hai tầng:**

- Tầng application nuốt lỗi lệch số tiền → bài API đỏ đúng chỗ nguy hiểm
  nhất: `HTTP 200` và đơn thành `PAID` từ một webhook báo SAI số tiền.
- Bỏ hẳn việc tạo intent (tức "cài hai lớp trước, bổ sung lớp ba sau" —
  đúng thứ mục này cảnh báo) → mọi webhook trả 404. Đáng chú ý: hỏng theo
  hướng ĐÓNG, không bao giờ tin con số bên ngoài.

**Kiểm trên hệ thống THẬT** (API + PostgreSQL có dữ liệu), đơn 420.000đ
trả bằng CARD:

```text
chữ ký SAI              → 401 · đơn PENDING_PAYMENT
chữ ký ĐÚNG, tiền SAI   → 422 · đơn PENDING_PAYMENT  ← lớp 3 chặn được
chữ ký ĐÚNG, tiền ĐÚNG  → 200 · đơn PAID
gửi TRÙNG               → 200 · already_processed: true
```

Log máy chủ ghi ERROR kèm số tiền BÊN NGOÀI BÁO, và cố ý KHÔNG nêu số
tiền hệ thống chờ thu ở phản hồi — nói ra là đưa cho kẻ dò đúng con số cần
gửi lần sau. Ràng buộc database cũng đã kiểm trên bản thật: `CAPTURED` mà
bỏ mốc thu tiền bị từ chối, và hai intent cho một đơn bị từ chối.

**Đối soát định kỳ (yêu cầu 5) — nửa làm được, làm 06/09.**

Bản đầy đủ là đi HỎI nhà cung cấp danh sách giao dịch rồi so với sổ của
mình; việc đó cần adapter PSP thật, thứ chưa có.

Cái làm được ngay là đối soát hai nguồn NỘI BỘ, và nó nhắm đúng một lỗ
hổng ĐÃ BIẾT chứ không phải giả định: handler webhook, khi thu tiền xong
mà `MarkOrderPaid` hỏng, cố ý KHÔNG quay ngược intent — tiền về là sự thật
đã xảy ra — nên nó ghi log rồi đi tiếp và để lại đúng trạng thái đó. Trước
job này, thứ duy nhất bắt được là người đọc log.

Job `đối soát tiền đã thu với trạng thái đơn` chạy 5 phút một lượt, soi
cửa sổ 1 giờ. Cửa sổ có giới hạn CÓ CHỦ Ý: quét lại toàn bộ lịch sử mỗi
lượt sẽ nặng dần theo tuổi hệ thống cho tới lúc tự thành sự cố.

**Hai chỉ số, hai mức độ khác nhau — và sự phân biệt đó là phần thiết kế
đáng kể nhất ở đây:**

```text
gouse_payment_reconcile_mismatch      ERROR   không có ca hợp lệ nào
gouse_payment_intent_pending_stale    theo dõi, KHÔNG cảnh báo
```

"Đã thu tiền mà đơn vẫn chờ thanh toán" không có ca hợp lệ nào, nên nó
không bao giờ kêu oan. "Intent chờ thu quá hạn" thì ngược lại: phần lớn là
khách bỏ giữa chừng, bình thường và nhiều. Cảnh báo ở đó sẽ luôn kêu, và
một cảnh báo luôn kêu thì không ai đọc — khi đó nó còn tệ hơn không có.
Giá trị của nó là XU HƯỚNG: tăng vọt nghĩa là webhook thôi tới.

**CHỈ cảnh báo, KHÔNG tự sửa.** Tự gọi `MarkOrderPaid` ở đây nghe hấp dẫn
và sai: job nền tự sửa dữ liệu tiền bạc là thứ chạy lúc 3 giờ sáng không
ai nhìn, và nếu giả định của nó sai thì nó sửa hàng loạt theo hướng sai,
im lặng.

**Kiểm trên hệ thống THẬT:** dựng đúng ca bất nhất trên database phát
triển (đưa một đơn đã PAID về PENDING_PAYMENT, giữ intent CAPTURED) →
worker thật ghi ERROR kèm mã intent, mã đơn, số tiền, thời điểm thu và
hành động cần làm; `gouse_payment_reconcile_mismatch` lên 1. Sửa dữ liệu
lại → lượt chạy sau về 0, không còn dòng ERROR nào.

**Vẫn còn nợ:** đối chiếu với PSP (cần adapter), và webhook VẬN CHUYỂN
chưa có gì tương đương.

**Sửa kèm ba chỗ lệch trong đặc tả webhook:** ví dụ dùng
`metadata.checkout_id` trong khi máy chủ đọc `order_id`; ví dụ lỗi 422
khai mã `VALIDATION_FAILED`, mã ánh xạ sang 400 chứ không phải 422; và
thiếu hẳn phản hồi 404 cho đơn không chờ thu tiền.

**CÒN NỢ, ghi rõ chứ không giấu:**

- **Doanh thu vẫn ghi sổ lúc `checkout.completed`, TRƯỚC khi tiền về.** Một
  đơn trả trước bị bỏ dở vẫn để lại bút toán doanh thu cho khoản tiền chưa
  bao giờ tới. Sửa đúng phải chuyển ghi nhận sang lúc thu tiền, mà điều đó
  động tới cả COD (tiền về lúc giao hàng) — việc lớn hơn hẳn và là quyết
  định của chủ dự án.
- **Job đối soát: NỬA nội bộ đã làm 06/09, nửa hỏi PSP thì chưa.** Xem
  ghi chú "Đối soát định kỳ" ngay dưới.
- **Chưa có adapter PSP thật**, nên `provider_intent_id` để trống và
  webhook tra intent bằng mã đơn. Cột đã có sẵn nên ngày nối PSP là thay
  đổi cộng thêm.

---

### PH-33 — Đường tiền chưa nối `[XONG 27/08]`

**Không một bút toán nào tồn tại.** 76 đơn hàng, 20 đã bàn giao cho vận
chuyển, `ledger_entry` có **0 dòng**.

`docs/07-workflows/customer-purchase.md` vẽ rõ: sau `order.placed`, event
bus gọi payment "ghi bút toán doanh thu, hoa hồng". Module payment đã dựng
xong — `RecordOrderRevenue`, `RecordRefund`, `RecordPayout`, sổ kép, kiểm
cân đối lúc khởi động — và đã được nối vào `internal/app`.

Nhưng nó **không nghe event nào**: thư mục payment không có `handlers.go`.
Các module đăng ký nghe `checkout.completed` là analytics, inventory,
notification, order, supplychain. Payment không có mặt.

Đếm bên gọi NGOÀI module, bằng grep trên toàn repo:

```text
RecordOrderRevenue   0
RecordRefund         0
RecordPayout         0
MarkOrderPaid        0      ← đơn không bao giờ rời PENDING_PAYMENT
MarkDelivered        0      ← fulfillment dừng vĩnh viễn ở HANDED_OVER
```

Đây KHÔNG phải lỗi logic — từng mảnh đều đúng và có test. Đây là một sợi
dây chưa nối, nằm đúng chỗ quan trọng nhất.

**Việc cần làm, theo thứ tự:**

1. `handlers.go` cho payment, nghe `checkout.completed`, ghi bút toán
   trong CÙNG giao dịch dispatcher đã mở — đúng khuôn `CommitInEventTx`
   của inventory.
2. Route cho `MarkDelivered`; với COD thì doanh thu ghi nhận lúc giao
   thành công, nên hai việc này là một chuỗi.
3. Sau đó mới tới quyết toán: bảng `settlement` chưa tồn tại.

**Kéo theo:** khẳng định "tối đa 1 bút toán sổ cái" trong
`internal/app/api_idempotency_test.go` hiện luôn đúng với 0 và CHƯA BAO
GIỜ có tác dụng. Nó chỉ thành kiểm tra thật sau khi mục 1 xong — lúc đó
phải kiểm chứng lại bằng cách phá.

---

### PH-35 — Nhà bán chưa tự đăng ký được `[XONG 27/08]`

`applyAsSeller` có trong đặc tả và `Module.ApplyAsSeller` chạy được, nhưng
KHÔNG có route. Cách duy nhất tạo nhà bán hiện nay là công cụ dòng lệnh
`cmd/taonhaban`.

**Chặn ở đâu:** đặc tả bắt buộc `bank_account` (bank_code, account_number,
account_holder) trong thân request. Domain hiện chỉ có cờ
`bankAccountVerified`; KHÔNG có chỗ nào lưu số tài khoản — đã kiểm cả
schema `seller` lẫn toàn bộ bảng.

Không thể trả tiền cho nhà bán mà không biết tài khoản, nên đây là mảnh
còn thiếu thật, không phải chi tiết bỏ qua được. Nhưng số tài khoản ngân
hàng là dữ liệu tài chính nhạy cảm, và `internal/platform/privacy` hiện
chỉ có băm địa chỉ IP — chưa có mã hóa trường.

**Đã giải quyết** — xem [ADR-0014](../adr/0014-ma-hoa-truong-nhay-cam.md):
AES-256-GCM, khóa từ `ENCRYPTION_KEY`, số đầy đủ KHÔNG nằm trong entity,
bốn số cuối lưu riêng ở dạng rõ để hiển thị.

Kèm theo, ràng buộc `seller_verified_needs_account` khiến
`bank_account_verified` từ nay có nghĩa: nó nói về một tài khoản cụ thể.
Trước đó dữ liệu thật có một nhà bán ACTIVE mang cờ đã-xác-minh mà không
tài khoản nào — họ bán được hàng nhưng không nhận được tiền.

**Còn phải làm:** xoay khóa, KMS, và ghi audit khi đọc số tài khoản. Ba
việc này liệt kê trong ADR mục "CHƯA làm"; việc thứ ba PHẢI xong trước khi
đường chi trả chạy.

---

### PH-32 — Phiên thanh toán hết hạn TRONG LÚC đang hoàn tất `[XONG 27/08]`

**Trạng thái:** đã xác định, CHƯA sửa. Ghi nhận theo quy tắc "phát hiện
ngoài phạm vi thì ghi lại, không tự ý triển khai".

**Cùng lớp lỗi với PH-31**, tìm ra khi quét những chỗ có chung hình dạng
sau khi PH-31 được giải thích xong.

Hình dạng: đọc thực thể A → kiểm bất biến TRONG BỘ NHỚ → đọc thực thể B →
ghi cả hai. PostgreSQL chạy READ COMMITTED, nên mỗi câu lệnh lấy một ảnh
chụp mới và hai lần đọc có thể thấy hai thời điểm khác nhau. Kiểm tra ở
tầng ứng dụng không thay được ràng buộc ở tầng dữ liệu.

`CompleteCheckout` mang đúng hình dạng đó:

```text
đọc checkout          → chưa hết hạn, chưa kết thúc
   ExpireStale chen vào: đánh dấu EXPIRED + nhả toàn bộ reservation
kiểm IsExpired() trên bản TRONG BỘ NHỚ   → vẫn "chưa hết hạn" → đi tiếp
tạo đơn hàng
```

Kết quả: đơn hàng tồn tại cho số hàng vừa được trả lại kho. Bên xử lý
`checkout.completed` sau đó chốt hàng thất bại, thử lại, rồi rơi vào hàng
đợi chết — trong khi khách đã nhận xác nhận đặt hàng.

Bảng `checkout` KHÔNG có cột `version`, và `CheckoutStore.Save` là upsert
không điều kiện — đúng tình trạng của `reservation` trước migration
000028.

**Hướng sửa đề xuất** (theo [ADR-0013](../adr/0013-write-transaction-boundary.md)
mục 2): thêm `checkout.version` và ràng buộc phiên bản vào `Save`. Xung
đột ở đây HIẾM (chỉ khi job dọn hạn chen đúng vào lúc khách bấm nút), nên
khóa lạc quan đúng hơn khóa bi quan.

**Chưa làm vì:** cần một bài tái hiện xác định trước, theo đúng cách
`internal/modules/inventory/nha_hai_lan_test.go` đã làm — chặn đúng khe
giữa hai lần đọc bằng đồng hồ tiêm vào, thay vì bắn nhiều lượt song song
và trông chờ may rủi. Module checkout chưa có cổng tiêm đồng hồ tương
đương; cần kiểm xem có sẵn không trước khi thêm.

### 2.1 PH — Commerce Core Hardening

| # | Việc | Trạng thái |
|---|---|---|
| PH-1 | **Mở rộng E2E PostgreSQL** — ma trận ở mục 2.5 | ✅ 12/12 kịch bản, 14 test |
| PH-2 | **Bất biến ownership**: Offer → Seller → Inventory Owner → Reservation → Fulfillment | ✅ xong 06/09 — phủ nốt HỦY TỪNG PHẦN với một kho hai chủ; xem ghi chú dưới bảng |
| PH-3 | Test tích hợp API qua HTTP thật | ✅ 6 test, xem 2.4b |
| PH-4 | **E2E giao diện** — ma trận ở mục 2.6 | ✅ 7/7 luồng · 21 test |

**PH-2 — phần `partial` còn thiếu, và một bài test tự nói dối `[06/09]`.**

`TestHuyMotPhanDonVanConHieuLuc` kiểm TRẠNG THÁI ĐƠN sau khi hủy một phần,
nhưng không nhìn tới tồn kho lần nào — nên một cài đặt trả hàng về SAI CHỦ
vẫn xanh. Đó là phần `partial` mà dòng cũ của PH-2 ghi là còn thiếu.

Hỏng thì hỏng thế này: hủy phần của nhà bán A mà hàng chạy về kho của B.
A vĩnh viễn thiếu, B tự nhiên thừa, không lỗi nào báo và không con số nào
âm — cả hai chỉ phát hiện ở lần kiểm kê thật.

**Bản đầu của bài test mới TỰ NÓI DỐI, và phép phá lộ ra điều đó.** Nó
dùng `stockFor`, thứ cấp cho MỖI CHỦ MỘT KHO RIÊNG. Truy vấn tìm dòng tồn
kho theo `(sku_id, stock_location_id, inventory_owner_id)`; hai kho khác
nhau thì hai cột đầu đã đủ, cột thứ ba không bao giờ phải làm việc. Bỏ hẳn
`inventory_owner_id` khỏi mệnh đề WHERE — đúng lỗi ADR-0012 sinh ra để
chặn — mà bài test VẪN XANH.

Sửa thành MỘT KHO DÙNG CHUNG cho hai chủ thì cùng phép phá đó đỏ ngay. Và
một kho hai chủ không phải ca dựng ra cho vui: hàng nhà bán gửi ở kho nền
tảng vẫn thuộc nhà bán, nên cùng SKU cùng kho có bản ghi riêng cho từng
chủ — chính là ca mà mục P1.6 đã ghi khi làm `adjustInventory`.

Bài học lặp lại: **một bài test xanh có thể xanh vì dữ liệu dễ chứ không
phải vì code đúng, và chỉ có phá mới phân biệt được hai thứ đó.**

### 2.2 PH — Reliability

| # | Việc | Trạng thái |
|---|---|---|
| PH-5 | **Chuẩn hóa idempotency** — bảng ở mục 2.7 | ✅ 5/5 có ràng buộc ở tầng dữ liệu |
| PH-6 | Chuẩn hóa retry / xử lý thất bại | ✅ ba tầng, có test cưỡng chế — xem 2.16 |
| PH-7 | **Event versioning** — quy tắc + test tự động | ✅ xong 06/09 — ADR-0016, bên nhận khai phiên bản, lệch thì HOÃN; xem 2.8 |
| PH-8 | Kiểm ranh giới giao dịch: Order · Inventory · Fulfillment · Payment · Outbox | ✅ tiêm lỗi ở tầng DB, tìm ra và bịt một khe hở thật — xem 2.15 |
| PH-40 | **Thuế luôn bằng 0** — `SetTax` không có bên gọi nào ở production | ✅ xong 06/09 — chủ dự án quyết: một tầng, mặc định 8%, sửa được lúc chạy; xem 2.2b |

### 2.2b PH-40 — thuế luôn bằng 0, và không có gì báo `[MỚI 06/09]`

Tìm ra khi rà chiều rộng sản phẩm (future-phases.md mục 6).

`TaxAmount` có mặt ở **mọi tầng** — `Checkout`, `Order`, `PlaceOrderInput`,
cột database, và nó được cộng vào `Total()` rồi ĐÓNG BĂNG vào đơn:

```text
checkout.SetTax()   tồn tại, CHỈ được gọi từ một bài test, với giá trị 0
production          KHÔNG có đường nào gọi tới nó
kết quả             mọi đơn có tax_amount = 0, vĩnh viễn
```

**Cùng hình dạng với `payment_method` trước P3-9:** một trường đi qua mọi
tầng mà không ai điền, và không tầng nào báo lỗi vì 0 là một con số hợp lệ.

**Khác biệt là hậu quả.** `payment_method` sai làm kho không biết thu tiền
lúc giao. Thuế bằng 0 thì hóa đơn sai, sổ cái ghi doanh thu chưa trừ thuế,
và đó là chuyện của cơ quan thuế chứ không phải của đội vận hành. Con số
sai lại nằm trong một cuốn sổ BẤT BIẾN (ADR-0008) — sửa phải ghi bút toán
đảo cho từng đơn.

**ĐÃ SỬA 06/09 — chủ dự án quyết: thuế MỘT TẦNG, mặc định 8%, sửa được
lúc chạy** (`checkout.tax_rate_bp`, phần vạn). Cùng đợt: miễn phí vận
chuyển áp trên TỔNG ĐƠN, ngưỡng mặc định 499.000đ
(`checkout.free_shipping_threshold`). Đặc tả ở
[checkout.md mục 7 và 7b](../04-modules/checkout.md).

**Giá niêm yết là giá ĐÃ GỒM VAT** (chủ dự án quyết 07/09). Khách trả đúng
con số đã thấy; thuế TÁCH NGƯỢC ra để ghi hóa đơn, không cộng thêm.

**Thứ tự tính, và vì sao nó phải nằm ở MỘT chỗ:**

```text
tiền hàng = subtotal − giảm giá          đã gồm VAT
phí ship  = 0 nếu tiền hàng ≥ ngưỡng, ngược lại = phí theo từng nguồn
tổng      = tiền hàng + phí ship         khách trả đúng con số này
thuế      = tổng × r / (10000 + r)       phần VAT nằm TRONG tổng
```

Ba con số PHỤ THUỘC NHAU theo đúng thứ tự đó. Giảm giá đổi thì ngưỡng miễn
phí ship phải xét lại, và phí ship đổi thì thuế phải tính lại. Nên chỉ có
MỘT hàm đặt cả hai (`Checkout.ApDungPhiVaThue`), và cả ba đường ghi tiền —
chọn cách giao, áp mã, gỡ mã — đều gọi lại nó. Để mỗi đường tự cập nhật
một mảnh là cách chắc chắn để ba con số lệch nhau.

**Áp mã giảm giá nay ghi MỘT LẦN thay vì hai.** Bản cũ ghi giảm giá rồi
mới đặt phí ở lần ghi thứ hai; nếu lần thứ hai hỏng, phiên nằm lại với ba
con số không khớp — vĩnh viễn.

**Bốn quyết định nhỏ, mỗi cái có phá để kiểm chứng:**

```text
TÁCH ngược, không cộng thêm    phá → "thuế 10400, mong 9630"
Total() KHÔNG cộng thuế         phá → "tổng 139630, mong 130000 — hai lần"
thuế tách từ CẢ phí ship        phá → "bỏ phí ra ngoài còn 7.407"
ngưỡng xét SAU giảm giá         phá → "phí 0, mong 30000"
làm tròn NỬA LÊN                phá → "thuế 7555, mong 7556"
```

Vận chuyển là dịch vụ chịu thuế, không phải khoản thu hộ. Ngưỡng xét trên
subtotal TRƯỚC giảm giá thì một mã giảm 200.000đ biến đơn 400.000đ thành
đơn được miễn phí ship. Và `RoundHalfUp` là quy tắc kernel đã ghi sẵn cho
thuế, khác `RoundDown` của hoa hồng — trộn hai quy tắc làm đối soát ra hai
kết quả cho cùng một đơn.

**Phá lại đúng lỗi gốc để chắc bài test có giá trị:** bỏ lời gọi đặt thuế
→ `"thuế trên ĐƠN = 0, mong 10400"`. Đó chính là trạng thái PH-40 mô tả.

**Bản đầu (06/09) cài NHẦM mô hình, và việc ghi giả định ra đã cứu nó.**
Docs không nói giá đã gồm VAT ở đâu và `Total()` cộng thuế vào, nên tôi cài
theo mô hình CỘNG THÊM rồi ghi rõ giả định đó vào checkout.md và commit
message. Chủ dự án đọc và sửa lại ngay hôm sau: giá niêm yết ĐÃ gồm VAT.

Sửa lại đụng ba chỗ, không chỉ một: thêm `money.ExtractRate`, bỏ
`Add(taxAmount)` khỏi `Total()` ở CẢ `Checkout` lẫn `Order`, và đổi mọi con
số trong test. Nếu giả định không được ghi ra, mô hình sai đã chạy tiếp và
mỗi đơn thu thừa 8%.

**CÒN THIẾU, do "một tầng":** thuế suất theo loại hàng, miễn thuế theo
nhóm khách, và câu hỏi marketplace "nền tảng hay nhà bán xuất hóa đơn".
Câu cuối quyết định thuế thuộc `order` hay `seller`; hôm nay nó nằm ở
`order` vì một tầng thì hai cách cho cùng kết quả.

### 2.3 PH — Security

| # | Việc | Trạng thái |
|---|---|---|
| PH-9 | **Audit authorization toàn bộ resource** | ✅ ma trận 175 cặp, xem 2.9 |
| PH-10 | Rà: xác thực · phân quyền · kiểm tra đầu vào · rate limit · CORS · lộ dữ liệu · nhật ký kiểm toán | ✅ xem 2.9 |

### 2.4 PH — API Contract

| # | Việc | Trạng thái |
|---|---|---|
| PH-11 | OpenAPI là nguồn sự thật DUY NHẤT | 🟢 `types:check` chặn ở CI |
| PH-12 | Kiểm chuỗi: OpenAPI → TypeScript sinh ra → Go → giao diện | ✅ test hợp đồng gọi API thật, xem 2.10 |
| PH-13 | Giao diện KHÔNG tự cài lại quy tắc nghiệp vụ | 🟢 đã sửa `is_sellable` |
| PH-14 | Tránh N+1 khi cửa hàng cần dữ liệu seller/product/offer | 🟢 tra theo lô |

### 2.4b Test tích hợp API (PH-3) `[XONG 26/08]`

`internal/app` dựng TOÀN BỘ ứng dụng — module thật, route thật, middleware
thật, PostgreSQL thật — rồi gọi qua HTTP.

**Vì sao cần lớp này khi đã có test module và test đầu-cuối.** Cả hai lớp
kia đều BỎ QUA tầng HTTP: chúng gọi thẳng service Go. Bốn loại lỗi chỉ
sống ở ranh giới HTTP và không lớp nào khác nhìn thấy:

```text
quên bọc RequireRole            → ai cũng gọi được đường quản trị
quên bọc RequireIdempotencyKey  → mất chống trùng, không báo gì
handler đúng nhưng SAI mã trạng thái
tên/kiểu trường JSON lệch đặc tả
```

Ba loại đầu là lỗi NỐI DÂY: bản thân module đúng, chỉ chỗ ráp sai.

**Một test chống bỏ sót.** `TestMoiDuongCanQuyenDeuDuocKiem` quét mã nguồn
tìm mọi route dưới `/api/v1/admin/` và `/api/v1/seller/`, rồi đối chiếu với
danh sách được kiểm phân quyền. Thêm route mới mà quên khai là ĐỎ, kèm tên
đường. Nó biến "phải nhớ" thành "test sẽ báo".

Nó phá vỡ ranh giới thường thấy giữa test và cài đặt — đánh đổi có ý thức:
cái giá của việc quên bọc `RequireRole` là mở toang một đường quản trị.

**Kiểm chứng bằng cách phá:**

```text
bỏ RequireRole của admin/sellers → token CUSTOMER nhận HTTP 200 kèm
                                    danh sách nhà bán VÀ tỷ lệ hoa hồng
thêm route admin mới, quên khai  → "1 đường cần quyền KHÔNG có trong
                                    danh sách kiểm phân quyền"
```

**Một ngoại lệ được ghi thành test.** `/api/v1/admin/me` nằm dưới tiền tố
`admin` nhưng CỐ Ý chỉ cần đăng nhập: Trung tâm người bán dùng chính nó để
khôi phục phiên. Ngoại lệ không có test là ngoại lệ sẽ bị ai đó "sửa" cho
nhất quán.

**Và chuỗi middleware trong test đã từng lệch production** — bản đầu bỏ
quên `Logging` nên `logger.FromContext` rơi về logger mặc định. Đầu ra
nhiễu chỉ là triệu chứng; vấn đề thật là test đang kiểm một hệ thống khác.

### 2.5 Ma trận E2E PostgreSQL (PH-1)

`internal/e2e` nay có **24 bài trong 10 file** — câu cũ ở đây ("hiện có một
test, điểm yếu lớn nhất của phase") đã lạc hậu từ lâu và giữ nguyên thì
người đọc sẽ đi sửa một vấn đề không còn tồn tại.

Lý do bộ test này quan trọng thì KHÔNG đổi: loại lỗi nguy hiểm nhất nằm ở
KHOẢNG GIỮA các module, và test từng module không thấy được — P3-18 đã
chứng minh điều đó.

| Kịch bản | Trạng thái |
|---|---|
| Own Brand + Marketplace trong CÙNG một đơn | ✅ `TestDonNhieuNhaBanDiHetChuoi` |
| Nhiều seller trong cùng giỏ/đơn | ✅ cùng test trên |
| Cô lập chủ sở hữu tồn kho | ✅ cùng test trên |
| Không đủ hàng giữa chừng — và NHẢ lại hàng đã giữ | ✅ `TestMotMonHetHangThiNhaHetHangDaGiu` |
| Không mượn kho người khác khi hết hàng (toàn chuỗi) | ✅ `TestOwnBrandVaNhaBanKhongDungChungKhoKhiHetHang` |
| Thử lại / gửi trùng request | ✅ `TestHoanTatHaiLanCungKhoaChiRaMotDon` |
| **Thanh toán ĐỒNG THỜI — không oversell** | ✅ `TestMuoiKhachTranhBaMonKhongAiMuaQua` (tới `StartCheckout`) + `TestHoanTatDongThoiKhongSinhHangTuKhongKhi` (**toàn chuỗi**, 06/09) |
| Phát LẠI event không trừ kho hai lần | ✅ `TestVetOutboxHaiLanKhongTruKhoHaiLan` `[06/09]` |
| Một giỏ, nhiều tab — không giữ hàng nhiều lần | ✅ `TestHaiTabCungGioChiGiuHangMotLan` |
| Thực hiện TỪNG PHẦN (một seller giao, một seller chưa) | ✅ `TestGiaoDuTungPhanRoiDuHet` |
| Giao hàng TỪNG PHẦN (một nguồn đã xuất, nguồn kia chưa) | ✅ `TestGiaoTungPhan` · `TestHuyMotPhanDonVanConHieuLuc` |
| Hủy đơn (trước và sau khi lấy hàng) | ✅ 4 test — `TestHuyDonTraHangVeKho` và 3 test cùng nhóm |
| Giao dịch cuộn ngược khi bên nhận lỗi | ✅ `TestBenNhanHongThiCuonNguocPhanGhiCuaChinhNo` |

**Bất biến không-oversell được chứng minh ở TOÀN CHUỖI**, không chỉ ở tầng
inventory. Hai mức, và mức thứ hai mới thêm 06/09:

Mức 1 — giữ hàng: 10 khách tranh 3 món qua `StartCheckout` thật (đọc giỏ →
tra chủ sở hữu → chọn kho → giữ hàng → ghi phiên) cho đúng 3 người thắng,
`available` về 0 và không bao giờ âm. Bỏ khóa lạc quan → **cả 10 người đều
giữ được hàng**, đỏ 3/3 lần chạy.

Mức 2 — trọn đường tiền và hàng: 10 khách cùng HOÀN TẤT phiên, rồi vét
outbox để Reserved → Committed thật sự xảy ra. Khẳng định là BẢO TOÀN chứ
không chỉ "không âm": `available + reserved + committed == số hàng đã
nhập`. Kho âm là lỗi tự lộ ra; hàng bốc hơi hay sinh thêm mà ba con số vẫn
dương thì không có gì báo cho tới lần kiểm kê thật.

**Giao TỪNG PHẦN khóa cả hai phía.** Một nửa thì phải là "một phần", đủ
cả thì phải chuyển sang trạng thái cuối. Chỉ kiểm một phía thì một cài đặt
luôn trả `PARTIALLY_DELIVERED` vẫn xanh. Kèm ca dễ sai theo hướng tai hại:
một nhà bán hủy KHÔNG được làm cả đơn thành đã hủy — khách sẽ mất phần
hàng còn lại mà không ai báo.

**Cuộn ngược kiểm ở đúng chỗ nó quan trọng.** Dispatcher chạy mỗi bên nhận
trong một savepoint riêng bao cả phần ghi LẪN việc đánh dấu đã xử lý. Bên
nhận dùng trong test GHI DỮ LIỆU rồi mới lỗi — một bên nhận chỉ trả lỗi
mà không ghi gì thì không kiểm chứng được gì. Bỏ `sp.Rollback` → "còn 1
dòng của bên nhận đã lỗi, cần 0".

Ghép chung một giao dịch cho mọi bên nhận thì một bên phụ (gửi email) hỏng
sẽ cuộn ngược cả việc chuyển tồn kho Reserved → Committed — và khi đó tiến
trình dọn có thể nhả hàng của một đơn ĐÃ THANH TOÁN.

**Giới hạn đã biết của hai bài "phát lại":** chúng kiểm KẾT QUẢ (hàng không
quay về hai lần) chứ không tách được lớp nào tạo ra kết quả. Bỏ riêng
`ON CONFLICT DO NOTHING` ở dispatcher KHÔNG làm chúng đỏ, vì trạng thái
domain cũng chặn. Cơ chế "đúng một lần" của dispatcher có test riêng ở
`internal/platform/eventbus`. Ghi lại để không ai đọc nhầm mức bảo đảm.

**Một phát hiện đáng ghi:** bất biến "một giỏ một phiên" có phòng vệ BA
lớp — chốt ở tầng ứng dụng, chỉ mục UNIQUE có điều kiện ở database, và
đường nhả hàng khi `Save` thất bại. Bỏ RIÊNG lớp đầu thì test vẫn xanh.
Điều đó dễ dẫn tới kết luận sai theo cả hai chiều: tưởng test vô dụng,
hoặc tưởng một lớp là đủ nên gỡ lớp kia.

### 2.5b PH-28 — hủy đơn làm rò rỉ tồn kho `[ĐÃ SỬA 20/08]`

**Lỗi.** Kiểm chứng bằng đơn thật trước khi sửa:

```text
đặt 5 món  →  15 khả dụng / 5 cam kết
hủy đơn    →  15 khả dụng / 5 cam kết   ← không đổi
```

Năm món kẹt vĩnh viễn ở trạng thái cam kết. Đường vào kho có
(Reserved → Committed khi đặt hàng), đường ra không: `order.cancelled`
được định nghĩa nhưng chưa ai phát, `fulfillment.progress` chỉ mang cờ
boolean, và inventory chỉ có một handler duy nhất là Commit.

**Sửa: event MỚI `fulfillment.cancelled`**, không mở rộng payload đang có.

Đây là lựa chọn có chủ ý. Mở rộng `fulfillment.progress` sẽ bắt ba bên
nhận hiện có (order, notification, analytics) tải dữ liệu họ không dùng —
và quan trọng hơn, ĐỔI payload đang chạy đòi hỏi triển khai bên nhận
trước bên phát (PH-7, khi đó chưa có quy trình — nay đã có, xem ADR-0016).
Thêm event mới thì bên nhận cũ không bị ảnh hưởng gì, nên không phải chờ.

```text
fulfillment.cancelled  (MỚI)
  order_id · fulfillment_id · seller_id · stock_location_id
  release_stock · lines[{sku_id, quantity}]
        ↓
  inventory.ReleaseOnFulfillmentCancelled   (handler MỚI)
        ↓
  Committed → Available

fulfillment.progress  (KHÔNG ĐỔI)
```

**Hai điều kiện, không phải một:**

1. `release_stock` — hàng còn trong kho hay đang trên đường trả về. Quy
   tắc ở domain: `FOStatus.StockStillInWarehouse()`, đúng với
   PENDING/ALLOCATED/CONFIRMED. Hủy sau khi GIAO THẤT BẠI thì hàng đã rời
   kho, KHÔNG trả về — bán một món chưa cầm trong tay là để lỗi hiện ra ở
   khách THỨ HAI. Hàng đó nhập lại qua quy trình hàng trả có kiểm tra
   chất lượng.
2. Chủ sở hữu suy ra từ nhà bán qua `OwnerForSeller` (ADR-0012). Event
   mang `seller_id` chứ KHÔNG mang `inventory_owner_id`: tự tính ở bên
   phát là cài quy tắc lần thứ hai — đúng lỗi P3-18.

**Kiểm chứng bằng cách phá** — bốn bản phá, bốn lần đỏ:

```text
bỏ phát event hủy          → "còn 15 khả dụng, cần 20 — hàng KHÔNG quay về kho"
dùng thẳng seller_id       → "hàng nền tảng 26/4, cần 30/0"  (own brand)
bỏ điều kiện release_stock → "tồn kho ĐỔI khi hàng đang trên đường về: 15/5 → 20/0"
phát lại event 3 lần       → xanh (dispatcher đảm bảo xử lý đúng một lần)
```

Bản phá thứ ba ban đầu KHÔNG bị bắt: tôi cài quy tắc nhưng chưa có test
nào đi qua đường DELIVERY_FAILED → CANCELLED. Đó là lý do phải phá — quy
tắc không có test là quy tắc sẽ mất trong lần sửa sau.

**Còn lại:** quy trình nhập lại hàng trả về (ReceiveReturn →
InspectionPassed/Failed đã có ở domain inventory, chưa có đường nối).

### 2.6 Ma trận E2E giao diện (PH-4)

| Luồng | Trạng thái |
|---|---|
| Khách mua hàng (chọn màu → size → nhà bán → giỏ) | ✅ |
| Khách VÃNG LAI thanh toán | ✅ tới tận mã đơn |
| Khách ĐÃ ĐĂNG KÝ mua hàng | ✅ tới tận mã đơn |
| Đơn nhiều nhà bán | ✅ giỏ nhóm theo nhà bán → 2 gói giao |
| Nhà bán tạo offer | ✅ kèm tồn kho ban đầu |
| Nhà bán cập nhật tồn kho | ✅ |
| Nhà bán thực hiện đơn (bàn giao vận chuyển) | ✅ tự đặt đơn rồi bàn giao |

### 2.6b PH-29 — `GET /api/v1/cart` nay CHỈ ĐỌC `[XONG 26/08]`

Tìm được khi dựng E2E luồng khách đã đăng nhập. Đỏ khoảng MỘT NỬA số lần
chạy, luôn ở cùng một chỗ:

```text
đặt hàng xong → refreshCart() → GET /api/v1/cart → 500
error: "cart: không tìm thấy"
```

Đơn hàng KHÔNG bị ảnh hưởng — đơn đã tạo, trang chi tiết hiển thị đúng.
Nhưng mỗi lần mua hàng thành công đều kèm một lỗi 500.

**Chẩn đoán.** Ngay sau `completeCheckout`, giỏ đã chuyển thành đơn nên
`findActive` không thấy giỏ nào. `GetOrCreateCart` tạo giỏ mới; hai request
song song cùng làm vậy; database chặn cái thứ hai bằng ràng buộc chủ sở
hữu; nhánh xử lý đọc lại bằng `findActive` — và ở đúng khoảnh khắc đó nó
có thể lại không thấy gì, trả `ErrNotFound` lên tầng trên thành 500.

**Có hai vấn đề, không phải một:**

1. `GET /api/v1/cart` GHI dữ liệu (`Sync` kết thúc bằng `Save`). Hai lượt
   đọc song song vì thế tranh chấp nhau — điều không ai chờ đợi ở một
   endpoint đọc.
2. "Không có giỏ" bị đối xử như lỗi máy chủ. Khách vừa mua xong thì không
   có giỏ là trạng thái BÌNH THƯỜNG, đáng ra trả giỏ rỗng.

**Đã sửa: đường đọc không ghi gì nữa.**

```text
GetOrCreateCart + Sync(ghi)  →  FindActiveCart + SyncView(chỉ tính)
chưa có giỏ                  →  trả giỏ RỖNG, không tạo
```

Giỏ được tạo ở lần THÊM MÓN đầu tiên — một đường ghi, đúng chỗ của nó.

**Vì sao không ghi mà vẫn đúng:** giỏ KHÔNG hứa gì với khách. Giá và tình
trạng hàng ở giỏ là thông tin tham khảo; cam kết chỉ có ở checkout. Con số
đã đồng bộ chỉ cần đúng TẠI THỜI ĐIỂM hiển thị — lưu xuống không làm nó
đúng lâu hơn, vì lần đọc sau lại đồng bộ lại từ đầu.

Ghi ở đường đọc còn có hại: mỗi lần khách mở giỏ là một lần ghi database,
và ở trang nhiều tab thì thành nhiều lần ghi tranh nhau cho cùng dữ liệu.

**Hai test cũ phải đổi** vì chúng mã hóa hợp đồng cũ. Bài kiểm cách ly
phiên nay dựng giỏ bằng cách THÊM MÓN — đúng cách khách dựng ra nó — và
kiểm thẳng điều đáng lo: phiên này không được thấy hàng của phiên kia.

**Giới hạn đã biết:** bài test đồng thời KHÔNG tái hiện được lỗi gốc (phá
bản sửa thì nó vẫn xanh) — thời điểm gây lỗi quá hẹp. Bài chứng minh bản
sửa là `TestDocGioKhongTaoGio`, kiểm thẳng nguyên nhân thay vì triệu
chứng. Ghi ra để không ai đọc nhầm mức bảo đảm.

### 2.6b Dựng nhà bán thứ hai (`cmd/taonhaban`)

Luồng "đơn nhiều nhà bán" bị chặn bởi DỮ LIỆU, không phải bởi code: dữ
liệu mẫu chỉ có MỘT nhà bán — Lumière, loại INTERNAL. Với một nhà bán thì
không có gì để tách, nên phần lõi của kiến trúc chợ chưa bao giờ được thử
qua giao diện.

Công cụ đưa nhà bán qua ĐỦ vòng đời, không tắt bước nào:

```text
nộp hồ sơ → duyệt → xác minh ngân hàng → kích hoạt
```

Ghi thẳng `ACTIVE` vào database sẽ tạo ra một nhà bán không bao giờ tồn
tại được ngoài đời, và test dựa trên nó sẽ nói dối.

**Hai rào cản gặp phải, cả hai đều là quy tắc ĐÚNG:**

1. Thương hiệu Lumière ở mức `RESTRICTED` — chỉ chủ thương hiệu được bán.
   Nhà bán ngoài cần hàng của thương hiệu `OPEN`, nên công cụ tạo một sản
   phẩm mới dưới Basics Co.
2. Sản phẩm thời trang bắt buộc có BẢNG SIZE, và bảng phải thuộc chính
   thương hiệu đó — "M" của thương hiệu này không phải "M" của thương hiệu
   kia, đúng lý do bảng size tồn tại.

Vai trò `SELLER_OWNER` được gán kèm PHẠM VI là gian hàng. Phạm vi là thứ
quyết định: vai trò không phạm vi nghĩa là người này làm chủ MỌI gian
hàng — đúng lỗ hổng mà cách ly giữa các nhà bán tồn tại để chặn.

Kèm một sửa nhỏ cho nhất quán: `catalog` là module duy nhất thiếu
`Service()`, trong khi product, seller và marketplace đều có.

### 2.6c Hai bài học về test giao diện

**Test TIÊU THỤ dữ liệu của chính nó thì chỉ xanh một lần.** Bài bàn giao
vận chuyển lấy một đơn đang chờ rồi bàn giao — lần chạy sau không còn đơn
nào, và nó tự bỏ qua. Một bài test bị bỏ qua trông y hệt một bài đã chạy,
nên nó âm thầm ngừng bảo vệ.

Sửa: bài test TỰ ĐẶT một đơn qua API trước khi làm việc của mình. Và
KHÔNG dùng `test.skip` khi chưa thấy dữ liệu — chờ có thời hạn rồi
`expect` thất bại, để im lặng không bị nhầm với thành công.

**Đơn thực hiện xuất hiện BẤT ĐỒNG BỘ.** `checkout.completed` vào outbox,
worker đọc theo nhịp rồi mới tách đơn. Nhìn ngay sau khi đặt là nhìn quá
sớm — hành vi đúng của kiến trúc event, không phải chỗ cần sửa. Test chờ
bằng `expect.poll`.

Cũng chính lúc dựng bài này mới lộ ra worker đã CHẾT từ lúc Postgres tắt,
để lại **67 event tồn đọng** mà không ai biết. Đó đúng là thứ
`gouse_outbox_pending_events` sinh ra để báo — nhưng chỉ số do CHÍNH worker
đặt, nên worker chết thì con số đứng yên thay vì kêu. Ghi thành PH-30.

### 2.6d PH-30 — nhịp tim của worker `[XONG 26/08]`

`OutboxPending` do CHÍNH worker đặt, nên worker chết thì con số đứng yên ở
giá trị cuối — và "0 event tồn đọng" của một worker đã chết trông y hệt "0
event tồn đọng" của một worker khỏe.

Đã xảy ra thật (26/08): worker chết cùng lúc PostgreSQL tắt, để lại 67
event tồn đọng suốt nhiều giờ. Không đơn thực hiện nào được tạo, không
email nào được gửi, và không có gì kêu.

```text
gouse_worker_heartbeat_timestamp_seconds   dấu thời gian lần cuối chạy xong
gouse_worker_job_duration_seconds          độ trễ từng job
gouse_worker_job_failures_total            đếm lượt thất bại theo job
```

**Cảnh báo trên ĐỘ TƯƠI, không phải trên giá trị:**

```promql
time() - gouse_worker_heartbeat_timestamp_seconds > 60
```

Kết hợp `up{job="gouse-worker"} == 0` thì phủ được CẢ HAI kiểu chết:

| Kiểu chết | Bắt bằng |
|---|---|
| Tiến trình biến mất | `up == 0` — `/metrics` không trả lời |
| Tiến trình sống, vòng lặp treo | nhịp tim cũ dần |

Ba quyết định nhỏ:

- **Đơn vị là GIÂY UNIX tuyệt đối**, không phải "số giây kể từ lần cuối":
  dấu thời gian vẫn đúng khi Prometheus thu thập trễ, còn khoảng thời gian
  tự tính thì không.
- **Nhịp tim đập sau MỌI lượt chạy, kể cả lượt thất bại.** Nó trả lời
  "còn sống", không phải "còn khỏe". Chỉ đập khi thành công sẽ khiến một
  job liên tục lỗi trông y hệt một worker đã chết.
- **Độ trễ job** bổ sung: nhịp tim nói "còn sống", độ trễ nói "còn kịp".

Kiểm chứng: nhịp tim ĐẬP LẠI sau 10 giây (không phải đặt một lần lúc khởi
động), và khi giết worker thì `/metrics` ngừng trả lời đúng như mong đợi.

### 2.6e PH-31 — nhả giữ hàng HAI LẦN sinh ra hàng từ không khí `[XONG 27/08]`

> **Cập nhật 27/08:** cơ chế ĐÃ xác định và dựng lại được một cách xác
> định. Nguyên nhân là khe hở READ COMMITTED giữa hai câu SELECT trong
> cùng một giao dịch — không phải cuộc đua kinh điển như phần dưới đoán.
> Xem [ADR-0013](../adr/0013-write-transaction-boundary.md) phụ lục và
> `internal/modules/inventory/nha_hai_lan_test.go`.
>
> Phần dưới giữ nguyên làm ghi chép quá trình điều tra.

Phát hiện nhờ chính chỉ số vừa thêm ở PH-30 — đúng loại sự cố mà nó sinh
ra để bắt.

**Triệu chứng.** Job "dọn giữ hàng quá hạn" thất bại MỌI lượt suốt nhiều
giờ: `inventory: không đủ hàng: reserved có 0, cần 1`. Nhịp tim toàn cục
vẫn tươi vì bốn job kia vẫn chạy, nên không gì nổi lên. Chỉ số
`job_last_success` theo từng job làm nó lộ ra ngay.

**Bằng chứng** từ nhật ký biến động, cùng `reference_id`:

```text
18:04:22.826  RESERVE  1  → còn 76   ref=rsv_...GGG
18:19:28.497  RELEASE  1  → còn 77   ref=rsv_...GGG
18:19:28.499  RELEASE  1  → còn 78   ref=rsv_...GGG
```

Cùng một reservation, hai lượt nhả cách nhau 1,5 mili giây. Số khả dụng
lên **78 — cao hơn 77 trước khi giữ**. Hàng sinh ra từ không khí: hệ thống
tin mình có nhiều hàng hơn thực tế và sẽ bán phần chênh cho ai đó.

Hệ quả kéo theo: một reservation khác kẹt ở `ACTIVE` với phần giữ chỗ đã
bị lượt nhả thừa ăn mất, nên job dọn hạn hỏng vĩnh viễn.

**Khoảng trống đã vá.** `ReservationStore.Save` là upsert KHÔNG ĐIỀU KIỆN
— bất biến "một reservation nhả đúng MỘT lần" chỉ được cưỡng chế bằng kiểm
tra trong bộ nhớ ở domain, và hai giao dịch cùng đi qua được. Khóa lạc
quan có ở `inventory_item`, không có ở `reservation`.

Đã thêm cột `version` + `WHERE reservation.version = $10` (migration
000028), cùng cơ chế với `inventory_item` và `fulfillment_order`.

**CHƯA GIẢI THÍCH ĐƯỢC — ghi ra thay vì đoán:**

- Chỉ có MỘT tiến trình worker chạy lúc đó (đã đối chiếu vòng đời hai file
  log). Giả thuyết "hai worker" của tôi SAI.
- `ExpireReservations` duyệt tuần tự, không xử lý một reservation hai lần
  trong một lượt, và có bỏ qua `ErrReservationNotActive`.
- Test tranh chấp 8 goroutine cùng nhả một reservation KHÔNG tái hiện
  được: khóa lạc quan của bản ghi tồn kho buộc thử lại, và lượt thử lại
  đọc thấy trạng thái đã đổi rồi từ chối đúng.

**Đã tìm ra ĐƯỜNG, chưa tìm ra CƠ CHẾ.** Hai job dọn hạn độc lập cùng
nhắm vào một reservation:

```text
inventory.ExpireReservations   mỗi 30 giây — nhả theo RESERVATION
checkout.ExpireStale           mỗi 60 giây — nhả theo PHIÊN (releaseAll)
```

Phiên hết hạn thì reservation của nó hết hạn CÙNG LÚC, nên hai job nhắm
đúng một bản ghi, ở hai goroutine trong cùng một worker. Đó giải thích
được hai lượt nhả cách 1,5ms trong một tiến trình.

Nhưng KHÔNG giải thích được vì sao cả hai THÀNH CÔNG. Đã loại trừ bằng
cách đọc code:

```text
✓ uow.Do cuộn ngược đúng khi fn trả lỗi
✓ mutate ghi item VÀ nhật ký qua CÙNG giao dịch, không qua pool
✓ Release và Expire đều từ chối khi trạng thái đã là cuối (IsFinal)
✓ ExpireReservations duyệt tuần tự, có bỏ qua ErrReservationNotActive
```

Và không tái hiện được: 8 lượt nhả song song, hai job dọn song song, mỗi
kịch bản chạy CÓ và KHÔNG có khóa lạc quan — tám lần đều xanh.

Nghĩa là cột `version` đóng khoảng trống ở tầng dữ liệu nhưng CHƯA chứng
minh được nó chặn đúng cơ chế đã xảy ra. **Bước tiếp theo:** ghi dấu vết ở
`releaseWith` (mã reservation, tiến trình, job gọi tới, số lần thử lại)
rồi chờ nó xảy ra lại, thay vì đoán thêm.

Bản ghi kẹt đã được đánh dấu `EXPIRED` bằng tay; sau đó cả 5 job đều thành
công và bộ đếm lỗi về 0.

### 2.7 Idempotency (PH-5) `[XONG 20/08]`

| Đường ghi | Khóa ở HTTP | Ràng buộc ở tầng dữ liệu |
|---|---|---|
| checkout complete | ✅ | ✅ `UNIQUE` khóa hoàn tất + `UNIQUE` một phiên mỗi giỏ |
| inventory reservation | ✅ | ✅ `UNIQUE (checkout_id, inventory_item_id) WHERE ACTIVE` |
| order place | ✅ | ✅ `UNIQUE (idempotency_key)` |
| payment | ✅ | ✅ `UNIQUE (idempotency_key)` |
| fulfillment / shipment | ✅ | ✅ khóa lạc quan `version` |

**Bắt buộc header KHÔNG phải là idempotent.** Middleware
`RequireIdempotencyKey` chỉ đảm bảo client GỬI khóa; nó không làm gì với
hai request mang cùng khóa chạy song song. Thứ duy nhất không có khe hở
giữa lúc kiểm và lúc ghi là ràng buộc ở database.

**Hai lỗi thật, tìm bằng test tranh chấp chứ không bằng đọc code:**

*Bàn giao vận chuyển hai lần.* 8 lệnh song song → 2–3 lệnh cùng thành
công. `fulfillment_order` không có khóa lạc quan, nên hai request cùng đọc
`PACKED`, cùng thấy hợp lệ, cùng ghi. Hậu quả không chỉ là một dòng thừa:
mỗi lần ghi phát một event tiến độ, nên khách nhận HAI email "đơn đã gửi",
analytics đếm hai lần, và mã vận đơn ghi sau đè lên mã ghi trước — hai
request mang hai mã khác nhau thì mã thật bị mất.

*Giữ hàng hai lần cho cùng một phiên.* Gọi `Reserve` lần thứ hai với cùng
`checkout_id` và cùng bản ghi tồn kho thì THÀNH CÔNG, khóa gấp đôi số hàng
khách cần. Số thừa treo tới khi hết hạn — với hàng bán chạy, đó là cách tự
tạo ra tình trạng hết hàng giả.

Chỉ mục UNIQUE của reservation CÓ ĐIỀU KIỆN trên `status = 'ACTIVE'`: phiên
hết hạn rồi mở lại, hoặc giữ hàng bị nhả rồi giữ lại, đều hợp lệ. Và loại
trừ `checkout_id` rỗng vì nhập kho, điều chỉnh thủ công không thuộc phiên
nào.

**Một bài học về `Restore`.** Thêm cột `version` xong thì mọi bước chuyển
trạng thái TUẦN TỰ đều báo xung đột. Nguyên nhân: `withLineIDs` dựng lại
thực thể từ đầu và tôi quên chép `Version` — nên phiên bản bị xóa trắng ở
mỗi lần đọc. Chú thích ngay trên hàm đó đã cảnh báo đúng điều này: "trường
nào quên là trường đó bị XÓA TRẮNG khi đọc, và lỗi chỉ lộ ra ở test đọc
lại sau khi ghi".

**Kiểm chứng bằng cách phá:**

```text
bỏ `AND version = $17`        → "2 lệnh bàn giao cùng thành công, cần 1"
                                 kèm "2 event fulfillment.progress, cần 1"
DROP INDEX reservation_active_uniq → "giữ hàng lần 2 THÀNH CÔNG"
```

### 2.8 Event versioning (PH-7) `[XONG 06/09]`

Trường đã có: `Event.Version` trong Go, cột `event_version` trong outbox,
mặc định 1. Cái CHƯA có là mọi thứ khác:

```text
✅ bên nhận ĐỌC Version              VersionedHandler + kiểm ở dispatcher
✅ quy tắc thay đổi payload           ADR-0016 mục "khi nào tăng phiên bản"
✅ chiến lược triển khai khi đổi      bên nhận lên TRƯỚC; vi phạm thì HOÃN
```

Đã có sự cố thật (19/08): thêm địa chỉ giao vào `checkout.completed`, một
tiến trình worker CŨ còn sống đã tiêu thụ event mới và **âm thầm bỏ qua**
trường mới. Không lỗi, không log — chỉ là đơn thực hiện thiếu địa chỉ.

**Quyết định nằm ở [ADR-0016](../adr/0016-phien-ban-event.md).** Viết ADR
trước là bắt buộc ở đây chứ không phải nghi thức: cách sửa hiển nhiên —
"bên nhận không hiểu thì trả lỗi" — đụng thẳng vào cơ chế thử lại, nơi mọi
lỗi tăng `attempts` và chạm 5 là `dead_lettered_at` VĨNH VIỄN. Tức là nó
biến một sự cố TỰ LÀNH (lệch phiên bản biến mất khi bản mới lên) thành mất
dữ liệu không hồi được. Không quyết chuyện đó trước thì viết code là chọn
bừa.

**Cơ chế:** bên nhận khai `MaxEventVersion(eventType)`; không khai thì được
coi là hiểu tới v1. Event mới hơn thì dispatcher **HOÃN** — giữ nguyên
trạng thái chờ, KHÔNG tăng `attempts`, ghi log WARN và tăng metric
`gouse_event_version_skew_total`. Event tự chảy tiếp khi bên nhận được nâng
cấp, không cần ai phát lại tay.

**Kiểm phiên bản cho TOÀN BỘ bên nhận trước khi chạy ai.** Nếu A hiểu v2
còn B chưa, chạy A rồi mới hoãn vì B sẽ để hệ thống ở trạng thái NỬA CHỪNG:
một nửa phản ứng của cùng một sự thật đã xảy ra, nửa kia thì chưa.

**Mặc định v1 là điểm mấu chốt**, và cũng là chỗ dễ làm sai nhất. "Chưa
khai thì hiểu mọi phiên bản" nghe an toàn hơn nhưng dựng lại đúng sự cố
19/08. Hôm nay mọi event đều v1 nên mặc định chặt không bắt bên nhận nào
phải sửa gì.

**Kiểm chứng bằng cách phá, hai lần:**

- Đổi mặc định thành "hiểu mọi phiên bản" → bài `TestHandlerKhongKhaiBao…`
  đỏ: handler thường nuốt event v2.
- Cho hoãn đi đường thất bại thường (gọi `markFailed`) → bài tái hiện sự cố
  đỏ đúng chỗ ADR cảnh báo: `dead letter = 1, mong 0`.

**CỐ Ý không nâng `checkout.completed` lên v2.** Ngưỡng tăng phiên bản là
"bên nhận BẮT BUỘC phải có trường mới", và `shipping_address` thì mọi bên
nhận đã có từ lâu. Nâng lại lúc này chỉ tạo ra một đợt lệch phiên bản tự
gây, trong khi không ai được lợi. Cơ chế dựng cho lần thay đổi TIẾP THEO.

**Còn nợ:** event bị hoãn được đọc lại mỗi nhịp worker nên tốn truy vấn vô
ích cho tới khi bên nhận được nâng cấp; và nếu bên nhận KHÔNG BAO GIỜ được
nâng cấp thì nó nằm mãi. `OldestPendingAge` bắt được, nhưng không có cơ chế
tự dọn — cố ý, vì tự dọn nghĩa là tự vứt dữ liệu.

### 2.8b ADR liên quan tới phase này

| ADR | Vì sao liên quan |
|---|---|
| [0006](../adr/0006-internal-events.md) | Outbox và ranh giới giao dịch của event — nền của PH-7, PH-8 |
| [0016](../adr/0016-phien-ban-event.md) | **Phiên bản event và thứ tự triển khai — chính là PH-7** |
| [0007](../adr/0007-marketplace-order-model.md) | Tách Order / FulfillmentOrder — nền của tách đơn nhiều nhà bán |
| [0008](../adr/0008-financial-ledger.md) | Sổ cái bất biến — nền của idempotency thanh toán |
| [0011](../adr/0011-audit-log.md) | Audit log cùng giao dịch với thao tác — PH-10 |
| [0012](../adr/0012-inventory-ownership.md) | **Bất biến chủ sở hữu tồn kho** — trung tâm của PH-2 |

### 2.15 PH-8 — tiêm lỗi ở ranh giới giao dịch `[XONG 03/09]`

Test theo CẶP không tìm ra được lỗi này, vì mỗi cặp đều đúng. Lỗi nằm ở
KHOẢNG TRỐNG giữa hai giao dịch liền nhau.

**Chuỗi hoàn tất phiên đi qua ba giao dịch, có chủ ý:**

```text
1. GiuDeHoanTat       giành quyền, gia hạn expires_at   (giao dịch riêng)
2. orders.PlaceOrder  TẠO ĐƠN                            (giao dịch riêng)
3. SaveWithEvents     trạng thái phiên + event outbox    (một giao dịch)
```

Bước 3 gộp phiên và event — đúng. Nhưng giữa bước 2 và bước 3 có một
khoảng mà **đơn đã tồn tại còn phiên vẫn `STARTED`**, tức vẫn nằm trong
tầm quét của `FindExpired`. Ân hạn 30 giây của `GiuDeHoanTat` rồi cũng hết.

Nếu bước 3 hỏng và khách không thử lại: job dọn nhặt phiên lên và **nhả
toàn bộ hàng của một đơn có thật**. Hàng bán tiếp cho người khác, đơn cũ
thành đơn không có hàng.

Chú thích ở `inventory.CommitOnCheckoutCompleted` đã gọi đúng tên mối nguy
này và nói nó "không thể để làm sau". `GiuDeHoanTat` sinh ra để chặn nó —
nhưng chỉ chặn được cuộc ĐUA ĐỒNG THỜI, không chặn được hoàn tất HỎNG GIỮA
CHỪNG. Chú thích của chính cổng ấy ghi giả định sai: *"tiến trình chết
giữa chừng thì phiên hết hạn bình thường, không có trạng thái nào kẹt
lại"* — đúng nếu chết TRƯỚC khi tạo đơn, sai hẳn nếu chết SAU.

**Cách dựng lại** (`internal/app/api_ph8_ranhgioi_test.go`): trigger
PostgreSQL từ chối ghi riêng event `checkout.completed`. Chặn theo LOẠI
chứ không chặn cả bảng là điểm mấu chốt — `PlaceOrder` cũng ghi outbox,
nên chặn cả bảng làm bước 2 hỏng trước và khe hở không bao giờ mở ra. Lần
chạy đầu đã hỏng đúng kiểu đó và suýt kết luận "không có khe hở".

**Bản sửa:** ghi mã đơn lên phiên NGAY sau bước 2 (`GhiNhanDaTaoDon`), và
`FindExpired` loại phiên đã có mã đơn. Sự kiện "đơn đã tồn tại" trở thành
sự thật bền vững mà job dọn đọc được, thay vì một biến trong bộ nhớ của
tiến trình có thể chết bất cứ lúc nào.

**Cái giá, và vì sao chấp nhận:** phiên đã ghi mã đơn không bao giờ bị dọn
tự động nữa; chuỗi hoàn tất không chạy xong thì hàng nằm giữ tới khi có
người đối soát. Thà HÀNG CHẾT còn hơn HÀNG MA — cùng hướng với lựa chọn ở
kiểm định hàng hoàn. Hàng chết đếm được, tìm được, sửa được; hàng ma bán
hai lần cho hai người rồi mới lộ.

Đánh đổi đó chỉ đứng vững nếu hàng chết ĐẾM ĐƯỢC, nên kèm
`CountHoanTatKetLai` (chỉ báo riêng, log lỗi lúc khởi động) và sửa
`CountExpiredPending` dùng CÙNG điều kiện với `FindExpired` — lệch nhau thì
cảnh báo "job dọn đã chết" kêu sai mãi, và cảnh báo kêu sai vài lần là
cảnh báo không ai đọc nữa.


### 2.16 PH-6 — ba tầng xử lý xung đột `[XONG 03/09]`

Chín module dùng khóa lạc quan. Câu hỏi "chuẩn hóa" không phải "tất cả
phải retry" — mà là **mỗi tầng có đúng một cơ chế, và không tầng nào rơi
vào khoảng trống**.

| Tầng | Cơ chế | Ví dụ |
|---|---|---|
| Trong tiến trình, xung đột DÀY | `withRetry` + chờ ngẫu nhiên tăng dần | inventory, promotion |
| Đường HTTP, xung đột THƯA | trả **409**, người gọi tải lại rồi thử lại | order, customer, fulfillment, returns |
| Đường worker | outbox giao lại event | payment (ghi đợt đối soát) |

**Lỗ hổng tìm thấy:** `apierror.From` biến mọi lỗi chưa ánh xạ thành
`INTERNAL_ERROR` (500). Ba module — `customer`, `fulfillment`, `inventory`
— không ánh xạ `ErrVersionConflict` ở tầng HTTP, nên xung đột phiên bản ra
**500**. Sai theo hai hướng cùng lúc: người gọi tưởng hệ thống hỏng nên
không thử lại (trong khi thử lại mới là việc đúng), và giám sát kêu báo
động cho một tình huống bình thường dưới tải.

Đáng chú ý ở `fulfillment`: cột version của nó thêm vào **chính vì PH-34**
(hủy đè lên giao hàng), tức xung đột ở đó là chuyện đã biết chắc sẽ xảy ra
— và nhà bán thua cuộc đua lúc bấm "Đã giao" nhận 500.

`payment` được MIỄN TRỪ có ghi lý do: xung đột của nó chỉ đến từ đường
worker, thêm ánh xạ HTTP sẽ là mã chết.

**Cưỡng chế bằng test** (`internal/app/api_xungdot_version_test.go`): quét
mã nguồn, mọi module vừa có khóa lạc quan vừa có route GHI thì phải ánh xạ
`ErrVersionConflict`; kèm một bài đo hành vi thật (8 request `PATCH
/api/v1/me` song song) khẳng định không bao giờ ra 5xx.

**Hai lần bài test tự nó xanh rỗng, và cách phát hiện:**

1. `strings.Contains(s, "ErrVersionConflict = errors.New")` — gofmt CĂN CỘT
   trong khối var nên chuỗi thật có nhiều dấu cách. Bộ lọc bỏ qua 4/5
   module, test xanh trong khi phủ gần như không có gì. Chỉ lộ ra khi phá
   `customer` mà test vẫn xanh.
2. Bài đo hành vi dùng helper song song sẵn có, nhưng helper đó không nhận
   header nên không mang token: cả 8 request trả **401**, không cái nào
   chạm tới handler.

Cả hai giờ có hàng rào: test IN RA số module đã kiểm và fail nếu < 3; bài
hành vi khẳng định có ít nhất một request qua được xác thực.


### 2.17 `AuthContext.SellerIDs[0]` — quyết định HOÃN `[04/09]`

Năm module lấy cứng phần tử đầu: `payment` · `marketplace` · `fulfillment`
· `inventory` · `returns`.

**Hệ quả:** một tài khoản được cấp quyền ở HAI gian hàng chỉ tác động được
lên gian hàng được cấp SỚM NHẤT (`ORDER BY granted_at`), và không có cách
nào chọn gian hàng còn lại. Không có lỗi nào báo — mọi request đều thành
công, chỉ là lên nhầm gian hàng.

**Đã quyết (04/09): GHI LẠI, CHƯA SỬA.** Dữ liệu hiện tại không có tài
khoản nào thuộc hai gian hàng; grant rác duy nhất đã dọn khi làm PH-17.

**Rủi ro đã chấp nhận:** lỗi vẫn nằm đó và sẽ nổ đúng lúc có người dùng
thật đầu tiên — tức lúc một nhà bán mở gian hàng thứ hai, và họ sẽ thấy
thao tác của mình rơi vào gian hàng cũ.

**Khi làm, hướng đã cân nhắc:** header `X-Seller-Id` nêu rõ gian hàng đang
thao tác; server kiểm nó nằm trong `SellerIDs`; thiếu header mà chỉ có một
gian hàng thì dùng luôn, có từ hai thì trả 400. Đó là thay đổi hợp đồng
API nên cần quyết có ý thức, không làm kèm.


### 2.18 Tràn số trong phép chia tiền `[XONG 04/09]`

`money.Allocate` nhân `amount * r` bằng int64. Chia **20 tỷ** theo tỷ lệ
19 tỷ : 1 tỷ cho tích 3,8e20 — vượt trần int64 (9,22e18).

**Kết quả: 9,78 tỷ / 10,22 tỷ thay vì 19 tỷ / 1 tỷ.** Gần 50/50 thay vì
95/5.

**Vì sao không ai thấy:** TỔNG vẫn đúng 20 tỷ, vì phần dư được rải bù. Bài
test hiển nhiên nhất cho phép chia tiền — "tổng các phần bằng tổng ban
đầu" — VẪN XANH trong khi tiền chia sai gần gấp đôi.

**Hậu quả thật:** `promotion.AllocateToLines` chia giảm giá xuống từng dòng
hàng, và con số đó là căn cứ hoàn tiền khi khách trả MỘT món. Chia sai
nghĩa là hoàn sai — đúng mối nguy mà module returns đã dựng hàng rào
`ErrGiamGiaChuaPhanBo` để tránh, nhưng hàng rào đó chỉ chặn trường hợp
KHÔNG phân bổ, không chặn phân bổ SAI.

**Ở tỷ lệ khác thì tệ hơn:** tràn làm `share` ÂM, `allocated` âm khổng lồ,
phần dư thành ~9e18, và vòng rải phần dư chạy tới chừng ấy lần — tiến
trình TREO. Một request treo giữ luôn kết nối database của nó, nên vài
request như vậy đủ cạn pool và kéo sập API.

**Sửa:** nhân 128 bit (`bits.Mul64` + `bits.Div64`) rồi mới chia. Thương
chắc chắn vừa 64 bit vì `r <= totalRatio`. Kèm kiểm tràn khi CỘNG tổng tỷ
lệ — mười dòng mỗi dòng 2 tỷ là đủ vượt trần, và sau khi tràn thì
`totalRatio` âm.

Tìm ra bằng cách rà từng lớp lỗi sau khi bộ test đã xanh — không bài test
nào của dự án chạm tới vùng số này.


### 2.19 Số lượng vượt sức chứa cột trả 500 `[XONG 04/09]`

Cột `quantity_*` là `INT` của PostgreSQL — 32 bit, trần 2.147.483.647. Go
dùng `int` 64 bit trên máy chủ thật, nên giá trị lớn hơn **đi qua hết mọi
kiểm tra ở tầng ứng dụng** rồi mới hỏng ở câu lệnh ghi.

Người kiểm kê gõ thừa vài số 0 nhận về "Đã có lỗi xảy ra" (500) thay vì
một lời nhắc sửa. Họ đi báo sự cố, còn giám sát đếm nó vào tỷ lệ lỗi máy
chủ — che mất lỗi thật.

Cùng lớp với lỗi `limit` không có trần ở mục trước: **ràng buộc tồn tại ở
tầng dữ liệu nhưng không được nói ra ở tầng nhận đầu vào.**

Hàng rào đặt ở DOMAIN (`Quantities`), không ở handler: mọi đường ghi —
quản trị viên kiểm kê, nhà bán kiểm kê, và đường thêm sau — đều đi qua đó.
Đặt ở handler thì đường thứ hai phải nhớ tự chặn.

**Cập nhật 04/09:** đã tách thành HAI trần theo quyết định của người dùng.

```text
MaxLuuTru       2.147.483.647   sự thật về cột INT, không sửa được
trần nghiệp vụ     10.000.000   mặc định, SỬA ĐƯỢC từ giao diện quản trị
```

Trần nghiệp vụ thấp hơn hẳn có chủ ý: 10 triệu đơn vị cho một SKU tại một
kho là con số không kho thời trang nào chạm tới, nên vượt nó gần như chắc
chắn là gõ thừa số 0 — bắt ngay lúc nhập rẻ hơn nhiều so với để một con số
vô lý nằm trong kho và làm sai mọi báo cáo tồn.

Nâng được khi có mặt hàng đếm bằng đơn vị nhỏ (chỉ, cúc, hạt cườm), nhưng
KHÔNG bao giờ vượt được trần lưu trữ — cưỡng chế ở hai nơi độc lập.


### 2.20 Mã tiền tệ chữ thường làm giá hiện SAI 100 LẦN `[XONG 04/09]`

Chuỗi lỗi đi thông suốt từ backend tới màn hình khách:

```text
money.Currency.valid()   chỉ kiểm len == 3  →  "vnd" hợp lệ
cột currency             CHAR(3), không CHECK chữ hoa  →  lưu được
giao diện                currency === "VND"  →  "vnd" KHÔNG khớp
                         →  chia số tiền cho 100
```

**250.000 ₫ hiện thành 2.500 ₫** — và vẫn kèm ký hiệu ₫ nên trông hoàn
toàn hợp lệ. Không có lỗi nào, không có cảnh báo nào.

Mã hỏng hơn (`""`, `"€$¥"`) làm `Intl.NumberFormat` NÉM RangeError, và một
lỗi ném lúc render làm TRẮNG cả trang thay vì hiện một ô sai.

**Sửa hai đầu, có chủ ý:**

- gốc: `money.New` chuẩn hóa chữ hoa (che cả đường ĐỌC LẠI, vì kho lưu trữ
  dựng Money bằng hàm này), và `valid()` đòi đúng ba chữ cái A–Z
- giao diện: so sánh sau khi chuẩn hóa, và bọc `try/catch` — giao diện
  không được hiện sai GIÁ dù nhận vào cái gì

Ba app có ba bản `money()` riêng; bài test chạy CẢ BA qua cùng bộ dữ liệu.
Sửa một bản mà quên hai bản kia là lỗi rất dễ mắc và không lộ ra ở đâu
khác — phá để kiểm xác nhận: phá storefront thì admin và seller vẫn xanh.

### 2.21 `npm test` chạy KHÔNG bài nào `[XONG 04/09]`

Script gốc là `npm run test --workspaces --if-present`, mà KHÔNG workspace
nào có script `test`. Lệnh chạy xong, thoát 0, không chạy bài nào.

Mười bài đơn vị có thật nhưng chỉ tới được bằng `npx playwright test
--project=unit`. Ai chạy lệnh chuẩn — người mới, hay CI sau này — nhận
xanh với 0 bài.

Sửa: nối thẳng `playwright test --project=unit` vào script `test`. Phần
e2e vẫn tách riêng vì nó cần storefront chạy ở cổng 3001.

Kiểm chứng bằng cách thêm một bài cố tình hỏng: `npm test` thoát 1.


### 2.22 Trang thanh toán tự mâu thuẫn về thời gian `[XONG 05/09]`

Hai chỗ cùng suy ra một sự thật, bằng hai phép tính riêng:

```text
trang    new Date(expires_at).getTime() - now  →  msLeft <= 0
format   countdown(expires_at)                 →  "00:00"
```

`expires_at` hỏng hoặc thiếu làm `msLeft` thành **NaN**, mà `NaN <= 0` là
**false** — nên trang coi phiên CHƯA hết hạn và mở hết nút, trong khi đồng
hồ hiện **"00:00"**.

Khách đọc "Hàng được giữ cho bạn trong 00:00" rồi vẫn bấm tiếp được, và
không biết tin cái nào.

**Sửa ở gốc, không vá triệu chứng:** thêm `msConLai()` trả `number | null`
làm nguồn DUY NHẤT; cả đồng hồ, cờ hết hạn và lớp "sắp hết giờ" cùng đọc
nó. Hai chỗ tự tính riêng thì chúng bất đồng được — và đã bất đồng.

`null` KHÁC 0 có chủ ý: không biết thì bên gọi tự quyết, chứ không bị ép
coi là đã hết hạn. Trả 0 sẽ KHÓA hết nút vì một mốc thời gian không đọc
được, chứ không phải vì phiên thật sự hết.

### 2.23 Quy tắc đếm món trong giỏ chưa có gì ghim `[XONG 05/09]`

"Đếm DÒNG chứ không cộng số lượng" là quyết định sản phẩm — `3` nghĩa là
ba món KHÁC nhau, không phải ba cái cùng một áo.

Nó chỉ nằm trong một chú thích. Một lần "sửa cho đúng hơn" là đủ để nó
biến mất, và không gì báo.

Đã ghim bằng test, kèm hàng rào cho giỏ rỗng: badge nằm ở thanh điều hướng
nên hiện trên MỌI trang — một lỗi ném ở đó làm trắng toàn bộ ứng dụng, chứ
không riêng trang giỏ hàng.


### 2.24 Giỏ hàng: phản hồi cũ ghi đè phản hồi mới `[XONG 05/09]`

`ShopProvider.run()` gọi `setCart(await fn())` không có gì bảo đảm thứ tự.

Kịch bản rất thường: trang vừa tải nên `GET /cart` chạy chậm; khách bấm
"Thêm vào giỏ"; lời gọi thêm xong TRƯỚC; rồi `GET /cart` cũ về sau và ghi
đè bằng giỏ CHƯA có món vừa thêm.

Khách thấy món mình vừa thêm biến mất dù máy chủ đã ghi nhận. Họ bấm thêm
lần nữa — và giờ có **hai** món trong giỏ.

Đúng hình dạng lỗi "đọc-rồi-ghi" ở backend, chỉ khác là cuộc đua nằm giữa
hai phản hồi MẠNG chứ không phải hai giao dịch database.

**Sửa:** `ThuTuLuot` — chỉ nhận kết quả của lượt mới nhất. Tách khỏi
component có chủ ý: quy tắc là logic thuần, kiểm được không cần dựng cây
React.

#### Khoảng trống còn lại, và vì sao

Phá để kiểm cho thấy: gỡ quy tắc khỏi `run()` thì **không test đơn vị nào
đỏ** — đơn vị được kiểm, nhưng VIỆC DÙNG nó thì không.

Lấp đúng khoảng đó cần render `ShopProvider` bằng React. Đã thử: thêm
`@testing-library/react` + `jsdom` và dựng môi trường DOM, nhưng bộ chạy
Playwright BIẾN ĐỔI JSX trong `.tsx` sang định dạng component-testing của
nó, nên `ShopProvider` trả về một object `{__pw_type, …}` chứ không phải
React element. Hỏng ngay ở lời gọi `render` đầu tiên.

Đã gỡ ba phụ thuộc đó ra — giữ lại mà không dùng được thì chỉ là nợ.

Bù tạm bằng một bài QUÉT MÃ NGUỒN: `run()` phải có hai chỗ bỏ lượt cũ (một
cho đường thành công, một cho đường lỗi). Yếu hơn hẳn — nó thấy đoạn mã có
mặt, không thấy nó chạy đúng — nhưng bắt được kiểu hỏng thực tế nhất: ai
đó dọn `run()` và bỏ mất hai dòng kiểm.

**Cần quyết:** muốn kiểm HÀNH VI của component thì phải chọn một bộ chạy
render được React — `vitest + jsdom`, hoặc Playwright component testing.
Cả hai đều đi ngược ghi chú trong `playwright.config.ts` ("một bộ chạy ít
hơn là một cấu hình ít hơn để lệch nhau"), nên đó là quyết định có ý thức,
không phải việc làm kèm.


### 2.9 Audit authorization (PH-9, PH-10)

Đã có, kiểm ở tầng domain/application chứ không phải ở HTTP:

```text
✅ Order.ViewableBy        đơn ĐÃ CÓ CHỦ thì chỉ chủ mở được
✅ Address.BelongsTo       địa chỉ của khách khác không đọc được
✅ Wishlist.BelongsTo
✅ FulfillmentOrder.BelongsTo   + WHERE seller_id ở SQL (phòng vệ hai lớp)
✅ marketplace.OwnedOffer  trả ErrNotFound khi không sở hữu, không phải 403
✅ inventory.FindOwnedItem lọc theo chủ sở hữu
```

Còn phải rà:

```text
✅ một bảng liệt kê MỌI resource × MỌI vai trò `[XONG 30/08]`
   `internal/app/api_ma_tran_quyen_test.go` — 175 cặp (đường × vai trò).
   Bài sẵn có chỉ thử MỘT vai trò sai (CUSTOMER) nên bắt được "quên bọc
   RequireRole" mà KHÔNG bắt được "bọc NHẦM vai trò" — lỗi nguy hiểm hơn:
   route sổ cái bọc `OPS_MERCHANDISING` vẫn chặn khách, vẫn xanh mọi test,
   trong khi nhân viên hàng hóa đọc được sổ cái. Kèm bài phủ bắt buộc mọi
   đường mới phải có nhóm trong ma trận.
✅ rate limit cho ĐĂNG NHẬP `[XONG 30/08]`
   `httpserver.RateLimitThatBai` + `internal/app/api_gioihan_dangnhap_test.go`.
   Rà lại mới thấy mô hình đe dọa ban đầu ghi sai: khóa theo TÀI KHOẢN
   (`identity.MaxFailedAttempts` = 5, khóa 15 phút) ĐÃ chặn việc dò mật khẩu
   một tài khoản. Lỗ thật còn hở là RẢI MẬT KHẨU — một mật khẩu phổ biến thử
   lên hàng nghìn email, mỗi email sai đúng một lần nên không tài khoản nào
   bị khóa. Chỉ nhìn từ phía đường mạng mới thấy.
   Chỉ đếm lượt THẤT BẠI: đếm mọi lượt thì một văn phòng sau NAT dùng chung
   tự khóa chính mình, và hỏng kiểu đó không ai thấy — log sạch, kẻ tấn công
   vẫn bị chặn, chỉ khách thật lặng lẽ bỏ đi.
   Chưa làm: TRA ĐƠN VÃNG LAI — endpoint này chưa tồn tại (`GET
   /api/v1/orders/{id}` nằm sau xác thực). Khi nào có thì phải kèm giới hạn.
⬜ bộ đếm rate limit nằm trong bộ nhớ (P3-16) — N bản sao = N lần hạn mức
✅ test khẳng định response công khai KHÔNG chứa dữ liệu nội bộ `[XONG 30/08]`
   `internal/app/api_ro_ri_test.go` — HAI lớp độc lập trên 8 đường công khai:
   danh sách ĐEN tên trường (duyệt đệ quy, bắt cả object lồng), và MỒI theo
   GIÁ TRỊ (nạp chuỗi độc nhất vào cột nội bộ rồi tìm trong phản hồi).
   Lớp hai kiếm được chỗ đứng: phá bằng cách trả `legal_name` dưới tên
   `display_ref` thì danh sách đen XANH, chỉ mồi bắt được.
```

Nguyên tắc đã áp và cần giữ: **không suy quyền từ id hay số điện thoại**.
`Order.ViewableBy` từng sai đúng chỗ này — đơn của khách đã đăng ký mở
được bằng số điện thoại.

### 2.10 Chuỗi hợp đồng API (PH-12)

`npm run types:check` sinh lại TypeScript rồi `git diff --exit-code`, nên
đặc tả và kiểu luôn khớp. Nhưng nó KHÔNG bắt được đặc tả lệch với Go —
lớp lỗi đã xảy ra bốn lần và mỗi lần một cách:

```text
availability   đặc tả khai, Go không trả  → nút mua khóa vĩnh viễn
seller/hex/size đặc tả khai object, Go trả chuỗi → chặn lúc biên dịch
updateInventory đặc tả PATCH, Go đăng ký PUT     → client sinh ra ăn 405
updated_at     đặc tả khai, Go không trả  → không ai phát hiện
```

Ba trong bốn lọt qua TypeScript vì trường không `required`.

**Đã có bước kiểm đặc tả ↔ response THẬT** (`api_contract_test.go`), và nó
tìm ra ngay lỗi thứ NĂM — lỗi nặng nhất trong nhóm:

```text
price_from   đặc tả khai BẮT BUỘC trên ProductSummary
             API chưa bao giờ trả
             → cửa hàng hiện dấu gạch thay cho giá, nhiều tuần
```

Lần này TypeScript không những không bắt được mà còn CHE ĐI: vì đặc tả khai
`required`, kiểu sinh ra là non-optional, nên `money(p.price_from)` biên
dịch sạch. Trang render `—` và không ai thấy gì bất thường.

**Cách sửa là một quyết định thiết kế, không phải thêm trường.** Giá thuộc
về OFFER, và `product` cùng tầng với `marketplace` nên không gọi được.
Nhồi giá vào danh mục sẽ bắt mọi lời gọi sản phẩm kéo theo truy vấn giá,
kể cả trang quản trị nơi không hiển thị giá bán.

Thêm `GET /api/v1/offers/buy-box?product_ids=` — tra theo LÔ, lấy từ BUY
BOX nên là giá khách THẬT SỰ mua được (offer hết hàng và nhà bán bị đình
chỉ đã bị loại). Sản phẩm không có offer bán được thì VẮNG MẶT, không trả
giá 0 vì 0 hiển thị ra là "miễn phí".

`price_from` và `compare_at_price` bị GỠ HẲN khỏi `ProductSummary` chứ
không chỉ bỏ `required`: để nguyên dưới dạng tùy chọn là giữ lại lời hứa
mà API không giữ, chỉ nhỏ hơn. Gỡ xong TypeScript báo đúng ba chỗ dùng.

### 2.12 PH — Hiệu năng và đồng thời (SAU khi E2E ổn định)

Xếp sau có chủ ý: đo hiệu năng của một hệ thống chưa chứng minh được tính
đúng là đo sai thứ. Chi tiết chiến lược ở [scale.md](scale.md).

| # | Việc | Trạng thái |
|---|---|---|
| PH-15 | Load test đường thanh toán | ✅ tìm ra lỗi thật: 9% lượt mua bị từ chối khi kho đầy — xem do-tai.md mục 2 |
| PH-16 | Giữ hàng đồng thời · đặt đơn đồng thời | ✅ 1000 đơn/200 luồng, 0 lỗi — do-tai.md mục 2 |
| PH-17 | Nhà bán cập nhật tồn kho đồng thời | ✅ nhà bán KHÔNG phải nguyên nhân; trần 1 dòng ~1000 lượt/giây — do-tai.md mục 4 |
| PH-18 | Tranh chấp database · connection pool | ✅ đo được: pool cạn liên tục nhưng 0 lượt bỏ cuộc — do-tai.md mục 3 |
| PH-19 | Thông lượng outbox · số worker chạy song song · bão thử lại | ✅ 20 → 2619 event/giây; phục hồi 7,5 phút → 8 giây — do-tai.md mục 5 |

**Bất biến phải chứng minh được, không phải mong đợi:** `[XONG 06/09]`

```text
available >= 0   luôn đúng
KHÔNG oversell   dưới thanh toán đồng thời
```

Nay có ĐỦ hai nửa:

| Bài | Chứng minh tới đâu |
|---|---|
| `TestMuoiKhachTranhNamSanPhamThiDungNamNguoiGiuDuoc` | `Reserve` an toàn với hai giao dịch PostgreSQL song song |
| `TestMuoiKhachTranhBaMonKhongAiMuaQua` | `StartCheckout` an toàn — đúng 3/10 khách giữ được |
| **`TestHoanTatDongThoiKhongSinhHangTuKhongKhi`** | **cả chuỗi**: hoàn tất → đơn → outbox → Reserved→Committed |
| **`TestVetOutboxHaiLanKhongTruKhoHaiLan`** | phát lại event KHÔNG trừ kho lần hai |

**Bất biến kiểm ở hai bài mới là BẢO TOÀN, không chỉ "không âm":**

```text
available + reserved + committed  ==  số hàng đã nhập
```

Mạnh hơn hẳn `available >= 0`. Kho âm là lỗi tự lộ ra; hàng BỐC HƠI hay
SINH THÊM mà ba con số vẫn dương thì không có gì báo, và nó chỉ hiện ra ở
lần kiểm kê thật — nhiều tuần sau.

**Ba lần phá, và một lần bị NUỐT — đáng ghi:**

```text
commit HAI LẦN mỗi reservation   → VẪN XANH
bỏ hẳn bước commit               → đỏ: "đã cam kết 0, cần 3"
commit mà KHÔNG trừ reserved     → đỏ: "= 6, chỉ nhập 3 — SINH RA TỪ KHÔNG KHÍ"
```

Dòng đầu không phải lỗ hổng của bài test: lần commit thứ hai trả
`ErrReservationNotActive` và handler coi đó là kết quả MONG MUỐN của một
event phát lại — chỗ đó đã idempotent sẵn, nên phá nó không tạo ra hành vi
sai để mà bắt. Ghi lại vì nó dễ dẫn tới kết luận sai theo cả hai chiều.

Đây là kịch bản **"thanh toán đồng thời"** trong điều kiện kết thúc phase
(mục 2).

### 2.13 PH — Observability (SAU khi E2E ổn định)

Thiết kế đã có ở [observability.md](../09-operations/observability.md);
đây là phần TRIỂN KHAI.

| # | Việc | Trạng thái |
|---|---|---|
| PH-20 | Log có cấu trúc | ✅ `slog` + `platform/logger` |
| PH-21 | Request ID | ✅ có ở mọi request và mọi lỗi |
| PH-22 | **Correlation ID xuyên tiến trình** | ✅ mặc định ở outbox + kế thừa qua bên nhận |
| PH-23 | **Metrics** | ✅ Prometheus, `/metrics` ở cả API và worker |
| PH-24 | Độ trễ · tỷ lệ lỗi | ✅ histogram theo mẫu route và mã trạng thái |
| PH-25 | Metrics database (pool, thời gian truy vấn) | ✅ pool + thời gian truy vấn, 5 cảnh báo |
| PH-26 | Metrics outbox (tồn đọng, độ trễ, số lần thử lại) | ✅ 3 gauge + đếm thất bại theo bên nhận |
| PH-30 | **Worker chết thì chỉ số outbox đứng yên** | ✅ nhịp tim + độ trễ job + đếm lỗi job |
| PH-27 | Đếm thất bại nghiệp vụ | ✅ payment và fulfillment đã nối, có test đọc registry thật |

**PH-26 và PH-27 quan trọng hơn vẻ ngoài của chúng.** Outbox tồn đọng là
triệu chứng SỚM của gần như mọi sự cố ở kiến trúc này: worker chết, event
kẹt, tồn kho không chuyển Reserved → Committed, và tiến trình dọn có thể
nhả hàng của một đơn đã thanh toán. Hiện không có gì báo động chuyện đó.

### 2.14 P0 cũ — đã xong, giữ lại làm lịch sử

| # | Việc | Trạng thái |
|---|---|---|
| P0-1 | Middleware xác thực (`Auth`, `RequireRole`) | ✅ xong |
| P0-2 | Middleware `RequireIdempotencyKey` | ✅ xong |
| P0-3 | Phát hành access token (`platform/token`) | ✅ xong |
| P0-4 | Endpoint đăng nhập/làm mới/đăng xuất + `/admin/me` | ✅ xong |
| P0-5 | Audit log (`platform/audit`) + endpoint đọc | ✅ xong |
| P0-6 | **Ranh giới giao dịch cho thao tác ghi + audit** | ✅ xong |
| P0-7 | Tách wiring khỏi `cmd/api/main.go` | ✅ xong (26/08) — sang `internal/app`, KHÔNG phải `platform/bootstrap` |

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

### P0-7 — tách wiring: ĐÃ LÀM (26/08), và đích đến KHÁC bản ghi ban đầu

Chỗ vướng thật đã xuất hiện: PH-3 cần test đi qua HTTP, mà bộ route nằm
trong `package main` nên không có cách nào dựng lại trong test.

**Đích đến ghi ban đầu là `internal/platform/bootstrap`. KHÔNG dùng được:**
quy tắc R3 của archcheck cấm mọi thứ dưới `platform/` import module nghiệp
vụ, và một gốc lắp ghép thì buộc phải import tất cả. Đó là ràng buộc đúng
chứ không phải chỗ cần lách — platform là tầng nền dùng chung; nó biết tới
`order` hay `seller` là thôi trung lập.

Gốc lắp ghép thuộc tầng TRÊN module, nên đích đúng là **`internal/app`**:
không phải platform, không phải module, archcheck không áp ràng buộc nào —
đúng như một gốc lắp ghép cần.

```text
cmd/api/main.go   760 → 87 dòng
internal/app      Build() dựng module · RegisterRoutes() nối route
                  + shopper.go, stock.go (adapter cầu nối) chuyển sang
```

main giờ chỉ còn: đọc cấu hình, mở log, mở database, bắt tín hiệu dừng,
chạy server. Ba ràng buộc của P0-7 giữ nguyên — không DI framework, không
trừu tượng hóa thừa, vẫn `New(Config)` tường minh.

Kiểm chứng trên server thật sau khi tách: mọi lớp đường dẫn hành xử y hệt
— công khai 200, admin có token 200, admin không token 401, thiếu
`Idempotency-Key` 400.

### Bản ghi gốc — giữ lại để đối chiếu

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

1. **Phí vận chuyển** — ĐÃ SỬA phần số nguồn (P3-8, 06/09): phí nay đến từ
   `fulfillment.EstimateShipping()` và tính theo TỪNG nguồn hàng. Vẫn chưa
   theo khoảng cách hay khối lượng, vì cả hai thiếu DỮ LIỆU (kho không có
   địa chỉ, dòng checkout không mang gram) — xem P3-8.
2. **`payment_method` được kiểm tra rồi BỎ QUA** — ĐÃ SỬA (P3-9 + PH-36,
   06/09). Đơn lưu lựa chọn của khách, và đơn TRẢ TRƯỚC nay có
   `payment_intent` để webhook thanh toán đối chiếu vào; thu tiền thành
   công thì đơn chuyển `PAID`. Còn thiếu adapter PSP thật và job đối chiếu
   định kỳ — xem PH-36.
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
3. **Lọc `status` và phân trang đều đã vào truy vấn** — xem P3-11, P3-12.

### P1.5 — Seller Center (11/11 xong) `[XONG 04/09]`

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

### P1.5 — Seller Center: XONG cả 11 operation `[04/09]`

Bảng cũ ghi "8 operation còn lại" nhưng đã lỗi thời: rà theo route thật
thì 10/11 đã có. Chỉ còn `getMyPerformance`, làm nốt 04/09.

**Ràng buộc bắt buộc:** mọi truy vấn giới hạn theo `AuthContext.SellerIDs`.
Seller không được thấy dữ liệu seller khác dù biết định danh.

#### `getMyPerformance` — đo được HAI trong năm chỉ số, và nói thẳng ra

Đặc tả khai năm chỉ số. Rà dữ liệu thật thì chỉ hai chỉ số tính được:

| Chỉ số | Trạng thái |
|---|---|
| `cancellation_rate` | ✅ từ `fulfillment_order.status` |
| `on_time_shipping_rate` | ✅ từ `shipped_at` vs `created_at` + SLA |
| `return_rate_description` | ❌ cần dữ liệu module returns (tầng khác) |
| `average_rating` | ❌ hệ thống KHÔNG có bảng đánh giá nào |
| `inventory_accuracy` | ❌ chưa thống nhất công thức tính lệch |
| `impact.buy_box_win_rate` | ❌ buy box tính tại chỗ, không lưu lịch sử |

Ba chỉ số thiếu KHÔNG bị bỏ im lặng: chúng nằm trong trường `not_measured`
của response, kèm lý do. Đặc tả nói rõ mục đích của endpoint là tránh "mô
hình chấm điểm hộp đen"; trả hai chỉ số rồi im lặng về ba chỉ số còn lại
tạo ra đúng thứ hộp đen đó, chỉ khác là ở phía người viết API.

`buy_box_win_rate` là chỗ quan trọng nhất phải cưỡng lại: đặc tả đặt nó ở
mục "tác động", nên một con số ước lượng ở đó sẽ dẫn tới quyết định kinh
doanh sai. Có test khẳng định nó KHÔNG xuất hiện trong `impact`.

Hai thứ thêm vào response mà đặc tả chưa có, vì thiếu chúng thì con số
không kiểm chứng được — tức vẫn là hộp đen:

  `sample_size`         tỷ lệ 5% là của bao nhiêu đơn?
  `shipping_sla_hours`  "đúng hạn" đo theo mốc nào?

Mẫu dưới 10 đơn thì KHÔNG chấm: gian hàng mới mở có 3 đơn, hủy 1, ra 33%
và bị chấm NGHIÊM TRỌNG — con số đó nói về cỡ mẫu, không nói về chất lượng.

**SLA 48 giờ giờ SỬA ĐƯỢC từ giao diện quản trị** `[04/09]` — không cần
build lại. Xem [ADR-0015](../adr/0015-cau-hinh-van-hanh.md). Con số mặc
định vẫn là 48 và vẫn chưa qua sản phẩm duyệt, nhưng nó không còn khóa
cứng trong mã.

### P1.6 — Admin (8/10 xong)

✅ **Trang duyệt hồ sơ seller**: `listSellers` · `getSellerDetail` ·
`approveSeller` · `suspendSeller`

✅ **Tài chính**: `createLedgerAdjustment` — endpoint nhạy cảm nhất hệ
thống, ba lớp bảo vệ nguyên tử với nhau (cân bằng · lý do · vết kiểm toán).

✅ **Đơn hàng (hỗ trợ khách)**: `listAdminOrders` · `getAdminOrderDetail` ·
`cancelAdminOrder` — xem chi tiết đơn **ghi vết việc ĐỌC**, vì response chứa
tên người nhận, số điện thoại và địa chỉ.

✅ `adjustInventory` `[XONG 30/08]` — `POST /api/v1/admin/inventory/adjustments`,
vai trò ADMIN + OPS_WAREHOUSE (đường đầu tiên OPS_WAREHOUSE dùng tới).

Ba điểm chốt lại trong lúc làm, đều là chỗ dễ hỏng âm thầm:

- **Chỉ đặt `available` và `damaged`.** `reserved`/`committed` là lời hứa
  đã đưa ra cho khách; ghi đè chúng là trả hàng đã bán về khả dụng rồi bán
  lần hai — đúng kiểu "sinh hàng từ không khí" của PH-31.
- **Trường không khai thì giữ nguyên** (con trỏ, không phải int). Dùng int
  thường thì mọi request không nhắc tới số hỏng sẽ lặng lẽ xóa nó về 0.
- **Cặp (sku, kho) KHÔNG đủ định danh** — hàng seller gửi ở kho nền tảng
  vẫn thuộc seller, nên cùng SKU cùng kho có bản ghi riêng cho từng chủ.
  Đặc tả ban đầu thiếu chỗ này. Thêm `inventory_owner_id` không bắt buộc,
  và trả 409 khi nhập nhằng thay vì đoán.

Vết kiểm toán dùng nhật ký BIẾN ĐỘNG (quy tắc 4) chứ không thêm cổng audit
mới cho inventory: movement gắn với chính bản ghi tồn kho và đối soát được
theo SKU, chặt hơn audit log cho việc này. Bản seller cũng dựa vào đúng nó.

✅ `getCustomerAsAdmin` `[XONG 30/08]` — `GET /api/v1/admin/customers/{id}`,
vai trò ADMIN + OPS_SUPPORT (cùng nhóm với đơn hàng: người trả lời khiếu
nại cần biết khách là ai; OPS_FINANCE và OPS_MERCHANDISING thì không).

Ghi vết TRƯỚC khi trả dữ liệu, và ghi vết hỏng thì KHÔNG trả — cùng thứ tự
với `order.GetOrderAsAdmin`. Kiểm bằng cách đổi tên bảng `audit_log` rồi
gọi endpoint: nếu vẫn trả hồ sơ thì câu "mọi lần gọi đều ghi audit log"
trong đặc tả là nói dối.

Lệch hợp đồng phát hiện trong lúc làm: đặc tả ghi `X-Access-Reason` tối
thiểu 10 ký tự, `audit.ValidateReason` cưỡng chế 20. API sẽ từ chối lý do
15 ký tự trong khi tài liệu hứa 10 là đủ — không làm hỏng test nào, chỉ lộ
ra khi người tích hợp gặp 400 không giải thích được. Sửa đặc tả theo giá
trị đang cưỡng chế (không nới lỏng kiểm soát sẵn có) và thêm bài đo ở ranh
giới 19/20 để hai bên không lệch lại.

Số đo cơ thể: đặc tả yêu cầu không trả nếu không có quyền đặc biệt. Module
customer CHƯA lưu số đo, nên yêu cầu này thoả mãn từ cấu trúc chứ không nhờ
câu lệnh lọc nào. Khi thêm số đo, đây là chỗ phải xem lại.

**Admin: 10/10 operation.** (`executePayouts` đã chuyển sang FUTURE.)

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

### P1.7 — Webhook (2/2 operation) — ✅ XONG

`receiveShippingWebhook` `[XONG 27/08]` · `receivePaymentWebhook`
`[XONG 06/09]`

Xác minh chữ ký HMAC là bắt buộc — không có bước này thì bất kỳ ai cũng
gửi được "thanh toán thành công" giả.

Nhưng với webhook THANH TOÁN, chữ ký một mình vẫn chưa đủ: nó chứng minh
NGUỒN, không chứng minh NỘI DUNG. Lớp thứ ba — đối chiếu số tiền với
`payment_intent` — là thứ chặn một con số sai đi vào sổ cái bất biến. Xem
PH-36 và [ADR-0017](../adr/0017-payment-intent.md).

**Yêu cầu 5 của `webhooks.yaml` (đối chiếu định kỳ) CHƯA đạt ở CẢ HAI
webhook.** Không có nó thì một webhook mất để đơn treo vĩnh viễn.

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
[`httpserver.RateLimit`](../../gouse/internal/platform/httpserver/ratelimit.go),
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

**Trạng thái 27/08/2026:** cả 7 luồng đã chạy được **qua HTTP**, có test
tích hợp trên chuỗi middleware thật và E2E trình duyệt chứng minh (xem mục
2.4b và 2.6). Luồng 7 chạy trên hệ thống thật, không chỉ trong test: 240
tín hiệu nhu cầu trong database sinh từ hoạt động HTTP qua outbox.

Câu trước ở đây — "chưa luồng nào chạy được qua HTTP" — đúng vào 15/08 và
đã sai. Việc còn lại KHÔNG phải là làm cho luồng chạy, mà là **nối đường
tiền**: xem PH-33 ở mục 2 (P0).

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
| P3-1 | **Test HTTP cho auth và audit-log** | ✅ xong — dòng ghi chú cũ đã lạc hậu; xem ghi chú dưới bảng |
| P3-2 | **Sửa test suite chập chờn** | ✅ xong — xem ghi chú dưới bảng |
| P3-3 | E2E: Product → Offer → Cart → Checkout → Order → Payment → Fulfillment | Sau P1 |
| P3-4 | Rate limit (`429` + `X-RateLimit-*`) | ✅ xong — xem ghi chú dưới bảng |
| P3-5 | 2FA cho `ADMIN` và `OPS_FINANCE` | Tăng cường SAU phát hành — chủ dự án đã gỡ khỏi điều kiện chặn (15/08) |
| P3-6 | Observability: metrics, tracing | |
| P3-7 | Chính sách lưu trữ `audit_log` | Bảng chỉ tăng; chờ có số liệu thật |
| P3-8 | **Phí vận chuyển thật** thay bảng cứng trong checkout | ✅ xong (06/09) — phí theo TỪNG NGUỒN; khoảng cách và khối lượng vẫn thiếu dữ liệu. Xem ghi chú dưới bảng |
| P3-9 | **Nối `payment_method` vào đơn hàng** | ✅ xong (06/09) — đơn GHI lựa chọn, và PH-36 nối nốt đường thu tiền. Xem ghi chú dưới bảng |
| P3-10 | Test cho `cart/lookup.go` (`offerLookup`) | ✅ xong (06/09) — 9 bài, xem ghi chú dưới bảng |
| P3-11 | Lọc `status` của `listMyOrders` trong TRUY VẤN | ✅ xong — xem ghi chú dưới bảng |
| P3-12 | Phân trang theo KHÓA thay vì offset | ✅ xong — xem ghi chú dưới bảng |
| P3-13 | **Dữ liệu mẫu MUA ĐƯỢC**: seed cho offer + tồn kho | ✅ xong — xem ghi chú dưới bảng |
| P3-14 | **Tùy chọn khách hàng** (số đo cơ thể, size ưa thích) | Cần thiết kế lưu trữ MÃ HÓA trước; đặc tả tự yêu cầu điều đó |
| P3-15 | **Xác minh email** → mở đường gộp lịch sử đơn vãng lai | ✅ xong (07/09) — và lộ ra rằng "lịch sử" nằm ở ĐƠN chứ không ở hồ sơ. Xem ghi chú dưới bảng |
| P3-16 | Bộ đếm tần suất DÙNG CHUNG giữa các tiến trình | Bộ đếm hiện nằm trong bộ nhớ; N bản sao = N lần hạn mức |
| P3-17 | SLA cho đơn thực hiện | ✅ xong (07/09) — hạn TÍNH RA, không lưu; và bài test tìm ra một chênh lệch thật. Xem ghi chú dưới bảng |
| P3-18 | **Giữ hàng chọn nhầm CHỦ SỞ HỮU tồn kho** | ✅ xong (19/08) — xem ghi chú dưới bảng |
| P3-19 | **Endpoint công khai tra hồ sơ nhà bán** | ✅ xong (20/08) — `GET /api/v1/sellers?ids=` |
| P3-21 | **Trang sản phẩm chưa cho chọn màu/size** | ✅ xong (20/08) — xem ghi chú dưới bảng |
| P3-22 | `Color` và `Size` là CHUỖI, chưa có mã màu và hệ size | 🟡 nửa `system` xong (07/09); `hex_code` BỊ CHẶN bởi P3-25 |
| P3-23 | Offer không bao giờ tự chuyển `OUT_OF_STOCK` | ✅ xong — bỏ hẳn trạng thái đó; xem ghi chú dưới bảng |
| P3-20 | `ProductDetail.buy_box_offer` trong đặc tả không bao giờ được trả | ✅ xong — xem ghi chú dưới bảng |
| P3-24 | **"Khách mua được không" có BA câu trả lời khác nhau** | ✅ xong (06/09) — xem ghi chú dưới bảng |
| P3-25 | **Toàn bộ phía GHI của product không có endpoint nào** | ⛔ mở — hàng hóa chỉ vào hệ thống được qua seed. Xem ghi chú |
| P3-26 | **Webhook vận chuyển MẤT thì không ai biết** (yêu cầu 5) | ✅ nửa nội bộ xong (07/09) — đo được 20 gói kẹt thật. Xem ghi chú |
| P3-27 | **Chỉ số đối soát tiền KHÔNG AI CANH; và một cảnh báo critical kêu oan vĩnh viễn** | ✅ xong (07/09) — promtool nay chạy trong CI. Xem ghi chú |

**P3-9 — phần GHI NHẬN đã xong (06/09); phần THU TIỀN vẫn chặn.**

Chia đôi vì hai nửa có điều kiện khác nhau. Ghi lại lựa chọn của khách
không cần cổng thanh toán nào; thu tiền thì cần `payment_intent`, tức là
PH-36, và PH-36 nói rõ đó là "việc lớn hơn bản thân webhook".

**Khiếm khuyết đã sửa:** `payment_method` được tầng HTTP kiểm hợp lệ rồi
VỨT ĐI — nó không có chỗ nào để đi tới, vì cả `order` lẫn `payment` đều
không có khái niệm này (`payment` là SỔ CÁI, ADR-0008). Hệ quả: khách chọn
COD và khách chọn BANK_TRANSFER sinh ra hai đơn GIỐNG HỆT nhau trong
database. Không ai trả lời được câu hỏi kho thật sự cần — **đơn này có
phải thu tiền lúc giao không?** — và câu đó KHÔNG suy được từ `status`: cả
hai đều nằm ở `PENDING_PAYMENT`.

**Đặt ở `order`, không phải sổ cái.** Sổ cái ghi tiền ĐÃ chuyển; với COD
thì lúc đặt đơn chưa có đồng nào chuyển, nên không có bút toán để gắn vào.
Đây là điều khoản của thỏa thuận mua bán, không phải một sự kiện tài
chính — và nó ĐÓNG BĂNG như giá và địa chỉ.

**Cột CHO PHÉP NULL, có chủ ý.** Hai đường tạo đơn không giống nhau:
`complete` bắt buộc `payment_method`, còn `placeOrder` (`POST
/api/v1/orders`) chỉ nhận `checkout_id` — đặc tả của nó vốn thế. Nên "chưa
chọn" là trạng thái CÓ THẬT. Đặt `DEFAULT 'COD'` cho gọn sẽ khiến kho đi
thu tiền của một đơn đã trả trước. 3176 đơn cũ cũng NULL, vì lựa chọn của
những khách ấy đã bị vứt đi và không có gì để khôi phục.

**Tìm ra một lỗi CÓ SẴN trong lúc làm, không liên quan tới phương thức
thanh toán.** Nhánh THỬ LẠI của `CompleteCheckout` (phiên đã `COMPLETED`)
trả về `CompleteResult` chỉ có `OrderID` — `OrderNumber` RỖNG. Mà
`order_number` là mã khách đọc qua điện thoại và dùng để tra đơn vãng lai.
Nhánh đó chạy đúng vào lúc client thử lại sau sự cố mạng: **khách gặp mạng
chập chờn thì mất mã đơn của mình, trong khi đơn đã tạo thành công.** Sửa
bằng cách đọc lại đơn (`OrderPort.LayDon`), và đọc hỏng thì TRẢ LỖI chứ
không trả bản thiếu trường — nuốt đi là dựng lại đúng lỗi vừa sửa.

**Phản hồi lấy từ ĐƠN, không dội lại thân request.** Hai cách viết giống
hệt nhau ở đường thường; chỉ đường thử lại phân biệt được. Gọi lại cùng
khóa idempotency với phương thức KHÁC phải nhận về phương thức của đơn ĐÃ
tạo — dội lại thân request sẽ báo "đã ghi nhận BANK_TRANSFER" trong khi
database ghi COD. Có bài test riêng khóa đúng ca này.

**Sửa kèm một ví dụ SAI trong đặc tả.** `completeCheckout` khai ví dụ
`status: PAID` — một trạng thái endpoint này KHÔNG BAO GIỜ trả về. Người
tích hợp đọc ví dụ rồi viết `if (status === 'PAID')` cho luồng thành công,
và nhánh đó không bao giờ chạy. Cùng họ với `price_from` và `buy_box_offer`:
đặc tả hứa thứ API không có.

**Kiểm chứng bằng cách phá:** bỏ dòng chép trường trong `withLines` — đúng
cái bẫy mà chú thích tại chỗ cảnh báo "ĐÃ XẢY RA HAI LẦN" — thì bài test
đỏ ở phần ĐỌC LẠI mà vẫn xanh ở phần phản hồi, tức là bắt đúng lớp lỗi
"ghi được nhưng đọc lên mất".

**Kiểm trên hệ thống THẬT:** ràng buộc CHECK từ chối `'BITCOIN'` ghi thẳng
vào database và nhận `'COD'`; đi trọn đường HTTP với COD và BANK_TRANSFER
cho hai đơn KHÁC nhau, đọc lại đúng giá trị; `BITCOIN` qua API trả 400.

**Nửa còn lại đã làm CÙNG NGÀY (PH-36).** Lúc viết ghi chú này, `CARD`,
`BANK_TRANSFER` và `E_WALLET` mới chỉ được GHI NHẬN. Nay đơn trả trước có
`payment_intent`, và webhook thanh toán đối chiếu số tiền rồi chuyển đơn
sang `PAID`. Chính trường `payment_method` của P3-9 là thứ cho phép quy
tắc "COD KHÔNG có intent" viết được.

**P3-8 — đã xong (06/09).** `fulfillment.EstimateShipping()` nay tồn tại,
và biểu phí đã rời khỏi checkout về đúng module biết về hãng vận chuyển.

**Lỗi thật sự được sửa không phải "bảng cứng" mà là THU MỘT LẦN CHO CẢ
ĐƠN.** Hàng của ba nhà bán nằm ở ba kho nên nó đi thành BA kiện và tốn ba
lần phí, trong khi biểu cũ thu một lần bất kể mấy nguồn. Nghĩa là nền tảng
bù phần chênh trên mọi đơn nhiều nguồn — và bù nhiều nhất đúng ở loại đơn
mà cái chợ tồn tại để tạo ra. Không lỗi nào báo, không con số nào âm; nó
chỉ hiện ra ở bảng lãi lỗ.

**Đây là thay đổi GIÁ mà khách nhìn thấy:** đơn trộn hai nhà bán đi từ
30.000đ lên 60.000đ. Con số nằm trong `fulfillment/domain` chứ không rải
rác; chuyển sang `opsconfig` (ADR-0015) để chỉnh lúc chạy là việc tiếp
theo, chưa làm.

**Nhiều MÓN của cùng một nhà bán vẫn là MỘT kiện** — phí nhân theo số
NGUỒN, không theo số món. `SellerIDs()` đã lọc trùng sẵn.

**Kiểm phương thức vận chuyển vẫn ở checkout, và đó KHÔNG phải chép đôi.**
Hai bên kiểm hai câu khác nhau: checkout kiểm tập giá trị API nhận từ
khách (chuỗi lạ → 400 ngay tại cửa), fulfillment kiểm tập giá trị nó có
biểu phí. Bỏ chốt ở checkout thì chuỗi lạ đi xuống tận fulfillment rồi
quay lên thành 500 — `TestPhuongThucVanChuyenPhaiHopLe` bắt được đúng lúc
tôi bỏ nó.

**Thiếu cổng ước tính thì `SetShippingMethod` TỪ CHỐI**, không rơi về số
mặc định: đoán phí vận chuyển là đoán tiền khách phải trả.

**Harness e2e tự làm mình dễ, lần thứ hai.** Bài mới đỏ với "phí đơn một
nhà bán = 0" dù domain đúng và phiên thanh toán đúng. Nguyên nhân:
`orderPort` trong `internal/e2e` bỏ qua `ShippingFee`, `DiscountAmount`,
`TaxAmount`, địa chỉ giao và `SourceCheckoutID` — nó chỉ chép dòng hàng.
Hệ quả rộng hơn bài này: **mọi khẳng định về tiền ở mức đơn trong
`internal/e2e` trước đây đều vô nghĩa**, vì các con số đó luôn 0 do adapter
không chép chứ không phải do hệ thống tính ra 0. Đã sửa adapter chép đủ.

Cùng bài học với `stockFor` ở PH-2: một harness dễ hơn thực tế thì bài test
xanh mà không chứng minh gì.

**CÒN THIẾU, và thiếu DỮ LIỆU chứ không thiếu phép tính:**

```text
khoảng cách   `stock_location` không có địa chỉ — không tỉnh, không tọa độ
khối lượng    dòng checkout mang sku_id và số lượng, KHÔNG mang gram
```

Cả hai cần thêm cột và thêm đường nạp dữ liệu, nên chúng nằm ngoài P3-8.
Cố ý KHÔNG nhận địa chỉ nhận vào `ShippingEstimateRequest` khi chưa dùng
được: đó đúng là thứ vừa bị bắt ở PH-40 — một trường đi qua mọi tầng mà
không ai đọc, và không ai biết nó không được đọc.

**Chính sách MIỄN PHÍ vận chuyển vẫn chưa có.** checkout.md mục 7 nêu câu
hỏi (áp trên tổng đơn hay trên từng nhà bán?) và khuyến nghị "tổng đơn",
nhưng khuyến nghị không phải quyết định — đây là quyết định kinh doanh của
chủ dự án.

**P3-17 — đã xong (07/09).** Đây là lần thứ TƯ của cùng một hình dạng —
một trường trong hợp đồng API mà không ai điền — sau `payment_method`
(P3-9), `TaxAmount` (PH-40) và `email_verified_at` (P3-15).

Khác ba lần trước ở một điểm đáng khen: **đặc tả nói thật**. Nó ghi rõ
"CHƯA được trả về — module `fulfillment` chưa có khái niệm SLA. Xem backlog
P3-17". Nhưng VÍ DỤ ngay phía trên lại in một giá trị `sla_deadline`, tức
tài liệu tự mâu thuẫn — cùng lớp với ví dụ `status: PAID` đã sửa ở PH-36.

**Hạn được TÍNH RA, không lưu thành cột**, và quyết định đó đã nằm sẵn
trong chú thích của `SLAGiaoHang`: đặc tả yêu cầu "chỉ số, ngưỡng, và tác
động đều công khai và tường minh", nên một thời hạn riêng cho từng đơn mà
không ai nhìn thấy chính là hộp đen. Hệ quả: đổi
`fulfillment.shipping_sla_hours` dời hạn của mọi đơn ĐANG CHẠY — cùng hành
vi mà điểm hiệu suất đã có, và đã ghi trong `HeQua` của tham số đó.

**Rủi ro thật của việc này là HAI CÀI ĐẶT cho một câu hỏi.** "Đơn có đúng
hạn không" nay có bản Go (màn hình nhà bán) và bản SQL (điểm hiệu suất).
Lệch nhau nghĩa là nhà bán thấy "còn 3 giờ" trong khi báo cáo ghi trễ —
tranh chấp không giải quyết được bằng dữ liệu, vì cả hai bên đều đọc đúng
thứ hệ thống nói với họ.

Nên bài test chính bắt CẢ HAI trả lời trên cùng dữ liệu và so kết quả. Nó
tìm ra một chênh lệch có thật ngay lần chạy đầu: **hạn hiển thị đi qua
RFC3339 nên mất phần dưới giây, còn SQL so ở micro-giây** — ca "bàn giao
đúng mốc hạn" cho hai kết luận ngược nhau, lệch 953 mili-giây.

Không sửa, có chủ ý: một hạn hiển thị cho người đọc thì tính bằng giây là
đúng, và cắt cả hai bên xuống giây sẽ làm phép chấm điểm bớt chính xác để
chiều một trường hợp cần bàn giao đúng vào giây thứ 172.800 mới chạm tới.
Ghi vào chú thích của `HanBanGiao` thay vì im lặng.

**Cờ `sla_breached` chỉ bật cho đơn CHƯA bàn giao.** Đơn bàn giao muộn đã
bị trừ điểm rồi; hiện lại nhãn trễ trên một đơn đang đi đường làm nhà bán
tưởng còn việc phải làm. Đơn đã hủy cũng không.

**Phá để kiểm chứng:** dời hạn lệch một giờ → *"MÀN HÌNH nói đúng hạn=true,
ĐIỂM HIỆU SUẤT nói false"*; cho đơn đã bàn giao mang cờ trễ → *"nhà bán
tưởng còn việc phải làm"*.

**P3-15 — đã xong (07/09), và nó lộ ra một giả định SAI đã nằm trong
docs từ đầu.**

Ngõ cụt ban đầu: khách đặt hàng vãng lai bằng email X rồi KHÔNG đăng ký
được bằng chính email đó. Từ chối là ĐÚNG khi chưa có cách chứng minh
quyền sở hữu email — đơn hàng chứa địa chỉ nhà và số điện thoại người
nhận — nhưng nó không để lại đường nào đi tiếp.

**Cách sửa: TÁCH hai việc.** Tạo tài khoản làm ngay; gộp lịch sử chờ tới
khi khách bấm liên kết trong hộp thư. Bấm được liên kết là bằng chứng đọc
được hộp thư đó.

**`User.VerifyEmail` là mã chết THỨ BA tìm thấy theo cùng một cách** — sau
`Order.MarkPaid` (PH-36) và `Checkout.SetTax` (PH-40). Cả ba đều tồn tại
đủ mọi tầng và không ai gọi. Đáng ghi thành một hình dạng để nhận ra sớm:
**một phương thức domain không có bên gọi nào ngoài test là một tính năng
đã dựng một nửa rồi bỏ dở.**

**GIẢ ĐỊNH SAI mà việc cài đặt lộ ra.** Cả backlog lẫn `customer.md` đều
mô tả "hồ sơ vãng lai chứa lịch sử mua hàng", và bản đầu của tôi gộp đúng
thứ đó — gắn `customer.user_id`. Chạy trên database phát triển thì `đã gộp
hồ sơ = false` và khách vẫn không thấy đơn nào.

Đo lại mới thấy:

```text
hồ sơ khách vãng lai (user_id rỗng)          0
đơn vãng lai (customer_id rỗng, có email)  3150
```

`EnsureByEmail` — hàm tạo hồ sơ cho khách vãng lai — **không có bên gọi
nào** trong toàn hệ thống. Nghĩa là hồ sơ vãng lai chưa từng tồn tại, và
quyết định bảo mật "email đã đặt hàng thì TỪ CHỐI đăng ký" đang bảo vệ một
trạng thái không xảy ra.

Lịch sử thật nằm ở `order.guest_email`. Nên phần gộp phải chuyển chủ các
ĐƠN, và `RegisterShopper` phải hỏi `order` chứ không tra hồ sơ.

**Ba lớp bảo vệ, mỗi lớp có phá để kiểm chứng:**

```text
token băm SHA-256, không lưu nguyên văn
token DÙNG MỘT LẦN     hai lớp: domain + `used_at IS NULL` ở lệnh ghi
hết hạn 24 giờ
chỉ đụng đơn customer_id RỖNG
```

Phá `customer_id = ''` khỏi mệnh đề WHERE: bài API một-khách VẪN XANH.
Phải thêm bài có hai chủ mới đỏ — *"chuyển 2 đơn, mong đúng 1 — con số 2
nghĩa là đã CƯỚP đơn của khách cus_…"*. Lần thứ ba trong tuần một bài test
xanh vì DỮ LIỆU DỄ chứ không vì code đúng.

Phá riêng lớp domain của "dùng một lần" cũng xanh — lệnh ghi bắt được.
Phải bỏ CẢ HAI mới đỏ.

**Kiểm trên hệ thống THẬT:** hai đơn vãng lai → đăng ký bằng chính email
đó (`email_verification_required = true`, hai đơn VẪN vô chủ) → bấm liên
kết lấy từ thân thư đã gửi → `orders_merged = 2`, 0 đơn vô chủ.

**Gửi lại liên kết — làm ngay sau, cùng ngày.** Hết 24 giờ mà chưa bấm thì
không có đường tự phục hồi, tức cùng loại ngõ cụt P3-15 vừa xóa.

`POST /api/v1/auth/verify-email/resend` **cần đăng nhập**, khác
`verify-email`. Lý do nằm ở thứ mỗi đường nhận vào: đường kia nhận TOKEN,
bản thân nó đã là bằng chứng. Đường này nếu nhận EMAIL thì bất kỳ ai cũng
hỏi được "địa chỉ này có tài khoản chưa" qua việc phản hồi khác nhau — tức
là công cụ dò danh sách email, đúng thứ `identity/public.go` đã cảnh báo
cho đường đăng ký.

Yêu cầu đăng nhập KHÔNG chặn ai: tài khoản dùng được ngay từ lúc đăng ký,
kể cả khi email chưa xác minh. Thứ chưa có là lịch sử đơn cũ.

Token mới VÔ HIỆU token cũ — người ta bấm "gửi lại" chính vì nghi ngờ thư
cũ, nên để hai liên kết cùng sống là làm ngược điều họ vừa yêu cầu.

**Lại một lần phá bị nuốt, và lần này nó dạy tôi đọc sai chính bản sửa của
mình.** Bỏ `httpserver.Auth` khỏi route thì bài test đỏ — nhưng đỏ ở lời
gọi ĐÃ đăng nhập (middleware không parse token nữa nên handler thấy danh
tính rỗng), không phải ở lời gọi chưa đăng nhập. Bỏ riêng chốt trong
handler thì VẪN XANH — middleware bắt được. Phải bỏ CẢ HAI mới thấy lời
gọi không đăng nhập đi lọt, và khi đó là 500 chứ không phải "gửi lại được":
`ids.Parse` trên chuỗi rỗng vẫn chặn ở lớp thứ ba.

**P3-10 — đã xong (06/09).** Dòng cũ ghi "cần cả bốn module thật" và đó
chính là thứ đã giữ mục này mở suốt từ trước P1.3. Không cần: `offerLookup`
nhận bốn interface, nên bốn bản giả là đủ — và bản giả còn mô tả được tình
huống mà module thật khó dựng ("nhà bán tra KHÔNG RA").

Bản giả **nhúng interface thật** thay vì cài đủ mọi phương thức. Lý do
chính không phải để viết ngắn: phương thức không được ghi đè sẽ panic con
trỏ nil nếu bị gọi, nên một cài đặt lén gọi thêm hàm khác không thể im
lặng trượt qua.

**Thứ đáng khóa nhất ở đây là một quyết định BẤT ĐỐI XỨNG.** Cùng một lần
tra nhà bán hỏng, hai trường xử lý ngược nhau:

```text
SellerActive  tra không ra → coi như ĐÌNH CHỈ   (hỏng thì ĐÓNG)
SellerName    tra không ra → để rỗng, đi tiếp   (hỏng thì MỞ)
```

Cả hai đều đúng và vì hai lý do khác nhau: đặt hàng của nhà bán đã bị đình
chỉ thì phải hủy đơn, còn thiếu một nhãn hiển thị không đáng làm hỏng giỏ.
Nhưng đọc riêng một nửa thì rất dễ kết luận quy tắc là "luôn đóng" hoặc
"luôn mở", rồi sửa theo kết luận đó và phá nửa kia. Hai bài test đứng cạnh
nhau chính là để chặn điều đó.

**Khóa luôn hợp đồng hiệu năng (PH-14).** Chú thích của `offerLookup` tự
đặt ra: giỏ 10 món tốn ĐÚNG bốn lượt gọi, không phải 40. Hồi quy N+1 không
làm test nào đỏ và không làm giao diện sai — nó chỉ làm mọi thứ chậm dần,
và không ai truy được nguyên nhân về đúng commit đã gây ra. Nay có bài
đếm: phá bằng cách hỏi nhà bán trong vòng lặp → "gọi seller 11 lần cho giỏ
10 món".

**Phá để kiểm chứng, hai lần, cả hai đỏ đúng chỗ:** san phẳng bất đối xứng
(nhà bán tra không ra → coi như hoạt động), và tái lập N+1.

**P3-1 — đã xong (05/09).** Dòng "chỉ kiểm chứng bằng curl thủ công" đã
lạc hậu từ lâu: đường auth có `api_auth_test.go`,
`api_gioihan_dangnhap_test.go`, `api_dem_thatbai_test.go`,
`api_ma_tran_quyen_test.go`, `api_phamvi_hong_test.go`; endpoint audit-log
đã được khóa về xác thực, vai trò và trần trang.

**Phần THẬT SỰ còn thiếu là bộ lọc theo NGÀY của endpoint** — và viết bài
cho nó lộ ra một lỗi.

**Lỗi: mốc ngày cắt theo UTC trong khi nghiệp vụ chạy ở UTC+7.**
`parseDate` dùng `time.Parse` không kèm múi giờ. Một thao tác lúc 03:00
sáng 20/08 giờ Việt Nam được lưu là 20:00 UTC ngày 19/08, nên bộ lọc ngày
20/08 KHÔNG trả nó — trong khi trang quản trị hiển thị chính bản ghi đó là
"20/08 03:00" (`Intl.DateTimeFormat` dùng giờ máy). Lọc và hiển thị nói hai
điều khác nhau trên cùng một màn hình.

Với nhật ký kiểm toán, thứ tồn tại để điều tra sự cố, mất bảy giờ đầu mỗi
ngày là khiếm khuyết thật: "cho tôi xem mọi việc đã xảy ra hôm đó" im lặng
bỏ sót ca đêm.

**Chủ dự án chọn hướng: mốc ngày tính theo giờ Việt Nam.** Không đổi hợp
đồng (`format: date` giữ nguyên), không đổi giao diện.

**Lỗi có ở HAI endpoint**, cùng một hàm `parseDate` chép đôi:
`GET /admin/audit-log` và `GET /admin/orders`. Gộp về
`types.PhanTichNgay` + `types.CuoiNgay` trong kernel.

**Dùng `FixedZone` chứ không `LoadLocation("Asia/Ho_Chi_Minh")`**, có lý
do: `LoadLocation` cần cơ sở dữ liệu múi giờ trên máy chủ, mà ảnh container
tối giản thường không có — xử lý cẩu thả là rơi về UTC, tức đúng lỗi này
quay lại nhưng CHỈ ở môi trường triển khai. Việt Nam giữ +07:00 từ 1975 và
không có giờ mùa hè, nên múi giờ cố định là mô tả đúng.

**Kiểm chứng bằng cách phá, và một lần phá SAI đã dạy được điều gì đó.**
Đưa handler đơn hàng về `time.Parse` làm một bài test đỏ — nhưng đọc kỹ thì
nó đỏ vì `time.Parse("")` báo lỗi với chuỗi rỗng (bộ lọc tùy chọn thành bắt
buộc), KHÔNG phải vì múi giờ. Tức là phần nối dây của endpoint đơn hàng vẫn
chưa được kiểm. Đã viết bài riêng cho nó, rồi phá lại bằng một hàm giữ
nguyên xử lý chuỗi rỗng và chỉ đổi múi giờ — lần này đỏ đúng lý do.

Bốn bài đơn vị cho `types.PhanTichNgay`/`CuoiNgay`, trong đó có bài khóa
giả định "múi giờ không đổi theo mùa" — đổi sang một múi giờ CÓ giờ mùa hè
thì mốc ngày lệch một tiếng trong nửa năm, kiểu lỗi chỉ lộ theo mùa.

**P3-23 — đã xong (05/09).** BỎ `OUT_OF_STOCK` khỏi `Offer.status`, chứ
không cài nốt event. Chủ dự án quyết hướng này.

**Vì sao không cài `inventory.depleted`.** Cửa hàng HÔM NAY đã đúng:
`ProductOffer.IsSellable = offer ACTIVE && còn hàng`, tính lúc đọc. Thêm
một trạng thái dẫn xuất vào offer là lưu một sự thật ở hai nơi — chính chú
thích của struct `Offer` đã viết "hai nơi cùng lưu một sự thật thì sớm
muộn chúng lệch nhau", rồi enum lại đi ngược.

**Và cài nó sẽ THÊM một kiểu hỏng chưa từng có.** Event chết sau 5 lần thử
là bị `dead_lettered_at` vĩnh viễn. Một `inventory.replenished` chết thì
offer kẹt `OUT_OF_STOCK`, mà `IsSellable = o.IsSellable() && coHang` nên
offer thành KHÔNG BÁN ĐƯỢC dù kho đầy hàng — nhà bán mất doanh thu, không
lỗi nào báo. Hôm nay điều đó không thể xảy ra.

**Yêu cầu sản phẩm không mất gì.** "Hết hàng vẫn hiển thị để khách đăng ký
nhận thông báo" (design-system.md 4.1) vẫn đúng: offer hết hàng ở trạng
thái `ACTIVE` nên vẫn hiện, chỉ `is_sellable` tắt.
`TestHetHangThiOfferKhongConBanDuoc` đã khóa đúng ba điều đó — hiện, không
mua được, không thắng buy box. Phá `IsVisibleToCustomer` → đỏ ngay.

**Migration 000040 SIẾT ràng buộc CHECK.** Sửa mỗi code là chưa đủ: ràng
buộc cũ vẫn cho ghi `'OUT_OF_STOCK'`, và một bản ghi như vậy lọt vào thì
domain không còn trạng thái tương ứng — offer vừa không bán được vừa không
hiển thị, âm thầm biến mất khỏi trang. Siết lại biến hỏng-âm-thầm-lúc-đọc
thành lỗi-ồn-ào-lúc-ghi. Kiểm trên database thật: INSERT
`status='OUT_OF_STOCK'` bị từ chối, `'ACTIVE'` qua. 0 bản ghi cần đổi, đúng
như dự đoán — giá trị này chưa bao giờ được ghi.

**Cùng bị xóa:** `Offer.MarkOutOfStock`, `Offer.MarkBackInStock`,
`Service.MarkOutOfStock`, `Service.MarkBackInStock` — toàn bộ là code chết.

**Phát hiện ngoài phạm vi — nay ĐÃ LÀM, xem P3-24 dưới đây.**
`SellerOffer.is_sellable` được đặt bằng `o.Status() == StatusActive`, tức
KHÔNG tính tồn kho — trong khi chú thích của chính trường đó dặn "đừng suy
lại từ status ở giao diện". Nghĩa là Seller Center chưa bao giờ có tín hiệu
"hết hàng", cả trước lẫn sau thay đổi này; bỏ nhãn `OUT_OF_STOCK` không lấy
mất gì đang chạy.

### P3-24 — "khách mua được không" có BA câu trả lời khác nhau `[XONG 06/09]`

Đi trả nốt món nợ P3-23 ghi lại ở trên, và hóa ra nó lớn hơn một dòng gán.

**Quy tắc có ba đầu vào thuộc ba module** — trạng thái offer
(`marketplace`), tồn kho (`inventory`), trạng thái nhà bán (`seller`) — nên
mỗi nơi cần trả lời đều tự ghép lấy phần mình có. Đếm ra ba bản:

```text
buy box          status + nhà bán + tồn kho   đúng
trang sản phẩm   status + tồn kho             THIẾU nhà bán
Seller Center    status                        THIẾU cả hai
```

**Dòng thứ hai là lỗi CHƯA ai ghi:** đình chỉ một nhà bán không đổi trạng
thái offer của họ (`seller.Suspend` chỉ sửa aggregate Seller), nên offer vẫn
`ACTIVE`, vẫn hiện, vẫn `is_sellable: true`. Đúng dấu hiệu mà chú thích của
`ProductOffer.IsSellable` đã mô tả cho trường hợp hết hàng — nhãn "Đề xuất"
biến mất trong khi nút "Thêm vào giỏ" vẫn sáng — chỉ là ở vế nhà bán thì
chưa ai đi kiểm. Đặc tả của `Offer.is_sellable` vốn đã khai đủ cả ba đầu
vào, nên đây là cài đặt lệch hợp đồng, không phải đặc tả thiếu.

Giỏ hàng KHÔNG bị ảnh hưởng: `cart` tra `SellerActive` riêng và đã chặn
đúng. Nên hậu quả dừng ở chỗ khách bấm mua rồi mới bị giỏ báo không mua
được, không có tiền nào chảy sai.

**Cách sửa: một hàm, ba nơi gọi.** `domain.CanCustomerBuy(offer, inStock,
sellerActive)`. `SelectBuyBox` nay nhận thêm `InStock` trên ứng viên thay vì
để tầng application lọc trước — để cả ba đi qua CÙNG một hàm thì không còn
gì để lệch. Thêm `application.ListSellerOffers` và `SellableNow` để tầng
HTTP của nhà bán KHÔNG phải tự ghép quy tắc (nó không có hai đầu vào kia, và
đó chính là lý do nó ghép sai).

**Tra nhà bán dùng chung một bộ nhớ đệm với buy box.** Bản đầu để mỗi bên tự
tra, tức hỏi database hai lần cùng một câu trong cùng một request — trên
đúng đường nóng nhất của cửa hàng. `buyBoxes` nay nhận cache từ bên gọi.

**Kiểm chứng bằng cách phá, bốn lần, mỗi lần một tầng:**

- Trả `toSellerOffer` về `status == ACTIVE` → bài API đỏ ở CẢ hai kịch bản
  hết hàng và đình chỉ.
- Trả `ListProductOffers` về `IsSellable() && coHang` → đỏ ở kịch bản đình
  chỉ, và đỏ từ phía NGƯỢC LẠI: lần này nhà bán nói `false` còn khách nói
  `true`. Bất biến bắt được lỗi từ cả hai đầu.
- Bỏ `is_sellable` khỏi huy hiệu Seller Center → bài đơn vị TS đỏ.
- Bỏ nốt ở giao diện rồi chạy trình duyệt → e2e đỏ đúng hình dạng lỗi cũ:
  offer hết sạch hàng vẫn đeo huy hiệu xanh "Đang bán".

**Kiểm trên hệ thống THẬT** (API + worker + hai giao diện, PostgreSQL có dữ
liệu): đưa tồn kho một SKU về 0 qua chính endpoint của nhà bán, rồi đọc cả
hai endpoint — `status=ACTIVE, is_sellable=false` ở cả hai, offer vẫn hiện.
12/12 bài e2e xanh.

**Việc còn nợ, CHƯA làm:** `AvailableForSKUs` trả tổng theo SKU chứ không
tách theo chủ sở hữu, nên nhà bán A hết hàng vẫn `is_sellable: true` khi nhà
bán B còn hàng cho cùng SKU. Giữ nguyên có chủ ý ở thay đổi này — tách ở góc
nhìn nhà bán mà không tách ở góc nhìn khách là dựng lại đúng chỗ lệch vừa
xóa. Sửa đúng phải đổi cả hai cùng lúc, và cần quyết định nghiệp vụ trước:
buy box hôm nay cũng chọn theo tồn kho mức SKU.

**P3-4 — đã xong (05/09).** `429` và `Retry-After` vốn đã có; phần thiếu
là bộ `X-RateLimit-Limit / -Remaining / -Reset`.

**CORS đã khai ba tiêu đề đó trong `Access-Control-Expose-Headers` từ
trước** — trình duyệt sẵn sàng đọc thứ chưa ai gửi.

**Gắn cho MỌI lượt, không riêng lượt bị chặn.** Đó mới là công dụng: client
thấy hạn mức cạn dần thì tự giãn nhịp, thay vì gõ tới khi ăn 429 rồi mới
biết. Phá thử bằng cách chỉ gắn ở nhánh 429 — bài test đỏ ngay từ lượt đầu.

**Đường ĐĂNG NHẬP cố ý KHÔNG có bộ tiêu đề này.** Bộ đếm ở đó đếm lượt
THẤT BẠI, và nói "còn 2 lượt nữa" là đưa cho kẻ rải mật khẩu đúng ngân
sách để đi chậm mà không bao giờ chạm ngưỡng. Không nói thì nó phải tự dò,
mà dò nghĩa là ăn 429 — tức là lộ diện. Người dùng thật không mất gì: đăng
nhập đúng thì không chạm hạn mức, và khi bị chặn vẫn có `Retry-After`.

**`Retry-After` sửa lại cho đúng cửa sổ TRƯỢT.** Trước đây luôn báo trọn
độ dài cửa sổ. Nhưng chỗ trống đầu tiên xuất hiện khi lượt CŨ NHẤT rơi ra,
nên bị chặn ở giây thứ 55 của cửa sổ 60 giây thì chỉ cần chờ 5 giây, không
phải 60. Báo dư là bắt người bị chặn oan ngồi đợi lâu hơn cần thiết.

**`X-RateLimit-Reset` là Unix timestamp** theo đúng đặc tả đã khai. Đặc tả
nay ghi thêm: client nên chờ theo `Retry-After` (số giây tương đối, miễn
nhiễm lệch đồng hồ) chứ đừng chờ theo mốc tuyệt đối đó.

**Vẫn còn giới hạn đã biết:** bộ đếm nằm trong bộ nhớ, N bản sao = N lần
hạn mức — xem P3-16, và nó cần Redis nên chưa làm.

**P3-20 — đã xong (05/09).** Bỏ `buy_box_offer` và `other_offers_count`
khỏi `ProductDetail` trong đặc tả, kèm lý do ngay tại chỗ.

**Dòng cũ trong bảng ghi sai chỗ:** hai trường nằm ở `ProductDetail`, không
phải `SKUSummary`. `SKUSummary` vốn sạch — nó đã có sẵn chú thích giải
thích vì sao KHÔNG có `available`.

**Và đây không phải "trường chưa cài".** Buy box quyết theo SKU:
`GetBuyBox(skuID)`. Một chiếc áo có SKU size S, M, L; mỗi size là một cuộc
cạnh tranh riêng giữa các nhà bán và người thắng có thể KHÁC nhau. Trường
buy box ở mức sản phẩm buộc máy chủ chọn bừa winner của một size rồi trình
bày nó như winner của cả sản phẩm — sai với khách đang xem size khác.
`other_offers_count` cũng vậy: đếm offer của size nào?

Nghĩa là cài trường này vào sẽ tạo ra một lỗi mới, chứ không phải trả nốt
món nợ cũ. Cùng nhóm quyết định với `price_from` (2.10): dữ liệu offer
không nhồi vào danh mục.

**Cửa hàng vốn đã làm đúng** — `products/[productId]/page.tsx` lọc offer
theo `sku_id` của size đang chọn rồi lấy `is_buy_box` trong đó. Không có
client nào mất gì khi bỏ hai trường: chúng chưa bao giờ được trả.

Ghim ở HAI tầng để không ai "cài nốt trường đặc tả còn thiếu": kiểm hợp
đồng ở `api_contract_test.go` và bài đơn vị ở `handler_test.go`. Phá thử
bằng cách trả `buy_box_offer` trong `productDetail` — cả hai đỏ.

Chú thích trong `dto.go` cũng sửa lại: nó ghi `buy_box_offer → module
inventory + marketplace (giai đoạn 3)`, đọc như một lời hứa sẽ làm sau.

**Còn lại trong nhóm này, CHƯA làm:** `size_recommendation` vẫn nằm trong
đặc tả và chưa bao giờ được trả. Khác hai trường trên ở chỗ nó là quyết
định sản phẩm, không phải ranh giới module — và "gợi ý size theo lịch sử
mua" nằm đúng vào nhóm tính năng chủ dự án đã gạch khỏi phạm vi. Cần chủ
dự án quyết: bỏ khỏi đặc tả, hay giữ như việc còn nợ.

**P3-12 — đã xong (05/09).** `listMyOrders` phân trang theo KHÓA
`(placed_at, id)`, không còn theo offset.

**Lỗi đã tái hiện trước khi sửa**, không phải suy luận. Đọc trang 1
(`limit=3`), đặt thêm một đơn, rồi đọc trang 2 bằng con trỏ trả về:

```text
trang 1: [...AATSEY0BAKYRPM71KS ...AKNFSJQZVJJBBP90XJ ...AV6J0RDFNWJP5KAYZ0]
trang 2: [...AV6J0RDFNWJP5KAYZ0 ...B3ASYJPJ6SQB1M5J2R ...B8WCPJH1WVM4FW9APP]
```

Đơn cuối trang 1 quay lại làm đơn đầu trang 2. Offset đếm theo VỊ TRÍ
trong danh sách sắp `placed_at DESC`, nên đơn mới chèn vào đầu đẩy mọi bản
ghi lùi một bậc. Chiều ngược lại tệ hơn vì im lặng: bản ghi rời khỏi tập
lọc thì một đơn bị NHẢY QUA và không bao giờ hiện ra.

**Mốc phải gồm CẢ `id`, không riêng `placed_at`.** Đơn tạo trong cùng một
transaction dùng chung `now()` của PostgreSQL, và nhập liệu hàng loạt thì
cả lô chung một mốc. So riêng `placed_at` thì trang sau hỏi "đơn nào cũ
hơn mốc" và loại sạch những đơn cùng mốc — kể cả đơn chưa ai đọc. Phá thử
đúng chỗ này: 7 đơn cùng mốc, đi hết các trang chỉ gặp 2.

`ORDER BY` cũng phải khớp ĐÚNG thứ tự phép so sánh. Bỏ `id DESC` khỏi
`ORDER BY` mà vẫn so theo cặp thì vừa lặp vừa sót.

**Không ký con trỏ, có cân nhắc.** Mốc chỉ chọn vị trí trong danh sách đơn
của CHÍNH người gọi — `customer_id` luôn lấy từ token, không từ request —
nên sửa mốc cùng lắm nhảy tới chỗ khác trong lịch sử của chính mình. Con
trỏ sai định dạng trả `400` chứ không đọc lại từ đầu: đọc lại từ đầu làm
client tưởng đang ở trang sau, nhận về trang đầu, rồi lặp vô hạn.

**Index `order_customer_idx (customer_id, placed_at DESC)` có sẵn** và
phục vụ thẳng dạng truy vấn này. Đây là chỗ khóa hơn hẳn offset kể cả khi
dữ liệu đứng yên: offset vẫn phải đọc rồi bỏ đi `offset` bản ghi đầu.

**Mặt public của module bỏ luôn tham số phân trang** (`ListCustomerOrders`
giờ chỉ nhận `limit`). Nó chưa có bên gọi nào; bày ra API phân trang chưa
ai dùng thì hoặc phải bịa hình dạng con trỏ, hoặc phải giữ offset — mà
offset chính là lỗi vừa sửa.

**Một chú thích đã bị SỬA LẠI vì không kiểm chứng được.** Bản đầu viết
rằng ghi mốc theo nano-giây sẽ làm bản ghi ở ranh giới trang lặp hoặc biến
mất. Phá thử: đổi sang nano-giây, không bài test nào đỏ — và đúng là
không thể đỏ, vì `placed_at` luôn đọc lên từ PostgreSQL nên đã tròn
micro-giây sẵn. Chú thích nay nói đúng lý do thật: micro-giây khớp độ phân
giải của `timestamptz`, thế thôi.

**Đặc tả đã mô tả đúng từ đầu** ("offset… gây lặp/nhảy bản ghi") — lại là
phần cài đặt lệch khỏi hợp đồng, giống hệt P3-11. Bổ sung vào đặc tả: con
trỏ là chuỗi ĐỤC, client không được tự dựng hay tự cộng.

**P3-11 — đã xong (05/09).** Lọc `status` của `listMyOrders` đi thẳng vào
truy vấn: `AND ($2 = '' OR status = $2)`, một câu lệnh cho cả hai trường
hợp thay vì dựng SQL động theo nhánh — nhánh ít dùng hơn là nhánh không ai
kiểm.

**Hậu quả thật tệ hơn dòng mô tả cũ trong bảng.** "Một trang trả ít hơn
`limit`" mới là trường hợp nhẹ. Khi CẢ trang bị loại thì `data` rỗng trong
lúc `has_more` vẫn `true`, và client thường coi trang rỗng là hết dữ liệu
rồi dừng phân trang — khách mất phần lịch sử còn lại mà không có lỗi nào
báo. Kiểm chứng bằng cách khôi phục lối lọc cũ: `{"data":[],"pagination":
{"next_cursor":"1","has_more":true}}`.

**Trạng thái gõ sai nay trả 400** kèm danh sách giá trị hợp lệ, chứ không
phải danh sách rỗng — rỗng trông giống "khách chưa có đơn nào" chứ không
giống "bạn gõ sai". Tập trạng thái đóng nằm ở domain (`Status.HopLe`),
không phải ở handler.

**Ba lỗi trong CHÍNH BỘ TEST, tìm ra nhờ phá code thật.** Bản test đầu vẫn
XANH khi lối lọc cũ được khôi phục — tức là nó không chứng minh gì:

1. SQL dựng dữ liệu huỷ 3 đơn MỚI NHẤT, tức một khối liền nhau, chứ không
   xen kẽ như chú thích nói. Trang đầu khi đó toàn `CANCELLED`, nên lọc
   trong truy vấn và lọc sau khi đọc cho ra CÙNG kết quả.
2. `UPDATE ... WHERE guest_email = lower($2)` không khớp bản ghi nào:
   `guest_email` lưu NGUYÊN VĂN chuỗi khách gửi, còn `customer.email` mới
   là cột lưu chữ thường. Không đơn nào được gắn vào khách, và bài test
   chạy trên danh sách rỗng.
3. `placed_at` của các đơn quá sát nhau, trong khi truy vấn sắp theo cột
   đó — thứ tự do PostgreSQL tuỳ chọn.

Nay dữ liệu xen kẽ một-một, `placed_at` cách nhau hẳn một phút, và có chốt
chặn: dựng xong mà không đủ 2 đơn mỗi trạng thái thì `t.Fatalf` ngay.

**P3-19 — đã xong (20/08).** `GET /api/v1/sellers?ids=` tra hồ sơ nhà bán
theo LÔ, công khai, không cần đăng nhập.

Tra theo lô chứ không phải `/sellers/{id}`: trang sản phẩm hiện N offer của
N nhà bán, và một endpoint đơn lẻ buộc trang gọi N lần. Endpoint offer vẫn
CHỈ trả `seller_id` — ghép dữ liệu là việc của TRANG.

**Handler tách hẳn khỏi bản quản trị, và đó là ranh giới bảo mật.** Hồ sơ
quản trị có tên pháp lý, mã số thuế, email, số điện thoại và tỷ lệ hoa
hồng; endpoint này không có Auth nên bất kỳ ai cũng gọi được. Dùng chung
một struct rồi lọc bằng `omitempty` là cách rò rỉ kinh điển: thêm một
trường vào hồ sơ quản trị, quên cập nhật bộ lọc, và nó ra ngoài mà không
ai biết. Ở đây trường công khai được LIỆT KÊ RA, nên thêm trường mới mặc
định là KHÔNG lộ.

Test kiểm danh sách TRẮNG (chỉ ba trường được phép) chứ không phải danh
sách đen, và quét cả thân response tìm giá trị nhạy cảm. Xác nhận đỏ khi
cố tình thêm `email` và `commission_rate_bp`.

**P3-21 — đã xong (20/08).**

Phát hiện khi xem kết quả render thật sau khi nối tên nhà bán. Trang liệt
kê offer của MỌI SKU thuộc sản phẩm, lẫn lộn, không nói offer nào ứng với
màu/size nào:

```text
469.000 ₫ · Đề xuất · Lumière · Chính hãng
490.000 ₫ · Đề xuất · Lumière · Chính hãng
390.000 ₫ · Đề xuất · Lumière · Chính hãng
```

Khách chọn theo giá mà không biết mình đang mua size nào. Với thời trang
thì đó không phải chi tiết phụ — màu và size LÀ thứ khách quyết định.

Cả ba đều hiện "Đề xuất" cũng vì lý do này: buy box tính theo TỪNG SKU nên
mỗi SKU có một offer thắng, và gộp ba SKU vào một danh sách thì cả ba cùng
thắng. Nhãn đúng ở tầng dữ liệu, vô nghĩa ở tầng hiển thị.

Cần: chọn màu → chọn size → mới lọc offer theo SKU đã chọn. Dữ liệu đã có
sẵn (`product.variants[].skus[]` và `offer.sku_id`); thiếu là ở giao diện.

**Đã dựng lại thành ba bước, đúng thứ tự khách nghĩ: màu → size → nhà bán.**
Màu và size xác định MÓN HÀNG (một SKU); nhà bán chỉ là mua món đó của ai.

Việc gom biến thể phẳng thành cây màu → size nằm ở `lib/chon-hang.ts` dưới
dạng hàm THUẦN, có test riêng chạy không cần trình duyệt (10 test). Giao
diện chỉ còn hiển thị và nối sự kiện.

Ba quyết định đáng ghi lại:

1. **Hết hàng thì HIỆN, không ẩn.** Ẩn size hết hàng làm khách kết luận
   thương hiệu không làm size của mình rồi rời đi, thay vì biết là tạm hết.
2. **Đổi màu thì GIỮ NGUYÊN size** nếu màu mới có size đó. Khách nghĩ "tôi
   mặc M, cho tôi xem M màu xanh". Đây cũng là chỗ dễ sinh lỗi im lặng
   nhất: giữ nguyên `sku_id` cũ thì khách tưởng đang xem màu xanh nhưng
   mua đúng chiếc áo trắng.
3. **Chọn sẵn tổ hợp MUA ĐƯỢC**, không phải cái đầu tiên. Mở trang ra mà
   mặc định đã hết hàng là bắt khách làm thêm một bước chỉ để về trạng
   thái dùng được.

**P3-22 — `Color` và `Size` mới là chuỗi (20/08).**

Đặc tả từng khai chúng là object — `Color {name, hex_code, color_family}`
và `Size {value, system, label}` — nhưng domain giữ thuộc tính biến thể
trong một map `{color, size, ...}` và không có chỗ cho những trường đó.
Đặc tả đã sửa về đúng sự thật; đây là việc còn thiếu, không phải lỗi.

Cái mất là thật, và lý do nằm ngay trong chính đặc tả cũ:

- `hex_code` — "khách chọn theo màu nhìn thấy, không theo tên". Bộ chọn
  màu hiện là chữ; ô màu thật dễ dùng hơn hẳn với hàng thời trang.
- `system` — "M của thương hiệu A khác M của thương hiệu B, và 38 giày
  khác 38 quần". Không có hệ size thì bảng quy đổi không dựng được, và
  đó là một nguyên nhân hoàn hàng.

Cần thêm trường ở domain + migration + chỗ nhập ở admin, nên không gộp
vào việc dựng lại luồng chọn hàng.

**Nửa `system` xong (07/09) — và nó KHÔNG cần trường mới nào.**

Đoạn trên đoán sai. Đọc mã thì hệ size đã có đủ từ trước:
`catalog.SizeChart` có `system` (ALPHA/NUMERIC/EU/US/UK/JP/FREE) cùng số
đo thực tế, `Product.SizeChartID` trỏ tới nó, và hai sản phẩm thật trong
DB đều gắn bảng size đúng. Không thiếu trường, thiếu ĐƯỜNG RA.

Thứ thực sự sai là một trường được khai ba nơi và không nơi nào điền:

```text
api/paths/storefront.yaml:142   hứa đích danh `size_chart`
api/components/schemas.yaml     ProductDetail.size_chart
dto.go:53                       SizeChart *sizeChart
                                → toProductDetail KHÔNG bao giờ gán
catalog.GetSizeChart            đủ ba tầng, KHÔNG ai gọi
```

Và không có endpoint bảng size nào khác, nên khách không có đường nào tới
số đo — trên chính thứ mà `docs/01-business/kpi.md` mục 3 gọi là nguyên
nhân hoàn hàng số một.

Lần thứ SÁU cùng một dạng lỗi: `payment_method` (P3-9), `TaxAmount`
(PH-40), `email_verified_at` (P3-15), `sla_deadline` (P3-17), rồi
`size_chart`. Bốn trong sáu lần kèm theo một hàm domain không ai gọi.
Xem "Trường không ai điền" ở cuối tài liệu này.

Đã nối: `CatalogPort.GetSizeChart` → `Service.SizeChartOf` → handler.
Chỉ ở CHI TIẾT sản phẩm, không ở danh sách (ở đó nó là N+1 và đặc tả cũng
không khai). Sản phẩm không cần bảng size (túi, phụ kiện) không gọi
catalog. Lỗi catalog KHÔNG chặn trang: thiếu số đo còn hơn không mua được.

Kiểm chứng trên hệ thống thật, cùng một sản phẩm, cùng một DB:

```text
mã trước:  "size_chart" không có trong response
mã sau:    system=ALPHA, entries=[S/M/L], nguc 86-90, eo 70-74 (cm)
```

**Còn lại của P3-22: `hex_code` — BỊ CHẶN, và không phải vì thiếu trường.**

Mã màu phải do người bán NHẬP: khác `color_family`, nó không suy ra được
từ tên (`color_family` đã có sẵn — migration 000038 suy ra từ tên và đánh
chỉ mục GIN, nên lọc theo nhóm màu đã chạy).

Về lưu trữ thì KHÔNG cần migration: `color_family` nằm trong map
`attributes` (JSONB), nên `color_hex` cũng vậy. Backlog đoán sai điểm này
lần thứ hai.

Cái chặn thật là **không có đường NHẬP nào cả** — xem P3-25. Thêm
`color_hex` vào hợp đồng API lúc này sẽ là **lần thứ bảy** của đúng dạng
lỗi mà mục 8 vừa liệt kê: một trường không ai điền được. Nên không thêm.

**P3-27 — chỉ số có rồi mà không ai canh (07/09).**

Cùng dạng lỗi của mục 8, lùi ra một bước: không phải "trường không ai điền"
mà là **tín hiệu không ai nghe**. Ba job đối chiếu (P3-26 và bản thanh toán
06/09) sinh ra để biến hỏng hóc vô hình thành một con số — rồi con số đó
nằm trong gauge mà `deploy/prometheus/alerts.yml` không hề nhắc tới:

```text
gouse_payment_reconcile_mismatch      0 luật cảnh báo
gouse_payment_intent_pending_stale    0 luật
gouse_fulfillment_delivery_silent     0 luật
```

17 luật hiện có phủ kín HẠ TẦNG — tiến trình sống, outbox, pool, tỷ lệ 5xx
— và không luật nào nói về TIỀN. Công việc dừng đúng một bước trước khi có
tác dụng.

**Và tìm ra một lỗi lớn hơn trong lúc làm.** `WorkerJobStalled` (mức
critical) dùng ngưỡng CỐ ĐỊNH 300 giây cho mọi job, trong khi nhịp thật
trải từ 5 giây tới 1 giờ:

```text
5s · 30s · 60s     dưới ngưỡng, đúng
300s · 300s        chạm đúng ngưỡng
600s               VƯỢT — luôn kêu
3600s · 3600s      VƯỢT — luôn kêu
```

Ba job luôn vượt ngưỡng khi hoàn toàn khỏe mạnh, nên bảng cảnh báo có một
dòng critical đỏ vĩnh viễn — thứ làm người trực học cách bỏ qua CẢ BẢNG,
và làm mọi luật khác trong file trở nên vô nghĩa.

Chứng minh bằng promtool chứ không suy luận: job nhịp 10 phút làm luật kêu
ở phút thứ 9, kèm đúng chú thích "Đây là treo thật, không phải một lượt
chạy dài".

Sửa bằng cách công bố nhịp thành chỉ số (`gouse_worker_job_interval_seconds`)
rồi so tương đối `> 3 × nhịp`. Luật tự điều chỉnh: thêm job hay đổi nhịp
không phải sửa luật. Chú thích của `WorkerJobLastSuccess` vốn đã nói đúng
điều cần làm — "ngưỡng khác nhau [theo job]" — nhưng luật không có cách nào
BIẾT nhịp, nên nó không làm được.

**Bài kiểm cảnh báo trước nay không ai chạy.** `alerts_test.yml` tồn tại từ
lâu, viết kỹ, có cả bài ngược — và `promtool` không có trong Makefile lẫn
CI. Đã thêm target `make alerts` và một job CI cài promtool ghim phiên bản.
Không gộp vào `make check` vì nó cần công cụ ngoài, đúng theo tiền lệ của
`api-lint`.

Kiểm chứng bằng cách phá — ba lần, mỗi lần một bài đỏ:

```text
WorkerJobStalled quay lại ngưỡng cố định 300s  → bài job nhịp 10 phút
DeliverySilenceGrowing đổi thành `> 0`         → bài "đứng yên ở mức 20"
PaymentReconcileMismatch nới lên `> 5`         → bài lệch kéo dài
```

Một chỉ số CỐ Ý không có cảnh báo: `gouse_payment_intent_pending_stale`.
Phần lớn là khách bỏ giữa chừng. Lý do ghi thẳng trong `alerts.yml` để lần
sau không ai "hoàn thiện cho đủ bộ".

---

**P3-26 — webhook vận chuyển mất thì không ai biết (07/09).**

Yêu cầu 5 của `api/paths/webhooks.yaml` — "KHÔNG TIN TUYỆT ĐỐI — phải có
đối chiếu định kỳ, vì webhook có thể mất" — đã được trả một nửa cho webhook
THANH TOÁN hôm 06/09. Nửa vận chuyển thì chưa, dù đặc tả của chính endpoint
đó nói rõ hơn: hệ thống dùng "hai cơ chế song song: webhook (thời gian
thực) và hỏi định kỳ (phòng khi webhook mất)". Chỉ có cơ chế thứ nhất.

**Đo trên dữ liệu thật trước khi viết dòng nào** (mục 8, bước 3):

```text
fulfillment_order   PENDING      3171
                    HANDED_OVER    24   ← già nhất 18 ngày
                    DELIVERED       0
                    COMPLETED       0
webhook_event       cong-tt         2   ← KHÔNG có webhook vận chuyển nào
ledger_entry        ORDER_REVENUE 3106  ← chỉ một loại
SELLER_PAYABLE      353.503.920 đ       ← chưa đồng nào được giải phóng
settlement          0 dòng
```

Chuỗi hệ quả có thật, không phải suy đoán: `CompleteDelivered` chỉ nhận đơn
DELIVERED, và nó là chỗ số dư nhà bán chuyển từ Pending sang Available. Gói
kẹt ở HANDED_OVER ⇒ không bao giờ COMPLETED ⇒ tiền nhà bán nằm im vô thời
hạn. Không có gì phát hiện được điều đó ngoài việc có người tình cờ mở đơn
ra xem.

**Làm nửa NỘI BỘ, cùng lý do với đối soát thanh toán.** Nửa ĐI HỎI cần
adapter hãng vận chuyển thật (chưa có, như PSP ở ADR-0017). Nửa nội bộ là
hệ thống TỰ BIẾT gói nào im lặng quá lâu — không thay được nửa kia, nhưng
biến một webhook mất từ vô hình thành một dòng cảnh báo.

**KHÔNG tự đánh dấu đã giao.** Cám dỗ rất thật vì nó gỡ kẹt tiền ngay, và
đó chính là lý do phải viết ra: suy "chắc giao rồi" từ việc im lặng là bịa
ra một sự kiện chưa xảy ra, mà ở đây nó bịa ra tiền — DELIVERED mở đường
cho `CompleteDelivered` chi trả cho một lần giao hàng không ai xác nhận.
Cùng nguyên tắc ADR-0017 phần 4: ghi nhận nghĩa là đã tin.

Ngưỡng nằm ở `fulfillment.delivery_silence_hours` (mặc định 168 giờ), đọc
lại MỖI lượt chạy chứ không chụp lúc khởi động. Mức log là WARN chứ không
ERROR: khác đối soát thanh toán, ở đây CÓ ca hợp lệ (tuyến xa im lâu), và
dùng ERROR cho thứ đôi khi kêu oan là cách làm hỏng ý nghĩa của ERROR ở mọi
chỗ khác.

**Hai điều chỉ chạy thật mới thấy:**

1. Job đầu tiên báo `chưa nối cấu hình vận hành` — worker dựng module
   fulfillment KHÔNG truyền `OpsConfig`, chỉ API mới truyền. Nếu hàm đã trả
   danh sách rỗng cho êm thay vì báo lỗi thì job này sẽ báo "0 gói kẹt" mãi
   mãi, và đó đúng là dạng hỏng nó sinh ra để bắt — hỏng ngay bên trong
   chính nó. Đây là lý do `ErrChuaNoiCauHinh` tồn tại.
2. Một lần phá — thêm `Deliver()` vào vòng lặp — LỌT qua cả sáu bài test
   đang có. Quy tắc quan trọng nhất của tính năng lại là quy tắc không ai
   kiểm. Đã bổ sung `TestDoiSoatKhongDuocTuDanhDauDaGiao`, đọc thẳng
   `status` và `delivered_at` từ database.

Chạy trên dữ liệu thật: **20 gói mất tin** (24 gói HANDED_OVER trừ 4 gói
mới bàn giao dưới ngưỡng), im lặng lâu nhất 454 giờ, kèm mã vận đơn thật.
`gouse_fulfillment_delivery_silent` = 20.

Còn thiếu: nửa ĐI HỎI hãng vận chuyển. Nó là nửa duy nhất phân biệt được
"webhook mất" với "hàng thật sự chưa đi tới đâu".

---

**P3-25 — toàn bộ phía GHI của product không có endpoint nào (07/09).**

Phát hiện khi tìm chỗ nhập `hex_code` cho P3-22. Module product đăng ký
đúng BA route, cả ba đều là đọc:

```text
GET /api/v1/products/{product_id}
GET /api/v1/products
GET /api/v1/search
```

Trong khi tầng application có tám use case GHI, đủ cả quy trình duyệt mà
`docs/04-modules/product.md` mục 1 khai là trách nhiệm của module:

```text
CreateProduct  AddVariant  AddSKU  SubmitForReview
Approve  Reject  Deactivate  Reactivate  Archive
```

Số nơi gọi chúng ngoài domain/seed/test: **0**. (`Approve` có ba kết quả
grep nhưng đều là `seller.Approve` và `catalog.Approve` — module khác.)

Domain có `StatusDraft`, `RejectionReason`, `CreatedBySellerID` — cả một
quy trình người bán tạo → sàn duyệt, cài xong xuôi, test xanh, và không
có cửa nào đi vào.

Hệ quả hôm nay: **hàng hóa chỉ vào hệ thống được qua `seed.go` hoặc ghi
thẳng vào DB.** Người bán tạo được OFFER (`POST /api/v1/seller/offers` —
một mức giá trên SKU có sẵn) nhưng không tạo được SKU để mà bán.

Đặc tả cũng chưa khai endpoint ghi nào cho product, nên đây không phải
"cài thiếu so với hợp đồng" mà là **một mặt của sản phẩm chưa được thiết
kế**. Theo quy tắc nhận việc (mục 7), việc này cần quyết định trước khi
code: ai được tạo sản phẩm — người bán tự tạo rồi sàn duyệt, hay sàn tạo
tập trung? Marketplace này định danh hàng hóa CHUNG (`SKUSummary`: "ba
seller đang bán cùng một món hàng"), nên để mỗi người bán tự tạo SKU sẽ
sinh trùng lặp và phá chính điều đó. Đây là câu hỏi ADR, không phải câu
hỏi triển khai.

Chặn: nửa `hex_code` của P3-22, và mọi việc cần dữ liệu sản phẩm thật.

---

**P3-23 — offer không bao giờ tự chuyển `OUT_OF_STOCK` (20/08).**

`Offer.MarkOutOfStock` tồn tại, có chú thích ghi rõ "do module inventory
phát event `inventory.depleted`". Nhưng event đó chưa từng được phát và
không ai đăng ký nghe — cả hai đầu dây đều chỉ có trong chú thích. Hàm là
code chết.

Hậu quả đã kiểm chứng bằng đơn thật TRƯỚC khi vá: đưa tồn kho về 0, cửa
hàng vẫn ghi "Còn hàng" và nút "Thêm vào giỏ" vẫn bấm được; khách chỉ phát
hiện ở bước thanh toán. Dấu hiệu nội tại rất rõ: nhãn "Đề xuất" BIẾN MẤT
trong khi nút mua vẫn sáng — buy box đã loại offer hết hàng còn
`is_sellable` thì không. Hai câu trả lời khác nhau cho cùng một câu hỏi,
trong cùng một response.

Đã vá ở tầng đọc: `ListProductOffers` nay tính `is_sellable` = offer đang
bán VÀ còn hàng, cùng nguồn dữ liệu buy box đang dùng. Nhưng đó là vá
TRIỆU CHỨNG — trạng thái offer trong database vẫn sai, nên mọi đường đọc
khác vẫn thấy `ACTIVE`. Sửa gốc là nối event, và đó là việc còn lại.


**P3-18 — giữ hàng chọn nhầm chủ sở hữu tồn kho (phát hiện và sửa 19/08).**

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

**Câu trả lời nằm sẵn trong tài liệu.** own-brand.md mục 7 và seller.md mục
3 đã khai own brand là seller NỘI BỘ với `inventory_owner: PLATFORM`. Tức
là chủ sở hữu tồn kho là thuộc tính SUY RA TỪ nhà bán, không phải bằng nhà
bán. Quy tắc nay nằm ở một chỗ duy nhất, `inventory.OwnerForSeller`, ngay
cạnh hằng số nó trả về.

Hàng KÝ GỬI vẫn chạy đúng: hàng của nhà bán nằm trong kho nền tảng là hợp
lệ và mô hình đã tách `stock_location_id` khỏi `inventory_owner_id` cho
đúng việc đó. Lọc theo CHỦ SỞ HỮU, không theo KHO.

Câu hỏi "nhà bán hết hàng có được mượn kho nền tảng không" — trả lời
**không**. Chủ sở hữu là điều kiện loại trừ; đơn thất bại thay vì lặng lẽ
trừ hàng của người không bán món đó.

**Ba đường cùng lỗi, không phải một:** giữ hàng lúc thanh toán, nhập kho
lúc tạo offer (`initial_inventory`), và kiểm kê. Hai đường sau tạo hoặc
tìm bản ghi ở nhầm chủ, nên chúng im lặng theo kiểu khác: bản ghi tồn tại
mà không đường nào đọc tới.

Trung tâm người bán (P2-6) làm lỗi này lộ ra ngay: nhà bán nhìn số tồn của
mình và thấy nó đứng yên dù đơn vẫn về.

**Chốt lại bằng test đi hết chuỗi.** `internal/e2e` dựng đơn hai nhà bán
cùng bán một SKU, đi qua giữ hàng → đặt đơn → event → tách đơn thực hiện.
Đây là loại lỗi mà test từng module KHÔNG thể thấy: mỗi module dựng bản giả
cho hàng xóm, và bản giả cư xử theo giả định của người viết test.

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

Nay [internal/platform/testdb](../../gouse/internal/platform/testdb/testdb.go) cấp
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

20 thao tác đã có đặc tả nhưng **không cài đặt bây giờ**. Đặc tả giữ
nguyên; chỉ hoãn phần cài đặt.

### Miền bị KHÓA trong phase Production Hardening

Không triển khai, kể cả khi "chỉ mất một buổi":

```text
Creator · Content platform · Social commerce · Live commerce
AI / advanced recommendation · Loyalty · Affiliate
Advanced supply-chain intelligence · Demand forecasting
```

**Vì sao viết ra thành danh sách thay vì tin vào kỷ luật:** mỗi miền mới
thêm một tập bất biến mới phải chứng minh, trong khi mục tiêu của phase
này là chứng minh tập bất biến ĐANG CÓ. Thêm miền lúc này làm đích lùi ra
xa đúng bằng tốc độ đang tiến tới nó.

`demand_signal` là NGOẠI LỆ đã có và được giữ: nó chỉ GHI tín hiệu, không
suy luận gì. Dự báo nhu cầu — phần suy luận — vẫn bị khóa.

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

## 8. Trường không ai điền — dạng lỗi hay gặp nhất của dự án này

Sáu lần trong khoảng một tháng, cùng một hình dạng: **một trường có trong
hợp đồng API, có cột trong DB, có phương thức domain — và không dòng mã nào
gán giá trị cho nó.**

| Trường | Mục | Hệ quả khi chưa sửa |
|---|---|---|
| `payment_method` | P3-9 | không biết đơn nào COD, không biết đơn nào chờ thu tiền |
| `TaxAmount` | PH-40 | thuế luôn bằng 0 trên mọi đơn |
| `email_verified_at` | P3-15 | không phân biệt được email đã xác minh |
| `sla_deadline` | P3-17 | không biết đơn nào trễ hạn bàn giao |
| `size_chart` | P3-22 | khách không thấy số đo — hoàn hàng vì sai size |
| `buy_box_offer` | P3-20 | khai ở `ProductDetail`, không bao giờ được trả |

**Hai cách kết thúc, và phải phân biệt.** Năm dòng đầu là trường ĐÚNG mà
chưa nối dây → nối dây. `buy_box_offer` thì khác: buy box quyết theo SKU
(mỗi size là một cuộc cạnh tranh riêng), nên trường ở mức sản phẩm không
có nghĩa đúng — nó bị BỎ khỏi đặc tả. Không phải trường nào trống cũng
đáng điền; câu hỏi đầu tiên luôn là "giá trị đúng của nó là gì".

Bốn trong sáu lần đi kèm một **phương thức domain không ai gọi**:
`Order.MarkPaid`, `Checkout.SetTax`, `User.VerifyEmail`,
`catalog.GetSizeChart`. Chúng được viết đúng, được test đúng, và không
nằm trên đường đi nào cả.

### Vì sao test không bắt được

Vì test kiểm tra thứ nó gọi. Một use case không ai gọi vẫn có test xanh
của riêng nó; một trường không ai gán vẫn có DTO đúng đặc tả. Cả hai đầu
đều xanh, còn khoảng trống ở giữa thì không ai đứng.

Test kiểm tra SỰ CÓ MẶT của khóa JSON cũng không bắt được: `size_chart`
vắng mặt hợp lệ (`omitempty`) nên response vẫn hợp đặc tả. Test cũ liệt kê
tên trường và xanh suốt trong lúc khách không bao giờ thấy số đo.

### Cách phát hiện, theo thứ tự rẻ dần

```text
1. grep tên trường trong hợp đồng API → có ai GÁN nó không (không tính
   khai báo struct và tên cột)
2. grep phương thức domain → có ai GỌI ngoài chính test của nó không
3. đo trên dữ liệu THẬT: đếm bản ghi có giá trị khác rỗng/0
4. chạy hệ thống thật và đọc response — cách duy nhất bắt được cả bốn
   trường hợp trên
```

Bước 3 đã một lần lật ngược kết luận: P3-15 ban đầu định gộp hồ sơ khách,
đo ra 0 hồ sơ khách vãng lai và 3150 đơn vãng lai, nên việc phải làm là
gộp ĐƠN.

Bước 4 bắt được lỗi mà cả bốn bước trên lẫn test xanh đều không: khi làm
P3-22, ba test đơn vị xanh và bản dựng thật vẫn trả 404 — vì máy chủ chạy
`storage=memory`. Chỉ khi chạy đúng `MODULES_STORAGE=postgres` trên dữ
liệu thật mới thấy được bảng size ra hay không ra.

Bước 2 chạy rộng thì bắt được thứ lớn hơn một trường: cũng phép grep ấy
cho thấy TÁM use case ghi của product không ai gọi — tức cả một mặt của
sản phẩm chưa có cửa vào. Xem P3-25.

---

## 9. Tài liệu liên quan

- [../README.md](../README.md) — Architecture Freeze
- [todo.md](todo.md) — việc đã làm và bằng chứng kiểm chứng
- [../../gouse/api/README.md](../../gouse/api/README.md) — trạng thái từng operation
- [mvp.md](mvp.md) — phạm vi MVP (đã đóng băng)
- [future-phases.md](future-phases.md) — Phase 2, 3, 4 (FUTURE)
