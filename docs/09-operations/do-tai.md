# Đo tải

**Cập nhật:** 04/09/2026

Tài liệu này chỉ ghi những phép đo ĐÃ CHẠY THẬT, kèm cách chạy lại. Con số
ước lượng không thuộc về đây.

---

## 1. Khóa bi quan trên giỏ hàng

### Vì sao đo

[ADR-0013](../adr/0013-write-transaction-boundary.md) chọn khóa **bi quan**
(`SELECT … FOR UPDATE`) cho giỏ hàng, với lý lẽ "cửa sổ giữ khóa chỉ gồm
vài câu lệnh trên chính database này".

Đó là một khẳng định về hiệu năng, và nó chưa từng được đo. Khóa bi quan
là thứ DUY NHẤT trong đợt gia cố có thể làm hệ thống chậm đi.

### Cách đo

Hai kịch bản khác nhau **đúng một biến**:

```text
riêng   N khách, mỗi người MỘT giỏ  → không ai tranh với ai
chung   N khách, cùng MỘT giỏ       → mọi lượt tranh cùng một dòng
```

Chênh lệch giữa hai con số chính là giá của khóa. So với một hệ thống
không có khóa là so nhầm: khi đó dữ liệu sai, và tốc độ của một hệ thống
cho kết quả sai thì không có nghĩa gì.

Thao tác đo là **sửa số lượng** (`PATCH /api/v1/cart/items/{id}`), không
phải thêm món. Thêm món có trần 10 mỗi offer, nên kịch bản chung hỏng
191/200 lượt vì lỗi nghiệp vụ và phép đo thành đo tốc độ trả lỗi — đã mắc
đúng lỗi đó ở lần chạy đầu.

### Kết quả

API và PostgreSQL chạy cùng máy (Apple Silicon, môi trường phát triển).
Mỗi khách 10 lượt.

| Khách song song | riêng p50 | chung p50 | riêng p95 | chung p95 | riêng ops/s | chung ops/s |
|---|---|---|---|---|---|---|
| 20  | 4,5 ms  | 5,2 ms  | 7,5 ms  | 10,7 ms | 4118 | 3147 |
| 50  | 10,0 ms | 15,5 ms | 12,5 ms | 23,3 ms | 4805 | 2995 |
| 100 | 19,4 ms | 31,8 ms | 21,0 ms | 38,1 ms | 5102 | 3068 |

**0 lỗi, 0 timeout ở mọi mức.**

### Đọc gì từ đây

**Thông lượng trên một dòng chạm trần ~3000 lượt/giây rồi GIỮ NGUYÊN.** Nó
không sụp khi tăng từ 20 lên 100 khách — độ trễ tăng tuyến tính, đúng hành
vi của một hàng đợi lành mạnh. Sụp đổ sẽ trông khác hẳn: thông lượng giảm
dần và độ trễ tăng vọt.

3000 lượt/giây trên một dòng nghĩa là phần bị tuần tự hóa mất khoảng
**330 micro giây** mỗi lượt. Khẳng định "vài câu lệnh" trong ADR-0013 đứng
vững.

Con số ấy còn xa mọi nhu cầu thật: **một giỏ hàng là của MỘT người**. Tranh
chấp trên cùng một giỏ chỉ xảy ra khi một người mở nhiều tab, tức là vài
lượt mỗi giây, không phải ba nghìn.

Giá phải trả: thông lượng giảm ~40% và độ trễ tăng ~1,6 lần so với không
tranh chấp. Đổi lấy việc không mất dữ liệu — xem
[ADR-0013](../adr/0013-write-transaction-boundary.md) để biết cái mất
trông như thế nào.

### Chạy lại

```bash
# Cần API đang chạy ở localhost:8080 với dữ liệu mẫu.
SONG_SONG=100 MOI_NGUOI=10 cd gouse && go run ./cmd/dotai
```

---

## 2. Toàn bộ luồng đặt hàng (PH-15)

### Vì sao đo

Đây là đường tiền. Mọi thứ khác hỏng thì khó chịu; đường này hỏng thì mất
đơn hoặc mất hàng.

Mục này nằm trong bảng "CHƯA đo" của bản trước, với lý do "cần tồn kho đủ
để không hết hàng giữa chừng". Lần này nạp tồn qua **API kiểm kê thật**
(`POST /api/v1/admin/inventory/adjustments`) chứ không sửa thẳng database
— vừa đủ điều kiện đo, vừa là lần dùng thật đầu tiên của endpoint đó.

### Cách đo

Năm bước, đo RIÊNG từng bước:

```text
1 thêm giỏ    2 mở phiên (GIỮ HÀNG)    3 địa chỉ    4 giao hàng    5 hoàn tất
```

Đo riêng từng bước là toàn bộ giá trị của phép đo: tổng thời gian đặt một
đơn không nói được nên sửa ở đâu, mà năm bước này đi qua bốn module và
khác hẳn nhau về bản chất.

Mỗi luồng bám một offer riêng để trải tải ra nhiều SKU. Kết quả nghiệp vụ
(hết hàng) đếm RIÊNG khỏi lỗi hệ thống.

### Kết quả — và một lỗi tìm được nhờ đo

API và PostgreSQL cùng máy (Apple Silicon, môi trường phát triển). Mỗi
khách 5 đơn.

| Song song | p50/đơn | p95/đơn | đơn/giây | thành công | bị từ chối |
|---|---|---|---|---|---|
| 20  | 24,7 ms  | 58,9 ms  | 635 | 99/100  | 1  |
| 50  | 60,5 ms  | 80,5 ms  | 741 | 239/250 | 11 |
| 100 | 114,8 ms | 136,0 ms | 780 | 457/500 | 43 |

**0 lỗi hệ thống ở mọi mức.** Nhưng tỉ lệ bị từ chối tăng theo tải, và mọi
lượt từ chối đều ở bước 2 — bước giữ hàng.

Kiểm tồn kho ngay sau đó: **còn hơn 49.000 đơn vị khả dụng**. Không hề hết
hàng.

Bước chậm nhất luôn là **2 (mở phiên)** rồi tới **5 (hoàn tất)** — đúng
như trông đợi, vì đó là hai bước ghi. Bước 3 và 4 chỉ ~6ms ngay ở mức 100.

### Nguyên nhân: hai lỗi khác nhau bị gộp làm một

`reserveAll` gói MỌI lỗi giữ hàng — kể cả xung đột phiên bản sau khi hết
lượt thử lại — thành `ErrOutOfStock`, với lý lẽ "với khách thì cả hai đều
là không mua được món này".

Với khách thì đúng. Với **nền tảng** thì sai hẳn:

```text
hết hàng    sự thật nghiệp vụ; thử lại vô ích
tranh chấp  vấn đề năng lực;   thử lại ĐƯỢC, hàng vẫn còn nguyên
```

Hệ quả: chỉ số ghi `out_of_stock` cho một tình huống kho đầy. Nền tảng
không có cách nào biết nó đang từ chối 9% lượt mua vì lý do kỹ thuật.

Module inventory ĐÃ tách sẵn hai lỗi này (`ErrInsufficientStock` vs
`ErrConflict`) và ghi rõ chúng "khác hoàn toàn"; phân biệt bị mất ở tầng
checkout. Đã sửa: giữ nguyên thứ khách nhìn thấy, nhưng nguyên nhân thật
sống sót tới chỉ số.

Sau khi sửa, cùng phép đo ở mức 100: **45/45 lượt từ chối mang nhãn
`version_conflict`**, không còn lượt nào ghi `out_of_stock`.

### Hiệu chỉnh số lần thử lại

`maxRetries = 3` chưa từng được đo. Đo xong thì thấy nó quá thấp cho một
dòng tồn kho nóng:

| maxRetries | thành công | bị từ chối | đơn/giây | p50 | p99 |
|---|---|---|---|---|---|
| 3 | 455/500 | 45 | 750 | 114,9 ms | 163,8 ms |
| 8 | 500/500 | 0  | 761 | 117,9 ms | 211,9 ms |

Đổi 9% lượt bị từ chối lấy ~48ms ở p99. Những lượt phải thử lại **chờ lâu
hơn thay vì thất bại** — đổi đúng thứ cần đổi. Đã đặt thành 8.

Kiểm ở mức cao hơn để chắc không chỉ đẩy vấn đề đi: **200 luồng, 1000/1000
đơn thành công, 847 đơn/giây**, độ trễ tăng tuyến tính (p50 219ms). Thông
lượng vẫn ĐANG TĂNG ở mức đó, nên trần chưa chạm tới.

### Chạy lại

```bash
# Cần API chạy ở localhost:8080 và tồn kho đủ.
cd gouse && KICH_BAN=datdon SONG_SONG=100 MOI_NGUOI=5 go run ./cmd/dotai
```

---

## 3. Tranh chấp pool kết nối (PH-18)

Đo cùng lúc với mục 2, dùng chỉ số pool mới (PH-25).

```text
sau ~2500 đơn ở các mức 20/50/100/200 luồng:

gouse_db_pool_empty_acquire_total     76268
gouse_db_pool_canceled_acquire_total  0
gouse_db_pool_max_connections         10
```

**Đọc gì từ đây:** pool cạn LIÊN TỤC — hàng chục nghìn lượt phải chờ để
lấy kết nối. Nhưng `canceled` bằng **0**: không ai bỏ cuộc, không ai bị từ
chối. Pool đang hoạt động như một hàng đợi, đúng như thiết kế.

Đó cũng là lý do chỉ số này cần tồn tại: **không có nó thì hiện tượng này
vô hình**. Không lỗi, không 5xx, chỉ có độ trễ — và người trực sẽ đi tìm ở
truy vấn chậm.

Chưa đổi `MaxConns` (đang là 10): thông lượng vẫn đang tăng ở 200 luồng
nên chưa có bằng chứng nói con số đó đang cản. Nâng nó lúc này là chỉnh
theo cảm giác, đúng thứ mục này sinh ra để tránh.

---

## 4. CHƯA đo

Ghi ra để không ai tưởng phần này đã xong:

| Việc | Vì sao chưa |
|---|---|
| Đường đọc danh mục | Chưa có chỉ mục nào được chọn dựa trên số đo, nên đo bây giờ là đo một thứ sẽ đổi |
| Bộ phát event khi hàng đợi dồn | Cần dựng được tình huống dồn trước đã |
| Đo trên phần cứng giống production | Chưa có môi trường đó |
