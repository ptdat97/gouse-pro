# Module: Notification

| | |
|---|---|
| **Bounded Context** | Platform |
| **Phân loại** | Generic |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Gửi thông báo qua các kênh: email, SMS, push, in-app
- Quản lý mẫu thông báo
- Quản lý tùy chọn nhận thông báo của người dùng
- Ghi log gửi và theo dõi trạng thái

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Quyết định nghiệp vụ nào cần thông báo | Module nghiệp vụ (qua event) |
| Nội dung marketing | `campaign` |
| Dữ liệu khách hàng | `customer` |

---

## 3. Ràng buộc kiến trúc quan trọng nhất

> **`notification` KHÔNG được gọi bất kỳ module nghiệp vụ nào.**

```text
SAI:  notification nhận event order.placed
      → gọi catalog.GetProduct() để lấy tên sản phẩm
      → gọi customer.GetCustomer() để lấy tên khách
      → notification phụ thuộc toàn hệ thống, không tách được

ĐÚNG: event order.placed đã chứa đủ thông tin
      (product_name đã đóng băng trong OrderLine, email khách)
      → notification chỉ dùng dữ liệu trong payload
```

**Hệ quả cho thiết kế event:** payload phải chứa đủ thông tin để bên nhận xử lý mà không phải gọi ngược. Đây là lý do nguyên tắc này được nêu tại [../02-domain/domain-events.md](../02-domain/domain-events.md) mục 3.

---

## 4. Bốn kênh gửi

```text
EMAIL     — xác nhận đơn, hóa đơn, thông báo dài
SMS       — mã OTP, cập nhật giao hàng khẩn
PUSH      — thông báo ứng dụng di động
IN_APP    — hộp thông báo trong ứng dụng
```

Mỗi kênh nằm sau interface (nguyên tắc P13):

```go
type NotificationChannel interface {
    Send(ctx, req SendRequest) (*SendResult, error)
    GetStatus(ctx, messageID string) (*DeliveryStatus, error)
}
```

Domain không biết nhà cung cấp dịch vụ nào.

---

## 5. Tùy chọn nhận thông báo — bắt buộc tuân thủ

```text
NotificationPreference {
    user_id
    channel          EMAIL | SMS | PUSH | IN_APP
    category         TRANSACTIONAL | MARKETING | SOCIAL
    enabled
}
```

### Phân biệt bắt buộc

```text
TRANSACTIONAL  — xác nhận đơn, giao hàng, hoàn tiền
                 → KHÔNG cần đồng ý marketing, luôn gửi
                 → khách không tắt được (là thông tin thiết yếu)

MARKETING      — khuyến mãi, sản phẩm mới
                 → BẮT BUỘC có đồng ý (kiểm tra customer_consent)
                 → phải có cách hủy đăng ký dễ dàng
```

Nhầm lẫn hai loại này là vi phạm pháp luật ở nhiều thị trường. Trước khi gửi marketing, phải gọi `customer.HasConsent()`.

**Lưu ý:** đây là **ngoại lệ có kiểm soát** duy nhất của quy tắc ở mục 3 — kiểm tra đồng ý là yêu cầu pháp lý bắt buộc, không thể chỉ dựa vào payload event vì đồng ý có thể thay đổi sau khi event được phát.

---

## 6. Các thông báo chính

| Sự kiện | Kênh | Loại |
|---|---|---|
| Đơn được đặt | Email, In-app | Transactional |
| Thanh toán thành công | Email | Transactional |
| Đơn đã xuất kho | Email, SMS, Push | Transactional |
| Đơn đã giao | Email, Push | Transactional |
| Yêu cầu trả hàng được duyệt | Email | Transactional |
| Hoàn tiền thành công | Email | Transactional |
| Seller: có đơn mới | Email, Push | Transactional |
| Seller: đối soát sẵn sàng | Email | Transactional |
| Creator: hoa hồng phát sinh | Email, In-app | Transactional |
| **Sản phẩm có hàng trở lại** | Email, Push | Transactional* |
| Giỏ hàng bỏ quên | Email | Marketing |
| Bộ sưu tập mới | Email, Push | Marketing |

*Thông báo "có hàng trở lại" là transactional vì khách chủ động đăng ký nhận.

---

## 7. Yêu cầu kỹ thuật

| Yêu cầu | Lý do |
|---|---|
| Idempotent | Event có thể đến hai lần → không gửi trùng |
| Thử lại có kiểm soát | Dịch vụ gửi có thể tạm lỗi |
| Giới hạn tần suất | Không spam khách |
| Ghi log mọi lần gửi | Hỗ trợ khách khi khiếu nại |
| Xử lý bất đồng bộ | Không làm chậm luồng nghiệp vụ |
| Ưu tiên theo loại | OTP phải gửi ngay, marketing có thể chờ |

---

## 8. Dữ liệu sở hữu

```sql
notification_template
notification_log
notification_preference
notification_queue
```

---

## 9. Interface công khai

```go
type PublicAPI interface {
    Send(ctx, req SendNotificationRequest) error
    GetPreferences(ctx, userID string) ([]Preference, error)
    UpdatePreference(ctx, req UpdatePreferenceRequest) error
    GetNotificationHistory(ctx, userID string, page Pagination) (*NotificationList, error)
}
```

Interface này chủ yếu dùng cho các trường hợp gửi trực tiếp (ví dụ OTP). Đa số thông báo được kích hoạt qua event.

---

## 10. Event

**Lắng nghe:** rất nhiều event từ các module nghiệp vụ (xem bảng mục 6).

**Phát ra:** `notification.sent`, `notification.failed`, `notification.bounced`

---

## 11. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | **Không gọi module nghiệp vụ nào** (trừ kiểm tra đồng ý) |
| 2 | Phân biệt transactional và marketing |
| 3 | Marketing bắt buộc kiểm tra đồng ý |
| 4 | Gửi phải idempotent |
| 5 | Xử lý bất đồng bộ |
| 6 | Ghi log mọi lần gửi |
| 7 | Có giới hạn tần suất |

---

## 12. Tài liệu liên quan

- [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md) — vì sao notification phải độc lập
- [customer.md](customer.md) — quản lý đồng ý
