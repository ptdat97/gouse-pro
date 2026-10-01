# Tài liệu Kiến trúc — Fashion Commerce Platform

Đây là bộ tài liệu kiến trúc cho một **Nền tảng Thương mại Thời trang** (Fashion Commerce Platform), không phải một website thương mại điện tử thông thường.

## Nguyên tắc đọc tài liệu

Tài liệu được xây dựng theo thứ tự **nghiệp vụ trước, kỹ thuật sau**. Nếu bạn đọc lần đầu, hãy đi theo thứ tự sau:

1. `00-overview/` — Tầm nhìn, mô hình kinh doanh, từ điển thuật ngữ, nguyên tắc
2. `01-business/` — Các tác nhân nghiệp vụ và mô hình doanh thu
3. `02-domain/` — Bản đồ miền, bounded context, aggregate, entity, domain event
4. `03-architecture/` — Kiến trúc tổng thể, modular monolith, quy tắc phụ thuộc
5. `04-modules/` — Đặc tả từng module
6. `05-data/` — Kiến trúc dữ liệu, nhất quán, idempotency, audit
7. `06-api/` — Chuẩn API và các nhóm API theo đối tượng
8. `07-workflows/` — Luồng nghiệp vụ quan trọng (sequence diagram)
9. `08-frontend/` — Kiến trúc Next.js
10. `09-operations/` — Vận hành, bảo mật, quan sát hệ thống
11. `10-roadmap/` — Lộ trình MVP → Phase 4
12. `11-oss/` — Nghiên cứu mã nguồn mở và tổng hợp kiến trúc
13. `adr/` — Các quyết định kiến trúc và lý do

## Kiến trúc tổng thể (một hình duy nhất)

Gouse là một **microkernel thương mại điện tử**: kernel ổn định, mọi mô hình
kinh doanh là tính năng bổ sung dựng bên ngoài nó.

```text
                      NGƯỜI DÙNG
                          │
                    Next.js UI
              (Chỉ trình bày — Presentation only)
                          │
                    HTTP / API
                          ▼
  ┌──────────────── GO BACKEND (một binary) ────────────────┐
  │                                                         │
  │   ┌──────────── EXTENSION — tháo ra được ───────────┐   │
  │   │  affiliate · returns · notification             │   │
  │   │  analytics · recommendation · supplychain       │   │
  │   └───────────────────────┬─────────────────────────┘   │
  │                           │ event · cổng công khai      │
  │                           ▼   (MỘT chiều)               │
  │   ┌──────────────── KERNEL — ổn định ───────────────┐   │
  │   │  đường mua hàng:  cart catalog checkout          │   │
  │   │    fulfillment inventory marketplace order       │   │
  │   │    payment product promotion seller              │   │
  │   │  cắt ngang:  identity customer                   │   │
  │   └──────────────────────────────────────────────────┘   │
  │                           │                              │
  │   ┌──────────────── PLATFORM ───────────────────────┐   │
  │   │  eventbus database opsconfig audit token metrics │   │
  │   └──────────────────────────────────────────────────┘   │
  └─────────────────────────────────────────────────────────┘
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
          Database     Storage    External APIs
```

Mũi tên chỉ đi **xuống**: extension đọc kernel, kernel KHÔNG biết extension
tồn tại. Ranh giới ấy ĐO được và được cưỡng chế bằng `cmd/archcheck` R10.

Chi tiết: [03-architecture/microkernel.md](03-architecture/microkernel.md) ·
lý do: [ADR-0022](adr/0022-microkernel-kernel-on-dinh.md) · ranh giới đo
được: [ADR-0021](adr/0021-ranh-gioi-commerce-kernel.md).

## Tài liệu nói gì, code nói gì

> **Tài liệu mô tả kiến trúc dự định và các ranh giới. Code, test và hành vi
> thật trên production là thẩm quyền cuối cùng. Khi triển khai cho thấy một
> giả định là sai, hãy cập nhật ADR và tài liệu — đừng ép code chạy theo
> tài liệu.**

Kiến trúc phục vụ việc triển khai, không phải ngược lại. Xem nguyên tắc P17
tại [00-overview/principles.md](00-overview/principles.md).

Điều này KHÔNG có nghĩa là tài liệu tùy tiện sửa cho khớp code. Khi sửa,
phải ghi lại **vì sao** giả định ban đầu sai — đó mới là thứ có giá trị cho
người đọc sau.

**Quy tắc bất biến:**

- Next.js **không** chứa logic nghiệp vụ. Không truy cập database trực tiếp.
- Go backend là nơi duy nhất sở hữu domain logic, dữ liệu và giao dịch.
- Bắt đầu bằng **Modular Monolith**, không phải microservices.
- Mọi năng lực đều được lộ ra qua API trước (API First).
- **Kernel ổn định; mô hình kinh doanh là EXTENSION.** Kernel chở định danh,
  extension sở hữu ý nghĩa. Không `if marketplace`, không `if livestream`.

## ARCHITECTURE FREEZE

**Hiệu lực từ 14/08/2026 · Xác nhận lại 15/08/2026**

```text
Architecture:  FROZEN
Domain:        FROZEN
MVP Scope:     FROZEN
API:           IMPLEMENTATION IN PROGRESS
Code:          PRIMARY FOCUS
Tests:         REQUIRED
```

**Kiến trúc hiện tại ĐỦ để triển khai. Dừng thêm trừu tượng hóa.**

Quy trình bắt buộc khi cần đổi kiến trúc:

```text
Implementation  →  Vấn đề THẬT  →  ADR  →  Architecture change
```

Không phải:

```text
Khả năng tương lai  →  Trừu tượng hóa mới
```

### Danh sách đóng băng — không tự ý thêm

```text
✗ microservices              ✗ workflow engine tổng quát
✗ plugin system              ✗ rule engine tổng quát
✗ GraphQL                    ✗ message broker
✗ gRPC                       ✗ service extraction
✗ generic abstraction        ✗ module KERNEL mới
```

Trừ khi **triển khai thực tế** chứng minh là cần, và có ADR.

### Một ngoại lệ: module EXTENSION mới ĐƯỢC phép

Dòng `✗ module mới` của bản trước đã đổi thành `✗ module KERNEL mới`, theo
[ADR-0022](adr/0022-microkernel-kernel-on-dinh.md).

Lý do: hai phép thử đã chạy (chặng 1 và 2 của ADR-0021) cho thấy dựng một
extension thật tốn **7 dòng** ở kernel, và 9 trong 20 module hiện tại không
bên nào import — tức chúng thật sự tháo ra được. Lệnh cấm cũ làm người đọc
tài liệu và người đọc ADR-0021 nhận hai câu trả lời ngược nhau.

Điều kiện — cả năm, không chọn bốn:

```text
1. Tài liệu `docs/04-modules/<ten>.md` khai HAI thứ TRƯỚC khi viết mã:
     | **Tầng (microkernel)** | EXTENSION — đã dựng |
     mục "N. Dữ liệu sở hữu" kèm khối SQL
                                       archcheck ĐỎ nếu thiếu
2. KHÔNG module nào import nó         archcheck R10a
3. Chỉ chạm bảng CỦA MÌNH             archcheck R6
4. Nhận dữ liệu qua event, hoặc đọc qua cổng công khai của kernel
```

Câu *"tháo nó ra thì mất gì"* chính là tiêu chí phân loại, và viết nó vào tài
liệu buộc người thêm module phải trả lời nó.

`✗ plugin system` **vẫn đúng** và không bỏ: microkernel ở đây là cách phân
tầng, không phải cơ chế nạp mã lúc chạy. Không `Registry`, không vòng đời
module — ADR-0021 đã từ chối cả hai, *"tới khi có người dùng thứ hai"*.

### Không mở rộng domain

Bảy nhóm bounded context hiện tại là đủ:

```text
Commerce · Marketplace · Inventory · Financial
Growth · Supply Chain · Platform
```

Không tạo bounded context mới, và không tạo module KERNEL mới. Module
EXTENSION thì được, theo điều kiện ở mục trên — một extension nằm trong một
bounded context đã có, nó không mở rộng domain.

Tính năng chưa cần cho MVP được đánh dấu `FUTURE` trong
[10-roadmap/backlog.md](10-roadmap/backlog.md) mục 6.

### Điều VẪN nên làm

```text
✓ Cập nhật tài liệu khi triển khai cho thấy giả định sai (P17)
✓ Ghi ADR mới khi gặp quyết định kiến trúc THẬT trong lúc code
✓ Bổ sung chi tiết còn thiếu của module chưa triển khai
✓ Sửa mâu thuẫn giữa tài liệu và code
```

### Thước đo một thay đổi kiến trúc có chính đáng không

Trả lời được câu này thì làm, không thì đừng:

> **Vấn đề cụ thể nào trong code hiện tại buộc phải thay đổi này?**

"Sẽ cần sau này", "chuẩn hơn", "OSS khác làm thế" — đều không phải câu trả lời.

### Quy tắc bao trùm

> **Tính năng đã có trong docs → xây nó.
> Chưa có trong docs → không tự thiết kế thêm; đề xuất ADR khi thật sự cần.**

---

## Current Implementation Phase

**Dự án đã rời giai đoạn thiết kế. Trọng tâm bây giờ là CODE.**

```text
Architecture / Design Phase   →   Implementation Completion Phase
        (kết thúc)                      (đang ở đây)
```

Mục tiêu duy nhất:

> Làm cho toàn bộ kiến trúc MVP đã thiết kế trở thành một hệ thống Go chạy
> được end-to-end.

### Khoảng trống thật, đo từ code

Đo lại 01/10/2026 bằng `cmd/archcheck -v` và `cmd/apicheck`:

```text
Module trong mã                        20
  kernel                               14   (12 đường mua hàng + 2 cắt ngang)
  extension                             6
  chưa xếp được                         0
Module có tầng HTTP                    14/20
Thao tác trong OpenAPI                100
Tuyến HTTP khớp đặc tả                 89   (15 hoãn, đã khai)
```

Chỗ nghẽn cũ — *"3/17 có tầng HTTP, 10/71 operation"* của 15/08/2026 — đã
hết. Đường `Application → HTTP → OpenAPI → Next.js` nay thông cho phần lớn
hệ thống.

Chỗ nghẽn hiện tại nằm ở **cuối chuỗi tiền và hàng**, không ở đường nối:

```text
fulfillment_order trên database phát triển, đo 01/10/2026:
  PENDING       3260      nhà bán chưa bấm gì
  HANDED_OVER     37      đã rời kho, chưa ai xác nhận đã giao
  COMPLETED        1      đi hết chuỗi — đúng MỘT đơn, do test tích hợp

NeedsReview của khung giá chưa đi đâu — chưa có hàng đợi rà soát (P3-88)
đợt đối soát nằm DRAFT vĩnh viễn — chưa có đường ký chi (P3-89, đã quyết)
```

Con số 3260/37/1 là dữ liệu phát triển, không phải production — nhưng nó cho
thấy đoạn CUỐI của chuỗi chưa có ai đi qua nhiều lần, nên nó là đoạn ít bằng
chứng nhất.

### Định nghĩa "xong"

Một tính năng CHỈ xong khi có đủ:

```text
Domain → Application → Infrastructure → API → OpenAPI
       → Integration test → Architecture check
```

**Viết xong domain + application KHÔNG phải là xong.** Đó là lý do 17/17
module "có logic" nhưng chỉ 14% API dùng được.

### Backlog duy nhất

Mọi việc còn phải làm nằm ở **[10-roadmap/backlog.md](10-roadmap/backlog.md)**
— P0 (blocking) · P1 (core MVP) · P2 (integration) · P3 (hardening) ·
FUTURE. Đừng tạo backlog thứ hai.

---

## Trạng thái tài liệu

| Nhóm | Trạng thái | Ghi chú |
|---|---|---|
| 00-overview | Hoàn thành | Nền tảng, cần đọc trước |
| 01-business | Hoàn thành | Định nghĩa tác nhân nghiệp vụ |
| 02-domain | Hoàn thành | Là "hợp đồng" của toàn hệ thống |
| 03-architecture | Hoàn thành | Quy tắc bắt buộc khi code |
| 04-modules | Hoàn thành | 28 tài liệu · 20 module có mã · mỗi tài liệu ghi TẦNG của nó |
| 05-data | Hoàn thành | |
| 06-api | Hoàn thành | |
| 07-workflows | Hoàn thành | |
| 08-frontend | Hoàn thành | |
| 09-operations | Hoàn thành | |
| 10-roadmap | Hoàn thành | |
| 11-oss | Hoàn thành | 12 dự án + 2 mô hình kinh doanh |
| adr | Hoàn thành | 22 ADR — ADR-0006 và ADR-0010 có mục "trạng thái triển khai" |

**Tài liệu kiến trúc đã ĐÓNG BĂNG.** Bảng trên không còn là việc phải làm —
nó chỉ để tra cứu. Việc phải làm nằm ở
[10-roadmap/backlog.md](10-roadmap/backlog.md).

**Đối chiếu với code (01/10/2026):** 14/20 module có tầng HTTP, 89/100 thao
tác đặc tả có tuyến thật. Khoảng cách "đã thiết kế" vs "chạy được" của
15/08/2026 — *3/17 HTTP, 10/71 operation* — đã đóng.

Chi tiết: [10-roadmap/backlog.md](10-roadmap/backlog.md) (việc còn lại) ·
[10-roadmap/todo.md](10-roadmap/todo.md) (việc đã làm).

Những chỗ tài liệu đã được sửa cho khớp code thật, kèm lý do:

| Tài liệu | Giả định ban đầu | Thực tế | Ghi ở |
|---|---|---|---|
| ADR-0010 | Dùng `sqlc` sinh code | `pgx` + SQL viết tay | ADR-0010 mục cuối |
| 05-data | Định danh kiểu `UUID` | `TEXT` + ULID có tiền tố | data-model.md mục 1.1 |
| 05-data | Tách 7 schema PostgreSQL | Dùng `public`, hoãn tới khi tách service | database.md mục 3 |
| 02-domain | "Không bên nhận nào đọc event version" | 12 bên nhận khai, 4 event ở v > 1 | domain-events.md mục 8.2 |
| 04-modules/affiliate | Nghe `order.placed` và `return.refunded` | Cả hai KHÔNG tồn tại | affiliate.md mục 9.2 |
| README (chính file này) | `✗ module mới` | module EXTENSION được, có điều kiện | ADR-0022 |

## Đặc tả OpenAPI

Hợp đồng API nằm ở [`/api/openapi.yaml`](../gouse/api/README.md) — **nguồn sự thật duy nhất**, được cập nhật cùng pull request với thay đổi code.

```text
100 thao tác · 0 lỗi lint · sinh kiểu TypeScript thành công
 89 có tuyến thật · 15 hoãn sang Phase 2/3 (đã khai lý do)
```

`cmd/apicheck` gác bốn tầng hợp đồng: tuyến ⇄ đặc tả · header ⇄ CORS ·
enum ⇄ hằng số Go · DTO ⇄ schema phản hồi.

**API-first phải trở thành API thật.** Trạng thái từng operation
(`DESIGNED` / `IMPLEMENTED` / `TESTED` / `INTEGRATED`) ở
[`api/README.md`](../gouse/api/README.md). Không thêm operation mới chỉ vì "sau
này có thể cần".

## Nghiên cứu mã nguồn mở

Xem [11-oss/synthesis.md](11-oss/synthesis.md) — tổng hợp sau khi nghiên cứu 12 dự án OSS và 2 mô hình kinh doanh. Nghiên cứu **xác nhận** phần lớn thiết kế và **thay đổi ba điểm**: link table, Adjustment, cơ sở tính hoa hồng creator.

## Tổng hợp bàn giao

Xem [10-roadmap/deliverables.md](10-roadmap/deliverables.md) — tổng hợp toàn bộ bản đồ, ma trận, rủi ro và thứ tự triển khai đề xuất, kèm kết quả rà soát tính nhất quán của tài liệu.

## Tiến độ triển khai

| Tài liệu | Vai trò |
|---|---|
| [10-roadmap/backlog.md](10-roadmap/backlog.md) | **Việc CÒN PHẢI LÀM** — backlog duy nhất |
| [10-roadmap/todo.md](10-roadmap/todo.md) | Việc ĐÃ LÀM + bằng chứng kiểm chứng |
| [`api/README.md`](../gouse/api/README.md) | Trạng thái từng operation API |

Đừng tạo backlog thứ hai. Việc mới ghi vào `backlog.md`.
