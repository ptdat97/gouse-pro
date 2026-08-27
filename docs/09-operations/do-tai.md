# Đo tải

**Cập nhật:** 27/08/2026

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

## 2. CHƯA đo

Ghi ra để không ai tưởng phần này đã xong:

| Việc | Vì sao chưa |
|---|---|
| Toàn bộ luồng đặt hàng dưới tải | Cần dữ liệu mẫu lớn hơn và tồn kho đủ để không hết hàng giữa chừng |
| Đường đọc danh mục | Chưa có chỉ mục nào được chọn dựa trên số đo, nên đo bây giờ là đo một thứ sẽ đổi |
| Bộ phát event khi hàng đợi dồn | Cần dựng được tình huống dồn trước đã |
| Đo trên phần cứng giống production | Chưa có môi trường đó |
