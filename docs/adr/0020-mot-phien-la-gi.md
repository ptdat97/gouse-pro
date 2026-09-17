# ADR-0020 — Một "phiên" là gì

**Trạng thái:** Đã chấp nhận · 17/09/2026

## Bối cảnh

Đo trên hệ thống chạy thật (Docker, 16/09) với dữ liệu có thật:

```text
gmv              2.219.000   ✓ đúng
order_count      2           ✓ đúng
aov              1.109.500   ✓ đúng
conversion_rate  0           ✗ bằng 0 VĨNH VIỄN
session_count    0           — chưa có lượt xem nào lúc tính
```

Phân biệt hai dòng cuối là việc đầu tiên phải làm, và ban đầu tôi gộp
nhầm chúng. `session_count` đếm lượt truy cập có xem sản phẩm; nó bằng 0
vì lúc chạy chưa ai gửi sự kiện `product_view`, và nó tự đúng ngay khi có.

`conversion_rate` thì khác hẳn: nó không sai vì thiếu dữ liệu, nó sai vì
**không thể đúng**.

### Nguyên nhân thứ nhất: một cái tên không ai ghi

`analytics` tính tử số bằng `CountDistinctSessions(EventPurchase, …)`, với
`EventPurchase = "purchase"`. Không dòng mã nào ghi một sự kiện tên
`purchase`. Sự kiện thật sự được ghi lúc đặt hàng tên là `order.placed`.

Đây là dạng lỗi mục 8 của backlog: một tên có trong hợp đồng, được ĐỌC, và
không ai GHI.

### Nguyên nhân thứ hai, sâu hơn: ba khái niệm trong một cột

Sửa cái tên KHÔNG làm kết quả khác 0. Quét toàn bộ cột định danh trên
database phát triển (3.207 đơn, 27.111 sự kiện) cho thấy cột
`event_log.session_id` chứa ba không gian mã:

```text
product_view · search   ses_…   phiên duyệt web, do CLIENT sinh
add_to_cart             crt_…   mã giỏ hàng
checkout_start          crt_…   mã giỏ hàng
order.placed            chk_…   mã phiên thanh toán
```

Mẫu số đếm `ses_…`, tử số đếm `chk_…`. **Hai tập không bao giờ giao nhau**,
nên tỷ lệ bằng 0 bất kể bán được bao nhiêu.

Mỗi lựa chọn đều có chú thích giải thích và đều hợp lý KHI XÉT RIÊNG:

> *"SessionID dùng id giỏ hàng: một giỏ là MỘT lượt mua sắm, và đó chính là
> đơn vị mà tỷ lệ chuyển đổi đếm."*

> *"SessionID dùng CheckoutID: nó nối các sự kiện của cùng một lượt mua, và
> đó chính là thứ tỷ lệ chuyển đổi cần."*

Cộng lại thì phễu không nối được một bước nào.

## Đặc tả ĐÃ trả lời câu hỏi này

Không cần phát minh định nghĩa mới. Hai chỗ trong tài liệu đã nói rõ:

`api/paths/storefront.yaml`, trường `session_id` của `POST /api/v1/events`:

> Do client sinh, KHÔNG phải định danh người. Nó nối các sự kiện của **một
> lượt truy cập** để đo được phễu.

`docs/04-modules/analytics.md`:

> `session_id` nối các sự kiện của **một lượt truy cập**.

Vậy **phiên = MỘT LƯỢT TRUY CẬP, do client sinh**. Giỏ hàng và phiên thanh
toán là những thứ khác, có mã riêng, và chúng đã có chỗ đứng đúng trong cột
`subject_id`.

## Vì sao vẫn cần một ADR

Vì nghĩa đúng đó **không tự đi tới nơi cần dùng**. Sự kiện hành vi sinh ở
TRÌNH DUYỆT; sự kiện nghiệp vụ sinh ở MÁY CHỦ, trong worker, từ domain
event. Máy chủ không biết lượt truy cập nào đã dẫn tới đơn hàng, và không có
đường nào để biết nếu không ai truyền qua.

Đó là một quyết định kiến trúc, không phải một lỗi gõ nhầm.

## Phương án

### A — Lượt truy cập, client truyền qua header *(chọn)*

Cửa hàng gửi mã lượt truy cập trên MỌI lời gọi ghi của giỏ và phiên thanh
toán. Mã ấy đi vào payload domain event, và analytics ghi nó vào
`session_id`.

```text
trình duyệt  X-Visit-Id: vis_…
      ↓
cart / checkout        giữ trên aggregate
      ↓
domain event           trường `visit_id` trong payload
      ↓
analytics              event_log.session_id
```

**Được:** đúng nghĩa đã khai trong đặc tả; phễu nối được từ lượt xem tới
đơn hàng — thứ `conversion_rate` tồn tại để đo.

**Mất:** mã do client sinh nên bịa được. Chấp nhận: nó chỉ làm sai số liệu
phân tích, không chạm tới tiền, và đường thu sự kiện đã có giới hạn tần
suất theo chính mã ấy. Cần tăng phiên bản hai event (`cart.item_added`,
`checkout.completed`) và cần cửa hàng gửi header.

### B — Phiên = cookie `shopper_session` của máy chủ

Máy chủ đã có cookie `shopper_session` (HttpOnly, 30 ngày). Dùng nó làm
định danh phiên thì không cần client đổi gì.

**Bác bỏ vì ba lẽ:**

1. Cookie ấy sống 30 ngày nên nó là **người truy cập**, không phải **lượt
   truy cập**. Chỉ số đổi nghĩa, và đổi theo hướng đẹp hơn thực tế.
2. Nó mâu thuẫn với đặc tả (*"Do client sinh"*).
3. Trang xem hàng CỐ Ý không cấp cookie ấy (`ResolveShopperChiDoc`, P3-50):
   không đặt cookie định danh vào trình duyệt của người mới lướt. Chọn B là
   phải đảo lại quyết định riêng tư đó, và đảo nó để lấy một chỉ số đẹp hơn
   là đổi sai thứ.

### C — Bỏ nửa trên của phễu, chỉ đo giỏ → đơn

Cả hai đầu đều ở máy chủ nên nối được ngay, không cần client đổi gì.

**Bác bỏ làm phương án chính:** nó đo một thứ KHÁC. "Bao nhiêu giỏ thành
đơn" không trả lời "bao nhiêu người xem hàng rồi mua" — và câu sau mới là
thứ nói cho nền tảng biết trang sản phẩm có thuyết phục không.

Nhưng nó là một chỉ số **có thật và đo được ngay**, nên giữ lại làm chỉ số
riêng với tên riêng (`cart_conversion_rate`), không mượn tên của chỉ số kia.

## Quyết định

1. **`session_id` mang ĐÚNG MỘT nghĩa: mã lượt truy cập do client sinh.**
   Không bao giờ thay bằng mã giỏ, mã phiên thanh toán hay bất kỳ mã nào
   khác. Không biết thì để TRỐNG.

2. **Tên trường đổi thành `visit_id`** ở mọi chỗ mới. `session_id` là cái
   tên đã gây ra chính lỗi này: nó nghe đúng với cả ba khái niệm, nên ba
   người viết mã ở ba module đều thấy mình điền đúng. Cột cũ giữ tên để
   khỏi migration vô ích, nhưng mọi trường mới và mọi chú thích dùng
   `visit_id`.

3. **`EventPurchase = "purchase"` bị XÓA.** Analytics đọc `order.placed` —
   tên thật sự được ghi. Một hằng không ai ghi là một lời hứa suông.

4. **Chừng nào chưa nối xong, `conversion_rate` báo CHƯA ĐO ĐƯỢC, không báo
   0.** Số 0 là một lời nói dối: nó bảo người đọc rằng không ai mua, trong
   khi sự thật là hệ thống không đo được. Đã có tiền lệ đúng trong chính dự
   án này — `GET /api/v1/seller/performance` trả `not_measured` kèm lý do
   cho từng chỉ số chưa tính được thay vì trả 0.

5. **Nợ được ghi ở nơi máy kiểm được**, không chỉ trong tài liệu:
   `TestKhongGianMaKhongLanNhau` giữ `event_log.session_id` trong sổ
   `noDaBiet` kèm lý do và trỏ về ADR này. Xóa nợ mà quên phép kiểm thì
   phép kiểm đỏ ngay.

## Thứ tự thực hiện

Ba bước, mỗi bước tự nó đã có ích — không có bước nào phải chờ bước sau mới
thấy tác dụng:

```text
1. conversion_rate báo "chưa đo được" thay vì 0        không cần client đổi
2. cart_conversion_rate — chỉ số giỏ → đơn             không cần client đổi
3. visit_id đi từ trình duyệt tới domain event         cần cửa hàng đổi
```

Bước 1 sửa một lời nói dối đang hiển thị. Bước 2 cho một con số thật để
theo dõi trong lúc chờ bước 3. Bước 3 là thứ trả lại đúng chỉ số mà đặc tả
hứa.

## Tiến độ

```text
✓ 1. conversion_rate báo "chưa đo được" thay vì 0
✓ 2. cart_conversion_rate — chỉ số phiên thanh toán → đơn
✓ 3. visit_id đi từ trình duyệt tới domain event
```

Cả ba xong ngày 17/09. `conversion_rate` đã ra khỏi sổ `ChuaDoDuoc`, và sổ
`noDaBiet` của phép quét không gian mã nay rỗng.

**Một phát hiện khi làm bước 3:** cửa hàng CHƯA TỪNG gửi sự kiện
`product_view` nào. Đường `POST /api/v1/events` có đặc tả, có test, có
giới hạn tần suất — và không có bên gọi. Nên kể cả khi backend đã đúng,
mẫu số vẫn rỗng ở production. Cùng dạng lỗi mục 8 của backlog, lần này ở
phía client, và nó chỉ lộ ra khi đi tìm bên gọi thật của một endpoint.

**Một lỗi khác lộ ra khi làm bước 2.** `order_count` — một trong ba dòng ✓ ở
trên — đếm số DÒNG sự kiện,
mà `order.placed` phát MỘT sự kiện cho MỖI nhà bán — nên một đơn trộn hàng
ba nhà bán được tính là ba đơn, và `aov` chia cho con số ấy nên nhỏ đi ba
lần. Sai ở đúng loại đơn mà cái chợ tồn tại để tạo ra, và sai theo hướng
số đơn trông ĐẸP hơn thực tế. Không bài test nào bắt được vì mọi bài đều
dựng đơn một nhà bán — nơi hai cách đếm cho cùng kết quả. Cũng vì thế mà
dấu ✓ ở bảng đầu ADR này là ✓ của một phép đo may mắn: hai đơn hôm ấy đều
một nhà bán.

Đã sửa cùng lúc vì nó nằm đúng trong đoạn mã đang chạm, và mẫu số của
`aov` nay tách khỏi `order_count`: một sự kiện thiếu `amount` là dữ liệu
hỏng của bên gọi, nó không được kéo AOV xuống, nhưng `order_count` vẫn
phải nói đúng số đơn đã đặt.

## Đánh đổi đã chấp nhận

**Mã lượt truy cập do client sinh thì bịa được.** Không có cách nào quanh
điều đó mà vẫn đo được nửa trên của phễu: nửa ấy chỉ tồn tại ở trình duyệt.
Giới hạn tần suất theo `session_id` đã có sẵn, và số liệu phân tích không
điều khiển tiền.

**Số liệu lịch sử không dựng lại được.** 27.111 sự kiện đã ghi mang mã giỏ
và mã phiên thanh toán; không có mã lượt truy cập nào để khôi phục. Phễu
chỉ đúng từ ngày nối xong trở đi. Đây là lý do bước 1 quan trọng: nó nói
thẳng ra rằng khoảng thời gian trước đó KHÔNG đo được, thay vì báo 0 và để
người đọc tưởng đã đo.

**Chỉ số mới `cart_conversion_rate` có thể bị nhầm với `conversion_rate`.**
Giảm rủi ro bằng cách đặt tên nói rõ mẫu số, và bằng mô tả đi kèm trong
`not_measured` của chỉ số kia.

## Liên quan

- P3-50, P3-51 trong [../10-roadmap/backlog.md](../10-roadmap/backlog.md) —
  cách lỗi này được tìm ra, hai lần, từ hai hướng khác nhau
- [0016-phien-ban-event.md](0016-phien-ban-event.md) — quy tắc tăng phiên
  bản payload, áp dụng cho bước 3
- `docs/04-modules/analytics.md` mục 13 — phễu từng bước thuộc Phase 2;
  ADR này KHÔNG mở phạm vi đó, nó chỉ làm cho hai chỉ số đã hứa nói thật
