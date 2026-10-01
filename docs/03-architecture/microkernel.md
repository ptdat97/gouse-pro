# Microkernel — kernel ổn định, business là tính năng bổ sung

Tài liệu này là **bảng tra cứu**. Lý do nằm ở
[ADR-0022](../adr/0022-microkernel-kernel-on-dinh.md); ranh giới đo được nằm ở
[ADR-0021](../adr/0021-ranh-gioi-commerce-kernel.md).

## 1. Một hình

```text
                        ┌──────── EXTENSION ────────┐
                        │  tháo ra → mất TÍNH NĂNG  │
                        │                           │
   affiliate ─┐         │  affiliate   notification │
   analytics  │         │  analytics   recommendation│
   returns    ├─ đọc ──►│  returns     supplychain  │
   ...        │         │                           │
              │         └───────────┬───────────────┘
              │                     │ event · cổng công khai
              │                     ▼  (MỘT chiều)
              │   ┌──────────────── KERNEL ─────────────────┐
              └──►│                                          │
                  │  ĐƯỜNG MUA HÀNG — bao đóng của checkout  │
                  │    cart  catalog  checkout  fulfillment  │
                  │    inventory  marketplace  order         │
                  │    payment  product  promotion  seller   │
                  │                                          │
                  │  NĂNG LỰC CẮT NGANG                     │
                  │    identity   customer                   │
                  └──────────────────────────────────────────┘
                                     │
                  ┌──────────────── PLATFORM ────────────────┐
                  │  eventbus · database · opsconfig · audit │
                  │  token · privacy · metrics · logger      │
                  └──────────────────────────────────────────┘
```

Mũi tên chỉ đi **lên**: extension đọc kernel, kernel KHÔNG biết extension tồn
tại. Chiều ấy được gác bằng `cmd/archcheck` R10, phát biểu theo chiều ngược —
*extension không bên nào import* — vì cách phát biểu ấy chặn được cả
extension → extension. Xem mục 6.

## 2. Ba tầng, và câu hỏi phân loại

| Tầng | Câu hỏi | Trả lời bằng |
|---|---|---|
| Kernel — đường mua hàng | Tháo ra thì khách còn mua được hàng? | ĐO: bao đóng phụ thuộc từ `checkout` |
| Kernel — năng lực cắt ngang | Mọi mô hình kinh doanh đều cần? | PHÁN ĐOÁN, ghi rõ ở ADR-0022 |
| Extension | Tháo ra thì mất một tính năng, không mất việc bán hàng | ĐO: không module nào import nó |

Phân loại bằng **câu hỏi đo được** là điểm khác biệt quan trọng nhất so với
phân loại bằng trực giác. "Marketplace nghe như một tính năng" là trực giác;
bao đóng cho thấy bỏ nó ra thì `cart` không còn `Offer` để tham chiếu, nên nó
là kernel. Xem ADR-0021.

## 3. Bảng tra — 20 module, đo 01/10/2026

> **Bảng này là ẢNH CHỤP.** Nguồn sự thật là hàng
> `| **Tầng (microkernel)** | … |` ở bảng đầu mỗi
> `docs/04-modules/<module>.md`, và `cmd/archcheck` R10 đọc chính nó. Khi hai
> chỗ lệch nhau, tin hàng kia — và sửa bảng này.

### Kernel — đường mua hàng (11)

| Module | Ai import nó | Vai trò primitive |
|---|---|---|
| `checkout` | — (là GỐC của bao đóng) | phiên mua một lần |
| `cart` | checkout | giỏ, và nguồn quy công |
| `catalog` | marketplace, product | thương hiệu, danh mục, quyền bán |
| `product` | cart, marketplace, recommendation | SKU và thuộc tính |
| `marketplace` | cart, checkout | **`Offer`** — primitive giá + người bán |
| `seller` | cart, checkout, marketplace, payment | gian hàng, trạng thái duyệt |
| `inventory` | cart, checkout, marketplace, returns | tồn kho sáu trạng thái |
| `order` | affiliate, checkout, returns | HỢP ĐỒNG với khách |
| `payment` | checkout, returns | sổ cái kép, số dư nhà bán |
| `promotion` | checkout | mã giảm, phân bổ chi phí |
| `fulfillment` | checkout | góc nhìn vận hành của việc giao |

`order` bị `affiliate` import — đó là chiều extension → kernel, chiều được
phép.

### Kernel — năng lực cắt ngang (2)

| Module | Ai import nó | Vì sao kernel dù ngoài bao đóng |
|---|---|---|
| `identity` | customer | ai đang gọi. Khách vãng lai mua được mà không đăng nhập, nhưng nhà bán và quản trị thì không |
| `customer` | — | khách là ai. `customer_consent` là bản ghi PHÁP LÝ (P3-54) |

### Extension — đã dựng (6)

| Module | Tháo ra thì mất gì |
|---|---|
| `affiliate` | phần ghi công và hoa hồng cho creator |
| `returns` | khách không tự xin trả hàng được |
| `notification` | thư giao dịch |
| `analytics` | phễu chuyển đổi |
| `recommendation` | gợi ý size |
| `supplychain` | tín hiệu nhu cầu cho sản xuất |

Cả sáu đều **không module nào import** — đo được, không phải tuyên bố.

### Chưa xếp được (1)

`pricing`. Tài liệu nói nó sở hữu `price_constraint` — *"khung giá ràng buộc
seller"* — nhưng `marketplace` (nơi duyệt giá Offer) không import `pricing`,
nên khung giá ấy không cưỡng chế gì. Xếp nó vào đâu phải chờ quyết định "ai
cưỡng chế khung giá" (P3-79).

### Extension — có tài liệu, chưa dựng (9)

`creator` · `content` · `campaign` · `loyalty` · `manufacturing` ·
`procurement` · `quality` · `warehouse` · `livestream`

## 4. Kernel ổn định nghĩa là gì

### 4.1. Không nhánh theo mô hình kinh doanh

```go
// CẤM
if donHang.LaLivestream { ... }
if offer.KieuBan == "preorder" { ... }
```

Một `if` như vậy là lời thú nhận rằng primitive chọn sai. Cách đúng là tìm
primitive đủ tổng quát: `Offer` thay cho "sản phẩm của nhà bán" (ADR-0007).

### 4.2. Kernel CHỞ định danh, extension sở hữu ý nghĩa

```text
kernel ĐƯỢC chở     mã creator, mã nội dung, mã chiến dịch
kernel KHÔNG chở    tỷ lệ hoa hồng, bên chịu chi phí, mô hình quy công
```

Quy tắc này đã trả giá để học. `order_line.attributed_creator_id` là một
**mã**: kernel không tra nó, `TestKernelKhongTraCreator` dùng một mã creator
KHÔNG tồn tại và đòi request phải thành công.
`order_line.creator_commission_rate` thì là một **tỷ lệ tiền** — ý nghĩa kinh
doanh — và nó đã bị bỏ ở migration 000059 sau khi đo 3298 dòng với 0 dòng
khác 0 (P3-83).

### 4.3. Thêm vào kernel phải là primitive

Thước đo: *mọi* mô hình kinh doanh đều cần nó, hay chỉ mô hình đang làm?

```text
PRIMITIVE  `shipping_method` trong phản hồi checkout (P3-73)
TÍNH NĂNG  `creator_commission_rate` trên dòng đơn (P3-83, đã bỏ)
```

### 4.4. Khi nào ĐƯỢC sửa kernel

```text
ĐƯỢC   thêm primitive mà mọi mô hình kinh doanh đều cần
ĐƯỢC   sửa một bất biến sai (P3-71: đợt đối soát trừ trên giấy)
ĐƯỢC   thêm TRƯỜNG quy công, nếu nó chỉ là tham chiếu
ĐƯỢC   BÀY RA dữ liệu kernel đã có mà chưa mặt đọc nào lấy được

KHÔNG  thêm nhánh theo mô hình kinh doanh
KHÔNG  thêm bảng của extension vào lược đồ kernel (R6 chặn)
KHÔNG  cho kernel import extension (R10a chặn)
```

Dòng thứ tư là bài học của chặng 2: bảy dòng core mà `affiliate` cần đều
thuộc loại ấy. `order_line.attributed_creator_id` nằm trong bảng từ tháng 8
và không mặt đọc nào bày nó ra — dữ liệu có mà không ai lấy được.

## 5. Dựng một extension mới

Bốn điều kiện, cả bốn, không chọn ba:

```text
1. Tài liệu `docs/04-modules/<ten>.md` khai HAI thứ TRƯỚC khi viết mã:
     | **Tầng (microkernel)** | EXTENSION — đã dựng |
     mục "N. Dữ liệu sở hữu" kèm khối ```sql
                                     archcheck ĐỎ nếu thiếu
2. KHÔNG module nào import nó       R10a cưỡng chế
3. Chỉ chạm bảng CỦA MÌNH           R6 cưỡng chế
4. Nhận dữ liệu qua event, hoặc đọc qua cổng công khai của kernel
                                     chiều extension → kernel OK
                                     chiều ngược lại KHÔNG
```

Điều 1 là điều dễ bỏ qua và là điều quan trọng nhất: câu *"tháo nó ra thì mất
gì"* chính là tiêu chí phân loại, và viết nó vào tài liệu buộc người thêm
module phải trả lời nó. Không trả lời được nghĩa là chưa biết mình đang dựng
kernel hay extension.

### Khuôn thư mục

```text
internal/modules/<ten>/
    domain/            mô hình nghiệp vụ, KHÔNG import module nào
    application/       trường hợp sử dụng, cổng do chính nó khai
    infrastructure/    postgres, chỉ chạm bảng của mình
    public.go          mặt công khai — điểm vào DUY NHẤT (R1)
    module.go          New(Config), dịch kiểu
    adapters.go        chỗ DUY NHẤT được import module khác
    handlers.go        bên nhận event
```

`adapters.go` là nơi khai một interface **hẹp** cho thứ cần từ kernel, không
nhận cả `order.API`:

```go
// affiliate/adapters.go
type OrderPort interface {
    GetOrder(ctx context.Context, orderID string) (*order.OrderView, error)
}
```

Mặt công khai của `order` có hơn hai chục hàm; nhận cả cụm nghĩa là một thay
đổi ở bất kỳ hàm nào cũng làm extension không biên dịch được.

### Nối dây

Extension nối ở **gốc ghép nối**, không ở trong module nào:

```text
internal/app/app.go      dựng module, giữ nó cho test tích hợp
cmd/worker/main.go       dựng module + đăng ký bên nhận event
```

Hai chỗ này là cái giá mọi module đều trả — chúng là định nghĩa của một gốc
ghép nối, không phải sửa kernel.

Bộ bên nhận ở hai chỗ phải **khớp nhau**: `cmd/eventcheck` R4 so danh sách
và ĐỎ khi lệch. Trước 01/10/2026 chúng lệch một cái —
`inventory.NewReleaseHandler`, đường ra của kho — và hệ quả là mọi bài test
tích hợp chạy với một hệ thống không nhả hàng về kho (P3-86).

## 6. Cưỡng chế

| Quy tắc | Gác bởi |
|---|---|
| Chỉ import qua `public.go` | archcheck R1 |
| Domain không biết hạ tầng | archcheck R2 |
| Platform không biết nghiệp vụ | archcheck R3 |
| Kernel chỉ dùng stdlib | archcheck R4 |
| Không phụ thuộc vòng | archcheck R5 |
| Chỉ chạm bảng của mình | archcheck R6 |
| Không có thư mục bị cấm | archcheck R7 |
| Chiều tầng trong module | archcheck R8 |
| Không SDK bên thứ ba trong kernel/domain | archcheck R9 |
| **Module EXTENSION không bên nào import** | **archcheck R10a** |
| **Tài liệu khai KERNEL thì phải CÓ hình dạng kernel** | **archcheck R10c** |
| **Mọi tài liệu module khai tầng của nó** | **archcheck, LỖI khởi động** |
| Event khai mà không ai phát / nghe | eventcheck R1–R3 |
| Bộ bên nhận worker ≠ bộ của test | eventcheck R4 |

Chạy cả bộ: `make check` (gồm `make arch` và `make hopdong`).

### 6.1. R10 phát biểu theo chiều NGƯỢC, và đó là chủ ý

Quy tắc không phải *"kernel không import extension"* mà **"extension không
bên nào import"**. Nó mạnh hơn: nó cũng chặn extension import extension —
thứ làm hai extension dính nhau rồi không tháo riêng được cái nào.

### 6.2. Bản đầu của R10 SAI, và nó sai theo cách đáng ghi

Bản đầu suy tập extension bằng bao đóng từ `checkout`: cái gì ngoài bao đóng
là extension. Nghe hợp lý, và nó là một lập luận **VÒNG** — `checkout` import
`notification` thì `notification` vào bao đóng, tức thành kernel, tức không
có vi phạm.

Phép phá đầu tiên cho thấy đúng thế: thêm một dòng import vào
`checkout/adapters.go` và công cụ báo **OK**.

> **Một quy tắc suy ra ranh giới TỪ thứ nó đang gác thì không gác được gì.**

Bản hiện tại tách hai việc:

```text
phân loại         là QUYẾT ĐỊNH  → hàng `**Tầng (microkernel)**` của
                                   docs/04-modules/<module>.md
tính nhất quán    là phép ĐO     → R10a và R10c đo nó
```

### Nguồn sự thật là TÀI LIỆU, cùng khuôn R6

R10 đọc hàng `**Tầng (microkernel)**` ở bảng đầu mỗi tài liệu module — cùng
cách R6 đọc mục "Dữ liệu sở hữu". Không có sổ phân loại nào trong Go.

Bản trước của R10 giữ ba sổ trong `cmd/archcheck/microkernel.go` *và* tài
liệu module cũng có hàng ấy — hai nguồn cho cùng một phân loại, đúng dạng lỗi
dự án này gác ở mọi chỗ khác. Bỏ sổ, đọc tài liệu.

Hai cách làm hỏng bảng phân loại đều là **LỖI khởi động**, không phải vi
phạm — không có phân loại đáng tin thì R10 không kiểm được gì, và một quy tắc
im lặng không kiểm gì vẫn in ra `OK`:

```text
tài liệu MẤT hàng `**Tầng (microkernel)**`
tài liệu ghi giá trị LẠ (ví dụ "Quan trọng")
```

Chỉ còn MỘT sổ trong Go: `nangLucCatNgang`, cho hai module tài liệu xếp
KERNEL mà nằm ngoài đường mua hàng. Đó là chỗ duy nhất R10 dựa vào phán đoán,
và thiếu một dòng ở đó thì R10c đỏ với chính module ấy.

### R10c — tài liệu khai KERNEL mà hình dạng là extension

Ngoài đường mua hàng **và** không ai import: đó đúng là hình dạng một
extension. Phân loại cũ sẽ làm người sau tin module ấy ổn định, nên R10c đỏ
và buộc chọn — sửa tài liệu, hoặc khai nó là năng lực cắt ngang kèm lý do.

### 6.3. Một phép phá nữa không đỏ

Ở bản giữa, phép phá "dựng module mới chưa phân loại" vẫn **không đỏ**. Lý
do: tập module lấy từ `c.imports`, nên một module chỉ có `public.go` không
import gì thì vô hình — đúng lúc quy tắc cần kêu nhất, vì module mới là lúc
người ta chưa phân loại. Chuyển sang lấy từ `c.pkgFiles` (mọi package đã
quét).

Bản hiện tại không còn cần nhánh ấy làm hàng rào chính: R6 đã ĐỎ khi một thư
mục module không tra ra tài liệu, và R10 ĐỎ khi tài liệu ấy thiếu hàng tầng.
Module mới không có đường nào đi qua mà không bị phân loại.

### 6.4. R10 phải chạy trong `go test`, không chỉ khi gọi tay

`TestSelfCheck` chạy đủ bộ quy tắc trên repo thật qua `moiChecker`. Dự án giả
của bộ test TẮT R10 (`goc` rỗng) — ba module không có kernel để mà gác — nên
nếu `TestSelfCheck` cũng dùng đường ấy thì R10 sẽ không bao giờ chạy trong
`go test`, và một quy tắc chỉ chạy khi gọi tay là quy tắc sẽ bị bỏ qua.

## 7. Đây KHÔNG phải plugin system

Dòng `✗ plugin system` trong danh sách đóng băng của
[../README.md](../README.md) **vẫn đúng**:

| Microkernel ở Gouse | Plugin system |
|---|---|
| Ranh giới ĐO được từ mã | Ranh giới khai trong sổ đăng ký |
| Nối dây lúc BIÊN DỊCH | Nạp lúc chạy |
| Cưỡng chế bằng `cmd/archcheck` | Cưỡng chế lúc chạy |
| Không `Registry`, không vòng đời module | Có cả hai |

ADR-0021 từ chối dựng `Registry` với lý do *"tới khi có người dùng thứ hai"*.
Dựng cơ chế trừu tượng cho một người dùng là tạo trước thứ phải bảo trì — và
dự án này đã có đủ ví dụ về thứ khai ra mà không ai dùng (P3-76: bốn hằng số
event; P3-83: một cột tỷ lệ tiền).

## 8. Quan hệ với "modular monolith"

Hai khái niệm trả lời hai câu khác nhau, và dùng cả hai là đúng:

```text
Modular monolith   TRIỂN KHAI thế nào   một binary, ranh giới biên dịch
Microkernel        PHÂN TẦNG thế nào    kernel ổn định + extension
```

[ADR-0001](../adr/0001-modular-monolith.md) không đổi.

## 9. Tài liệu liên quan

- [ADR-0022](../adr/0022-microkernel-kernel-on-dinh.md) — quyết định và lý do
- [ADR-0021](../adr/0021-ranh-gioi-commerce-kernel.md) — ranh giới đo được
- [modular-monolith.md](modular-monolith.md) — cấu trúc một module
- [dependency-rules.md](dependency-rules.md) — quy tắc phụ thuộc và công cụ
- [module-boundaries.md](module-boundaries.md) — mặt công khai từng module
- [evolution-to-services.md](evolution-to-services.md) — vì sao CHƯA tách
