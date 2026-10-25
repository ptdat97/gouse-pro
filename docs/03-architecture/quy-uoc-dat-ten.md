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

Hai method ấy giữ tên tiếng Việt kèm chú thích nói vì sao, đổi cùng lúc với
`order`.

### Method cài interface của module KHÁC thì GIỮ tên cũ

Một adapter ở `internal/app` cài cổng do module khác khai. Đổi tên method ở
adapter mà không đổi bên khai cổng làm nó không còn cài interface — và lỗi
hiện ra ở chỗ GÁN, không ở chỗ sửa:

```text
goiYSizeAdapter does not implement producthttp.GoiYSizePort
    (missing method GoiYSize)
```

Đã xảy ra 24/10/2026 với `GoiYSize`: cổng thuộc module `product` (chưa đổi
tên), nên method của adapter giữ tên tiếng Việt kèm một chú thích nói vì sao.

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

### Tên DB đã đổi — migration 000062 (24/10/2026)

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
CÒN    1888 định danh Go ở 8 module

HOÃN   customer.OrderPort — hai method còn tiếng Việt vì `order.Module` CÀI
       chúng và `order` chưa đổi. Đổi cùng lúc với `order`.

Bảy module extension đã xong hết. Nhóm KERNEL đi từ LÁ lên GỐC — module
không import module nghiệp vụ nào trước, `checkout` gần cuối, `internal/app`
sau cùng vì nó gọi mọi module.
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
