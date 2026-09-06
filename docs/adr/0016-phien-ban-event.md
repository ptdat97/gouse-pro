# ADR-0016: Phiên bản event và thứ tự triển khai

**Trạng thái:** Accepted — **đã triển khai** (xem mục cuối)

---

## Bối cảnh

`Event.Version` tồn tại từ [ADR-0006](0006-internal-events.md), cùng cột
`event_version` trong `event_outbox`, mặc định 1. Nhưng nó chưa bao giờ
được ĐỌC: mọi handler `Unmarshal` thẳng payload và không hỏi phiên bản.

Một con số không ai đọc không phải cơ chế — nó là chú thích.

### Sự cố 19/08

Thêm địa chỉ giao vào `checkout.completed`. Một tiến trình worker CŨ còn
sống đã tiêu thụ event MỚI và **âm thầm bỏ qua** trường mới:

```text
bên phát (mới)  →  event có shipping_address
bên nhận (cũ)   →  struct không có trường đó
encoding/json   →  bỏ qua trường lạ, KHÔNG báo gì
kết quả         →  FulfillmentOrder có địa chỉ RỖNG
```

Không lỗi, không log. Seller in phiếu giao không có địa chỉ, và thứ đầu
tiên phát hiện ra là con người.

### Vì sao "bỏ qua trường lạ" vừa đúng vừa nguy hiểm

Nó là điều kiện để bên nhận CŨ sống sót qua một lần thêm trường — và
`TestBenNhanChiuDuocTruongLa` đang khóa đúng hành vi đó, có chủ ý. Bỏ nó
đi (`DisallowUnknownFields`) sẽ làm MỌI bên nhận vỡ ngay khi bên phát
được nâng cấp trước.

Vấn đề không phải việc bỏ qua. Vấn đề là **bỏ qua trong im lặng** khi
trường đó là thứ bên nhận BẮT BUỘC phải có. Hai tình huống nhìn giống hệt
nhau ở tầng JSON:

```text
trường vắng vì nó vốn tùy chọn        → bỏ qua là ĐÚNG
trường vắng vì mình là bản CŨ hơn     → bỏ qua là MẤT DỮ LIỆU
```

Chỉ có phiên bản mới phân biệt được hai câu đó.

### Vì sao không thể chỉ "trả lỗi"

Cơ chế thử lại hiện tại: mọi lỗi của handler đều tăng `attempts`, và
`attempts >= 5` thì `dead_lettered_at` — **vĩnh viễn**. Nên nếu bên nhận
cũ gặp event mới và trả lỗi, kết quả là năm lần thử trong vài giây rồi
event chết hẳn.

Điều đó BIẾN MỘT SỰ CỐ TẠM THỜI THÀNH MẤT DỮ LIỆU VĨNH VIỄN. Lệch phiên
bản là chuyện tự lành: nó biến mất ngay khi bản mới của bên nhận lên. Xử
lý nó bằng dead letter là chọn hậu quả nặng hơn chính vấn đề.

---

## Quyết định

**Bên nhận KHAI BÁO phiên bản cao nhất nó hiểu. Event mới hơn thế được
HOÃN, không phải bị bỏ.**

### Phần 1: khi nào tăng phiên bản

```text
TĂNG      thêm trường mà bên nhận BẮT BUỘC phải có để làm đúng việc
KHÔNG     thêm trường thuần hiển thị, không ai bắt buộc phải đọc
CẤM       đổi nghĩa của trường đã có
CẤM       xóa trường
```

Hai dòng CẤM không có phiên bản nào cứu được: một bên nhận cũ đọc trường
đã đổi nghĩa sẽ làm sai một cách tự tin. Muốn đổi nghĩa thì thêm trường
MỚI và bỏ trường cũ ở một đợt sau, khi không còn bên nhận nào đọc nó.

Ngưỡng "bắt buộc phải có" cố ý hẹp. Tăng phiên bản cho mọi lần thêm
trường sẽ bắt mọi bên nhận khai lại số sau mỗi thay đổi vặt, và một nghi
thức làm phiền mà không cứu được gì thì người ta sẽ tìm cách đi vòng.

### Phần 2: bên nhận khai báo, mặc định là 1

```go
type VersionedHandler interface {
    Handler
    MaxEventVersion(eventType string) int
}
```

Handler KHÔNG cài interface này được coi là hiểu tới **phiên bản 1**.

Mặc định đó là điểm mấu chốt: hôm nay mọi event đều là v1, nên không
handler nào phải sửa gì và hành vi không đổi. Nhưng ngày ai đó nâng
`checkout.completed` lên v2, mọi bên nhận của nó **phải khai báo** mới
nhận được — quên khai là hoãn ầm ĩ, không phải xử lý sai trong im lặng.

Mặc định ngược lại — "chưa khai thì hiểu mọi phiên bản" — sẽ dựng lại
đúng sự cố 19/08: bên nhận cũ vui vẻ nuốt event mới.

### Phần 3: HOÃN, không phải THẤT BẠI

```text
event.Version > handler.MaxEventVersion(type)

    → event GIỮ NGUYÊN trạng thái chờ
    → KHÔNG tăng attempts, KHÔNG dead letter
    → ghi log mức WARN kèm cả hai con số
    → tăng metric eventbus_hoan_lech_phien_ban
```

Event nằm lại hàng đợi cho tới khi một bên nhận đủ mới xử lý được nó.
`OldestPendingAge` (đã có, ngưỡng 60 giây) là thứ báo động — và nó báo
đúng thứ đang xảy ra: có event không ai xử lý được.

Sự khác biệt so với dead letter là **khả năng hồi phục**. Triển khai sai
thứ tự thì việc cần làm là đưa bên nhận mới lên; event tự chảy tiếp. Với
dead letter thì phải có người đi tìm và phát lại từng cái.

### Phần 4: thứ tự triển khai

**Bên nhận LÊN TRƯỚC bên phát.** Quy tắc này không đổi.

Cơ chế hoãn không thay thế nó — nó chỉ làm cho việc VI PHẠM nó trở thành
một sự chậm trễ nhìn thấy được thay vì một mất mát âm thầm.

---

## Phương án đã cân nhắc và vì sao bị loại

**Chuyển đổi payload cũ sang mới (upcasting) ở tầng eventbus.** Cách làm
kinh điển của event sourcing: giữ một chuỗi hàm nâng v1→v2→v3. Bị loại vì
ở đây event là phương tiện tích hợp giữa các module trong MỘT tiến trình,
không phải nguồn sự thật được phát lại từ đầu lịch sử. Event sống vài
giây rồi thành `published`; không có kho event cũ cần đọc lại. Dựng bộ
máy upcasting cho một cửa sổ vài giây là trả giá kiến trúc cho một vấn đề
triển khai.

**`DisallowUnknownFields` để bắt payload lạ.** Làm vỡ chiều tương thích
NGƯỢC — bên nhận cũ không sống nổi qua bất kỳ lần thêm trường nào, kể cả
lần thêm hoàn toàn vô hại. Đã có test khóa chiều này
(`TestBenNhanChiuDuocTruongLa`).

**Trả lỗi và để dead letter lo.** Đã phân tích ở mục Bối cảnh: biến sự cố
tự lành thành mất dữ liệu vĩnh viễn.

**Kiểm phiên bản trong từng handler.** Đúng chỗ để biết ngữ nghĩa, nhưng
sai chỗ để cưỡng chế: mười bên nhận là mười lần nhớ, và bên nhận quên
kiểm chính là bên nhận gây sự cố. Đặt ở dispatcher thì không ai quên
được.

---

## Hệ quả

**Tốt:**

- Lệch phiên bản thành sự kiện NHÌN THẤY ĐƯỢC (log + metric + báo động
  tồn đọng) thay vì dữ liệu sai âm thầm.
- Việc vi phạm thứ tự triển khai tự hồi phục.
- `Event.Version` từ chú thích thành cơ chế.

**Xấu:**

- Event bị hoãn được đọc lại mỗi nhịp worker, tốn truy vấn vô ích cho tới
  khi bên nhận được nâng cấp. Chấp nhận được: nó ồn ào có chủ đích, và
  cái giá đó thấp hơn hẳn việc mất dữ liệu.
- Quên khai `MaxEventVersion` khi nâng phiên bản làm event đứng lại. Đó
  là **chủ ý** — đứng lại và kêu to thì tốt hơn chạy tiếp và làm sai — và
  đó cũng là lý do ngưỡng tăng phiên bản được giữ hẹp.
- Một event bị hoãn vĩnh viễn (bên nhận không bao giờ được nâng cấp) sẽ
  nằm mãi trong hàng đợi. `OldestPendingAge` bắt được, nhưng không có cơ
  chế tự dọn — cố ý, vì tự dọn nghĩa là tự vứt dữ liệu.

---

## Đánh đổi đã chấp nhận

Đổi **sự im lặng** lấy **sự ồn ào**. Một event đứng lại làm báo động kêu
lúc 3 giờ sáng là phiền; một đơn hàng giao tới địa chỉ rỗng mà không ai
biết trong ba ngày thì đắt hơn nhiều.

Và đổi **tính đầy đủ** lấy **tính đơn giản**: cơ chế này KHÔNG xử lý được
việc đổi nghĩa hay xóa trường — nó chỉ cấm hai việc đó. Một hệ thống
tương thích thật sự đầy đủ cần upcasting, và cái giá của nó không xứng
với vòng đời vài giây của event ở đây.

---

## Liên quan

- [ADR-0006](0006-internal-events.md) — domain event và outbox, nơi
  `Version` ra đời
- [ADR-0013](0013-write-transaction-boundary.md) — ranh giới giao dịch mà
  dispatcher chạy trong đó
- `docs/10-roadmap/backlog.md` mục 2.8 (PH-7) — ba khoảng trống mà ADR
  này lấp

---

## Tình trạng triển khai

```text
✅ VersionedHandler + mặc định v1              internal/platform/eventbus/event.go
✅ Dispatcher hoãn thay vì thất bại            internal/platform/eventbus/dispatcher.go
✅ Metric eventbus_hoan_lech_phien_ban         internal/platform/metrics
✅ Test: hoãn KHÔNG tăng attempts              internal/platform/eventbus/eventbus_test.go
✅ Test: event tự chảy tiếp khi bên nhận mới lên
```
