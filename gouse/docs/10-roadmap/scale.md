# Chiến lược mở rộng quy mô

## 1. Nguyên tắc

> Mở rộng quy mô theo **vấn đề đo được**, không theo phỏng đoán hay lịch trình.

Mỗi bước mở rộng phải trả lời: *vấn đề cụ thể là gì, số liệu nào chứng minh, đã thử cách rẻ hơn chưa?*

---

## 2. Thứ tự ưu tiên khi gặp vấn đề hiệu năng

```text
1. Đo lường          → xác định điểm nghẽn thật, không đoán
2. Tối ưu truy vấn   → chỉ mục, viết lại truy vấn, giảm N+1
3. Cache             → cho dữ liệu đọc nhiều, đổi ít
4. Mở rộng theo chiều dọc  → máy mạnh hơn (rẻ và đơn giản nhất)
5. Mở rộng theo chiều ngang → nhiều bản API
6. Bản sao chỉ đọc   → tách tải đọc khỏi tải ghi
7. Tách hạ tầng chuyên biệt → chỉ mục tìm kiếm, lưu trữ analytics
8. Tách service      → cuối cùng, theo ADR-0009
```

**Điểm quan trọng:** bước 8 là **cuối cùng**, không phải đầu tiên. Bảy bước trước rẻ hơn và ít rủi ro hơn nhiều.

---

## 3. Điểm nghẽn dự kiến theo thứ tự xuất hiện

### 3.1 Truy vấn danh mục sản phẩm

```text
Triệu chứng: trang danh mục chậm khi có nhiều SKU và nhiều bộ lọc

Xử lý theo thứ tự:
    1. Chỉ mục có điều kiện cho offer ACTIVE
    2. Cache kết quả danh mục phổ biến
    3. Chỉ mục tìm kiếm riêng (Phase 2)
```

### 3.2 Tranh chấp tồn kho

```text
Triệu chứng: tỷ lệ xung đột khóa lạc quan tăng, đặc biệt khi
             có sản phẩm hot hoặc live commerce

Xử lý:
    1. Đã có khóa lạc quan từ MVP — kiểm tra tỷ lệ thử lại
    2. Hàng đợi ở tầng ứng dụng cho SKU điểm nóng
    3. Giới hạn tốc độ theo khách (chống bot)
    4. Chia nhỏ tồn kho thành nhiều "ô" (chỉ khi thật sự cần)
```

**Lưu ý:** cách 4 phức tạp và dễ sai. Chỉ làm khi đo được rằng cách 1–3 không đủ.

### 3.3 Tải ghi analytics

```text
Triệu chứng: ghi event_log và click ảnh hưởng database giao dịch

Xử lý:
    1. Ghi bất đồng bộ qua hàng đợi (đã có từ đầu)
    2. Phân vùng bảng theo ngày
    3. Chính sách lưu trữ: chi tiết 90 ngày, sau đó tổng hợp
    4. Tách sang lưu trữ chuỗi thời gian riêng (Phase 3)
    5. Tách thành service riêng (theo ADR-0009)
```

### 3.4 Truy vấn báo cáo

```text
Triệu chứng: dashboard admin và báo cáo seller làm chậm giao dịch

Xử lý:
    1. Bảng tổng hợp tính sẵn, cập nhật định kỳ
    2. Bản sao chỉ đọc cho truy vấn báo cáo
    3. Kho dữ liệu riêng (Phase 4)
```

**Cảnh báo:** không đọc dữ liệu tài chính hoặc tồn kho từ bản sao khi cần ra quyết định — độ trễ sao chép dẫn tới quyết định sai.

### 3.5 Bảng ledger lớn

```text
Triệu chứng: tính số dư chậm do phải tổng hợp nhiều bút toán

Xử lý:
    1. balance_snapshot (đã thiết kế từ đầu)
    2. Snapshot định kỳ, kiểm tra khớp hàng ngày
    3. Phân vùng ledger theo thời gian

KHÔNG BAO GIỜ: lưu số dư như trường được cập nhật
```

---

## 4. Mở rộng theo chiều ngang

### API — dễ mở rộng

```text
cmd/api không giữ trạng thái
→ thêm bản chạy tùy ý
→ giới hạn thực tế là database, không phải API
```

### Worker — cần cẩn thận

```text
Outbox publisher: phải đảm bảo không xử lý trùng
    → SELECT ... FOR UPDATE SKIP LOCKED
    → hoặc chỉ chạy một bản

Job định kỳ: dùng khóa để tránh chạy trùng
    → nhiều bản worker, chỉ một bản thực thi mỗi job
```

### Database — giới hạn thật sự

```text
Ghi:  chỉ một bản chính → giới hạn cuối cùng
Đọc:  nhiều bản sao → mở rộng được

Khi tải ghi vượt khả năng một bản chính:
    → đây là lúc cân nhắc tách service nghiêm túc
    → hoặc phân mảnh dữ liệu (rất phức tạp, tránh nếu được)
```

---

## 5. Chiến lược cache

```text
Cache CÁI GÌ:
    ✓ Danh mục, thương hiệu, bộ sưu tập (đổi ít)
    ✓ Thông tin sản phẩm (đổi ít)
    ✓ Kết quả tìm kiếm phổ biến
    ✓ Bảng size

KHÔNG cache:
    ✗ Số lượng tồn kho (đổi liên tục, sai là bán quá số lượng)
    ✗ Số dư tài chính (phải chính xác)
    ✗ Giá trong checkout (đã đóng băng, không cần cache)
    ✗ Dữ liệu cá nhân (rủi ro rò rỉ giữa người dùng)
```

### Vô hiệu hóa cache

```text
Dùng domain event để vô hiệu hóa:
    product.published    → xóa cache sản phẩm
    offer.price_changed  → xóa cache giá
    collection.launched  → xóa cache bộ sưu tập
```

**Nguyên tắc:** thà cache miss còn hơn phục vụ dữ liệu sai. Với tồn kho và tiền, luôn đọc nguồn sự thật.

---

## 6. Xử lý sự kiện đột biến

Thương mại thời trang có các đợt đột biến dự đoán được:

```text
Ra mắt bộ sưu tập
Chiến dịch giảm giá lớn
Live commerce
Nội dung creator viral
```

### Chuẩn bị trước

```text
✓ Tăng số bản API trước sự kiện
✓ Làm nóng cache
✓ Tăng giới hạn kết nối database
✓ Tạm dừng job nền không khẩn cấp
✓ Chuẩn bị kịch bản suy giảm có kiểm soát
```

### Suy giảm có kiểm soát

```text
PHẢI hoạt động:
    ✓ Xem sản phẩm, thêm giỏ, đặt hàng, thanh toán, ghi sổ

CÓ THỂ suy giảm:
    ~ Gợi ý → hiển thị "bán chạy"
    ~ Tìm kiếm nâng cao → tìm kiếm cơ bản
    ~ Feed nội dung → nội dung mới nhất

CÓ THỂ tạm ngừng:
    ✗ Analytics, báo cáo, tổng hợp tín hiệu nhu cầu
```

Xem [../09-operations/disaster-recovery.md](../09-operations/disaster-recovery.md) mục 2.

---

## 7. Live commerce — kịch bản khắc nghiệt nhất

```text
Người dẫn: "chỉ còn 50 chiếc, giá sốc trong 5 phút"
→ hàng nghìn người bấm mua trong vài giây trên MỘT SKU
```

### Yêu cầu kiến trúc

```text
1. Khóa lạc quan (đã có từ MVP)
   → không dùng khóa bi quan, sẽ tạo hàng đợi tuần tự

2. Hàng đợi ở tầng ứng dụng cho SKU điểm nóng

3. Giới hạn tốc độ theo khách (chống bot)

4. Thà TỪ CHỐI RÕ RÀNG còn hơn bán rồi hủy
   → oversell làm mất niềm tin nhiều hơn hết hàng

5. Giá riêng trong phiên live có thời hạn rõ ràng,
   đóng băng vào đơn khi đặt
```

**Lý do mô hình tồn kho phải chịu được tranh chấp cao ngay từ MVP:** đổi cơ chế giữ tồn kho sau này rất rủi ro.

---

## 8. Chỉ số theo dõi để biết khi nào cần mở rộng

| Chỉ số | Ngưỡng hành động |
|---|---|
| API p95 | > 300ms → điều tra |
| API p99 | > 1s → hành động |
| Kết nối database | > 80% → mở rộng hoặc tối ưu |
| Tỷ lệ xung đột khóa lạc quan | > 5% → xem lại chiến lược tồn kho |
| Độ trễ outbox | > 60s → thêm worker |
| Truy vấn chậm (> 1s) | > 10/phút → tối ưu |
| Tỷ lệ trúng cache | < 95% → xem lại chiến lược cache |
| CPU database | > 70% liên tục → mở rộng |

---

## 9. Khi nào tách service

Theo [ADR-0009](../adr/0009-service-extraction.md). Tóm tắt điều kiện:

```text
1. Có số liệu chứng minh vấn đề cụ thể
2. Đã thử các cách rẻ hơn (bước 1–7 ở mục 2)
3. Module thật sự độc lập, interface ổn định
4. Có đội vận hành
5. Chấp nhận được chi phí (độ trễ mạng, nhất quán cuối, gỡ lỗi khó)
```

### Thứ tự ứng viên

```text
Nhóm 1 (dễ):    media processing · search · notification · analytics
Nhóm 2 (khó):   recommendation · content · inventory
Không tách:     order · cart · checkout · catalog · product · identity
```

---

## 10. Giới hạn của kiến trúc hiện tại

Trung thực về điều kiến trúc này **không** giải quyết được:

```text
✗ Ghi cực lớn trên một bảng
   → cần phân mảnh dữ liệu, rất phức tạp

✗ Đa vùng địa lý với độ trễ thấp toàn cầu
   → cần thiết kế lại về nhất quán dữ liệu

✗ Nhiều đội làm việc hoàn toàn độc lập
   → cần tách service thật sự
```

**Khi nào gặp:** ba giới hạn này chỉ xuất hiện ở quy mô rất lớn. Nếu chạm tới, đó là dấu hiệu kinh doanh thành công — và khi đó có đủ nguồn lực để thiết kế lại.

**Nguyên tắc:** không thiết kế trước cho quy mô chưa đạt tới. Chi phí độ phức tạp phải trả ngay, lợi ích có thể không bao giờ đến.

---

## 11. Tài liệu liên quan

- [../adr/0009-service-extraction.md](../adr/0009-service-extraction.md)
- [../03-architecture/evolution-to-services.md](../03-architecture/evolution-to-services.md)
- [../09-operations/deployment.md](../09-operations/deployment.md)
- [../04-modules/inventory.md](../04-modules/inventory.md) mục 5
