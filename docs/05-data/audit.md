# Audit và lưu trữ dữ liệu

## 1. Ba loại nhật ký — phân biệt rõ

```text
1. AUDIT LOG      — ai làm gì, khi nào (con người và hệ thống)
                    → mục đích: trách nhiệm giải trình, điều tra

2. DOMAIN HISTORY — lịch sử thay đổi trạng thái nghiệp vụ
                    → mục đích: hiển thị cho người dùng, truy vết nghiệp vụ

3. APPLICATION LOG — nhật ký kỹ thuật
                    → mục đích: gỡ lỗi, giám sát
```

Ba loại này có yêu cầu lưu trữ và truy cập khác nhau. Gộp chung sẽ vừa thiếu (không truy vấn được nghiệp vụ) vừa thừa (lưu quá nhiều dữ liệu kỹ thuật).

---

## 2. Audit log — ghi cái gì

### Bắt buộc ghi

```text
Tài chính:
    - Ghi bút toán thủ công
    - Điều chỉnh sổ cái
    - Thực hiện chi trả
    - Hoàn tiền
    - Thay đổi tỷ lệ hoa hồng

Tồn kho:
    - Điều chỉnh thủ công
    - Kết quả kiểm kê

Quản trị:
    - Phê duyệt/từ chối seller, creator
    - Đình chỉ tài khoản
    - Thay đổi vai trò và quyền
    - Gỡ nội dung
    - Thay đổi chính sách (hoa hồng, đổi trả)

Dữ liệu cá nhân:
    - Truy cập dữ liệu khách hàng bởi nhân viên
    - Xuất dữ liệu
    - Xóa/ẩn danh hóa

Cấu hình:
    - Thay đổi cấu hình hệ thống
    - Bật/tắt tính năng
```

### Cấu trúc

```sql
CREATE TABLE audit_log (
    id            UUID PRIMARY KEY,
    actor_type    TEXT NOT NULL,      -- USER | SYSTEM | API_CLIENT
    actor_id      TEXT NOT NULL,
    action        TEXT NOT NULL,      -- "seller.approve", "ledger.adjust"
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    before_state  JSONB,
    after_state   JSONB,
    reason        TEXT,               -- BẮT BUỘC với thao tác nhạy cảm
    ip_address    INET,
    user_agent    TEXT,
    request_id    TEXT,               -- liên kết với application log
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_resource ON audit_log (resource_type, resource_id, occurred_at DESC);
CREATE INDEX idx_audit_actor ON audit_log (actor_id, occurred_at DESC);
```

### Trường `reason` bắt buộc

Với thao tác nhạy cảm, hệ thống phải **yêu cầu người thực hiện nhập lý do**:

```text
- Điều chỉnh tồn kho thủ công
- Điều chỉnh sổ cái
- Đình chỉ seller/creator
- Hoàn tiền ngoài quy trình
- Gỡ nội dung
```

Lý do trống hoặc "test", "fix" không đủ — cần cấu hình độ dài tối thiểu và rà soát định kỳ.

---

## 3. Audit log là bất biến

```sql
CREATE RULE audit_log_no_update AS ON UPDATE TO audit_log DO INSTEAD NOTHING;
CREATE RULE audit_log_no_delete AS ON DELETE TO audit_log DO INSTEAD NOTHING;
```

Nếu người có quyền sửa được audit log, nó mất hết giá trị.

**Kiểm soát bổ sung:** người vận hành database production không nên là người viết code nghiệp vụ. Đây là kiểm soát tổ chức, không phải kỹ thuật.

---

## 4. Domain history — lịch sử nghiệp vụ

Khác với audit log, đây là dữ liệu **hiển thị cho người dùng**:

```sql
order_status_history        -- khách xem tiến trình đơn
shipment_tracking           -- khách theo dõi vận chuyển
seller_status_history       -- seller xem lịch sử tài khoản
tier_history                -- khách xem lịch sử hạng
price_history               -- phân tích, minh bạch giá
inventory_movement          -- vận hành truy vết tồn kho
```

**Phân biệt:** `order_status_history` cho khách xem "đơn của bạn đã xuất kho lúc 14:00". `audit_log` ghi "nhân viên X đã đổi trạng thái đơn thủ công lúc 14:00, lý do: khách yêu cầu".

---

## 5. Truy cập dữ liệu cá nhân — ghi nhận bắt buộc

```text
Mỗi lần nhân viên truy cập dữ liệu khách hàng:
    → ghi audit log

Vì sao:
    - Yêu cầu tuân thủ ở nhiều thị trường
    - Phát hiện truy cập bất thường (nhân viên xem dữ liệu không liên quan công việc)
    - Bằng chứng khi có sự cố rò rỉ
```

**Cảnh báo tự động:** nhân viên truy cập bất thường nhiều hồ sơ khách trong thời gian ngắn → cảnh báo.

---

## 6. Chính sách lưu trữ dữ liệu

| Loại dữ liệu | Thời gian giữ | Lý do |
|---|---|---|
| Bút toán tài chính | Theo quy định kế toán (nhiều năm) | Nghĩa vụ pháp lý |
| Đơn hàng | Theo quy định kế toán | Chứng từ giao dịch |
| Audit log tài chính | Bằng dữ liệu tài chính | Kiểm toán |
| Audit log khác | 1–3 năm | Điều tra sự cố |
| Dữ liệu cá nhân khách | Đến khi khách yêu cầu xóa | Quyền được xóa |
| Lịch sử duyệt web | 6–12 tháng | Đủ cho phân tích |
| `click` (affiliate) | Chi tiết 90 ngày, sau đó tổng hợp | Khối lượng lớn |
| `event_log` (analytics) | Chi tiết 90 ngày, sau đó tổng hợp | Khối lượng lớn |
| `demand_signal` | Chi tiết 12 tháng, sau đó tổng hợp | Cần cho dự báo mùa vụ |
| Application log | 30–90 ngày | Gỡ lỗi |

**Lưu ý về `demand_signal`:** giữ 12 tháng vì dự báo thời trang cần so sánh cùng kỳ năm trước. Đây là yêu cầu nghiệp vụ, không phải kỹ thuật.

---

## 7. Xóa dữ liệu cá nhân — quy trình

Khi khách yêu cầu xóa tài khoản:

```text
XÓA / ẨN DANH HÓA:
    customer.name          → "Đã xóa"
    customer.email         → "deleted-<hash>@invalid"
    customer.phone         → NULL
    customer_address       → xóa toàn bộ
    customer_preference    → xóa (bao gồm số đo cơ thể)
    lịch sử duyệt web      → xóa

GIỮ LẠI (đã ẩn danh):
    order                  → giữ, customer_id giữ nguyên
    order_line             → giữ (bao gồm địa chỉ giao đã đóng băng — cần ẩn danh)
    ledger_entry           → giữ nguyên hoàn toàn
    attribution            → giữ (không chứa dữ liệu cá nhân)
```

### Vì sao giữ lại đơn hàng

```text
- Nghĩa vụ lưu trữ chứng từ kế toán
- Đơn hàng đã dùng để tính hoa hồng trả cho seller
- Xóa đơn làm sổ sách không cân
```

**Cách xử lý địa chỉ trong đơn:** địa chỉ giao hàng đã đóng băng trong `order_address` là dữ liệu cá nhân. Phải ẩn danh hóa (giữ tỉnh/thành để thống kê, xóa chi tiết) chứ không giữ nguyên.

---

## 8. Xuất dữ liệu theo yêu cầu

Khách có quyền yêu cầu bản sao dữ liệu của mình.

```text
Bao gồm:
    - Hồ sơ cá nhân
    - Địa chỉ
    - Lịch sử đơn hàng
    - Đánh giá đã viết
    - Wishlist
    - Điểm thưởng

KHÔNG bao gồm:
    - Dữ liệu nội bộ (điểm rủi ro, ghi chú nhân viên)
    - Dữ liệu của người khác
```

Thao tác xuất dữ liệu phải được ghi vào audit log.

---

## 9. Truy vết yêu cầu (request tracing)

Mỗi request có một định danh, truyền qua toàn bộ hệ thống:

```text
X-Request-ID: req_01J9X...
    → ghi trong application log
    → ghi trong audit_log.request_id
    → truyền vào correlation_id của domain event
    → truyền sang lời gọi dịch vụ bên ngoài
```

**Giá trị thực tế:** khi khách khiếu nại "tôi bị trừ tiền hai lần lúc 14:23", có thể tra ngược toàn bộ chuỗi từ request tới bút toán.

Xem [../09-operations/observability.md](../09-operations/observability.md).

---

## 10. Kiểm tra tính toàn vẹn định kỳ

```text
Hàng ngày:
    - Mọi ledger_entry có Σ DEBIT = Σ CREDIT?
    - balance_snapshot khớp tổng bút toán?
    - Có inventory_item nào âm?

Hàng tuần:
    - Đối chiếu với sao kê PSP
    - Đối chiếu với sao kê ngân hàng
    - Kiểm tra tham chiếu treo (order_line trỏ tới offer không tồn tại)

Hàng tháng:
    - Rà soát audit log các thao tác nhạy cảm
    - Rà soát truy cập dữ liệu cá nhân bất thường
```

**Nguyên tắc:** kết quả kiểm tra được ghi lại. Bất kỳ sai lệch nào ở nhóm hàng ngày là **sự cố nghiêm trọng**, không phải sai số chấp nhận được.

---

## 11. Tài liệu liên quan

- [consistency.md](consistency.md) — điều hòa dữ liệu
- [../09-operations/security.md](../09-operations/security.md) — bảo vệ dữ liệu
- [../04-modules/payment.md](../04-modules/payment.md) — sổ cái bất biến
- [../04-modules/customer.md](../04-modules/customer.md) — quyền riêng tư
