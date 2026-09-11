# Cấu hình nghiệp vụ sửa được lúc chạy

> Con số nào người **kinh doanh** quyết thì phải sửa được từ giao diện quản
> trị. Con số nào **kỹ sư** quyết thì không bao giờ.

Tài liệu này là **bản kiểm kê**: hôm nay có những con số nào, cái nào đã
sửa được, cái nào còn nằm cứng trong mã và phải đưa lên.

Cơ chế, quyền hạn và cách ghi vết nằm ở [ADR-0015](../adr/0015-cau-hinh-van-hanh.md).
Đây là danh sách, không phải quyết định kiến trúc — nó thay đổi mỗi khi
thêm một tham số, nên nó sống ở đây chứ không sống trong ADR.

---

## 1. Bài kiểm ba câu để phân loại

Trước khi đưa một hằng số lên giao diện, trả lời cả ba:

```text
1. Ai đổi nó?         người kinh doanh → ứng viên
                      kỹ sư            → DỪNG, giữ trong mã

2. Đổi nó có nới      CÓ  → DỪNG. Một kiểm soát tự nới lỏng được từ giao
   lỏng một kiểm            diện thì không còn là kiểm soát: người muốn
   soát nào không?          lách chỉ cần đổi một con số, và thao tác đổi
                            đó cũng do chính họ ký.
                      KHÔNG → tiếp

3. Nó có phải đổi     CÓ  → DỪNG. Hai con số phải khớp nhau mà sửa được
   CÙNG một con số           độc lập là một cái bẫy đặt sẵn.
   khác để đúng?      KHÔNG → đưa lên, kèm BIÊN và mô tả HỆ QUẢ
```

Câu 3 là câu hay bị bỏ sót nhất. Ví dụ: `checkout.DefaultTTL` trông đúng
hệt một tham số nghiệp vụ, nhưng nó gắn chặt với thời hạn giữ hàng của
`inventory` — nới một bên mà không nới bên kia thì phiên sống lâu hơn hàng
đã giữ, và khách đi tới bước trả tiền cho hàng vừa bị bán cho người khác.

---

## 2. Đã sửa được từ giao diện

Sổ đăng ký: `internal/platform/opsconfig/registry.go`.
Giao diện: **Admin → Cấu hình vận hành** (chỉ vai trò `ADMIN`).

| Khóa | Mặc định | Quyết định điều gì |
|---|---|---|
| `fulfillment.shipping_sla_hours` | 48 | Hạn nhà bán phải bàn giao cho hãng vận chuyển |
| `fulfillment.max_cancellation_rate` | 0,03 | Tỷ lệ hủy tối đa còn được coi là đạt |
| `fulfillment.min_on_time_rate` | 0,95 | Tỷ lệ giao đúng hạn tối thiểu để đạt |
| `fulfillment.min_sample_size` | 10 | Số đơn tối thiểu mới chấm hiệu suất |
| `fulfillment.delivery_silence_hours` | 168 | Bao lâu không có tin thì coi là gói hàng mất tích |
| `fulfillment.carrier_cost_standard` | 0 | Tiền nền tảng TRẢ hãng cho một kiện giao thường |
| `fulfillment.carrier_cost_express` | 0 | Tiền nền tảng TRẢ hãng cho một kiện giao nhanh |
| `returns.window_hours` | 168 | Thời hạn khách được đổi trả sau khi nhận hàng |
| `inventory.max_quantity_per_sku` | 10.000.000 | Trần nghiệp vụ khi kiểm kê một SKU tại một kho |
| `checkout.tax_rate_bp` | 800 | Thuế suất theo phần vạn (800 = 8%) |
| `checkout.free_shipping_threshold` | 499.000 | Tiền hàng tối thiểu để miễn phí vận chuyển |

**Hai khóa giá hãng vận chuyển mặc định 0 là im lặng CÓ CHỦ Ý.** Chưa khai
thì không bút toán chi phí nào được ghi. Đặt một con số đoán vào mã nghĩa
là sổ cái ghi sai mà không ai biết, và ngày khai giá thật thì mọi bút toán
cũ đều lệch.

### Đo trên dữ liệu thật (11/09/2026)

```text
khóa đã từng được đặt giá trị:  1/11
  checkout.tax_rate_bp = 800    (đúng bằng mặc định)
```

Mười khóa còn lại chạy bằng mặc định biên dịch sẵn. Đó không phải lỗi —
mặc định là giá trị đúng cho tới khi có lý do đổi. Nhưng nó nói một điều
cần biết: **đường ghi cấu hình gần như chưa được dùng thật**, nên đừng
giả định nó đã được kiểm chứng bởi vận hành hằng ngày.

---

## 3. CÒN NẰM CỨNG — phải đưa lên

Xếp theo mức độ ảnh hưởng tới **tiền của người khác**.

### 3.1 Trọng số buy box — quyết định nhà bán nào có doanh thu

```text
vị trí        internal/modules/marketplace/domain/buybox.go
hiện tại      Giá 40% · Thời gian giao 30% · Hiệu suất 30%
```

Đây là con số nặng nhất trong cả danh sách: nó quyết định **offer của ai
được hiển thị mặc định**, tức là ai bán được hàng. ADR-0015 lấy đúng loại
tác động này làm lý do bắt buộc ghi vết mọi lần đổi.

Chú thích ngay tại chỗ khai báo đã tự nói ra nhu cầu: *"Con số cụ thể nên
hiệu chỉnh lại khi có dữ liệu thật về hành vi khách hàng."* Hiệu chỉnh
theo dữ liệu là một vòng lặp đo → đổi → đo lại, và vòng đó không thể đi
qua một lần build.

**Đường nối đã có sẵn nhưng bỏ trống.** `application.Deps` có trường
`Weights`, và không ai truyền vào, nên nó luôn rơi về `DefaultWeights`. Tệ
hơn: trọng số được chốt **lúc khởi động** chứ không đọc mỗi lần tính, nên
kể cả khi nối dây thì đổi vẫn cần khởi động lại.

Khi đưa lên, đọc mỗi lần tính giống `fulfillment.carrier_cost_*` đang làm —
đổi trọng số phải có tác dụng ở lượt xem trang kế tiếp.

Biên bắt buộc: ba trọng số phải cộng đúng 100. Đây là ràng buộc **giữa các
tham số**, không phải biên của từng cái — sổ đăng ký hiện chỉ kiểm được
`Min`/`Max` từng khóa, nên cần thêm một phép kiểm nhóm.

### 3.2 Điểm hiệu suất mặc định của nhà bán mới

```text
vị trí        internal/modules/marketplace/domain/buybox.go
hiện tại      50 / 100
```

Con số này quyết định **nhà bán mới có cửa thắng buy box hay không**. Đặt
80 thì người mới gần như ngang người đã có lịch sử tốt; đặt 20 thì họ gần
như không bao giờ bán được đơn đầu tiên, mà không có đơn đầu tiên thì
không bao giờ có lịch sử để thoát ra.

Đây là một lựa chọn tăng trưởng, không phải một hằng số kỹ thuật.

### 3.3 Biểu phí vận chuyển — khách trả tiền theo nó

```text
vị trí        internal/modules/fulfillment/domain/uoc_tinh_phi.go
hiện tại      STANDARD  30.000 đ / nguồn hàng · dự kiến 3 ngày
              EXPRESS   60.000 đ / nguồn hàng · dự kiến 1 ngày
```

Hai con số tiền là **giá hiện trên màn hình thanh toán**. Hai con số ngày
là **lời hứa giao hàng** — chúng được dùng để tính ngày giao dự kiến báo
cho khách lúc bàn giao.

Đổi giá vận chuyển là việc chạy khuyến mãi, đàm phán lại với hãng, hoặc
phản ứng với đối thủ. Không việc nào trong đó nên chờ một lần triển khai.

Lưu ý khi đưa lên: phí này là thứ **khách trả**, còn
`fulfillment.carrier_cost_*` là thứ **nền tảng trả hãng**. Hai con số khác
nhau và hiệu của chúng là lãi/lỗ mảng vận chuyển. Giao diện phải đặt chúng
cạnh nhau, vì đặt phí thu thấp hơn phí trả là lỗ trên mỗi đơn — và không
có gì trong hệ thống chặn điều đó.

### 3.4 Thời gian chuẩn bị hàng mặc định của offer

```text
vị trí        internal/modules/marketplace/domain/offer.go
hiện tại      24 giờ
```

Áp cho offer nào không khai. Nó đi thẳng vào điểm buy box (mục 3.1) và vào
ngày giao dự kiến báo cho khách.

### 3.5 Tỷ lệ hoa hồng: mức đề xuất và SÀN

```text
vị trí        chưa có — hoa hồng lưu theo TỪNG nhà bán
hiện tại      biên [0%, 100%] kiểm ở biên module
              mặc định của cột trong database: 0
```

Hoa hồng của từng nhà bán là **dữ liệu**, không phải cấu hình — đúng như
vậy, vì nó khác nhau theo từng hợp đồng. Nhưng hai con số **chính sách**
quanh nó thì chưa có nhà:

```text
mức đề xuất   con số điền sẵn khi duyệt một nhà bán mới
sàn           dưới mức này thì duyệt phải có lý do riêng
```

Không có sàn thì duyệt nhầm một nhà bán ở 0% là **nền tảng không thu được
đồng nào** trên mọi đơn của họ, và không có gì báo. Biên `[0, 10000]` hiện
tại chỉ chặn con số vô nghĩa (150%), không chặn con số tai hại.

Đo trên dữ liệu thật (11/09/2026) — **chưa có ca nào sai**, và đáng ghi lại
vì sao:

```text
Lumière          INTERNAL  0%      đúng: own brand không chịu hoa hồng,
                                   và một ràng buộc DB CƯỠNG CHẾ điều đó
Xuong May That   BUSINESS  0%      chưa duyệt (APPLIED) — 0 là mặc định
                                   của cột, người duyệt sẽ phải điền
Xưởng May Bảy    BUSINESS  12%     đã duyệt, có tỷ lệ thật
```

Nghĩa là rủi ro ở đây là **dự phòng**, không phải sự cố đang xảy ra: hôm
nay chưa nhà bán nào được duyệt ở 0%. Nhưng cũng chưa có gì ngăn điều đó,
và cột mặc định 0 làm "bỏ trống" và "quyết định 0%" trông giống hệt nhau
trên màn hình duyệt.

### 3.6 Nhịp tạo đợt đối soát cho nhà bán

```text
vị trí        cmd/worker/main.go
hiện tại      mỗi 1 giờ
```

Nhịp **gom** bút toán thành đợt. Việc chi trả vẫn do người duyệt, nên đây
không phải một kiểm soát tiền — nhưng nó quyết định nhà bán **nhìn thấy**
khoản của mình sớm hay muộn, và đó là câu hỏi họ hỏi nhiều nhất.

Mức ưu tiên thấp hơn năm mục trên: đổi nó chỉ đổi độ trễ hiển thị.

---

## 4. CỐ Ý giữ cứng trong mã

Đưa những thứ này lên giao diện là tự tay mở cửa. Ghi ra đây kèm hậu quả
để lần sau không ai phải suy luận lại.

| Hằng số | Nếu đưa lên và bị hạ |
|---|---|
| `audit.minReasonLen` (20) | vô hiệu hóa toàn bộ nhật ký truy cập |
| `token.minSecretLen` | mở cửa cho khóa yếu |
| `eventbus.maxAttempts` | đẩy cả hàng đợi vào dead letter |
| `identity.MaxFailedAttempts` (5) | mở cửa cho dò mật khẩu |
| `identity.LockDuration` (15 phút) | như trên |
| `identity.AccessTokenTTL` (15 phút) | kéo dài cửa sổ của token bị lộ |
| `identity.RefreshTokenTTL` (30 ngày) | như trên |
| `HanTokenXacMinh` (24 giờ) | kéo dài cửa sổ của liên kết xác minh bị lộ |
| `database.MaxConns` | thuộc cấu hình TRIỂN KHAI (biến môi trường) |
| `checkout.DefaultTTL` (15 phút) | gắn chặt với TTL giữ hàng — xem câu 3 ở mục 1 |
| `checkout.ExtendDuration` (10 phút) | như trên |
| `maxCategoryDepth` (5) | ràng buộc cấu trúc cây, không phải chính sách |
| trần trang (`maxPageSize`…) | bảo vệ tài nguyên, không phải chính sách |

**Trần lưu trữ không bao giờ là tham số.** `inventory.max_quantity_per_sku`
là trần NGHIỆP VỤ và sửa được; trần 2.147.483.647 của cột `INT` là SỰ THẬT
về nơi lưu và không ai chọn nó. Đặt trần nghiệp vụ cao hơn trần lưu trữ
nghĩa là con số đi qua hết kiểm tra rồi mới hỏng ở câu lệnh ghi.

---

## 5. Thêm một tham số: việc phải làm

Sổ đăng ký **đóng** — không có đường thêm tham số từ giao diện, và đó là
chủ ý (ADR-0015 mục 2).

```text
1. Trả lời ba câu ở mục 1. Không qua được thì dừng.
2. Khai trong internal/platform/opsconfig/registry.go:
     khóa · kiểu · Min · Max · MacDinh · MoTa · impact
3. ĐỌC TẠI CHỖ DÙNG, mỗi lần dùng — không chụp lúc khởi động.
   Đổi giá thỏa thuận với hãng phải có tác dụng ở kiện hàng KẾ TIẾP.
4. Mặc định phải là giá trị ĐANG chạy hôm nay. Thêm một tham số không
   được đổi hành vi của hệ thống.
5. Thêm dòng vào bảng ở mục 2 của tài liệu này.
6. Viết một bài test PHÁ: đổi tham số và chứng minh hành vi đổi theo.
   Tham số không ai đọc là dạng lỗi hay gặp nhất của dự án này — xem
   mục 8 của backlog.
```

Bước 6 không phải thủ tục. `fulfillment.carrier_cost_*` đã khai đủ, đã nối
đủ, và vẫn im lặng suốt vì một trường khác (`shipping_method`) rỗng — hai
lý do độc lập cùng dẫn tới giá 0. Chỉ một bài test đi hết đường mới thấy.

---

## Liên quan

- [ADR-0015](../adr/0015-cau-hinh-van-hanh.md) — cơ chế, quyền, ghi vết
- [ADR-0011](../adr/0011-audit-log.md) — nhật ký thao tác
- `internal/platform/opsconfig/registry.go` — sổ đăng ký
- `gouse-web/apps/admin/src/app/config/page.tsx` — giao diện
