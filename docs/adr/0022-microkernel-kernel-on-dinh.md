# ADR-0022 — Microkernel: kernel ỔN ĐỊNH, business là tính năng bổ sung

**Trạng thái:** Đã chấp nhận · 01/10/2026

## Bối cảnh

[ADR-0021](0021-ranh-gioi-commerce-kernel.md) đã **đo** ranh giới kernel từ
bao đóng phụ thuộc của `checkout`, và trả lời câu hỏi "marketplace có phải
extension không" (không — nó nằm trên đường mua hàng).

ADR ấy trả lời một câu hỏi phân loại. Nó **không** nói hệ thống nên tiến hóa
theo hướng nào, và hệ quả là hai chỗ trong tài liệu đang nói ngược nhau:

```text
docs/README.md    ARCHITECTURE FREEZE, danh sách đóng băng gồm:
                  "✗ plugin system"  ·  "✗ module mới"

ADR-0021          phép thử nghiệm thu là DỰNG module mới (affiliate) và đo
                  nó cần bao nhiêu dòng core
```

Một người đọc tài liệu rồi đi dựng extension sẽ gặp một lệnh cấm; một người
đọc ADR-0021 rồi làm đúng thế thì được khen. Mâu thuẫn ấy không phải chuyện
nhỏ: nó quyết định câu trả lời cho mọi yêu cầu tính năng sắp tới.

### Hai phép thử đã chạy, và kết quả của chúng

```text
chặng 1 (25/09)  nối chuỗi quy công giỏ→phiên→đơn
                 44 dòng core, 1 module, 1 migration — khoản MỘT LẦN

chặng 2 (27/09)  dựng module `affiliate` thật
                 7 dòng core bắt buộc, HẾT ở mặt ĐỌC của `order`
                 0 module kernel nào biết affiliate tồn tại
```

Bảy dòng ấy là giá của việc **bày ra** dữ liệu kernel đã có
(`order_line.attributed_creator_id` nằm trong bảng từ tháng 8 mà không mặt
đọc nào lấy được), không phải giá của việc dạy kernel về hoa hồng creator.

Đo lại 01/10/2026, sau khi `affiliate` đã chạy:

```text
20 module trong mã

TRONG bao đóng của checkout — 11
    cart · catalog · checkout · fulfillment · inventory
    marketplace · order · payment · product · promotion · seller

NGOÀI — 9
    affiliate · analytics · customer · identity
    notification · pricing · recommendation · returns · supplychain
    (`pricing` vào đường mua hàng từ 01/10 — ADR-0023)

KHÔNG AI import (tháo ra được) — 9
    affiliate · analytics · customer · notification · pricing
    recommendation · returns · supplychain   (+ checkout, là GỐC)
```

Con số quan trọng nhất ở đây là **9 module không ai import**. Nó không phải
mục tiêu ai đặt ra — nó là kết quả của việc tuân thủ R1/R5 suốt nhiều tháng,
và nó có nghĩa: chín module ấy tháo ra được mà hệ thống vẫn bán được hàng.

## Quyết định

**Gouse là một microkernel thương mại điện tử.** Kernel ổn định; mọi mô hình
kinh doanh là tính năng bổ sung dựng bên ngoài nó.

Ba tầng, định nghĩa bằng câu hỏi **đo được** chứ không bằng ý kiến:

| Tầng | Câu hỏi | Hệ quả |
|---|---|---|
| **Kernel — đường mua hàng** | Tháo ra thì khách còn mua được hàng không? | Không. Nằm trong bao đóng của `checkout` |
| **Kernel — năng lực cắt ngang** | Mọi mô hình kinh doanh đều cần nó? | Có. Ngoài bao đóng nhưng không tháo được |
| **Extension** | Tháo ra thì mất một TÍNH NĂNG, không mất việc bán hàng | Không module nào import nó |

Phân loại đầy đủ nằm ở ADR-0021 và
[03-architecture/microkernel.md](../03-architecture/microkernel.md). Tài liệu
sau là nơi tra cứu; ADR này là nơi ghi **vì sao**.

### Kernel ổn định nghĩa là gì — ba điều cụ thể

**1. Kernel không bao giờ có nhánh theo mô hình kinh doanh.**

```go
// CẤM, và `cmd/archcheck` R1+R5+R6 chặn các đường dẫn tới nó
if donHang.LaLivestream { ... }
if offer.KieuBan == "preorder" { ... }
```

Một `if` như vậy là lời thú nhận rằng primitive chọn sai. Cách đúng là tìm
primitive đủ tổng quát — `Offer` thay cho "sản phẩm của nhà bán" là ví dụ đã
làm (ADR-0007).

**2. Kernel CHỞ định danh, extension sở hữu ý nghĩa.**

Đây là quy tắc đã trả giá để học. `order_line.attributed_creator_id` là một
**mã** — kernel không tra nó, không biết creator là gì.
`order_line.creator_commission_rate` thì là một **tỷ lệ tiền**, tức ý nghĩa
kinh doanh, và nó đã bị bỏ ở migration 000059 sau khi đo được 3298 dòng
`order_line` với 0 dòng khác 0 (P3-83).

```text
kernel được chở    mã creator, mã nội dung, mã chiến dịch
kernel KHÔNG chở   tỷ lệ hoa hồng, bên chịu chi phí, mô hình quy công
```

Quy tắc này đã được áp dụng HAI lần theo hai chiều ngược nhau, và cặp ấy là
cách đọc nó rõ nhất:

```text
BỎ   order_line.creator_commission_rate   migration 000059 — một TỶ LỆ TIỀN
THÊM order_line.attributed_content_id     migration 000060 — một MÃ
```

Cùng bảng, cùng tuần, ngược chiều — và cả hai đều theo đúng một quy tắc.

**3. Thêm vào kernel phải là PRIMITIVE, không phải tính năng.**

Thước đo: *mọi* mô hình kinh doanh đều cần nó, hay chỉ mô hình đang làm?

```text
PRIMITIVE  `shipping_method` trong phản hồi checkout (P3-73) — mọi mô hình
           bán hàng vật lý đều cần biết cách giao

TÍNH NĂNG  `creator_commission_rate` trên dòng đơn — chỉ creator commerce
           cần, và nó có nhà rồi: bảng `attribution` của affiliate
```

### Hướng tiến hóa: extension KHÔNG còn bị đóng băng

Danh sách đóng băng của `docs/README.md` được sửa theo ADR này:

```text
TRƯỚC    ✗ module mới            (mọi module)
SAU      ✓ module EXTENSION mới  (có điều kiện, xem dưới)
         ✗ module KERNEL mới     (vẫn đóng băng)
```

Điều kiện dựng một extension mới — cả bốn, không chọn ba:

```text
1. Tài liệu module khai `**Tầng (microkernel)**` và mục "Dữ liệu sở hữu"
   TRƯỚC khi viết mã. archcheck ĐỎ nếu thiếu.
2. KHÔNG module nào import nó. archcheck R10a cưỡng chế.
3. Chỉ chạm bảng CỦA MÌNH. archcheck R6 cưỡng chế.
4. Nhận dữ liệu qua event, hoặc đọc qua cổng công khai của kernel.
   Chiều extension → kernel được phép; chiều ngược lại KHÔNG.
```

Phần còn lại của danh sách đóng băng **giữ nguyên**: microservices, GraphQL,
gRPC, message broker, workflow engine, rule engine, service extraction,
generic abstraction — tất cả vẫn cấm.

### Đây KHÔNG phải plugin system, và đó là chủ ý

Dòng `✗ plugin system` trong danh sách đóng băng **vẫn đúng**, nên ADR này
không bỏ nó. Microkernel ở đây là một **cách phân tầng**, không phải một cơ
chế nạp mã lúc chạy:

| Microkernel ở Gouse | Plugin system |
|---|---|
| Ranh giới ĐO được từ mã | Ranh giới khai trong một sổ đăng ký |
| Extension nối dây ở `internal/app` và `cmd/worker` lúc biên dịch | Nạp lúc chạy |
| Cưỡng chế bằng `cmd/archcheck` | Cưỡng chế bằng kiểm tra lúc chạy |
| Không có `Registry`, không có vòng đời module | Có cả hai |

ADR-0021 đã từ chối dựng `Registry` và vòng đời module, với lý do: *"tới khi
có người dùng thứ hai"*. Quyết định ấy **giữ nguyên**. Dựng cơ chế trừu tượng
cho một người dùng là tạo trước thứ phải bảo trì, và dự án này đã có đủ ví dụ
về thứ khai ra mà không ai dùng (P3-76: bốn hằng số event; P3-83: một cột tỷ
lệ tiền).

### Vì sao không gọi là "modular monolith" nữa

Vẫn gọi. Hai từ ấy nói về **cách đóng gói** — một tiến trình, nhiều module,
`internal/` của Go làm hàng rào. ADR-0001 không đổi.

Microkernel nói về **cách phân tầng** bên trong cái gói đó: module nào ổn
định, module nào thay đổi theo mô hình kinh doanh. Hai khái niệm trả lời hai
câu khác nhau, và dùng cả hai là đúng:

```text
Modular monolith   TRIỂN KHAI như thế nào   → một binary, ranh giới biên dịch
Microkernel        PHÂN TẦNG như thế nào    → kernel ổn định + extension
```

## Những gì KHÔNG làm

### Không đổi `pricing` thành kernel hay extension

Nó vẫn chưa xếp được, đúng như ADR-0021 ghi: `price_constraint` có mục đích
*"khung giá ràng buộc seller"* mà `marketplace` không import `pricing`, nên
khung giá ấy hôm nay không cưỡng chế gì. Xếp nó vào đâu phải chờ quyết định
"ai cưỡng chế khung giá" (P3-79).

Xếp trước rồi sửa sau tệ hơn để trống: một module được gán nhãn "kernel" sẽ
được người sau tin là ổn định.

> **Cập nhật 01/10/2026:** câu hỏi ấy đã được trả lời —
> [ADR-0023](0023-khung-gia-cuong-che-luc-ghi.md) chọn chặn lúc GHI, và
> `pricing` thành KERNEL. Việc ADR này KHÔNG đoán trước là đúng: quyết định
> cần một câu trả lời nghiệp vụ, không phải một phép suy từ đồ thị phụ thuộc.
>
> Trạng thái `CHƯA XẾP ĐƯỢC` vẫn là trạng thái hợp lệ của bảng phân loại, và
> R10 vẫn không kết luận gì về module ở trạng thái ấy.

### Không tách kernel thành repo hay service riêng

Cùng lý do ADR-0009 hoãn tách service: ranh giới đo được là điều kiện CẦN để
tách, không phải lý do để tách. Tách khi có vấn đề thật về quy mô hoặc nhịp
phát hành, và lúc đó ranh giới này đã sẵn.

### Không viết lại 28 tài liệu module

Mỗi tài liệu module nhận **một dòng** ghi tầng của nó. Viết lại nội dung theo
từ vựng microkernel là một vòng sửa lớn mà không thêm sự thật nào — và tài
liệu càng sửa nhiều càng xa mã.

## Cưỡng chế

Một quy tắc viết trong tài liệu mà không ai cưỡng chế là một quy tắc sẽ bị
phá — đó là bài học của R6 (sở hữu bảng đã nằm trong
`dependency-rules.md` mục 9 nhiều tháng mà chưa bao giờ được cài).

Nên ranh giới của ADR này được gác bằng mã:

| Quy tắc | Gác bởi | Trạng thái |
|---|---|---|
| Chỉ import qua `public.go` | archcheck R1 | đang chạy |
| Không phụ thuộc vòng | archcheck R5 | đang chạy |
| Chỉ chạm bảng của mình | archcheck R6 | đang chạy |
| Kernel không dùng SDK bên thứ ba | archcheck R9 | đang chạy |
| **Module EXTENSION không bên nào import** | **archcheck R10a** | **ADR này** |
| **Tài liệu khai KERNEL phải CÓ hình dạng kernel** | **archcheck R10c** | **ADR này** |
| **Mọi tài liệu module khai tầng của nó** | **archcheck, LỖI khởi động** | **ADR này** |

R10 là quy tắc MỚI mà ADR này đòi. R5 chặn phụ thuộc VÒNG, nhưng nó không
chặn một phụ thuộc một chiều `checkout → notification` — đồ thị ấy vẫn là
DAG, và nó phá ranh giới microkernel mà không vi phạm quy tắc nào hiện có.

### R10 phát biểu theo chiều NGƯỢC, và đó là chủ ý

Không phải *"kernel không import extension"* mà **"extension không bên nào
import"**. Mạnh hơn: nó cũng chặn extension import extension — thứ làm hai
extension dính nhau rồi không tháo riêng được cái nào.

### Bản đầu của R10 là một lập luận VÒNG

Bản đầu suy tập extension bằng bao đóng từ `checkout`: cái gì ngoài bao đóng
là extension. Phép phá đầu tiên cho thấy nó vô dụng — thêm
`checkout → notification` thì `notification` vào bao đóng, tức thành kernel,
tức công cụ báo **OK**.

> Một quy tắc suy ra ranh giới TỪ thứ nó đang gác thì không gác được gì.

Bản hiện tại tách hai việc: **phân loại là QUYẾT ĐỊNH**, **tính nhất quán là
phép ĐO**.

Và quyết định ấy ghi ở **TÀI LIỆU MODULE**, không ở một sổ trong Go — hàng
`| **Tầng (microkernel)** | … |` ở bảng đầu `docs/04-modules/<module>.md`,
cùng khuôn R6 đọc mục "Dữ liệu sở hữu". Bản giữa giữ sổ trong Go *và* hàng
trong tài liệu, tức hai nguồn cho một phân loại — đúng dạng lỗi ADR này đang
nói về. Bỏ sổ.

Tài liệu thiếu hàng ấy, hoặc ghi một giá trị lạ, là **LỖI khởi động** chứ
không phải vi phạm: không có phân loại đáng tin thì R10 không kiểm được gì,
và một quy tắc im lặng không kiểm gì vẫn in ra `OK`.

## Đánh đổi đã chấp nhận

**Phân loại dựa trên bao đóng của `checkout` là một lựa chọn, không phải sự
thật tự nhiên.** Nếu một ngày `checkout` import thêm một module, module ấy tự
động thành kernel theo định nghĩa này — kể cả khi nó là tính năng. Hàng rào
chống điều đó không phải công cụ mà là mục "Khi nào được sửa Kernel" của
ADR-0021: thêm phụ thuộc cho `checkout` là một quyết định cần ADR.

**`identity` và `customer` là kernel theo ý kiến, không theo đo.** Khách vãng
lai mua được hàng mà không cần cả hai, nên bao đóng không chứa chúng. Xếp
chúng vào kernel là một phán đoán: mọi mô hình kinh doanh đều cần biết ai
đang gọi, và `customer_consent` là bản ghi pháp lý (P3-54). Phán đoán ấy ghi
ra để sau này bàn lại được.

**Extension đọc kernel đồng bộ tạo ghép nối thời gian.** `affiliate` gọi
`order.GetOrder` trong lúc xử lý event, nên `order` chậm làm `affiliate`
chậm. Chấp nhận vì nó chạy ở worker, ngoài đường thanh toán của khách — và
vì đường kia (nhồi dòng hàng vào payload `order.paid`) bắt payment và
fulfillment khai lại `MaxEventVersion` cho một trường chỉ affiliate dùng
(ADR-0016).

## Liên quan

- [ADR-0021](0021-ranh-gioi-commerce-kernel.md) — ranh giới đo được, `Offer` là primitive
- [ADR-0001](0001-modular-monolith.md) — modular monolith là cách ĐÓNG GÓI
- [ADR-0005](0005-module-boundaries.md) — ranh giới module tường minh
- [ADR-0009](0009-service-extraction.md) — hoãn tách service
- [ADR-0016](0016-phien-ban-event.md) — phiên bản event, và cái giá của nó
- [03-architecture/microkernel.md](../03-architecture/microkernel.md) — bảng tra cứu đầy đủ
