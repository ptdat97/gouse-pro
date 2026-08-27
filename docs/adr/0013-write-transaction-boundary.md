# ADR-0013: Ranh giới giao dịch bao trọn phép đọc-rồi-ghi

**Trạng thái:** Accepted
**Ngày:** 26/08/2026

---

## Context

Hai lỗi mất dữ liệu được tìm ra trong cùng một ngày, ở hai module khác
nhau, có CHUNG một hình dạng.

### Lỗi 1 — thêm hàng vào giỏ

`Service.AddItem` đọc giỏ bằng `FindByID`, sửa trong bộ nhớ, rồi ghi bằng
`Save`. Hai giao dịch riêng biệt. Và cách ghi món là XÓA HẾT RỒI GHI LẠI
(hợp lý, vì món trong giỏ không bất biến và không có gì tham chiếu tới).

Hai lượt thêm hàng chạy chồng nhau:

```text
A: đọc giỏ (1 món) ─┐
B: đọc giỏ (1 món) ─┤ cả hai cùng thấy 1
A: xóa hết, ghi 2 món
B: xóa hết, ghi 2 món   ← ghi đè trọn vẹn lượt của A
```

Kết quả đo được: 12 request thêm hàng song song, **cả 12 trả HTTP 200**,
giỏ tăng từ 1 lên 3. Chín lần bấm biến mất mà không ai được báo.

Có một lượt ghi THỨ HAI nữa cũng nằm ngoài giao dịch: `AddItem` kết thúc
bằng `Sync`, và `Sync` ghi lại bản giỏ trong bộ nhớ sau khi giao dịch đã
commit — mở lại đúng cái cửa sổ vừa đóng.

### Lỗi 2 — hoàn tất phiên thanh toán

`CompleteCheckout` có ba lớp phòng vệ chống tạo đơn trùng. Cả ba đều thủng
khi hai request chạy chồng nhau:

| Lớp | Cách làm | Vì sao thủng |
|-----|----------|--------------|
| 1 | Đọc trạng thái phiên, thấy COMPLETED thì phát lại | Đọc ngoài mọi giao dịch. Ai vào trong khoảng trống cũng thấy "chưa hoàn tất" |
| 2 | `order.PlaceOrder` idempotent theo khóa | Khóa chống CÙNG một lần bấm bị gửi lặp, không chống hai lần bấm THẬT |
| 3 | Ghi phiên COMPLETED | Chạy SAU khi đơn đã tạo. Quá muộn |

Kết quả đo được: 8 request hoàn tất song song trên **một** phiên, mỗi
request một khóa idempotency riêng (mô phỏng nhiều tab) → **3–7 đơn hàng**
cho một giỏ. Khách nhận nhiều xác nhận và bị tính tiền nhiều lần.

Điểm chung của hai lỗi: **phép đọc-rồi-ghi bị cắt làm hai giao dịch.** Đó
không phải lỗi thiếu khóa. Thêm khóa vào một cấu trúc sai ranh giới chỉ
làm cửa sổ hẹp lại chứ không đóng được.

## Decision

**Mọi phép đọc-rồi-ghi phải nằm trọn trong MỘT giao dịch.** Cách cưỡng chế
tùy theo xung đột xảy ra thường xuyên tới mức nào:

### 1. Xung đột THƯỜNG XUYÊN → khóa bi quan trên dòng tổng hợp

Dùng cho giỏ hàng: một khách, một giỏ, nhiều tab — đụng nhau là chuyện
bình thường, và bắt khách thử lại vì "xung đột phiên bản" là vô nghĩa với
thao tác thêm hàng.

`Repository.MutateWithEvents` mở giao dịch, `SELECT ... FOR UPDATE` dòng
giỏ, đọc lại, chạy hàm sửa của bên gọi, ghi, rồi commit.

Ràng buộc kèm theo: **mọi lệnh gọi ra module khác phải xong TRƯỚC khi vào
giao dịch.** Bên trong đang giữ khóa dòng.

### 2. Xung đột HIẾM → khóa lạc quan bằng cột `version`

Đã dùng cho `inventory_item`, `fulfillment_order`, `reservation`. Xung đột
ở đó là bất thường, nên trả lỗi và để bên gọi quyết định là đúng.

**Khóa lạc quan phải đặt trên MỌI thực thể mà bất biến chạm tới, không chỉ
thực thể bị ghi nhiều nhất.** Đây là bài học của PH-31, và nó đắt:

`inventory_item` có cột `version` từ đầu; `reservation` thì không. Ai cũng
tưởng thế là đủ, vì tồn kho mới là chỗ con số thay đổi. Nhưng bất biến
"một reservation chỉ được nhả đúng một lần" nói về RESERVATION, và không
gì cưỡng chế nó ở tầng dữ liệu.

Hậu quả nhìn thấy được là hàng sinh ra từ không khí — chi tiết ở mục
*Phụ lục: PH-31*.

### 3. Bất biến VƯỢT tổng hợp → ràng buộc UNIQUE

"Một phiên thanh toán sinh tối đa một đơn" không phải bất biến của riêng
đơn hàng hay riêng phiên — nó nối hai tổng hợp ở hai module. Không lớp
kiểm tra nào ở tầng ứng dụng giữ được nó, vì việc KIỂM và việc GHI luôn
nằm ở hai thời điểm khác nhau.

Chỉ trong database thì hai việc đó mới là một:

```sql
CREATE UNIQUE INDEX order_one_per_checkout
    ON "order" (source_checkout_id)
 WHERE source_checkout_id <> '';
```

Bên thua cuộc đua nhận lỗi UNIQUE, đọc lại đơn đã có, và trả về cho khách
như một lần phát lại. Khách thấy MỘT đơn.

Đây cũng chính là lý lẽ đã ghi trong
`internal/platform/httpserver/idempotency.go` về việc vì sao middleware
idempotency cố tình KHÔNG tự đệm response.

## Alternatives

**Mutex trong tiến trình.** Loại. Nó chỉ đúng khi có đúng một tiến trình
API, và giấu đi việc ranh giới giao dịch đang sai — lỗi sẽ quay lại nguyên
vẹn vào ngày chạy hai bản sao.

**Thử lại khi xung đột.** Loại cho ca giỏ hàng. Thử lại là cách HOÀN TẤT
một phép khóa lạc quan đã phát hiện được xung đột, không phải cách phát
hiện xung đột. Không có gì phát hiện thì thử lại chỉ là chạy lại cùng một
lỗi.

**`SERIALIZABLE` cho toàn hệ thống.** Loại. Nó giải quyết đúng vấn đề
nhưng bắt mọi đường ghi trả giá cho vài đường ghi có xung đột, và đẩy việc
xử lý lỗi tuần tự hóa lên mọi handler.

**Khóa dòng phiên giữ suốt `CompleteCheckout`.** Loại — và đây là phương
án nguy hiểm nhất vì trông có vẻ đúng. `CompleteCheckout` gọi
`SaveWithEvents`, hàm này mở giao dịch RIÊNG để cập nhật chính dòng phiên
mà giao dịch ngoài đang giữ `FOR UPDATE`. Tự khóa chính mình, chết cứng.

**Thêm trạng thái `COMPLETING` để giành quyền.** Cân nhắc nghiêm túc. Một
câu `UPDATE ... WHERE status = 'ACTIVE'` là phép so-sánh-rồi-đặt nguyên tử
và giải quyết đúng vấn đề. Loại vì nó thêm một trạng thái vào máy trạng
thái, và mọi đường thất bại phải nhớ trả trạng thái về `ACTIVE` — quên
một đường là phiên kẹt vĩnh viễn. Ràng buộc UNIQUE không có đường thất bại
nào để quên.

## Consequences

**Tốt**

- Bất biến "một phiên một đơn" thành cấu trúc: không code nào phá được nó,
  kể cả code viết sau này bởi người không đọc ADR này.
- Thao tác giỏ hàng xếp hàng theo giỏ, không theo toàn hệ thống. Hai khách
  khác nhau không chờ nhau.
- `Sync` không còn ghi trên đường sửa. Giá và tình trạng hàng trong bảng
  chỉ còn là bộ đệm hiển thị, đúng như PH-29 đã định nghĩa cho đường đọc.

**Xấu**

- `order` giữ `source_checkout_id`, tức module order biết tới sự tồn tại
  của phiên thanh toán. Không có khóa ngoại, cùng cách `reservation.checkout_id`
  đã làm từ migration 000004 — nhưng vẫn là một sợi dây nối hai module.
- `MutateWithEvents` khó dùng đúng hơn `FindByID` + `Save`: bên gọi phải
  nhớ đưa mọi lệnh gọi ra ngoài lên TRƯỚC. Ràng buộc này chỉ nằm trong tài
  liệu, chưa có gì kiểm tự động.
- Giỏ có tranh chấp cao sẽ có request chờ khóa. Chấp nhận được: cửa sổ giữ
  khóa chỉ gồm vài câu lệnh trên chính database này.

## Trade-offs

Chấp nhận một sợi dây nối order → checkout để đổi lấy một bất biến mà
database cưỡng chế. Bất biến này là chuyện TIỀN của khách; sự sạch sẽ của
biên module không đáng giá bằng.

Chấp nhận hai cơ chế khóa khác nhau trong cùng một hệ thống. Chúng khác
nhau vì tần suất xung đột khác nhau, và mỗi ADR/mỗi comment đều nói rõ
chỗ nào dùng cái nào.

## Kiểm chứng

`internal/app/api_idempotency_test.go` — ba bài, chạy song song thật trên
PostgreSQL thật:

| Bài | Đo gì |
|-----|-------|
| `TestThemGioSongSongKhongMatCapNhat` | Mỗi lượt thêm thành công cộng đúng 1 món |
| `TestCungKhoaIdempotencyChayCungLucChiTaoMotDonHang` | Cùng khóa → một đơn, một bút toán |
| `TestMotPhienThanhToanChiSinhMotDonHang` | Một phiên → một đơn, dù khóa khác nhau |

Cả ba đã được kiểm chứng bằng cách PHÁ code thật rồi xác nhận chúng đỏ:

- Bỏ `FOR UPDATE` → giỏ mất 6–8 trong 12 lượt
- Trả `Sync` về đường sửa → giỏ mất 4–9 trong 12 lượt
- Không ghi `source_checkout_id` → một phiên sinh 5–7 đơn


---

## Phụ lục: PH-31 — vì sao khóa lạc quan trên một bảng là không đủ

Ghi lại vì cơ chế này phản trực giác, và ba lần thử tái hiện đầu tiên đều
thất bại vì đoán sai nó.

### Hiện trường

Job dọn giữ hàng quá hạn thất bại MỌI lượt suốt nhiều giờ với
`reserved có 0, cần 1`. Nhật ký biến động tồn kho:

```text
18:04:22.826  RESERVE  1  → còn 76   ref=rsv_…GGG
18:19:28.497  RELEASE  1  → còn 77   ref=rsv_…GGG
18:19:28.499  RELEASE  1  → còn 78   ref=rsv_…GGG
```

78 cao hơn 77 — số trước khi giữ. Hàng sinh ra từ không khí.

### Chi tiết quyết định

Con số **77 rồi 78**. Lượt nhả thứ hai ĐỌC ĐƯỢC kết quả của lượt thứ nhất.

Nếu đây là cuộc đua kinh điển — hai bên cùng đọc một giá trị cũ rồi cùng
ghi đè — cả hai đã cùng ghi 77, và khóa lạc quan trên `inventory_item` đã
chặn bên thứ hai. Việc bên thứ hai thấy 77 nghĩa là nó đọc bảng tồn kho
SAU khi bên thứ nhất commit. Phiên bản nó đọc là phiên bản mới nhất, nên
câu `UPDATE` của nó hoàn toàn hợp lệ.

Ba lần thử tái hiện đầu đều bắn nhiều lượt nhả song song và trông đợi cuộc
đua kinh điển. Chúng không bao giờ trúng.

### Cơ chế

PostgreSQL chạy `READ COMMITTED`: **mỗi câu lệnh** trong một giao dịch lấy
một ảnh chụp mới. Hai câu `SELECT` trong cùng một giao dịch có thể thấy
hai thời điểm khác nhau của thế giới.

```text
T2: BEGIN
T2: SELECT reservation  → ACTIVE                  (T1 chưa commit)
T1: COMMIT                                        (item 76→77, reservation kết thúc)
T2: kiểm IsFinal() trên bản TRONG BỘ NHỚ          → vẫn ACTIVE → cho qua
T2: SELECT item         → 77, phiên bản mới nhất  (thấy việc của T1)
T2: UPDATE item WHERE version = <mới nhất>        → HỢP LỆ → 78
T2: UPDATE reservation  (upsert KHÔNG điều kiện)  → ghi đè
T2: COMMIT
```

Khe hở rộng đúng bằng khoảng giữa hai câu `SELECT` của T2 — vài chục
micro-giây. Nó trúng một lần trong nhiều giờ chạy thật.

Điều kiện phụ khiến nó khó gặp hơn nữa: phải có reservation KHÁC còn sống
trên cùng bản ghi tồn kho. Không có nó, `reserved` về 0 sau lượt nhả đầu
và bất biến của chính bản ghi tồn kho chặn lượt sau bằng "không đủ hàng".
Trên SKU đang bán thì điều kiện ấy gần như luôn đúng.

### Cách dựng lại một cách xác định

`releaseWith` gọi `s.clock.Now()` ĐÚNG giữa hai lần đọc, và `Clock` vốn đã
là cổng tiêm sẵn có của module. Một đồng hồ chặn ở lần gọi đầu cho ta điều
khiển được đúng khe ấy mà không sửa một dòng code production nào.

Xem `internal/modules/inventory/nha_hai_lan_test.go`.

### Kết quả

`migrations/000028` thêm `reservation.version`. Bên thứ hai giữ số phiên
bản CŨ, nên câu `UPDATE` của nó khớp 0 dòng, cả giao dịch quay lui, và lần
thử lại đọc lại thấy trạng thái đã kết thúc.

Kiểm chứng bằng cách phá — gỡ ràng buộc `WHERE reservation.version = $10`:

| Bài | Khi phá |
|-----|---------|
| `TestNhaGiuHangHaiLan_TaiHienXacDinh` | 2 biến động RELEASE · khả dụng 6 thay vì 5 · T2 báo thành công |
| `TestChotVaHetHanChongNhau_TaiHienXacDinh` | 1 biến động COMMIT trên reservation ĐÃ NHẢ — hàng vừa trả về kho lại bị bán |

Bài thứ hai tồn tại vì `commitWith` có cùng hình dạng với `releaseWith`.
Sửa một chỗ mà không kiểm chỗ có cùng hình dạng thì mới chỉ sửa triệu chứng.

Việc quét tiếp tìm ra `CompleteCheckout` cũng mang hình dạng đó — ghi ở
`docs/10-roadmap/backlog.md` mục **PH-32**, chưa sửa.
