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
