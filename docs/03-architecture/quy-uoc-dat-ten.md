# Quy ước đặt tên

**Định danh Go bằng TIẾNG ANH. Chú thích và tài liệu bằng TIẾNG VIỆT.**

Quyết định 24/10/2026. Trước đó định danh là tiếng Việt không dấu
(`kiemKhungGia`, `DoiSoat`, `TaoHoaHong`), và việc đổi đang làm theo từng
module — xem "Tiến độ" ở cuối.

## 1. Vì sao tách hai thứ

Chú thích của dự án này mang phần LỊCH SỬ QUYẾT ĐỊNH: vì sao chọn cách này,
cái gì đã hỏng, phép phá nào đã chạy. Dịch chúng sang tiếng Anh là viết lại,
không phải đổi tên — và bản viết lại sẽ mất sắc thái.

Định danh thì khác: nó là từ vựng, không phải văn. Một tên tiếng Anh đọc được
với mọi người đọc mã Go, kể cả thư viện chuẩn mà nó nằm cạnh.

```go
// Reverse đảo khoản hoa hồng khi đơn bị hủy hoặc hoàn hàng (quy tắc 5).
//
// `reason` BẮT BUỘC: một khoản tiền biến mất mà không nêu lý do là điểm mù
// trong kiểm toán.
func (c *CommissionRecord) Reverse(reason string, now time.Time) error {
```

## 2. Một chú thích nêu tên ĐÃ CHẾT tệ hơn không có chú thích

Nên mỗi lượt đổi tên phải sửa CẢ những chỗ NÊU TÊN định danh:

```text
chú thích trong .go
docs/**              kể cả backlog và ADR
sổ của cmd/archcheck · cmd/apicheck · cmd/eventcheck
                     chúng lưu tên kiểu Go dưới dạng CHUỖI
```

Sổ của apicheck là chỗ dễ quên nhất: `capEnumDaKiem` trỏ tới
`"modules/payment/domain"` + `"TrangThaiDoiSoat"` bằng chuỗi, nên đổi tên
kiểu mà quên sổ sẽ làm công cụ đỏ ở một thông điệp không liên quan.

## 3. Công cụ: `gofmt -r`, không phải `sed`

```bash
gofmt -r 'kiemKhungGia -> checkPriceFrame' -w internal/modules/marketplace
```

`gofmt -r` làm việc ở tầng **AST**: nó đổi đúng định danh, và KHÔNG chạm
chú thích, chuỗi, hay thẻ `json:`. `sed` thì chạm cả ba.

Hệ quả cần biết: vì nó không chạm chú thích, chú thích nêu tên cũ phải sửa
bằng một lượt RIÊNG (mục 2).

### Phạm vi phải giới hạn theo THƯ MỤC

`gofmt -r` đổi mọi định danh cùng tên trong cây được truyền vào. Nhiều tên
ngắn tồn tại ở nhiều module với nghĩa khác nhau, nên luôn truyền thư mục
module, không truyền `.`:

```text
ĐÃ XẢY RA 24/10/2026: một lượt sửa văn bản không giới hạn thư mục đổi
`BenChiuNenTang` của `checkout` trong khi `BenChiuGiamGia` và
`BenChiuChiaDoi` ở cạnh thì không — một enum NỬA ANH NỬA VIỆT, tệ hơn cả
hai thái cực. Đã lùi và làm lại có giới hạn.
```

### Đích đổi tên cũng phải tránh TÊN ĐÃ CÓ trong cùng module

Trước khi áp, so tập ĐÍCH với tập định danh đang có của module. `notification`
có bốn chỗ va: `err`, `channel`, `reason`, `sent` đều đã tồn tại, nên
`loi`/`kenh`/`lyDo`/`daGui` phải nhận tên khác (`failure`, `channelName`,
`skipReason`, `wasSent`).

`gofmt -r` sẽ đổi mọi định danh cùng tên trong cây được truyền vào, nên một
đích trùng trong CÙNG phạm vi tạo khai báo trùng — lỗi biên dịch, không phải
lỗi im lặng, nhưng mất một lượt sửa.

Trùng tên ở hai GÓI khác nhau thì không sao: `skipReason` là trường ở
`domain` và biến cục bộ ở gói test — hai phạm vi, một tên, hợp lệ.

### Loại trước những tên CÒN khai ở module khác

Lượt sửa chú thích/văn bản ngoài phạm vi module phải bỏ mọi tên còn tồn tại ở
nơi khác. Cách đo: sao cây mã sang thư mục tạm, XÓA module đang đổi, liệt kê
định danh còn lại, rồi lấy phần giao với bản đồ.

Ở lượt `returns` + `analytics`, phép này loại 21 tên — gồm `HanDoiTra`,
`TimTheoNhaBan`, `hoaHong`, `dem`, `doc`, `loai`. Không loại thì lượt sửa sẽ
đổi tên của `payment`, `fulfillment`, `order` theo, và lại ra một enum nửa
Anh nửa Việt như sự cố 24/10.

### Định danh CÓ DẤU vẫn hợp lệ trong Go — và vẫn phải đổi

`pricing` có hai biến cục bộ tên `kỳ` và `có`. Go cho phép, nên không có gì
báo. Bộ lọc bắt được chúng bằng nhánh "có dấu tiếng Việt", không bằng từ
điển âm tiết.

### Từ tiếng Việt THƯỜNG GẶP: đổi định danh, ĐỪNG đổi trong chú thích

`có`, `kỳ`, `trong`, `khongCo` là từ xuất hiện khắp văn xuôi. `gofmt -r` an
toàn vì nó ở tầng AST, nhưng lượt sửa chú thích thì phải GIỚI HẠN theo độ
dài tên — ở lượt này là ≥ 10 ký tự. Đổi `trong` trong chú thích sẽ phá câu.

### Hai bài test CÙNG TÊN ở hai gói: hợp lệ mà khó đọc

`TestCayDanhMuc → TestCategoryTree` tạo ra hai `TestCategoryTree`, một ở
`application` một ở `infrastructure/postgres`. Go cho phép và `go test` chạy
cả hai — nhưng một dòng `FAIL TestCategoryTree` không nói được cái nào.

Phép so tập đích với tập tên sẵn có bắt được điều này TRƯỚC khi áp. Đặt tên
phân biệt: `TestCategoryTreeFromStore`.

### Đừng để sót tên NGẮN

Bản đồ đầu của `returns` bỏ qua 14 tên một âm (`don`, `dong`, `tien`, `han`,
`cong`, `doi`, `giam`, `phan`, …) vì chúng khó đặt tên. Kết quả là một module
có mặt công khai tiếng Anh và thân hàm tiếng Việt — đúng cái bất nhất cần
tránh. Làm nốt trong cùng lượt.

Đo lại sau mỗi lượt bằng chính bộ lọc: module coi là XONG khi chỉ còn dương
tính giả (`Module`, `module`, `newModule`, `day`).

### Trường của struct literal lồng nhau: regex không khớp được

Đổi tên trường `DongY:` → `Consent:` trong `notification.CustomerPortFunc{…}`
bằng regex `\{[^}]*?` KHÔNG chạy: literal chứa `func(){…}` nên dấu `}` đầu
tiên kết thúc sớm. Khớp theo DÒNG kèm thụt lề chính xác thay vì theo khối.

### Method của interface do PLATFORM sở hữu thì GIỮ tên cũ

`privacy.BoMaHoa` có method `MaHoa`. Bản đồ của `seller` đổi `MaHoa →
Encrypted` cho một TRƯỜNG của nó, và `gofmt -r` đổi luôn lời gọi
`s.maHoa.MaHoa(...)` thành `s.encrypted.Encrypted(...)` — method của
`platform/privacy`, module không nằm trong lượt.

Trường thì đổi được (`maHoa → encrypted`); method của bên khác thì không.
Phân biệt hai thứ trùng tên khác chữ đầu là chỗ `gofmt -r` không giúp được.

### Cổng do module NÀY khai mà module KHÁC cài: cũng phải hoãn

Ngược với trường hợp dưới. `customer.OrderPort` do `customer` khai nhưng
`order.Module` cài. Interface của Go là CẤU TRÚC, nên đổi tên method ở cổng mà
không đổi bên cài làm `order.Module` không còn thỏa — và lỗi hiện ở chỗ GÁN
trong `internal/app`, không ở chỗ sửa.

Hai method ấy từng giữ tên tiếng Việt kèm chú thích nói vì sao. Lượt `order`
đã đổi cả hai đầu cùng nhau và chú thích hoãn ĐOÁN ĐÚNG: lỗi hiện ra ở
`internal/app/app.go` chứ không ở chỗ sửa —

```text
*order.Module does not implement customer.OrderPort
    (missing method DemDonVangLai)
```

Quy tắc còn giá trị cho lượt sau: thấy cổng kiểu này thì đổi HAI ĐẦU trong
cùng một lượt, đừng đổi một đầu rồi chạy `go build` ở module đó mà tưởng
xong.

### Method cài interface của module KHÁC thì GIỮ tên cũ

Một adapter ở `internal/app` cài cổng do module khác khai. Đổi tên method ở
adapter mà không đổi bên khai cổng làm nó không còn cài interface — và lỗi
hiện ra ở chỗ GÁN, không ở chỗ sửa:

```text
goiYSizeAdapter does not implement producthttp.GoiYSizePort
    (missing method GoiYSize)
```

Đã xảy ra 24/10/2026 với `GoiYSize`. Lúc ấy cổng thuộc `product` — module
chưa đổi tên — nên adapter giữ tên cũ kèm chú thích. Lượt `product` đã gỡ:
đổi CỔNG và BÊN CÀI trong CÙNG một lượt (`GoiYSizePort` →
`SizeSuggestionPort`, `GoiYSize` → `SuggestSize`) thì không còn gì phải hoãn.

Quy tắc còn giá trị: khi bên khai cổng chưa tới lượt thì hoãn, và khi tới
lượt thì đổi hai đầu cùng nhau.

### `find` KHÔNG có đường dẫn mặc định về `.` — và quét CẢ REPO

```bash
D=internal/modules/payment            # gán trong MỘT lệnh Bash
# ... lệnh khác, shell MỚI, D rỗng ...
find $D -name '*.go' 2>/dev/null | xargs sed -i '' "s/old/new/"
```

`find -name '*.go'` không có đường dẫn: BSD find lấy `.`, nên 248 luật `sed`
quét toàn bộ cây thay vì một module. `2>/dev/null` che mất lời cảnh báo duy
nhất. `go vet` XANH — vì sed đổi nhất quán cả khai báo lẫn mọi chỗ dùng, nên
nó vẫn biên dịch được: 59 tệp ngoài module bị đổi, trong đó `platform/webhook`
và `fulfillment` bị đổi tên định danh của RIÊNG chúng.

Chỉ `git diff --stat` ngoài module mới thấy. Hai cách phòng:

```bash
find internal/modules/payment -name '*.go' > /tmp/files.txt   # ĐƯỜNG DẪN ghi thẳng
while IFS= read -r f; do sed -i '' "s/old/new/" "$f"; done < /tmp/files.txt
```

và sau MỖI lượt quét: `git diff --name-only -- . ':!<module>' | wc -l` phải
bằng số tệp mình CHỦ Ý sửa ngoài module, không phải "ít thôi".

### Từ Việt KHÔNG DẤU là từ duy nhất nguy hiểm khi quét chú thích

Chú thích của dự án viết CÓ DẤU, nên `Dong`, `Huy`, `Buoc` không khớp văn
xuôi ("dòng", "hủy", "bước"). Nhưng `Ghi` thì khớp: nó không dấu, và là động
từ mở đầu câu rất thường gặp. Luật `Ghi -> Log` đã đổi 18 chỗ văn xuôi thành
"Log vào vết", "Log hai lần", "Log sổ".

Thước đo đúng không phải ĐỘ DÀI mà là: tên cũ có phải một từ Việt KHÔNG DẤU
hợp lệ không. Danh sách phải loại tay: `ghi doc tra lai moc sau truoc den
cho khong gia tien ky phan nhan dau bao cong dao don goc lech dong huy buoc`.

### Sổ gác ghim tên định danh ở BA chỗ, không phải một

Tới lượt `internal/platform` thì cả ba đã lộ ra:

```text
cmd/apicheck/enum_cap.go   "TrangThaiDoiSoat"    tên KIỂU, dạng chuỗi
cmd/apicheck/dto_cap.go    "sellerFOJSON"        tên KIỂU, dạng chuỗi
cmd/apicheck/header.go     "HeaderChoPhep"       tên BIẾN, đọc bằng AST
```

Cái thứ ba khác hai cái đầu: `apicheck` PHÂN TÍCH `cors.go` để đọc danh
sách header, nên đổi tên biến làm nó không tìm thấy gì. Cả ba lần thông báo
lỗi đều tự nói phải sửa đâu — hàng rào viết tốt thì không cần ai đoán.

Phép rà trước khi đổi tên một định danh EXPORT của `platform`:

```bash
grep -rn '"<TenCu>"' cmd/           # ghim dạng chuỗi
grep -rn 'Name != "<TenCu>"' cmd/   # ghim dạng AST
```

### `grep -vc '`'` loại CẢ DÒNG, không loại phần trong backtick

Phép đo văn xuôi báo `ban` 0 dòng, nên tôi cho nó vào lượt quét và nó biến
`// Đặc tả ban đầu ghi 10 ký tự trong khi \`audit.ValidateReason\` cưỡng
chế` thành `// Đặc tả shop2 đầu …`. Dòng ấy CÓ backtick — ở chỗ khác — nên
`grep -v '\`'` bỏ luôn cả dòng.

Đo đúng là BỎ PHẦN trong backtick rồi mới đếm:

```python
cmt.append(re.sub(r'`[^`]*`', '``', line))   # giữ dòng, bỏ phần backtick
```

Đo lại đúng cách ra: `kho` 131 dòng, `sau` 74, `tham` 40, `duy` 29, `cam`
19, `khung` 12, `ban` 2.

### `gofmt -r` an toàn với văn xuôi — chỉ lượt QUÉT mới cần loại trừ

Tôi đã loại 40 tên khỏi CẢ hai lượt, nên 40 định danh ấy không được đổi và
phải làm thêm hai lượt nữa. Danh sách loại trừ thuộc lượt QUÉT CHÚ THÍCH,
không thuộc lượt `gofmt -r`.

### Phục hồi chuỗi: so với HEAD, đừng áp luật nghịch

Lượt `internal/app` làm hỏng 44 chuỗi. Áp bảng NGHỊCH trong chuỗi thì đổi
luôn những chuỗi vốn đã tiếng Anh ở HEAD (`"ord-1"` thành `"don-1"`).

Cách đúng là đọc HEAD: với mỗi tệp, so danh sách literal của bản hiện tại
với bản HEAD theo THỨ TỰ, và phục hồi mọi chỗ khác nhau — trừ chuỗi có nêu
tên định danh đã đổi. Số literal phải khớp; không khớp thì bỏ qua tệp ấy và
đọc tay.

### Chỗ thứ TƯ sổ gác ghim tên định danh

```text
cmd/apicheck/enum_cap.go     "TrangThaiDoiSoat"   tên KIỂU
cmd/apicheck/dto_cap.go      "sellerFOJSON"       tên KIỂU
cmd/apicheck/header.go       "HeaderChoPhep"      tên BIẾN, đọc bằng AST
cmd/eventcheck/bo_ben_nhan.go "dangKyBenNhan"     tên HÀM trong tệp TEST
```

Cái thứ tư gác một hàm của `internal/app/api_ledger_test.go`. `make check`
đỏ kèm câu "đổi tên hàm thì phải sửa cả công cụ này".

### Chuỗi CÓ KHI phải đổi: phân biệt bằng backtick

Ba công cụ kiểm nhúng tên sổ gác vào thông báo lỗi, để người đọc lỗi biết
sửa đâu:

```go
"— đổi tên kiểu thì phải sửa cả sổ `capEnumDaKiem`"
{"khongAiPhat", c.khongPhat}   // tên sổ, in ra qua `sổ %s`
```

Đổi tên sổ mà KHÔNG đổi chuỗi thì thông báo lỗi trỏ vào một cái tên không
còn tồn tại — đúng thứ mục 2 cấm. Nên phép rà literal không phải "phải
rỗng" mà là "phải PHÂN LOẠI ĐƯỢC":

```python
mat = [chuỗi có ở dòng '-' mà không có ở dòng '+']
ngoai_backtick = [s for s in mat if '`' not in s]   # <- đọc tay từng cái
```

Lượt này: 31 chuỗi đổi, 27 trong backtick (đúng), 4 ngoài — đọc tay thì cả
bốn cũng là tên định danh nhúng vào thông báo. Lượt `cmd/worker` thì ngược
lại: 4 chuỗi, cả 4 đều sai.

### Lượt quét chú thích cũng đổi CHUỖI — rà bằng cách đếm literal

`sed` không biết đâu là chú thích, đâu là chuỗi. Luật `chung -> shared` đổi
`chay("chung", true)` thành `chay("shared", true)` — một nhãn tiếng Việt
đứng cạnh `"riêng"`, và chẳng gì đỏ.

Phép rà chạy SAU mỗi lượt quét, đọc chính diff:

```python
# chuỗi nào có trong các dòng BỊ XÓA mà không có trong các dòng THÊM VÀO?
pre  = literals trong mọi dòng '-'
post = literals trong mọi dòng '+'
[s for s in pre if s not in post]      # phải RỖNG
```

Nó tìm ra bốn chuỗi: `"chung"`, `"khach@gouse.test"`, và hai câu tiếng Việt
có chứa "dùng chung".

### Phép ĐO cũng chết vì zsh không tách từ

```bash
DIRS="cmd/worker cmd/dotai ..."
grep -rn '^\s*//' --include='*.go' $DIRS | grep -w "chung"   # 0 dòng!
```

Phép đo báo "không tên nào là văn xuôi", nên tôi cho `chung` vào lượt quét
và nó hỏng ba chỗ. `grep` nhận cả danh sách làm MỘT đường dẫn và không khớp
gì — rỗng trông giống hệt sạch.

Dùng danh sách TỆP, như lượt quét:

```bash
while IFS= read -r f; do grep -h '^[[:space:]]*//' "$f"; done < files.txt \
  | grep -w "$old" | grep -vc '`'
```

Đo lại đúng cách ra: `ghi` 46 dòng, `ngay` 8, `chung` 6, `xong` 5.

### `collide.py` không thấy TRƯỜNG của struct

Nó cắt tệp theo `^func `, nên khai `type X struct` nằm ngoài tầm. Lượt
`cmd/worker` vì vậy lọt:

```go
type job struct { interval time.Duration; intervalFrom func() … }
func (j job) nhip() time.Duration            // nhip -> interval
```

→ `field and method with the same name interval`. Cùng lớp lỗi với
`fulfillment` (`han`/`hanDoiTra`), và lần đó cũng chỉ trình biên dịch bắt.

`collide2.py` thêm lớp B: với mỗi `type X struct`, so tập TRƯỜNG với tập
METHOD của chính kiểu ấy SAU khi áp luật. Chạy thử trên ca vừa gặp thì nó
bắt đúng.

### Phép ĐO danh sách loại trừ — dùng từ lượt `internal/kernel`

Thay vì nhớ, đếm: tên cũ nào xuất hiện trong chú thích NGOÀI backtick nhiều
dòng thì là văn xuôi.

```bash
tho=$(grep -rn '^\s*//' --include='*.go' <dir> | grep -w "$old" | grep -vc '`')
[ "$tho" -ge 5 ] && echo "LOẠI (văn xuôi): $old"
```

Lượt `internal/kernel` loại đúng `ngay` (62 dòng) và `trong` (18 dòng), và
để lại `CuoiNgay` (2 dòng — tên hàm thật). Không chỗ văn xuôi nào bị đụng.

Ngưỡng 5 là kinh nghiệm: một tên hàm được nhắc tới 1–3 lần trong chú thích
doc của chính nó; một từ tiếng Việt xuất hiện hàng chục lần.

### Danh sách từ Việt KHÔNG DẤU phải ĐẦY, không gần đầy

Lượt `checkout` loại sẵn 29 tên theo bài học hai lượt trước, nhưng quên
`nguong` ("ngưỡng"). Một chú thích viết `// TestMienPhiShipKhiDAT nguong`
— tác giả cố ý bỏ dấu để nhấn — thành `KhiDAT threshold`.

Chuỗi ba lượt: `Ghi` (payment), `GiaTri` (fulfillment), `nguong` (checkout).
Mỗi lượt loại thêm một tên và lượt sau lại sót một tên khác, vì danh sách
được dựng bằng cách NHỚ. Dựng bằng cách ĐO thì không sót:

```bash
# tên cũ nào xuất hiện trong chú thích TRƯỚC khi áp luật?
grep -rn '^\s*//' internal/modules/<m> | grep -w "<ten_cu>"
```

Tên nào có mặt trong chú thích mà KHÔNG nằm trong backtick thì là văn xuôi,
và phải loại khỏi lượt quét.

### Một ĐÍCH đặt sai không làm gì đỏ

`UocTinhPhiGiao` là HÀM ở `fulfillment` nhưng là KIỂU ở `checkout`. Tái dùng
đích `EstimateShippingFees` cho cả hai làm `checkoutapp.EstimateShippingFees{}`
— một kiểu mang tên động từ. Biên dịch được, test xanh, chỉ đọc mới thấy.
Cùng một từ Việt ở hai module có thể là hai loại định danh khác nhau.

### Lượt quét chú thích cũng đụng MÃ của gói khác

Nó là `sed` văn bản trên CẢ tệp, nên nó không phân biệt chú thích với mã.
`GiaTri -> Value` làm vỡ `opsconfig.DatInput{GiaTri: 24}` — một trường của
gói KHÁC mà `gofmt -r` đã bỏ qua đúng. Hai lần, vì lần đầu tôi chỉ sửa chỗ
vỡ chứ không sửa BẢNG.

Mục "Loại trước những tên CÒN khai ở module khác" áp cho CẢ lượt quét chú
thích, không chỉ lượt `gofmt -r`:

```bash
go run ident/main.go internal/platform > /tmp/ngoai.txt
go run ident/main.go internal/kernel  >> /tmp/ngoai.txt
comm -12 <(cut -f1 map.tsv | sort -u) <(awk '{print $1}' /tmp/ngoai.txt | sort -u)
```

Ra `DaNhanTruocDo DaXuLyXong DanhDauXong GiaTri` — bốn tên `platform/webhook`
và `opsconfig` cũng dùng.

### Đích đổi tên phải tránh cả TÊN METHOD của cùng kiểu

```go
type Service struct{ han HanDoiTraPort }        // han          -> returnWindow
func (s *Service) hanDoiTra() time.Duration     // hanDoiTra    -> returnWindow
```

`field and method with the same name returnWindow`. Hai luật khác nhau trỏ
về CÙNG một đích trên CÙNG một kiểu. Kiểm trước bằng cách soi các cặp
old→new có đích trùng nhau, không chỉ đích trùng tên đã có.

### Cổng ngược: module KHÁC khai, `payment` cài

`checkout.PaymentAPI` khai `TaoIntent`, `payment.Module` cài. Đổi tên ở
`payment` làm `internal/app` không gán được — cùng dạng với
`customer.OrderPort`, nhưng chiều NGƯỢC LẠI. Đổi tên method ở cổng của
`checkout` trong cùng lượt là đủ; không phải đổi cả module `checkout`.

### Ba phép rà TRẢ LỜI "SẠCH" MÀ KHÔNG RÀ GÌ CẢ

Cả ba đều xảy ra thật trong lượt `inventory`/`product`, và cả ba đều IM LẶNG
hoặc gần im lặng — đúng loại nguy hiểm nhất, vì kết quả rỗng trông giống hệt
kết quả tốt.

```text
grep -rl "[[:<:]]Ten[[:>:]]"      grep BSD KHÔNG hiểu [[:<:]] — luôn rỗng
grep -rlw "Ten"                   đúng

FILES=$(find . -name '*.go')      zsh KHÔNG tách từ $FILES
sed -i '' "s/a/b/" $FILES         sed nhận cả danh sách làm MỘT tên tệp
find . -print0 | xargs -0 sed …   đúng

set -- $pair                      zsh KHÔNG tách; $2 giữ giá trị cũ
while IFS=' ' read -r a b         đúng
```

`sed` BSD thì CÓ hiểu `[[:<:]]`, nên quét chú thích bằng `sed` chạy đúng
trong khi phép rà bằng `grep` ngay cạnh nó báo sạch. Hai công cụ, hai cú
pháp — và chỉ một trong hai báo lỗi.

Phép rà "còn chỗ nào ngoài module nhắc tên cũ không" báo RỖNG hai lần liền.
Lần thứ ba, chạy bằng `grep -w`, nó tìm ra 23 chỗ: đặc tả OpenAPI nhắc tên
HÀM Go (`SuyRaNhomMau`), `openapi.d.ts` sinh ra từ đặc tả, một chú thích ở
`gouse-web`, một bài test e2e, và `docs/10-roadmap/backlog.md`.

### Rà lại bằng `ident` sau khi áp là BẮT BUỘC

Bốn tên lọt lượt `product` vì chỉ nghĩ trong đầu mà không ghi vào bảng:
`tao`, `boQua`, `coKetQua`, `tenPG`. `go build`, `go vet`, `make check` đều
XANH — không có cách nào thấy ngoài việc đếm lại.

### CẢ HAI công cụ đo đều bỏ sót — và đó mới là vấn đề

Bộ âm tiết thiếu `ly`, `trang`, `cac`, `goi`, `khop`, `chuan`, nên
`LyDo`, `TrangThai`, `CacSize`, `GoiY`, `KhopBienThe`, `SizeDaChuan` KHÔNG
bị đếm. Mười hai module ghi "XONG" mà vẫn còn tên tiếng Việt; `returns` còn
mười lăm, trong đó có cả hai kiểu enum lõi.

Vá bộ âm tiết thì nó lại sót tiếp (`chinh`, `sach` → `seller` vẫn giữ
`ChinhSachPort`). Một ALLOWLIST âm tiết Việt KHÔNG BAO GIỜ đầy: mỗi lần vá
là mỗi lần đoán thêm. Cách đúng là đo NGƯỢC — liệt kê định danh có từ KHÔNG
có trong `/usr/share/dict/words` (và không nằm trong danh sách từ kỹ thuật).
Bộ ngược ồn hơn, nhưng ồn thì ĐỌC ĐƯỢC, còn sót thì không.

Hai bộ bổ nhau: bộ Việt cho con số, bộ ngược cho danh sách phải đọc. Dùng
CẢ HAI mỗi lượt.

### Và công cụ LIỆT KÊ định danh cũng có điểm mù

`ident` chỉ đọc `f.Decls` cộng `AssignStmt` kiểu `:=`, nên KHÔNG thấy:

```text
var ( a, b atomic.Int64 )   khai `var` NHÓM trong thân hàm
for _, x := range xs        biến của RangeStmt
func(a, b int) { … }        tham số của func literal
func f() (n int, err error) kết quả CÓ TÊN
type t struct{…}            `type` khai trong hàm
```

Sửa thành MỘT lượt `ast.Inspect` qua mọi nút: `inventory` nhảy từ 566 lên
603 định danh, và lộ ra `hetHang`, `thanhCong`, `xungDot`, `soKhach`,
`batDau`, `daGiu` — một bài test đua tranh viết toàn tiếng Việt mà hai lượt
đo trước đó báo "sạch".

Thứ tự kiểm là: sửa công cụ TRƯỚC, đo lại SAU. Đo bằng công cụ hỏng thì con
số tử tế chỉ nói rằng công cụ không nhìn thấy gì.

Bài học không phải "vá bộ lọc" mà là: con số ở bảng Tiến độ chỉ đáng tin
bằng công cụ đo nó. Thấy một tên sót thì ĐỪNG sửa riêng tên ấy — vá bộ lọc
rồi ĐO LẠI mọi module đã ghi xong.

Lượt đo lại tìm ra 29 tên ở 9 module, trong đó `returns` giữ CẢ HAI kiểu
enum lõi (`LyDo`, `TrangThai`) cùng ba hằng trạng thái — một enum NỬA Anh
nửa Việt (`ReasonSizeTooSmall` cạnh `LyDoChatLuong`, `StatusReceived` cạnh
`TTYeuCau`), đúng thứ mục 2 gọi là tệ hơn cả hai thái cực. Cả hai kiểu còn
bị `cmd/apicheck/enum_cap.go` ghim bằng CHUỖI, nên đổi tên mà quên sổ gác
thì `apicheck` đỏ — lần này là hàng rào chạy đúng.

Và một chú thích nêu tên ĐÃ CHẾT sống sót qua lượt `affiliate`:
`// TrangThai là trạng thái…` đứng trên một khối `const` không còn kiểu
nào tên ấy.

### Biến vòng lặp trùng tên HÀM cùng gói

```go
func capOf(t int) int { … }

for _, capOf := range []int{0, -1, -999} {   // che hàm capOf
```

Biên dịch được, chạy được, đọc thì sai. Đích của một luật phải tránh cả tên
HÀM trong gói, không chỉ tên biến trong hàm.

### Tên bài test trùng nhau TÍCH LŨY qua các lượt

Bốn cặp sinh ra ở các lượt TRƯỚC, không lượt nào báo gì:

```text
TestMalformedIDReturnsErrInvalidID   seller + pricing
TestNotFoundReturnsErrNotFound       catalog + pricing
TestDoesNotStoreRawIP                identity + customer + analytics
TestEmailIsNormalized                identity + customer
```

Go cho phép, nên không có lỗi; chỉ một dòng `FAIL TestX` không nói được là
bài nào. Mỗi lượt phải chạy:

```bash
grep -rh "^func Test" --include='*_test.go' . \
  | sed 's/func \(Test[A-Za-z0-9_]*\).*/\1/' | sort | uniq -d
```

### Tên định danh Go nằm trong ĐẶC TẢ và SỔ GÁC

`cmd/apicheck/enum_cap.go` giữ tên kiểu Go dưới dạng CHUỖI
(`"NhomMau"`), và `api/components/common.yaml` nhắc tên hàm Go trong phần
mô tả. `go build` không thấy cả hai; `apicheck` thấy cái thứ nhất; cái thứ
hai chỉ người đọc thấy.

### Đích đổi tên phải tránh TÊN GÓI

```text
don → order    SAI ở file nào import gói `order`: tên cục bộ che tên gói
don → orders   đúng
```

## 4. Từ vựng đã chốt

Dùng bảng này cho các lượt sau, để hai module không gọi cùng một thứ bằng hai
tên.

| Việt | Anh | Ghi chú |
|---|---|---|
| quy công | `Attribution` / `Attribute` | danh từ / động từ |
| hoa hồng | `Commission` | |
| tiền gốc (tính hoa hồng) | `CommissionBase` | |
| đợt đối soát | `Settlement` | |
| bên chịu (chi phí) | `CostBearer` | |
| mô hình quy công | `AttributionModel` | |
| chờ đến hạn / rút được / đã đảo | `Pending` / `Available` | trạng thái tiền |
| đảo | `Reverse` | |
| chuyển sang rút được | `MarkAvailable` | |
| xác nhận / đánh dấu đã trả | `Confirm` / `MarkPaid` | hai bước ký |
| người ký | `SignedBy` | |
| khung giá | `PriceFrame` | |
| tín hiệu nhu cầu | `DemandSignal` | |
| nhà bán | `Seller` | |
| đơn / dòng đơn | `Order` / `OrderLine` | |
| giỏ / phiên | `Cart` / `Checkout` | |
| tạo | `New` | hàm dựng, theo quy ước Go |
| khôi phục (từ kho) | `Restore` | |
| lưu / cập nhật | `Save` / `Update` | |
| tìm theo X | `ByX` | `TimTheoNhaBan` → `BySeller` |
| tổng theo X | `TotalsByX` | |
| lý do | `reason` | |
| trạng thái | `Status` | |
| mốc thời gian | `CreatedAt` / `UpdatedAt` / `ReversedAt` | |
| cổng (port) | `XxxPort` | `DonPort` → `OrderLinesPort` |
| bên nhận event | `XxxHandler` + `NewXxxHandler` | |
| thiếu X | `ErrMissingX` | |
| X không hợp lệ | `ErrInvalidX` | |
| đã X rồi | `ErrAlreadyX` | |

### Tên bài test

Tên bài test là một CÂU KHẲNG ĐỊNH, và nó phải nói được điều gì sẽ sai:

```text
TestTienGocKepVe0                  →  TestBaseClampsToZero
TestChuaHetHanThiKhongRutDuoc      →  TestNotAvailableBeforeReturnWindowCloses
TestDaoIdempotentVaChanRutDuoc     →  TestReverseIsIdempotentAndBlocksAvailable
```

## 5. Những gì KHÔNG đổi

```text
chú thích và docs/**        tiếng Việt — xem mục 1
thẻ `json:`                 là HỢP ĐỒNG API, đổi là thay đổi phá vỡ
khóa JSONB trong audit_log  `gia_tri_cu`, `gia_tri_moi`. KHÔNG đổi được:
                            `audit_log` bất biến, trigger `audit_log_no_update`
                            chặn UPDATE, nên dòng cũ sẽ mãi mang khóa cũ. Đổi
                            nghĩa là bên đọc phải nhận CẢ HAI khóa vĩnh viễn
khóa log có cấu trúc        `"ly_do"`, `"nha_cung_cap"` trong `slog`. Không
                            phải lược đồ; chưa quyết
tên file                    phần lớn còn tiếng Việt
                            (`ben_chiu_giam_gia.go`). Đổi khi nội dung file
                            đã đổi, như `tien_goc.go` → `commission_base.go`
```

### Tên DB đã đổi — migration 000062 (24/10/2026), rồi 000063–65

```text
1 bảng    quan_sat_size → size_observation
9 cột     ket_qua · nguon_loai · nguon_id · quan_sat_luc
          khoa · gia_tri · sua_luc · sua_boi · ly_do
4 ràng buộc CHECK
5 chỉ mục
1 hàm + 1 trigger   kiem_but_toan_can_bang → check_ledger_entry_balanced
                    ledger_entry_phai_can_bang → ledger_entry_must_balance
```

**Migration MỚI, không sửa file cũ.** `migrate` theo dõi theo PHIÊN BẢN, nên
sửa `000039_ops_config.up.sql` sẽ không chạy lại ở database đã áp nó — file
và database sẽ nói hai chuyện khác nhau.

**`ALTER FUNCTION ... RENAME TO` không đủ cho hàm:** nó đổi được tên nhưng
không đổi được biến cục bộ trong thân (`so_dong`, `lech`), mà thân là chỗ
người đọc thật sự đọc. Phải tạo hàm tên mới, trỏ trigger sang, rồi bỏ hàm cũ
— thứ tự ấy giữ cho bất biến "mọi bút toán phải cân" không có khoảng trống
nào.

**PostgreSQL KHÔNG đổi tên chỉ mục khi đổi tên bảng.** Quên chúng để lại
`quan_sat_size_khach_idx` trên một bảng không còn tên ấy.

### Ba migration NỮA mới khép được tầng DB — 000063/64/65 (29/10/2026)

000062 tưởng là xong. Phép rà lược đồ (`information_schema` +
`pg_constraint` + `pg_indexes`, lọc tên tiếng Việt) nói **28 tên còn lại**:

```text
000063  1 cột   cho_thanh_toan → awaiting_payment   (lọt khỏi 000062)
        23 ràng buộc            tên sinh ra từ bảng/cột ĐÃ ĐỔI
        3 chỉ mục
000064  3 giá trị enum          DA_MUA/CHAT/RONG → PURCHASED/TIGHT/LOOSE
        1 tiền tố mã            evt → emt  (P3-82)
000065  11 tên handler          dữ liệu, xem dưới
```

**PostgreSQL không đổi tên RÀNG BUỘC khi đổi tên bảng hay cột.** Hệ quả
không phải thẩm mỹ — nó nằm trong THÔNG BÁO LỖI:

```text
ERROR: null value in column "key" violates not-null constraint
constraint "ops_config_khoa_not_null"
```

Người đọc lỗi đi tìm cột `khoa` và không có cột nào tên thế. `size_observation`
còn 14 ràng buộc mang tên bảng cũ `quan_sat_size`, kể cả khoá chính.

### Đổi TÊN khác đổi DỮ LIỆU, và `Handler.Name()` là dữ liệu

```go
// eventbus/dispatcher.go:208
INSERT INTO event_processed (event_id, handler) VALUES ($1, $2)
ON CONFLICT DO NOTHING
```

Tên handler là KHOÁ IDEMPOTENCY trong database. Đổi nó ở Go mà không đổi dữ
liệu thì mọi cặp (event, handler) đã xử lý không còn khớp, và một event phát
lại — chuyện bình thường của giao hàng ít-nhất-một-lần — sẽ được xử lý LẠI:
bút toán ghi hai lần, lượt dùng mã trừ hai lần.

Đo trước khi làm: `event_processed` có 42.362 hàng, 169 thuộc 11 handler cần
đổi. Phép kiểm sau khi làm không phải "test xanh" mà là:

```bash
comm -23 <tên handler trong DB> <Name() của Go>   # phải RỖNG
```

Cùng loại dữ liệu: giá trị enum (`size_observation.outcome`) và tiền tố mã
(`email_verification_token.id`). Cả ba đều cần `UPDATE`, và phép đảo phải
đổi lại TỪNG HÀNG — không chỉ đổi ràng buộc.

### Database KHUÔN của test không tự migrate

`testdb` dùng `gouse_test` làm khuôn rồi `CREATE DATABASE ... TEMPLATE` cho
mỗi gói. Thêm migration mà quên migrate khuôn thì `make check` KHÔNG nói
"khuôn tụt hậu" — nó đỏ ở một bài test không liên quan:

```text
--- FAIL: TestASellerBornePromotionRecordsTheRightBearer
    sổ cái trừ nhà bán 0 đ, cần 49000
```

Vì lỗi handler IM LẶNG: `drain()` ghi lỗi vào `event_outbox.last_error` rồi
đi tiếp, nên bài test chỉ thấy một bút toán thiếu. Mất một lượt bisect
(`git stash` rồi chạy lại ở HEAD) mới biết là do mình.

Sau khi thêm migration: `migrate -database "$TEST_DATABASE_URL" up`.

### `| head` cắt mất bằng chứng

`grep -rn cho_thanh_toan ... | head` trả 10 dòng và tôi tin là hết. Dòng thứ
11 ở `internal/e2e` làm một bài test đỏ sau đó. Cùng lớp với `| tail && echo`:
một pipeline rút gọn bằng chứng rồi để người đọc kết luận trên phần đã rút.

Phép rà cuối thì ĐỪNG rút gọn.

**Phân biệt CONSTRAINT với INDEX trước khi viết lệnh.** Bản đầu xếp
`email_verification_token_con_hieu_luc` vào khối `RENAME CONSTRAINT`;
PostgreSQL từ chối (`constraint ... does not exist`) vì nó là chỉ mục một
phần. Migration chạy trong MỘT giao dịch nên không để lại trạng thái nửa
chừng — kiểm bằng cách đọc lược đồ sau khi lỗi, rồi `force` về phiên bản
trước và áp lại.

## 6. Tiến độ

```text
XONG   định danh DATABASE              migration 000062
XONG   modules/affiliate               103 định danh
XONG   modules/recommendation           42
XONG   modules/supplychain              27
XONG   modules/notification             37
XONG   modules/promotion                30
XONG   modules/returns                 121
XONG   modules/analytics                61
XONG   modules/catalog                  26   (kernel, lá)
XONG   modules/pricing                  72   (kernel, lá)
XONG   modules/identity                 36   (kernel, lá)
XONG   modules/customer                 82   (kernel, lá)
XONG   modules/seller                   43   (kernel, lá)
XONG   modules/inventory               107   (kernel)
XONG   modules/product                 237   (kernel) + 12 sót ở recommendation
XONG   modules/marketplace              74   (kernel)
XONG   lượt DỌN SÓT                     140   ở 13 module đã ghi XONG
       (29 lượt một + 44 lượt hai + 47 lượt ba, sau khi sửa CẢ HAI công cụ)
XONG   modules/cart                     52   (kernel)
XONG   modules/order                    91   (kernel) + 2 method của customer
XONG   modules/payment                 216   (kernel) + 1 method của checkout
XONG   modules/fulfillment             248   (kernel)
XONG   modules/checkout                170   (kernel, GỐC) — HẾT 21 MODULE
XONG   internal/kernel                  45   (ids, money, types)
XONG   internal/platform               222   (13 gói, hai lượt)
XONG   cmd (worker, dotai, doisoat*, tao*)  151
XONG   cmd (apicheck, archcheck, eventcheck) 252  — HẾT cmd
XONG   internal/e2e                    120
XONG   internal/app                    555   (ba lượt)
HẾT    Bộ lọc tiếng Việt trả về RỖNG trên toàn repo.
       7934 định danh, không còn tên tiếng Việt nào.

ĐÃ GỠ HẾT phần hoãn. `customer.OrderPort` đổi cùng lượt `order`
       (`GanDonVangLaiChoKhach` → `AttachGuestOrdersToCustomer`,
       `DemDonVangLai` → `CountGuestOrders`), và cổng gợi ý size đổi cùng
       lượt `product`. Không còn method nào giữ tên tiếng Việt vì lý do
       "chờ module khác".

       Rồi `internal/platform`, `internal/app`, `cmd`.

Con số nay ĐO BẰNG CẢ HAI bộ (xem mục 3), nên cao hơn các lượt trước dù
công việc đã làm nhiều hơn — lượt trước đo bằng công cụ có điểm mù.

Nhóm KERNEL đi từ LÁ lên GỐC — module không import module nghiệp vụ nào
trước, `checkout` gần cuối, `internal/app` sau cùng vì nó gọi mọi module.
```

Đo lại bất cứ lúc nào bằng cách liệt kê định danh KHAI BÁO qua `go/ast` rồi
lọc — đếm bằng `grep` cho con số vô nghĩa, vì văn tiếng Việt trong chú thích
làm nhiễu (`ghi` xuất hiện ở 371 file, gần hết là văn xuôi).

Thứ tự đề xuất: module nhỏ và ít bị import trước (`recommendation`,
`supplychain`, `notification`), để mỗi lượt đổi gọn trong một commit có
`make check` xanh. `internal/app` làm SAU CÙNG: nó gọi mọi module.

## 7. Tài liệu liên quan

- [microkernel.md](microkernel.md) — phân tầng kernel / extension
- [modular-monolith.md](modular-monolith.md) — cấu trúc một module
