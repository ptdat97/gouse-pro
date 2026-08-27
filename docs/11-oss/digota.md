# Digota

| | |
|---|---|
| Repository | `github.com/digota/digota` |
| License | MIT |
| Sao / Fork | 524 / 86 |
| Cập nhật cuối | **2021-02-14 — ngừng phát triển hơn 5 năm** |
| Vai trò | Tham chiếu **biểu diễn tiền tệ, khóa đồng thời, mô hình SKU** |

---

## 1. Cảnh báo về tình trạng bảo trì

Digota **không còn được phát triển**. Lần đẩy code cuối là tháng 2/2021, tổng cộng 89 commit.

```text
Hệ quả:
  ✗ KHÔNG dùng làm thư viện phụ thuộc
  ✗ KHÔNG sao chép code (đã lỗi thời, có thể có lỗ hổng chưa vá)
  ✓ CHỈ đọc để học hai ý tưởng cụ thể
```

Đây là ví dụ vì sao [dependency-registry.md](dependency-registry.md) bắt buộc ghi **ngày cập nhật cuối** cho mọi ứng viên phụ thuộc.

---

## Năng lực: Biểu diễn tiền bằng đơn vị nhỏ nhất

### Cách OSS làm

Tiền lưu bằng **số nguyên theo đơn vị nhỏ nhất**, không dùng số thực. Tài liệu Digota nêu ví dụ trực tiếp:

```text
4726 là $47.26
```

### Điểm mạnh

- Không có sai số dấu chấm động
- Lưu và so sánh trong database đơn giản
- Nhanh

### Điểm yếu

Cần biết "đơn vị nhỏ nhất" của từng tiền tệ (USD 2 chữ số, VND 0 chữ số, JPY 0 chữ số). Nếu không xử lý đúng, hiển thị sai.

### Yêu cầu của chúng ta

Độ lệch đối soát phải bằng **0**. Nền tảng giữ tiền hộ nhiều bên.

### Adopt

**Đã cài đặt.** Đây là xác nhận độc lập cho quyết định của chúng ta:

```go
// internal/kernel/money/money.go
type Money struct {
    amount   int64      // đơn vị nhỏ nhất
    currency Currency
}

func (c Currency) exponent() int {
    switch c {
    case VND: return 0    // đồng là đơn vị nhỏ nhất
    case USD: return 2    // cent
    }
}
```

Hai dự án độc lập (Digota, và chúng ta) chọn cùng cách tiếp cận; Flamingo chọn `big.Float`. Với ưu tiên **lưu trữ đơn giản + hiệu năng ổn định**, số nguyên là lựa chọn đúng.

### Quyết định cuối

```text
✓ Đã cài — xác nhận thiết kế đúng
```

---

## Năng lực: Khóa phân tán và xử lý đồng thời

### Cách OSS làm

Mẫu **lock → thao tác → release** để đảm bảo truy cập độc quyền giữa nhiều node. Request đồng thời chờ khóa; hết thời gian chờ thì nhận lỗi.

Hỗ trợ ba loại lock server: Zookeeper, Redis, in-memory.

### Điểm mạnh

- Bảo vệ được bất biến khi có nhiều tiến trình
- Cắm được nhiều backend khóa
- Mô hình dễ hiểu

### Điểm yếu

**Khóa bi quan tạo hàng đợi tuần tự.** Với sản phẩm bán chạy hoặc live commerce (hàng nghìn người mua cùng một SKU trong vài giây), mọi request xếp hàng chờ:

```text
Độ trễ tăng vọt
Kết nối database cạn kiệt
Trải nghiệm khách hàng tệ đúng lúc quan trọng nhất
```

Ngoài ra khóa phân tán thêm một thành phần hạ tầng phải vận hành, giám sát, và xử lý khi nó hỏng.

### Yêu cầu của chúng ta

Kịch bản khắc nghiệt nhất là **live commerce**: người dẫn nói "chỉ còn 50 chiếc, giá sốc trong 5 phút" → hàng nghìn request trên **một** SKU trong vài giây.

Đồng thời: tồn kho âm là một trong **ba chỉ số phải luôn bằng 0**.

### Adapt

**Lấy vấn đề, không lấy giải pháp.**

Digota đúng khi nhận diện tranh chấp tồn kho là vấn đề thật. Nhưng giải pháp của họ (khóa bi quan phân tán) sai với ràng buộc của chúng ta.

Chúng ta dùng **khóa lạc quan với điều kiện nguyên tử**:

```sql
UPDATE inventory_item
SET quantity_available = quantity_available - $qty,
    quantity_reserved  = quantity_reserved + $qty,
    version = version + 1
WHERE id = $id
  AND version = $expected_version
  AND quantity_available >= $qty;
```

Hai điều kiện trong `WHERE` là mấu chốt:

```text
version = $expected     phát hiện có người khác vừa sửa
quantity >= $qty        kiểm tra VÀ cập nhật NGUYÊN TỬ trong một câu lệnh
```

Điều kiện thứ hai một mình đã đủ chống bán quá số lượng — không cần khóa riêng.

### So sánh

| | Khóa bi quan (Digota) | Khóa lạc quan (chúng ta) |
|---|---|---|
| Tranh chấp thấp | Chậm hơn (chi phí khóa) | Nhanh |
| Tranh chấp cao | Hàng đợi tuần tự | Song song, chỉ xung đột thật mới thử lại |
| Hạ tầng thêm | Có (Zookeeper/Redis) | Không |
| Live commerce | Không chịu được | Chịu được |

### Reject

```text
✗ Khóa phân tán — không cần ở monolith một database
✗ Zookeeper/Redis làm lock server — thêm hạ tầng không có lý do
```

Nếu sau này tách `inventory` thành service riêng (nhóm 2 theo [ADR-0009](../adr/0009-service-extraction.md)), cân nhắc lại — nhưng database vẫn là điểm đồng bộ duy nhất nên khóa lạc quan vẫn đủ.

### Quyết định cuối

```text
✓ Khóa lạc quan + ràng buộc CHECK ở database
✓ Phân biệt lỗi xung đột (thử lại) và hết hàng (không thử lại)
✗ Không dùng khóa phân tán
```

---

## Năng lực: Mô hình Product / SKU

### Cách OSS làm

`Product` là **tập hợp** các món mua được; `SKU` là cấu hình cụ thể có thuộc tính, tiền tệ và giá.

Ví dụ trong tài liệu Digota: vé bóng đá là product; khu vực khán đài cụ thể là SKU.

### Điểm mạnh

Tách rõ "thứ khách nhìn thấy" khỏi "đơn vị bán được". Đây là phân biệt đúng và nhiều nền tảng làm sai.

### Điểm yếu

**Không có tầng Variant.** Với thời trang, đây là thiếu sót nghiêm trọng:

```text
Digota:    Product → SKU
Cần thiết: Product → Variant (màu) → SKU (màu + size)
```

Không có Variant thì:
- Không nhóm được SKU theo màu để đổi bộ ảnh
- Không trả lời được "màu trắng còn size nào"
- Khách phải chọn từ danh sách phẳng hàng chục SKU

**Giá gắn vào SKU.** Điều này ngăn mô hình marketplace — nhiều seller không thể cùng bán một SKU với giá khác nhau.

### Yêu cầu của chúng ta

```text
Product → Variant → SKU → Offer → Inventory
```

`Offer` tách giá và người bán khỏi SKU — đây là điều kiện để có marketplace.

### Adopt

Phân biệt Product (trình bày) vs SKU (đơn vị bán) — đúng và đã có.

### Reject

```text
✗ Bỏ tầng Variant — không dùng được cho thời trang
✗ Gắn giá vào SKU — chặn mô hình marketplace
```

### Quyết định cuối

Giữ nguyên mô hình 5 tầng trong [02-domain/entities.md](../02-domain/entities.md). Digota xác nhận phần Product/SKU nhưng thiếu hai tầng quan trọng nhất với chúng ta.

---

## 2. Tổng kết Digota

| Hạng mục | Quyết định |
|---|---|
| Tiền bằng số nguyên đơn vị nhỏ nhất | **ADOPT** — đã cài, xác nhận độc lập |
| Nhận diện vấn đề tranh chấp tồn kho | **ADOPT** vấn đề |
| Khóa bi quan phân tán | **REJECT** — dùng khóa lạc quan |
| Zookeeper/Redis làm lock server | **REJECT** — hạ tầng thừa |
| Phân biệt Product / SKU | **ADOPT** |
| Bỏ tầng Variant | **REJECT** |
| Giá gắn vào SKU | **REJECT** — chặn marketplace |
| Dùng làm thư viện phụ thuộc | **REJECT** — ngừng bảo trì từ 2021 |
| gRPC/protobuf làm API chính | **REJECT** — chúng ta chọn REST + OpenAPI |
| MongoDB | **REJECT** — cần giao dịch ACID cho tài chính |

**Nhận xét cuối:** Digota đóng góp **một** ý tưởng đáng giá (tiền bằng số nguyên) và **một** vấn đề đáng nhận diện (tranh chấp tồn kho). Giải pháp của họ cho vấn đề thứ hai không phù hợp với chúng ta. Dự án đã chết — chỉ đọc, không dùng.

---

## 3. Tài liệu liên quan

- [../02-domain/value-objects.md](../02-domain/value-objects.md)
- [../04-modules/inventory.md](../04-modules/inventory.md) mục 5
- [../05-data/consistency.md](../05-data/consistency.md) mục 6
