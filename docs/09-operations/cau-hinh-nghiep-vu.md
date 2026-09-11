# Cấu hình nghiệp vụ sửa được lúc chạy

> Con số nào người **kinh doanh** quyết thì phải sửa được từ giao diện quản
> trị. Con số nào **kỹ sư** quyết thì không bao giờ.

Tài liệu này là **bản kiểm kê**: hôm nay có những con số nào, cái nào đã
sửa được, cái nào cố ý giữ cứng và vì sao.

**Trạng thái 11/09/2026:** cả sáu nhóm từng nằm cứng đã được đưa lên (22
tham số). Không còn con số chính sách kinh doanh nào phải build lại để đổi
— xem mục 3 để biết cái bẫy riêng của từng nhóm trước khi đổi.

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
| `marketplace.buybox_weight_price` | 40 | Trọng số GIÁ trong công thức buy box |
| `marketplace.buybox_weight_handling` | 30 | Trọng số THỜI GIAN CHUẨN BỊ |
| `marketplace.buybox_weight_performance` | 30 | Trọng số ĐIỂM HIỆU SUẤT nhà bán |
| `marketplace.default_performance_score` | 50 | Điểm của nhà bán CHƯA có lịch sử |
| `marketplace.default_handling_hours` | 24 | Thời gian chuẩn bị áp cho offer không tự khai |
| `marketplace.commission_rate_default_bp` | 0 | Hoa hồng ĐỀ XUẤT khi duyệt (0 = không đề xuất) |
| `marketplace.commission_rate_floor_bp` | 0 | SÀN hoa hồng (0 = không có sàn) |
| `fulfillment.shipping_fee_standard` | 30.000 | Phí vận chuyển KHÁCH TRẢ, giao thường |
| `fulfillment.shipping_fee_express` | 60.000 | Phí vận chuyển KHÁCH TRẢ, giao nhanh |
| `fulfillment.shipping_days_standard` | 3 | Số ngày HỨA với khách, giao thường |
| `fulfillment.shipping_days_express` | 1 | Số ngày HỨA với khách, giao nhanh |
| `payment.settlement_batch_interval_hours` | 1 | Nhịp gom bút toán thành đợt cho nhà bán |

**Hai khóa giá hãng vận chuyển mặc định 0 là im lặng CÓ CHỦ Ý.** Chưa khai
thì không bút toán chi phí nào được ghi. Đặt một con số đoán vào mã nghĩa
là sổ cái ghi sai mà không ai biết, và ngày khai giá thật thì mọi bút toán
cũ đều lệch.

### Đo trên dữ liệu thật (11/09/2026)

```text
khóa đã từng được đặt giá trị:  1/22
  checkout.tax_rate_bp = 800    (đúng bằng mặc định)
```

Hai mươi mốt khóa còn lại chạy bằng mặc định biên dịch sẵn. Đó không phải lỗi —
mặc định là giá trị đúng cho tới khi có lý do đổi. Nhưng nó nói một điều
cần biết: **đường ghi cấu hình gần như chưa được dùng thật**, nên đừng
giả định nó đã được kiểm chứng bởi vận hành hằng ngày.

---

## 3. Ghi chú khi đổi từng nhóm

Sáu nhóm dưới đây vừa được đưa lên (11/09/2026). Chúng xếp theo mức ảnh
hưởng tới **tiền của người khác**, và mỗi nhóm có một cái bẫy riêng.

### 3.1 Trọng số buy box — quyết định nhà bán nào có doanh thu

Con số nặng nhất trong cả danh sách: nó quyết định **offer của ai được
hiển thị mặc định**, tức là ai bán được hàng.

**Chúng là trọng số TƯƠNG ĐỐI, không phải phần trăm.** Công thức chia cho
tổng, nên 40/30/30 và 4/3/3 cho cùng kết quả. Đó là lý do KHÔNG có ràng
buộc "cộng đúng 100": một ràng buộc như thế sẽ từ chối mọi bước trung
gian, và không ai đi từ 40/30/30 tới 50/25/25 được nữa.

Đặt cả ba về 0 thì mọi offer cùng 0 điểm và buy box thành ngẫu nhiên — hệ
thống **rơi về mặc định** trong trường hợp đó thay vì làm theo.

Cái bẫy đã có sẵn ở đây trước khi đưa lên: `Deps.Weights` là một GIÁ TRỊ
chốt lúc khởi động, và không ai truyền vào. Nay là một CỔNG đọc mỗi lần
tính.

### 3.2 Điểm hiệu suất mặc định của nhà bán mới

Lựa chọn **tăng trưởng**, không phải hằng số kỹ thuật. Đặt thấp thì nhà
bán mới gần như không bán được đơn đầu tiên — mà không có đơn đầu tiên thì
không bao giờ có lịch sử để thoát ra.

**Chưa quan sát được từ bên ngoài**, và đó là sự thật cần biết: mọi nhà bán
chưa có lịch sử đều dùng CÙNG con số này, nên họ luôn hòa nhau ở thành
phần hiệu suất. Nó chỉ tạo khác biệt khi module chấm điểm thật ra đời
(Phase 2). Tham số đã sẵn sàng; hành vi thì chưa có gì để đo.

### 3.3 Biểu phí vận chuyển — khách trả tiền theo nó

Hai con số tiền là **giá hiện trên màn hình thanh toán**. Hai con số ngày
là **lời hứa giao hàng**: chúng thành ngày giao dự kiến trên trang theo dõi
đơn, tính từ lúc bàn giao cho hãng.

**Đặt cạnh `fulfillment.carrier_cost_*` khi đổi.** Phí là thứ khách trả,
carrier cost là thứ nền tảng trả hãng, và hiệu của chúng là lãi/lỗ mảng
vận chuyển. Đặt phí thu thấp hơn phí trả là lỗ trên mỗi đơn — và không có
gì trong hệ thống chặn điều đó.

### 3.4 Thời gian chuẩn bị hàng mặc định

Áp cho offer nào **không tự khai**. Nhà bán CÓ khai thì lựa chọn của họ
được giữ nguyên — chính sách không lấn lên cam kết của người bán.

Con số này đi vào cả điểm buy box lẫn ngày giao dự kiến báo cho khách, nên
đặt thấp là hứa thay cho nhà bán một điều họ chưa cam kết.

### 3.5 Hoa hồng: mức đề xuất và SÀN

Tỷ lệ của từng nhà bán là **dữ liệu** (khác nhau theo hợp đồng). Hai con số
ở đây là **chính sách** quanh nó.

```text
mức đề xuất   con số ĐIỀN SẴN khi duyệt. Nó không tự áp cho ai — nhưng
              phần lớn người duyệt giữ nguyên giá trị điền sẵn, nên trên
              thực tế nó là tỷ lệ của đa số nhà bán mới.
sàn           duyệt DƯỚI mức này bị TỪ CHỐI, kèm thông báo nói rõ sàn
              đang là bao nhiêu.
```

**Nhà bán own brand luôn được miễn sàn**: nền tảng không thu hoa hồng của
chính mình, và một ràng buộc `CHECK` ở database cưỡng chế điều đó.

Cả hai mặc định 0 = giữ nguyên hành vi cũ. Đặt sàn là một quyết định có
chủ đích, không phải thứ bật sẵn.

### 3.6 Nhịp tạo đợt đối soát

Nhịp **gom** bút toán thành đợt. Việc chi trả vẫn do người duyệt, nên nới
nhịp không nới một kiểm soát tiền nào — nó chỉ đổi việc nhà bán **thấy**
khoản của mình sớm hay muộn.

**Nhịp này cũng được công bố cho luật cảnh báo** (`gouse_worker_job_interval_seconds`),
và ngưỡng "job treo" tính theo chính nó. Đổi nhịp sẽ tự nới ngưỡng — nếu
không thì nới nhịp sẽ tạo một cảnh báo luôn kêu, đúng thứ đã phải đi sửa
một lần rồi.

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
6. Viết một bài test PHÁ: đổi tham số qua ĐÚNG API quản trị mà người thật
   dùng, rồi đo ở chỗ NGƯỜI DÙNG NHÌN THẤY. Không gọi thẳng adapter —
   adapter đúng mà tham số vẫn không tới nơi là chuyện đã xảy ra.
```

Bài test ở bước 6 phải **đo được hành vi**, không chỉ đo rằng tham số đặt
được. Nếu một tham số chưa quan sát được từ ngoài — như
`marketplace.default_performance_score`, nơi mọi nhà bán mới đều dùng cùng
một con số nên luôn hòa nhau — thì **ghi rõ điều đó** thay vì viết một bài
test chỉ kiểm biên rồi coi là xong.

Bước 6 không phải thủ tục. `fulfillment.carrier_cost_*` đã khai đủ, đã nối
đủ, và vẫn im lặng suốt vì một trường khác (`shipping_method`) rỗng — hai
lý do độc lập cùng dẫn tới giá 0. Chỉ một bài test đi hết đường mới thấy.

---

## Liên quan

- [ADR-0015](../adr/0015-cau-hinh-van-hanh.md) — cơ chế, quyền, ghi vết
- [ADR-0011](../adr/0011-audit-log.md) — nhật ký thao tác
- `internal/platform/opsconfig/registry.go` — sổ đăng ký
- `gouse-web/apps/admin/src/app/config/page.tsx` — giao diện
