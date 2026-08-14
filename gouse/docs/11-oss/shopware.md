# Shopware 6

| | |
|---|---|
| Repository | `github.com/shopware/shopware` |
| License | MIT |
| Sao / Fork | 3.402 / 1.199 |
| Ngôn ngữ | PHP |
| Cập nhật cuối | 2026-08-12 (tích cực) |
| Vai trò | Tham chiếu **rule engine**, sales channel, kiến trúc mở rộng |

---

## Năng lực: Rule system — điều kiện nghiệp vụ tách khỏi code

### Cách OSS làm

Shopware có hệ thống quy tắc tổng quát: định nghĩa điều kiện (khách thuộc nhóm nào, giỏ hàng trên bao nhiêu, ngày nào, quốc gia nào) rồi gắn quy tắc đó vào khuyến mãi, phí vận chuyển, phương thức thanh toán.

Quy tắc được đánh giá tại thời điểm chạy dựa trên ngữ cảnh (`SalesChannelContext`).

### Điểm mạnh

**Người vận hành cấu hình được điều kiện mà không cần lập trình viên.**

Với thương mại thời trang, điều này có giá trị thật: đội merchandising muốn tạo khuyến mãi "giảm 20% cho bộ sưu tập Thu Đông, khách VIP, đơn trên 500.000đ, trong tháng 9" — không nên phải chờ một lần triển khai code.

### Điểm yếu

- Quy tắc phức tạp khó gỡ lỗi — không biết vì sao một khuyến mãi không áp dụng
- Đánh giá quy tắc tốn thời gian nếu số lượng lớn
- Dễ tạo quy tắc mâu thuẫn nhau

### Yêu cầu của chúng ta

[04-modules/promotion.md](../04-modules/promotion.md) mục 5 liệt kê các điều kiện áp dụng:

```text
Giá trị đơn tối thiểu · Danh mục/thương hiệu/bộ sưu tập
Seller cụ thể · Hạng khách hàng · Khách mới
Khoảng thời gian · Số lần dùng · Ngân sách
```

Hiện tài liệu chưa nói **cách cài đặt** các điều kiện này.

### Adopt

**Ý tưởng: điều kiện là dữ liệu, không phải code.**

```text
PromotionCondition {
    type      MIN_ORDER_VALUE | CATEGORY | BRAND | COLLECTION
              | CUSTOMER_TIER | FIRST_PURCHASE | SELLER | DATE_RANGE
    operator  EQ | GT | GTE | IN | BETWEEN
    value     JSONB
}
```

Đánh giá bằng bộ điều kiện tường minh trong Go — mỗi loại điều kiện là một hàm thuần nhận `(condition, context) → bool`.

### Adapt

**Không làm rule engine tổng quát.** Shopware cho phép quy tắc lồng nhau tùy ý với AND/OR nhiều tầng. Chúng ta giới hạn:

```text
✓ Danh sách điều kiện phẳng, nối bằng AND
✓ Mỗi loại điều kiện là một hàm Go tường minh
✗ Không lồng nhau, không OR phức tạp ở MVP
```

Lý do (nguyên tắc P16): rule engine tổng quát là trừu tượng hóa sớm. Bắt đầu bằng AND phẳng; nếu thực tế cần OR, thêm sau khi đã hiểu nhu cầu thật.

**Bắt buộc: giải thích được vì sao quy tắc không khớp.**

```text
Khi khuyến mãi không áp dụng, API trả về:
    "Không đạt điều kiện: giá trị đơn 400.000đ < mức tối thiểu 500.000đ"

Không phải: "Mã giảm giá không hợp lệ"
```

Đây là điểm yếu lớn nhất của rule engine mà chúng ta phải tránh ngay từ đầu.

### Quyết định cuối

```text
✓ Điều kiện khuyến mãi là dữ liệu có kiểu, không phải code
✓ Bộ điều kiện phẳng nối bằng AND
✓ BẮT BUỘC trả về lý do không khớp
✗ Không rule engine tổng quát
```

---

## Năng lực: Data Abstraction Layer (DAL)

### Cách OSS làm

Tầng trừu tượng dữ liệu riêng của Shopware: định nghĩa entity bằng `EntityDefinition`, truy vấn qua `Criteria` với hệ thống filter/aggregation/association.

### Điểm mạnh

Thống nhất mọi truy cập dữ liệu, hỗ trợ mở rộng entity từ plugin.

### Điểm yếu với chúng ta

DAL là **framework riêng** — phải học một ngôn ngữ truy vấn mới thay vì dùng SQL. Truy vấn phức tạp trở nên khó viết và khó tối ưu.

Với chúng ta, truy vấn thương mại phức tạp là chuyện thường ngày:

```sql
-- Đề xuất bổ sung: tồn kho + tốc độ bán + tín hiệu nhu cầu + MOQ nhà cung cấp
-- Viết bằng SQL: rõ ràng, tối ưu được
-- Viết bằng DAL: phải học API, khó kiểm soát kế hoạch thực thi
```

### Reject

Không dùng tầng trừu tượng truy vấn riêng. Chúng ta viết **SQL thật** trực tiếp trong `infrastructure/postgres/`.

Xem [ADR-0010](../adr/0010-database-layer.md).

---

## Năng lực: Sales Channel

### Cách OSS làm

Một hệ thống phục vụ nhiều kênh bán: website, app, marketplace bên thứ ba, POS. Mỗi kênh có danh mục, giá, ngôn ngữ, tiền tệ riêng.

### Adapt

Cùng kết luận với Vendure và Saleor (xem [vendure.md](vendure.md) mục "Channel vs Offer"):

```text
✓ Khái niệm kênh hữu ích cho: đa thị trường, đa tiền tệ (Phase 4)
✗ KHÔNG dùng làm mô hình marketplace
```

---

## Năng lực: Kiến trúc mở rộng bằng plugin và app

### Cách OSS làm

Plugin mở rộng qua event, service decoration, và điểm mở rộng. App chạy ngoài tiến trình, giao tiếp qua HTTP.

### Điểm mạnh

Mô hình **App** (chạy ngoài tiến trình) an toàn hơn plugin (chạy trong tiến trình) — lỗi ở app không làm sập hệ thống lõi.

### Reject

Không làm plugin system — cùng lý do đã nêu ở [vendure.md](vendure.md): chi phí chỉ đáng khi bán framework cho bên thứ ba.

Nhưng ghi nhận **mô hình App** cho tương lai: nếu cần tích hợp bên thứ ba, cho họ dùng **webhook + API**, không cho chạy code trong tiến trình của chúng ta. Đã có nền tảng cho việc này trong [06-api/webhook.md](../06-api/webhook.md).

---

## 2. Tổng kết Shopware

| Hạng mục | Quyết định |
|---|---|
| Điều kiện là dữ liệu có kiểu | **ADOPT** |
| Bắt buộc giải thích lý do không khớp | **ADOPT** — tránh điểm yếu của rule engine |
| Rule engine tổng quát, lồng nhau | **REJECT** — trừu tượng hóa sớm |
| DAL (tầng truy vấn riêng) | **REJECT** — dùng SQL thật, viết tay |
| Sales Channel làm marketplace | **REJECT** |
| Sales Channel cho đa thị trường | **ADAPT** — Phase 4 |
| Plugin trong tiến trình | **REJECT** |
| Mô hình App ngoài tiến trình | **ADAPT** — webhook + API cho đối tác |

**Nhận xét cuối:** Shopware đóng góp một ý tưởng đáng lấy (điều kiện là dữ liệu) và một bài học phòng tránh (rule engine không giải thích được thì vô dụng khi vận hành).

---

## 3. Tài liệu liên quan

- [../04-modules/promotion.md](../04-modules/promotion.md)
- [../adr/0010-database-layer.md](../adr/0010-database-layer.md)
- [../06-api/webhook.md](../06-api/webhook.md)
