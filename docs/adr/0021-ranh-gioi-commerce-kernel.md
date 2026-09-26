# ADR-0021 — Ranh giới Commerce Kernel: `Offer` là primitive, marketplace không phải extension

**Trạng thái:** Đã chấp nhận · 25/09/2026

## Bối cảnh

Một đề xuất "Commerce Kernel Optimization & Extension Architecture" đề nghị
tách hệ thống thành hai tầng:

```text
Kernel      Product · SKU · Catalog · Price · Cart · Order · Inventory
            Payment · Tax · Promotion · Fulfillment primitives

Extension   Marketplace · Seller · Commission · Affiliate · Creator
            Livestream · Preorder · Subscription · Loyalty · ERP
```

Kèm yêu cầu: *"Kernel không được biết chi tiết về business model cụ thể như
fashion, marketplace, creator commerce…"*, và câu hỏi nghiệm thu:

> *"Nếu ngày mai cần xây Marketplace + Creator Commerce + Affiliate +
> Livestream + ERP trên Commerce Kernel này, chúng ta có thể làm chủ yếu
> bằng cách tạo module mới, hay phải liên tục sửa Core?"*

Đề xuất ấy đúng cho phần lớn hệ thống thương mại. Nó **không đúng cho
Gouse**, và ADR này ghi lý do — vì không ghi thì câu hỏi sẽ quay lại mỗi
sáu tháng, và mỗi lần lại tốn một vòng phân tích.

### Đồ thị phụ thuộc thật

Đo ngày 25/09/2026 trên mã đang chạy:

```text
order      → (không phụ thuộc module nào)
inventory  → (không)
catalog    → (không)
product    → catalog

cart       → inventory · marketplace · product · seller
checkout   → cart · fulfillment · inventory · marketplace · order
             payment · promotion · seller
payment    → seller
```

Ba module mà đề xuất gọi là Kernel — `cart`, `checkout`, `payment` — phụ
thuộc hai module nó gọi là Extension.

Theo đề xuất, đó là nợ kỹ thuật cần trả. Không phải.

### Vì sao `Offer` bắt buộc mang nhà bán

[vision.md](../00-overview/vision.md) định nghĩa sản phẩm:

> *Một nền tảng thương mại thời trang **sở hữu thương hiệu riêng, dùng
> marketplace** (nhà bán và thương hiệu bên thứ ba) để mở rộng độ phủ hàng
> hóa.*

Và [ADR-0007](0007-marketplace-order-model.md) quyết định từ đầu:

> *Mô hình hóa `Offer` tách khỏi `Product`, **kể cả khi mới chỉ có own brand
> bán**.*

Thứ khách đặt vào giỏ **không phải Product, mà là Offer** — một lời chào
bán cụ thể, của một nhà bán cụ thể, với giá và tồn kho cụ thể. Không có
nhà bán thì không có Offer; không có Offer thì không biết trừ kho của ai,
trả tiền cho ai, giao từ đâu.

Nên `cart → seller` không phải rò rỉ. Nó là hệ quả của việc đơn vị mua bán
trong miền này có một chủ sở hữu.

vision.md còn nói thêm một điều quyết định:

> *Ba hiệu ứng này **độc lập** — hệ thống phải vận hành được khi một trong
> ba còn yếu. Không được thiết kế sao cho marketplace chỉ chạy được khi đã
> có creator, hay own brand chỉ chạy được khi đã có marketplace.*

"Own brand chạy được khi chưa có marketplace" **đã** đạt: một nền tảng chỉ
own brand vẫn có `Offer`, chỉ là mọi Offer thuộc một nhà bán duy nhất.
Điều KHÔNG được yêu cầu là "hệ thống chạy được khi tháo hẳn khái niệm nhà
bán ra" — đó là một sản phẩm khác.

## Quyết định

### Ranh giới suy ra từ ĐO, không từ ý kiến

Câu hỏi phân loại:

> **Tháo module này ra thì khách có còn mua được hàng không?**

Trả lời được bằng mã: lấy bao đóng phụ thuộc từ `checkout`. Đo 25/09/2026:

```text
ĐƯỜNG MUA HÀNG — 11 module
    cart · catalog · checkout · fulfillment · inventory
    marketplace · order · payment · product · promotion · seller

NGOÀI đường mua hàng — 8 module
    analytics · customer · identity · notification
    pricing · recommendation · returns · supplychain
```

`marketplace` và `seller` **nằm trên đường mua hàng**. Đó là kết quả đo,
không phải lựa chọn thiết kế có thể bàn lại: bỏ chúng ra thì `cart` không
còn Offer để tham chiếu.

### Ba nhóm, không phải hai

Bao đóng ấy chưa đủ để phân loại, vì có thứ nằm ngoài đường mua hàng mà
vẫn không phải extension.

```text
KERNEL — đường mua hàng (11 module ở trên)

KERNEL — năng lực cắt ngang
    identity     ai đang gọi. Khách vãng lai mua được mà không đăng nhập,
                 nên nó không nằm trên đường mua hàng — nhưng mọi mô hình
                 kinh doanh đều cần nó cho nhà bán và quản trị.
    customer     khách là ai: địa chỉ, đồng ý nhận thư, yêu thích.
                 `customer_consent` là bản ghi PHÁP LÝ (P3-54).

EXTENSION — đã dựng, KHÔNG module nào phụ thuộc
    returns · recommendation · analytics · supplychain · notification

EXTENSION — chưa dựng, đã có tài liệu
    affiliate · creator · content · campaign · loyalty
    manufacturing · procurement · quality · warehouse
```

Năm extension đã dựng đều có **không ai import** (đo được), nên chúng thật
sự tháo ra được. Đó là bằng chứng mạnh nhất hiện có cho câu hỏi nghiệm thu
của đề xuất — và nó có trước khi đề xuất ra đời.

### `pricing` — một trường hợp chưa xếp được

Đo cho thấy `pricing` không ai import, và nó cũng không có route nào. Nó
chỉ được dựng trong `app.go` để gieo dữ liệu mẫu.

Tài liệu nói nó sở hữu `price_constraint` — *"khung giá ràng buộc seller"*.
Nhưng `marketplace` (nơi duyệt giá của Offer) **không** import `pricing`,
nên khung giá ấy hôm nay không cưỡng chế gì.

Đây không phải lỗi phân loại mà là một **khoảng trống chức năng**: một bảng
có mục đích ghi rõ mà không ai đọc. Xếp `pricing` vào Kernel hay Extension
phải chờ quyết định "ai cưỡng chế khung giá" — ghi vào backlog thay vì
đoán ở đây.

### Đường ranh này khác đề xuất ở đúng HAI chỗ

```text
đề xuất xếp EXTENSION     đo được là KERNEL
    marketplace           trên đường mua hàng
    seller                trên đường mua hàng
```

Mọi chỗ còn lại trùng nhau.

### Khi nào được sửa Kernel

```text
ĐƯỢC   thêm một primitive mà mọi mô hình kinh doanh đều cần
       (ví dụ: `shipping_method` trong phản hồi checkout — P3-73)

ĐƯỢC   sửa một bất biến sai (P3-71: đợt đối soát trừ trên giấy)

ĐƯỢC   thêm TRƯỜNG quy công vào primitive, nếu nó chỉ là tham chiếu
       (`SourceCreatorID` trên CartItem là một mã, không phải logic
       creator commerce)

KHÔNG  thêm nhánh theo mô hình kinh doanh: `if marketplace`, `if preorder`,
       `if livestream`

KHÔNG  thêm bảng của extension vào lược đồ của kernel (R6 chặn)

KHÔNG  cho kernel import extension (R1 + R5 chặn)
```

## Những gì KHÔNG làm, và vì sao

### Không dựng Registry (§7 của đề xuất)

Đề xuất muốn chín registry: `PolicyRegistry`, `StrategyRegistry`,
`EventHandlerRegistry`, `CommandRegistry`, `QueryRegistry`,
`RouteRegistry`, `ValidatorRegistry`, `PermissionRegistry`,
`ProviderRegistry`.

Hiện 19 module được nối **tường minh** trong `internal/app/app.go`. Nối
tường minh là type-safe lúc biên dịch và đọc được bằng mắt. Registry đổi nó
sang tra cứu lúc chạy — mất type-safety, và sinh ra một lớp hỏng mới
(trùng đăng ký, thiếu đăng ký, thứ tự khởi tạo) mà chính đề xuất §16.5 rồi
lại đòi phải kiểm.

Thêm một lớp hỏng để giải quyết lớp hỏng nó vừa tạo ra không phải một khoản
đầu tư. Dựng khi có **bên thứ ba** cần mở rộng mà không sửa được `app.go`.

### Không dựng hệ thống Policy/Strategy (§4)

Đề xuất liệt kê 9 Strategy và 8 Policy nên dựng. Chính nó, ở §17, nói mỗi
extension point phải có *"ít nhất một implementation thực tế"*.

Đo ngày 25/09: **ba** nhánh theo mô hình kinh doanh trong toàn bộ
`domain/` và `application/` — cả ba là phép kiểm nil của trường quy công
(`SourceContentID`, `SourceCreatorID`, `CreatorID`). Không có `if
marketplace`, `if fashion`, `if preorder` nào.

Không có variation nào để mà trừu tượng hóa. Và **67 cổng** (`Port`,
`Repository`, `Provider`) đã tồn tại trong `domain/` và `application/` —
mẫu ports/adapters mà §4 đề nghị đã có sẵn khắp nơi.

### Không dựng interface `Module` có `Boot()` (§6)

Mỗi module đã có `New(Config)` và `Register*Routes(mux, log)`. Một
interface chung với `Boot()` thêm một lớp gián tiếp mà chưa có người dùng
thứ hai để biện minh. Xem thêm [ADR-0009](0009-service-extraction.md) —
cùng lý do đã hoãn tách service.

### Không tách thư mục thành hai tầng `kernel/` và `extensions/`

Ranh giới đã được **máy cưỡng chế** bằng R1 (chỉ import `public.go`), R3
(domain không import module khác), R5 (đồ thị là DAG) và R6 (sở hữu bảng).
Sắp lại 487 file vào hai thư mục không thêm một ràng buộc nào mà máy chưa
kiểm — nó chỉ đổi chỗ, và mọi liên kết tài liệu phải sửa theo.

## Cưỡng chế

ADR này không tự bảo vệ được. Chín quy tắc dưới đây làm việc đó, và tất cả
chạy trong CI qua `make check`:

```text
R1  chỉ import public.go của module khác          archcheck
R2  domain không import platform / module khác    archcheck
R3  platform không biết nghiệp vụ                 archcheck
R4  kernel chỉ dùng thư viện chuẩn                archcheck
R5  đồ thị phụ thuộc module là DAG                archcheck
R6  SQL của module X chỉ chạm bảng của X          archcheck  (25/09)
R7  không có thư mục bãi rác                      archcheck
R8  chiều phụ thuộc trong module                  archcheck
R9  kernel/domain không dùng SDK bên thứ ba       archcheck  (25/09)

    đặc tả API ⇄ tuyến · header ⇄ CORS · enum ⇄ Go    apicheck
    mọi loại event có nơi phát VÀ bên nghe             eventcheck
```

Hai công cụ cuối **chưa bao giờ chạy trong CI** cho tới 25/09/2026: CI gọi
`make arch`, và mục tiêu ấy chỉ chạy `archcheck`. Xem P3-78 — một hàng rào
không ai chạy thì không gác gì, và tệ hơn là nó làm người ta tin ngược lại.

## Đánh đổi đã chấp nhận

```text
Nhận      Ranh giới khớp với mô hình nghiệp vụ thật. Không phải viết một
          lớp trừu tượng để làm marketplace tháo được trong một sản phẩm
          mà nó không bao giờ tháo được.

Mất       Không bán được Kernel này cho một khách chỉ cần shop một người
          bán. Họ vẫn dùng được — mọi Offer thuộc một nhà bán — nhưng phải
          mang theo khái niệm `seller` mà họ không cần.

Rủi ro    Nếu sau này thật sự cần một bản không-marketplace, phải trả
          khoản refactor ấy lúc đó. Đổi lại là không trả nó HÔM NAY cho một
          nhu cầu chưa tồn tại — nguyên tắc P13 và §17 của chính đề xuất.
```

## Câu trả lời cho câu hỏi nghiệm thu

> *Marketplace + Creator + Affiliate + Livestream + ERP — làm bằng module
> mới hay phải sửa Core?*

```text
Marketplace   ĐÃ ở trong Kernel. Câu hỏi không áp dụng.
Creator       module mới + trường quy công đã có trên CartItem/OrderLine
Affiliate     module mới + nghe event. Đang dựng để ĐO — xem dưới.
Livestream    module mới + quy công, cùng khuôn với creator
ERP           adapter sau một cổng; R9 chặn SDK của họ vào domain
```

### ĐO được, 25/09/2026 — chặng 1 của phép thử

Bốn dòng trên là dự đoán. Đây là số đo.

Đi dựng `affiliate` thì việc đầu tiên đụng phải là: **chuỗi quy công đứt ở
giữa**. Hạ tầng đã có ở CẢ HAI ĐẦU từ tháng 8 —

```text
cart_item.source_content_id        migration 000009
cart_item.source_creator_id        migration 000009, CÓ index
order_line.attributed_creator_id   migration 000008, CÓ index
order.PlaceOrderLineInput.AttributedCreatorID
```

— và `checkout.domain.Line` không mang trường quy công, nên thông tin
creator biến mất ở chính giữa. Mọi cột và index nói trên **chưa bao giờ có
dữ liệu khác rỗng**.

Khoản sửa core, đo bằng `git diff`:

```text
44 dòng mã · 1 module (checkout) · 1 migration (2 cột + 1 index)

KHÔNG sửa: cart · order · payment · marketplace · inventory · product
```

Và nó là khoản **MỘT LẦN**: creator, livestream, campaign sau này dùng lại
đúng chuỗi ấy, không cần thêm dòng core nào.

Kernel chỉ **CHỞ hai cái mã**. Nó không tra creator có tồn tại không —
`TestKernelKhongTraCreator` dùng một mã creator KHÔNG tồn tại và đòi request
phải thành công. Ngày nào kernel đi tra, bài ấy đỏ, và đó là tín hiệu đúng:
lúc ấy kernel đã biết về creator commerce.

Một quyết định trong lúc làm đáng ghi: bản đầu của migration thêm cả
`creator_commission_rate` cho cân với `commission_rate` của nhà bán. Bỏ đi,
vì affiliate.md mục 7 đã quyết bảng `attribution` của CHÍNH NÓ đóng băng tỷ
lệ. Hai cột cho cùng một con số tiền là hai nguồn để lệch nhau — và cột ở
kernel sẽ không ai ghi.

**Chặng 2 chưa làm:** dựng module `affiliate` thật (5 bảng, nghe
`order.paid`, ghi attribution + commission) và đo nó cần **0** dòng core.
Xem P3-81.

## Liên quan

- [ADR-0007](0007-marketplace-order-model.md) — `Offer` tách khỏi `Product`
- [ADR-0005](0005-module-boundaries.md) — ranh giới module tường minh
- [ADR-0009](0009-service-extraction.md) — hoãn tách service, cùng lý do
- [ADR-0012](0012-inventory-ownership.md) — chủ sở hữu tồn kho suy ra từ nhà bán
- [dependency-rules.md](../03-architecture/dependency-rules.md) — chín quy tắc
- [vision.md](../00-overview/vision.md) — ba hiệu ứng độc lập
- backlog P3-76 · P3-78 — hai hàng rào và một CI chưa bao giờ chạy
