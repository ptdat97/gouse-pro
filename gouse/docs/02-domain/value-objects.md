# Value Objects

## 1. Value Object là gì

Value object là đối tượng **không có định danh riêng**, được so sánh bằng **giá trị**, và **bất biến** sau khi tạo.

```text
Entity:        "Khách hàng #123" — vẫn là khách đó dù đổi tên
Value Object:  "300.000đ"        — đổi số tiền là một giá trị khác hẳn
```

### Vì sao dùng value object thay vì kiểu nguyên thủy

```go
// Cách tệ
func ApplyDiscount(price float64, discount float64) float64

// Cách đúng
func ApplyDiscount(price Money, discount Percentage) Money
```

Cách đầu cho phép mọi lỗi sau đây biên dịch thành công:
- Cộng tiền VND với tiền USD
- Truyền nhầm thứ tự tham số
- Sai số làm tròn với số thực dấu chấm động
- Giảm giá 150%

Cách thứ hai làm những lỗi đó **không biên dịch được** hoặc bị chặn ngay khi tạo.

---

## 2. Money — value object quan trọng nhất

### 2.1 Định nghĩa

```go
type Money struct {
    amount   int64   // đơn vị nhỏ nhất (đồng với VND, cent với USD)
    currency Currency
}
```

### 2.2 Quy tắc bắt buộc tuyệt đối

> **KHÔNG BAO GIỜ dùng `float64` cho tiền.**

Lý do:

```text
0.1 + 0.2 = 0.30000000000000004

Với 1.000.000 giao dịch, sai số tích lũy thành số tiền thật.
Đối soát sẽ lệch. Và nguyên tắc P8 yêu cầu độ lệch đối soát phải bằng 0.
```

Dùng số nguyên với đơn vị nhỏ nhất:

```text
VND:  300000  = 300.000đ   (VND không có đơn vị nhỏ hơn)
USD:  29900   = $299.00    (lưu bằng cent)
```

### 2.3 Các phép toán

```go
func (m Money) Add(other Money) (Money, error)       // lỗi nếu khác tiền tệ
func (m Money) Subtract(other Money) (Money, error)
func (m Money) MultiplyByQuantity(qty int) Money
func (m Money) ApplyPercentage(p Percentage) Money   // có quy tắc làm tròn rõ ràng
func (m Money) Allocate(ratios []int) []Money        // chia không mất đồng nào
func (m Money) IsZero() bool
func (m Money) IsNegative() bool
```

### 2.4 Bài toán chia tiền — vì sao cần `Allocate`

Tình huống thực tế: chia 100.000đ theo tỷ lệ 1:1:1.

```text
Cách sai:
    100.000 / 3 = 33.333,33 → làm tròn 33.333 mỗi phần
    Tổng: 99.999đ
    → Mất 1đ. Sổ sách không cân bằng.

Cách đúng (Allocate):
    33.334 + 33.333 + 33.333 = 100.000đ
    → Phần dư được phân bổ theo quy tắc xác định
```

Đây không phải chi tiết nhỏ. Với hàng triệu giao dịch chia tiền cho seller và creator, việc mất từng đồng sẽ làm sổ cái không cân và vi phạm bất biến `Σ DEBIT = Σ CREDIT`.

### 2.5 Quy tắc làm tròn

Phải được định nghĩa **một lần**, dùng nhất quán toàn hệ thống:

```text
Tính hoa hồng:      làm tròn xuống (có lợi cho bên trả)
Tính thuế:          theo quy định pháp luật
Chia tiền:          dùng Allocate, phần dư về phần đầu tiên
Hiển thị:           làm tròn tới đơn vị nhỏ nhất của tiền tệ
```

Ghi rõ trong code, không để mỗi nơi tự quyết định.

---

## 3. Value Object đặc thù thời trang

### 3.1 Size

```go
type Size struct {
    system SizeSystem  // ALPHA | NUMERIC | EU | US | UK | JP | FREE
    value  string      // "M", "38", "28"
    label  string      // hiển thị cho khách
}
```

**Vì sao phức tạp hơn một chuỗi ký tự:**

```text
"M" trong hệ ALPHA của brand A  ≠  "M" của brand B
"38" giày EU  ≠  "38" quần
Cùng một chiếc áo: M (VN) = S (US) = 38 (EU)
```

Đây là nguồn gốc lớn nhất của việc hoàn hàng trong thời trang. Mô hình phải hỗ trợ:

```text
SizeChart {
    id
    brand_id
    product_type
    system
    entries[]  →  { size: "M", chest_cm: 96-100, length_cm: 70, ... }
}
```

Có bảng số đo thực tế giúp khách chọn đúng — giảm trực tiếp tỷ lệ hoàn hàng.

### 3.2 Color

```go
type Color struct {
    name      string  // "Trắng ngà"
    hex_code  string  // "#F5F5DC" — để hiển thị ô màu
    color_family ColorFamily  // WHITE | BLACK | RED | BLUE | ...
}
```

**Vì sao cần `color_family`:** khách lọc theo "màu xanh" chứ không lọc theo "Xanh navy đậm mã #1B2A4A". Nhóm màu cho phép lọc hữu ích. Còn `hex_code` để hiển thị chính xác.

### 3.3 MaterialComposition

```go
type MaterialComposition struct {
    components []MaterialComponent  // { material: "Cotton", percentage: 80 }
}
```

**Bất biến:** tổng phần trăm = 100.

Thông tin này ảnh hưởng quyết định mua (dị ứng, cách bảo quản, cảm giác mặc) và là yêu cầu công bố bắt buộc ở nhiều thị trường.

---

## 4. Value Object chung

### 4.1 Address

```go
type Address struct {
    recipient_name string
    phone          PhoneNumber
    street_address string
    ward           string
    district       string
    province       string
    postal_code    string
    country_code   string
    address_type   HOME | OFFICE
    delivery_note  string
}
```

**Quan trọng:** khi tạo đơn hàng, địa chỉ được **sao chép** vào đơn, không tham chiếu tới sổ địa chỉ.

Lý do: khách sửa địa chỉ trong sổ sau khi đặt hàng → đơn cũ sẽ hiển thị địa chỉ mới, và không biết hàng đã giao đi đâu. Đây là ứng dụng nguyên tắc P9.

### 4.2 Percentage

```go
type Percentage struct {
    basis_points int  // 1000 = 10.00%
}
```

Dùng basis point (phần vạn) thay vì số thực để tránh sai số. `10%` = `1000` basis points.

Ràng buộc: với tỷ lệ hoa hồng, giá trị phải trong [0, 10000].

### 4.3 Quantity

```go
type Quantity struct {
    value int
}
```

Ràng buộc: `value >= 0`. Kiểu riêng ngăn việc vô tình gán số lượng âm.

### 4.4 DateRange

```go
type DateRange struct {
    start time.Time
    end   time.Time
}
```

Bất biến: `end` sau `start`. Dùng cho chiến dịch, khuyến mãi, mùa vụ, chu kỳ đối soát.

### 4.5 EmailAddress / PhoneNumber

Kiểm tra định dạng ngay khi tạo. Chuẩn hóa số điện thoại về định dạng quốc tế — điều này quan trọng vì số điện thoại dùng để định danh khách vãng lai.

```text
"0901234567"     → "+84901234567"
"84 901 234 567" → "+84901234567"
```

Không chuẩn hóa sẽ dẫn tới việc cùng một người bị coi là hai khách khác nhau.

---

## 5. Value Object trạng thái tồn kho

```go
type InventoryQuantities struct {
    available  int
    reserved   int
    committed  int
    in_transit int
    damaged    int
    returned   int
}

func (q InventoryQuantities) Total() int
func (q InventoryQuantities) IsValid() bool   // mọi thành phần >= 0
```

Gói tất cả trạng thái vào một value object giúp **kiểm tra bất biến tại một chỗ**, thay vì rải rác mỗi khi sửa một trường.

---

## 6. Nguyên tắc thiết kế value object

| Nguyên tắc | Lý do |
|---|---|
| Bất biến — không có setter | Chia sẻ an toàn, không có tác dụng phụ bất ngờ |
| Kiểm tra hợp lệ trong constructor | Không tồn tại giá trị không hợp lệ |
| So sánh bằng giá trị | Hai `Money{300000, VND}` là bằng nhau |
| Không có định danh | Không lưu thành bảng riêng (thường nhúng) |
| Phép toán trả về giá trị mới | `a.Add(b)` không sửa `a` |
| Đặt tên theo ngôn ngữ nghiệp vụ | `Money` chứ không phải `Amount` |

---

## 7. Cách lưu trữ

Value object thường được **nhúng** vào bảng của entity chứa nó:

```sql
-- Money nhúng thành hai cột
CREATE TABLE offer (
    id            UUID PRIMARY KEY,
    sku_id        UUID NOT NULL,
    price_amount  BIGINT NOT NULL,      -- Money.amount
    price_currency CHAR(3) NOT NULL,    -- Money.currency
    ...
);

-- Address nhúng nhiều cột, có tiền tố
CREATE TABLE "order" (
    id                        UUID PRIMARY KEY,
    shipping_recipient_name   TEXT NOT NULL,
    shipping_phone            TEXT NOT NULL,
    shipping_street_address   TEXT NOT NULL,
    shipping_ward             TEXT,
    ...
);
```

**Không** tạo bảng riêng cho value object trừ khi có lý do rõ ràng (ví dụ `SizeChart` được dùng lại nhiều nơi nên là entity, không phải value object).

---

## 8. Tài liệu liên quan

- [entities.md](entities.md) — entity dùng các value object này
- [../05-data/data-model.md](../05-data/data-model.md) — chi tiết lưu trữ
- [../05-data/consistency.md](../05-data/consistency.md) — bất biến và nhất quán
