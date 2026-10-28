# ADR-0023 — `pricing` thuộc Kernel: khung giá cưỡng chế lúc GHI

**Trạng thái:** Đã chấp nhận · 01/10/2026

## Bối cảnh

[ADR-0021](0021-ranh-gioi-commerce-kernel.md) và
[ADR-0022](0022-microkernel-kernel-on-dinh.md) cố ý để `pricing` **CHƯA XẾP
ĐƯỢC**, vì xếp nó vào kernel hay extension phụ thuộc một câu chưa ai trả lời:
**ai cưỡng chế khung giá, và cưỡng chế ở đâu.**

### Một bảng có mục đích mà không ai đọc

```text
price_constraint            migration 000003, tháng 8
  min_price / max_price     "khung giá ràng buộc seller"
  reference_price
  suspicious_below_bp       ngưỡng cảnh báo lỗi nhập liệu

pricing.ValidateSellerPrice cài ĐỦ, có test ở hai tầng
marketplace                 KHÔNG import pricing
```

Chú thích của `ValidateSellerPrice` ở `pricing/public.go` nói đúng điều cần
làm, và nó đã nằm đó nhiều tháng:

> *"Module marketplace gọi TRƯỚC khi cho seller lưu offer — đây là hàng rào
> chống bán phá giá VÀ chống lỗi nhập liệu (gõ thiếu một số 0)."*

Nó **chưa bao giờ có bên gọi**. Nhà bán đặt giá ngoài khung vẫn qua, và không
gì báo.

Đây là dạng lỗi hay gặp nhất của dự án, lần này ở mức BẢNG: cùng họ với bốn
hằng số event không ai phát (P3-76) và cột `creator_commission_rate` không ai
ghi (P3-83) — khác ở chỗ `price_constraint` có lý do tồn tại thật, và thiết
kế đã chỉ rõ bên gọi.

### Ba đường, và cả ba đổi ranh giới Kernel

```text
marketplace nhập pricing      ghép nối mới, pricing thành KERNEL
pricing nghe event offer.*    kernel không đổi, nhưng phát hiện MUỘN
ràng buộc ở tầng database     chặn sớm nhất, nhưng khung giá là CẤU HÌNH
                              nghiệp vụ, không phải bất biến lược đồ
```

## Quyết định

**`marketplace` import `pricing` và chặn lúc GHI.** `pricing` thuộc **KERNEL
— đường mua hàng**.

```text
CreateOffer   bước 1 SKU còn kinh doanh
              bước 2 nhà bán đang hoạt động
              bước 3 HÀNG RÀO CHỐNG HÀNG GIẢ
              bước 4 KHUNG GIÁ          ← ADR này

UpdatePrice   kiểm khung giá TRƯỚC khi đổi
```

### Vì sao chặn lúc GHI, không phát hiện sau

Giá sai lên sàn nghĩa là **khách có thể đã mua ở giá đó**. Lúc ấy không có
đường sửa nào ngoài hủy đơn hoặc chịu lỗ.

PH-41 đã rút ra quy tắc ấy cho dữ liệu tiền, và nó là quy tắc chung của dự án
này:

> **Phát hiện xong thì SỬA được không?** Trả lời "không, chỉ đảo được" thì
> việc cần làm là **chặn**, không phải đếm.

Đường "pricing nghe event `offer.*`" giữ được kernel không đổi, nhưng nó còn
cần một đường xử lý vi phạm mà hôm nay chưa có — hạ offer? báo admin? — và
trong lúc chờ, giá sai đã ở trên sàn.

### Vì sao `pricing` là KERNEL, không phải extension

Tiêu chí của ADR-0021: *tháo module này ra thì khách có còn mua được hàng
không?* Sau quyết định này, tháo `pricing` ra làm `marketplace` không biên
dịch được — tức **không**, và nó là kernel theo đúng phép đo.

Phán đoán đi kèm: khung giá là thứ **mọi marketplace đều cần**. Một sàn không
ràng buộc giá nhà bán sẽ có hàng giả bán 1/10 giá thật nằm cạnh hàng thật, và
đó là rủi ro sống còn của marketplace thời trang (mục 5 của đặc tả) — cùng họ
rủi ro với hàng rào chống hàng giả đã là kernel.

### Thứ tự kiểm KHÔNG đổi: chống hàng giả trước

Khung giá là **bước 4**, sau hàng rào chống hàng giả. Hai lý do:

```text
bước 3 chặn nhiều hơn và rẻ hơn  — một lượt tra quyền bán
bước 3 là rủi ro SỐNG CÒN        — bước 4 là rủi ro tiền
```

Việc này có hệ quả thật cho bài test: SKU mẫu duy nhất có khung giá thuộc một
thương hiệu HẠN CHẾ, nên bước 3 chặn trước và bài test đỏ vì một lý do KHÁC.
Bài test phải tự dựng SKU thương hiệu MỞ kèm khung giá — xem
`internal/app/api_khung_gia_test.go`.

### Thất bại theo hướng ĐÓNG

Lời gọi `pricing` lỗi hạ tầng thì **chặn**, không cho qua: không chứng minh
được giá hợp lệ thì với dữ liệu tiền phải xử như "không hợp lệ". Cùng lựa
chọn với `notification.HasConsent` — không chứng minh được đồng ý thì không gửi.

### Cổng `PriceFramePort` nil = KHÔNG kiểm

Một môi trường chưa dựng `pricing` vẫn bán được hàng, cùng cách
`PolicyPort` nil rơi về mặc định của domain. Nối dây ở **cả hai** gốc —
`internal/app` và `cmd/worker` — dù worker không tạo offer: để nil ở một gốc
tạo ra khác biệt IM LẶNG giữa hai chỗ, đúng dạng lỗi `eventcheck` R4 vừa được
dựng để gác ở tầng bên nhận event (P3-86).

## Những gì KHÔNG làm

### Không ràng buộc ở tầng database

Khung giá là **cấu hình nghiệp vụ** sửa được lúc chạy, không phải bất biến
lược đồ. Một `CHECK` trên `offer.price` tham chiếu `price_constraint` sẽ cần
trigger, và nó làm mọi lần đổi khung giá thành một migration.

Ràng buộc database vẫn giữ phần nó làm đúng: `price_constraint_not_empty` và
`price_constraint_min_below_max` chặn một khung giá VÔ NGHĨA được ghi vào.

### Không xử lý `NeedsReview`

`pricing` trả `NeedsReview` cho giá lệch xa thị trường mà **vẫn hợp lệ** —
thường là lỗi nhập liệu, nhưng cũng có thể là hàng thanh lý thật, nên chặn
thẳng sẽ cản việc bán hàng hợp lệ.

Hôm nay cờ ấy **không đi đâu**: chưa có hàng đợi rà soát và chưa có màn hình
admin. Đó là một khoảng trống ĐÃ BIẾT, ghi ở P3-88 — không phải một chỗ bị
quên.

Dựng hàng đợi rà soát bây giờ là dựng thứ chưa có người dùng, đúng lỗi ADR
này đang sửa.

### Không thêm route cho `CreateOffer` của nhà bán

Chưa có, và ADR này không thêm. Bài test gọi tầng application — đường
production thật hiện có — và nó vẫn đi qua đúng chỗ cần kiểm.

## Cưỡng chế

| Điều | Gác bởi |
|---|---|
| `pricing` được xếp KERNEL và có hình dạng kernel | archcheck R10 |
| Giá ngoài khung bị chặn lúc TẠO | `TestGiaNgoaiKhungBiChanLucGhi` |
| Giá ngoài khung bị chặn lúc SỬA | `TestDoiGiaCungPhaiQuaKhungGia` |
| Giá TRONG khung vẫn đi qua | khẳng định thứ ba của bài đầu |

Khẳng định cuối quan trọng ngang hai cái trên: một hàng rào chặn mọi thứ cũng
"xanh" ở hai bài đầu, và nó làm nhà bán không đăng được gì.

Kiểm chứng bằng cách phá:

```text
bỏ `Pricing` khỏi nối dây     → cả hai bài đỏ (tái hiện hiện trạng cũ)
bỏ kiểm ở `UpdatePrice`       → bài thứ hai đỏ, bài đầu vẫn xanh
```

## Đánh đổi đã chấp nhận

**`marketplace` có thêm một phụ thuộc.** Nó nay import `catalog`,
`inventory`, `pricing`, `product`, `seller` — năm module. Đó là module có
nhiều phụ thuộc nhất của hệ thống, và nó đúng với vai trò: `Offer` là
primitive nối giá, người bán, hàng và quyền bán lại với nhau (ADR-0007).

**Một lượt tra thêm trên đường tạo/sửa offer.** Nằm ngoài đường thanh toán
của khách, nên nó không ảnh hưởng độ trễ khách thấy.

**`pricing` thành kernel nghĩa là nó khó đổi hơn.** Trước đây không ai import
nên sửa gì cũng được; nay đổi mặt công khai của nó là đổi một phụ thuộc của
kernel. Đó là cái giá của việc nó thật sự được dùng.

## Liên quan

- [ADR-0021](0021-ranh-gioi-commerce-kernel.md) — ranh giới kernel đo được; để `pricing` chưa xếp
- [ADR-0022](0022-microkernel-kernel-on-dinh.md) — microkernel; kernel ổn định
- [ADR-0007](0007-marketplace-order-model.md) — `Offer` là primitive
- [04-modules/pricing.md](../04-modules/pricing.md) — đặc tả module
- P3-79 (quyết định này) · P3-88 (`NeedsReview` chưa đi đâu)
