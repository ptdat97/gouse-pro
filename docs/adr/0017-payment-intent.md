# ADR-0017: `payment_intent` — bản ghi số tiền CHỜ THU, và là thứ webhook đối chiếu vào

**Trạng thái:** Accepted — **đã triển khai** (xem mục cuối)

---

## Bối cảnh

Webhook **vận chuyển** đã cài xong (27/08). Webhook **thanh toán** thì
không, và `api/paths/webhooks.yaml` quy định ba lớp bảo vệ cho nó:

```text
1. Xác minh chữ ký HMAC          → cài được, đã có httpserver.KiemChuKyHMAC
2. Idempotency theo event_id     → cài được, đã có bảng webhook_event
3. ĐỐI CHIẾU SỐ TIỀN với         → KHÔNG cài được: không có gì để đối chiếu
   payment_intent trong hệ thống
```

Không có bảng `payment_intent` và không có `PaymentIntent` nào trong mã
nguồn. Module `payment` là một SỔ CÁI (ADR-0008), không phải phần tích hợp
cổng thanh toán.

### Vì sao không cài hai lớp trước rồi bổ sung lớp ba sau

Như thế là dựng đúng nửa nguy hiểm. Một endpoint nhận `payment.succeeded`
mà không đối chiếu số tiền sẽ **ghi nhận doanh thu theo con số bên ngoài
gửi vào**.

Chữ ký đúng chỉ chứng minh thông điệp đến từ nhà cung cấp. Nó KHÔNG chứng
minh nội dung khớp với thứ khách đã trả:

```text
lỗi tích hợp phía nhà cung cấp   → số tiền sai, chữ ký vẫn đúng
một khóa HMAC bị lộ              → kẻ tấn công ký được mọi con số
môi trường test gọi nhầm prod    → chữ ký đúng, đơn không tồn tại
```

Cả ba đều thành tiền ghi sai, và sổ cái là nơi bất biến — ghi sai rồi thì
phải ghi bút toán đảo, không xóa được.

### Thứ đang chờ sẵn

`Order.MarkPaid` tồn tại đủ ba tầng — domain, application, module — và
**không ai gọi**. Nó được viết cho đúng lúc này. Hôm nay mọi đơn nằm mãi ở
`PENDING_PAYMENT`, kể cả khi khách đã trả tiền.

---

## Quyết định

**`payment_intent` là bản ghi nội bộ của SỐ TIỀN HỆ THỐNG CHỜ THU cho một
đơn, tạo lúc hoàn tất phiên thanh toán, và là con số duy nhất webhook được
đối chiếu vào.**

### Phần 1: nó thuộc module `payment`

`docs/04-modules/payment.md` mục 8 đã liệt kê `payment_intent` trong dữ
liệu module này sở hữu. ADR này không phát minh khái niệm, nó cài cái đặc
tả đã khai.

### Phần 2: CHỈ tạo cho phương thức TRẢ TRƯỚC

```text
CARD · BANK_TRANSFER · E_WALLET   → tạo intent
COD                               → KHÔNG tạo
rỗng (đường placeOrder)           → KHÔNG tạo
```

COD không có cuộc trao đổi nào với cổng thanh toán, nên không có gì để đối
chiếu và không có webhook nào sẽ tới. Tạo một intent rồi để nó treo vĩnh
viễn là dựng ra một hàng đợi rác mà cảnh báo "intent quá hạn" sẽ kêu suốt.

Đây là chỗ P3-9 trả nợ: trước 06/09 hệ thống không biết đơn nào là COD,
nên quy tắc này không viết được.

### Phần 3: số tiền ĐÓNG BĂNG, lấy từ đơn

`amount` = tổng của đơn tại thời điểm đặt, cùng con số khách đã nhìn thấy.
KHÔNG tính lại tại thời điểm webhook về: nếu tính lại thì một thay đổi giá
giữa chừng làm phép đối chiếu tự nói dối, và cả lớp bảo vệ thành vô nghĩa.

### Phần 4: đối chiếu là SO SÁNH TUYỆT ĐỐI, lệch thì TỪ CHỐI

```text
webhook.amount == intent.amount  VÀ  webhook.currency == intent.currency
    → thu tiền: intent CAPTURED, order MarkPaid, phát payment.captured

ngược lại
    → 422, KHÔNG xử lý, ghi log mức ERROR, tăng metric
```

Không có ngưỡng sai số, không làm tròn. Tiền ở đây là số nguyên đơn vị nhỏ
nhất (ADR-0008), nên "gần bằng" không phải một khái niệm có thật — nó chỉ
là chỗ để một lỗi trốn qua.

Từ chối chứ không phải "ghi nhận rồi cảnh báo": ghi nhận nghĩa là đã tin,
và cảnh báo thì có người đọc lúc 3 giờ sáng hoặc không.

### Phần 5: KHÔNG dựng cổng thanh toán

`docs/04-modules/payment.md` mục 10 mô tả một interface `PaymentGateway`
(`CreateIntent`, `Capture`, `Refund`, `VerifyWebhook`). ADR này **không**
cài nó, vì chưa có nhà cung cấp nào để cài adapter — một interface không
có bên cài là mã chết, và quy tắc nhận việc của dự án cấm dựng theo lý do
"sau này có thể cần".

Hệ quả cụ thể: `payment_intent.id` là mã của CHÚNG TA (`pin_...`).
`provider` và `provider_intent_id` để trống cho tới khi có PSP thật; khi
đó adapter điền vào, và webhook tra intent theo mã nhà cung cấp thay vì mã
nội bộ. Cột đã có sẵn nên đó là thay đổi cộng thêm, không phải migration
phá vỡ.

### Phần 6: vòng đời intent

```text
REQUIRES_PAYMENT ──→ CAPTURED    webhook payment.succeeded, số tiền khớp
                 ──→ FAILED      webhook payment.failed
                 ──→ CANCELLED   đơn bị hủy trước khi trả tiền

CAPTURED là trạng thái CUỐI: tiền đã về.
```

Không có đường từ `CAPTURED` đi đâu nữa. Hoàn tiền là một bản ghi KHÁC
(`refund`, mục 8 của đặc tả), không phải một lần chuyển trạng thái ngược —
cùng nguyên tắc bất biến với sổ cái.

---

## Phương án đã cân nhắc và vì sao bị loại

**Đối chiếu thẳng với `order.total`, bỏ hẳn bảng intent.** Hấp dẫn vì ít
bảng hơn, và với luồng hôm nay thì đủ. Bị loại vì nó không có chỗ cho hai
thứ chắc chắn tới: trả GÓP hoặc trả một phần (số tiền một lần thu KHÁC
tổng đơn), và mã giao dịch của nhà cung cấp. Quan trọng hơn, nó không phân
biệt được "đơn này chờ thu tiền" với "đơn này COD, không chờ gì cả" — mà
đó chính là câu hỏi job đối chiếu định kỳ phải trả lời.

**Tin số tiền trong webhook, chỉ dùng chữ ký làm hàng rào.** Là trạng thái
hiện tại nếu cài hai lớp đầu. Đã phân tích ở mục Bối cảnh: chữ ký chứng
minh NGUỒN, không chứng minh NỘI DUNG.

**Tạo intent cho cả COD để "nhất quán".** Nhất quán về hình dạng, sai về
nghĩa: không có cuộc trao đổi nào với cổng thanh toán để mà có ý định.
Hàng nghìn intent không bao giờ được thu sẽ làm mọi cảnh báo dựa trên tồn
đọng trở nên vô dụng — và một cảnh báo luôn kêu thì không ai đọc.

**Chuyển ghi nhận doanh thu sang lúc thu tiền.** Xem mục Hệ quả xấu; đó là
thay đổi lớn hơn ADR này và động tới cả COD.

---

## Hệ quả

**Tốt:**

- Lớp bảo vệ thứ ba của `webhooks.yaml` cài được → webhook thanh toán mở
  khóa (PH-36).
- `Order.MarkPaid` hết là mã chết; đơn trả trước không còn kẹt vĩnh viễn ở
  `PENDING_PAYMENT`.
- Có chỗ để job đối chiếu định kỳ (yêu cầu 5) đứng: danh sách intent
  `REQUIRES_PAYMENT` quá hạn chính là danh sách cần đi hỏi PSP.

**Xấu, và cần ghi rõ:**

- **Doanh thu vẫn được ghi sổ lúc `checkout.completed`, TRƯỚC khi tiền
  về.** ADR này không đổi điều đó. Nghĩa là một đơn trả trước bị bỏ dở vẫn
  để lại bút toán doanh thu cho khoản tiền chưa bao giờ tới. Sửa đúng phải
  chuyển ghi nhận sang lúc thu tiền, mà điều đó động tới cả COD (tiền về
  lúc giao hàng, không phải lúc đặt) — việc lớn hơn hẳn, và là quyết định
  của chủ dự án. **Ghi vào backlog, không giấu.**
- Yêu cầu 5 của `webhooks.yaml` đạt MỘT NỬA. Nửa nội bộ — hệ thống tự
  phát hiện bất nhất mà không cần hỏi ai — nay có ở cả hai webhook. Nửa
  ĐI HỎI nhà cung cấp vẫn thiếu, và nó là nửa duy nhất phân biệt được
  "webhook mất" với "nhà cung cấp chưa gửi".
- `provider_intent_id` để trống cho tới khi có PSP thật, nên hôm nay
  webhook phải tra intent bằng mã nội bộ. Đó là đường mà PSP thật sẽ
  không dùng.

---

## Đánh đổi đã chấp nhận

Dựng một bảng cho một luồng chưa có nhà cung cấp thật. Cái giá là một mức
gián tiếp chưa ai dùng hết; đổi lại là **endpoint webhook không bao giờ
tồn tại ở trạng thái tin-mọi-con-số**. Thứ tự đó quan trọng: một khi
endpoint đã mở mà thiếu lớp ba, lớp ba sẽ được "bổ sung sau" — và trong
lúc đó nó đang chạy thật.

---

## Liên quan

- [ADR-0008](0008-financial-ledger.md) — sổ cái bất biến; lý do ghi sai
  không xóa được
- [ADR-0016](0016-phien-ban-event.md) — phiên bản event, áp dụng cho
  `payment.captured` khi payload đổi
- `docs/04-modules/payment.md` mục 8, 9, 10 — nơi `payment_intent` được
  đặc tả từ đầu
- `api/paths/webhooks.yaml` — năm yêu cầu bắt buộc của webhook
- `docs/10-roadmap/backlog.md` PH-36

---

## Tình trạng triển khai

```text
✅ Bảng payment_intent + ràng buộc     migrations/000042
✅ Domain PaymentIntent + vòng đời      internal/modules/payment/domain
✅ Tạo intent lúc hoàn tất phiên        chỉ phương thức TRẢ TRƯỚC
✅ Webhook + ba lớp bảo vệ              internal/modules/payment/interfaces/http
✅ Thu tiền → MarkPaid + event
✅ Test: lệch số tiền bị TỪ CHỐI
🟡 Job đối chiếu định kỳ (yêu cầu 5)    nửa NỘI BỘ xong cho CẢ HAI webhook
                                       (thanh toán 06/09, vận chuyển 07/09);
                                       nửa ĐI HỎI nhà cung cấp vẫn thiếu
⬜ Adapter PSP thật                     chưa có nhà cung cấp
```
