# Quan sát hệ thống (Observability)

## 0. Trạng thái triển khai (20/08/2026)

Tài liệu này mô tả THIẾT KẾ. Bảng dưới ghi mức đã thực sự cài — đọc phần
còn lại với hiểu biết đó.

| Phần | Trạng thái |
|---|---|
| Log có cấu trúc (`slog`, `platform/logger`) | ✅ dùng ở mọi module |
| Request ID trong log và trong mọi response lỗi | ✅ |
| Correlation ID xuyên tiến trình | 🔴 trường có (`Event.WithTrace`), nhưng chỉ 2 chỗ gọi — phần còn lại luôn rỗng |
| Metrics kỹ thuật (mục 3) | 🔴 chưa có gì |
| Metrics nghiệp vụ (mục 4) | 🔴 chưa có gì |
| Distributed tracing (mục 5) | 🔴 chưa có gì |
| Cảnh báo (mục 7) · Dashboard (mục 8) | 🔴 chưa có gì |
| Nhật ký kiểm toán (`platform/audit`) | ✅ có, kèm ranh giới giao dịch |

**Ưu tiên khi bắt tay vào:** metrics outbox trước tiên. Outbox tồn đọng là
triệu chứng SỚM của gần như mọi sự cố ở kiến trúc này — worker chết, event
kẹt, tồn kho không chuyển Reserved → Committed, và tiến trình dọn có thể
nhả hàng của một đơn đã thanh toán. Hiện không có gì báo động chuyện đó.

Việc theo dõi ở [backlog.md mục 2.13](../10-roadmap/backlog.md).

---

## 1. Ba trụ cột và một bổ sung

```text
LOGS     — chuyện gì đã xảy ra, chi tiết
METRICS  — số liệu tổng hợp theo thời gian
TRACES   — một request đi qua đâu, mất bao lâu

+ BUSINESS METRICS — chỉ số nghiệp vụ (khác với chỉ số kỹ thuật)
```

Trụ cột thứ tư thường bị bỏ qua nhưng quan trọng nhất với nền tảng thương mại: hệ thống có thể "khỏe" về mặt kỹ thuật nhưng đang mất doanh thu.

---

## 2. Logging

### Cấu trúc log

```json
{
  "timestamp": "2026-08-11T14:23:11.123Z",
  "level": "error",
  "message": "failed to reserve inventory",
  "request_id": "req_01J9X...",
  "correlation_id": "cor_01J9X...",
  "user_id": "usr_01J9X...",
  "module": "inventory",
  "operation": "Reserve",
  "sku_id": "sku_01J9X...",
  "error": "insufficient quantity: requested 5, available 2",
  "duration_ms": 45
}
```

### Quy tắc

| Quy tắc | Lý do |
|---|---|
| Log có cấu trúc (JSON), không phải văn bản tự do | Truy vấn được |
| Luôn có `request_id` | Liên kết mọi log của một request |
| Có `correlation_id` | Liên kết chuỗi nghiệp vụ qua nhiều request |
| **Không log dữ liệu nhạy cảm** | Mật khẩu, số thẻ, số đo cơ thể, token |
| Không log toàn bộ payload request | Có thể chứa dữ liệu cá nhân |
| Mức log phù hợp | `error` cho lỗi thật, không phải mọi thứ |

### Mức log

```text
error  — cần người xử lý
warn   — bất thường nhưng hệ thống tự xử lý được
info   — sự kiện nghiệp vụ quan trọng (đơn được tạo, payout thực hiện)
debug  — chi tiết gỡ lỗi, TẮT ở production
```

---

## 3. Metrics kỹ thuật

```text
Theo endpoint:
    - Số request/giây
    - Độ trễ (p50, p95, p99)
    - Tỷ lệ lỗi theo mã HTTP

Database:
    - Số kết nối đang dùng
    - Độ trễ truy vấn
    - Truy vấn chậm
    - Độ trễ sao chép

Event:
    - Độ trễ outbox (event chờ phát)
    - Tỷ lệ xử lý event thất bại
    - Kích thước dead letter queue

Tiến trình:
    - CPU, bộ nhớ
    - Số goroutine
    - Thời gian GC
```

### Ngưỡng cảnh báo

| Chỉ số | Ngưỡng |
|---|---|
| Độ trễ API p95 | > 300ms |
| Độ trễ API p99 | > 1s |
| Tỷ lệ lỗi 5xx | > 0,1% |
| Kết nối database | > 80% giới hạn |
| Độ trễ outbox | > 60 giây |
| Dead letter queue | > 10 |

---

## 4. Metrics nghiệp vụ — quan trọng nhất

Đây là điểm phân biệt: hệ thống "khỏe" về kỹ thuật vẫn có thể đang mất tiền.

```text
Đơn hàng:
    - Số đơn/giờ  ← so với cùng giờ hôm qua, tuần trước
    - Tỷ lệ chuyển đổi checkout
    - Tỷ lệ thanh toán thất bại
    - Giá trị đơn trung bình

Tồn kho:
    - Số SKU hết hàng
    - Tỷ lệ hủy do hết hàng
    - Reservation quá hạn chưa giải phóng

Tài chính:
    - Độ lệch đối soát        ← PHẢI BẰNG 0
    - Bút toán không cân bằng ← PHẢI BẰNG 0
    - Payout thất bại

Marketplace:
    - Số seller hoạt động
    - Đơn quá hạn SLA chưa xử lý

Creator:
    - Tỷ lệ quy kết bị đảo ngược
```

### Ví dụ cảnh báo nghiệp vụ

```text
"Số đơn hàng trong 1 giờ qua giảm 60% so với cùng giờ tuần trước"

→ Kỹ thuật có thể hoàn toàn bình thường:
  API trả 200, độ trễ tốt, database khỏe

→ Nhưng: có thể nút "Thêm giỏ" bị lỗi hiển thị,
  hoặc cổng thanh toán từ chối im lặng,
  hoặc trang chủ hiển thị sai
```

**Nguyên tắc:** cảnh báo nghiệp vụ bắt được lỗi mà cảnh báo kỹ thuật bỏ sót.

---

## 5. Distributed tracing

```text
Mỗi request có trace_id, truyền qua:
    HTTP handler → use case → module khác → database
                              → dịch vụ ngoài
                              → domain event → bên xử lý event
```

### Vì sao cần ngay từ monolith

```text
Ngay trong một tiến trình, một request đi qua nhiều module:
    checkout → inventory → pricing → promotion
             → fulfillment → payment → order

Trace cho biết: bước nào chậm, bước nào lỗi.

Và: khi tách service sau này, trace đã sẵn sàng
    → không phải thêm vào sau
```

### Liên kết trace với nghiệp vụ

```text
request_id      — một lời gọi HTTP
correlation_id  — toàn bộ chuỗi nghiệp vụ
                  (checkout → payment → order → fulfillment)
causation_id    — event nào sinh ra event nào
```

Ba định danh này cho phép trả lời: *"khách khiếu nại bị trừ tiền hai lần lúc 14:23"* → tra ngược toàn bộ chuỗi từ request tới bút toán.

Xem [../05-data/audit.md](../05-data/audit.md) mục 9.

---

## 6. Phân biệt Observability và Analytics

| | Observability | Analytics |
|---|---|---|
| Mục đích | Hệ thống có chạy đúng không | Nghiệp vụ có hiệu quả không |
| Người dùng | Kỹ sư, vận hành | Quản lý, merchandiser |
| Độ trễ | Thời gian thực | Chấp nhận trễ |
| Lưu trữ | Ngắn (30–90 ngày) | Dài |
| Ví dụ | "API p99 = 800ms" | "Tỷ lệ hoàn hàng size M = 30%" |

Hai hệ thống riêng, không gộp. Xem [../04-modules/analytics.md](../04-modules/analytics.md).

---

## 7. Cảnh báo — nguyên tắc

```text
✓ Cảnh báo khi cần NGƯỜI HÀNH ĐỘNG
✗ Không cảnh báo cho mọi bất thường nhỏ

Cảnh báo quá nhiều → người ta bỏ qua → bỏ sót cảnh báo thật
```

### Phân cấp

```text
P1 — Sự cố nghiêm trọng, gọi ngay bất kể giờ nào
    · Không đặt hàng được
    · Không thanh toán được
    · Độ lệch đối soát tài chính ≠ 0
    · Bút toán không cân bằng
    · Tồn kho âm
    · Rò rỉ dữ liệu

P2 — Cần xử lý trong giờ làm việc
    · Độ trễ API cao
    · Tỷ lệ lỗi tăng
    · Outbox chậm
    · Payout thất bại

P3 — Theo dõi, xử lý theo lịch
    · Truy vấn chậm
    · Chỉ mục không dùng
    · Chứng nhận nhà cung cấp sắp hết hạn
```

### Cảnh báo P1 về tài chính

```text
Ba chỉ số PHẢI LUÔN BẰNG 0:
    1. Độ lệch đối soát
    2. Bút toán không cân bằng
    3. Số SKU có tồn kho âm

Bất kỳ giá trị nào khác = sự cố nghiêm trọng, KHÔNG phải sai số.
```

---

## 8. Dashboard cần có

```text
1. Sức khỏe hệ thống
   Độ trễ, tỷ lệ lỗi, throughput, database

2. Phễu thương mại (thời gian thực)
   Truy cập → xem SP → thêm giỏ → checkout → đơn → thanh toán
   → phát hiện ngay chỗ rò rỉ

3. Tài chính
   Độ lệch đối soát, payout, bút toán bất thường

4. Chuỗi cung ứng
   SKU hết hàng, đơn sản xuất trễ, đề xuất bổ sung chờ duyệt

5. Marketplace
   Đơn quá hạn SLA, seller có vấn đề, tỷ lệ hủy
```

Dashboard số 2 là quan trọng nhất — nó phát hiện vấn đề nhanh hơn mọi cảnh báo kỹ thuật.

---

## 9. Kiểm tra định kỳ tự động

```text
Hàng ngày:
    ✓ Mọi ledger_entry cân bằng?
    ✓ balance_snapshot khớp tổng bút toán?
    ✓ Có inventory_item âm?
    ✓ Reservation quá hạn chưa xử lý?

Hàng tuần:
    ✓ Đối chiếu với sao kê PSP
    ✓ Đối chiếu với sao kê ngân hàng
    ✓ Tham chiếu treo (order_line → offer không tồn tại)
    ✓ Order PAID nhưng không có FulfillmentOrder

Hàng tháng:
    ✓ Rà soát audit log thao tác nhạy cảm
    ✓ Rà soát truy cập dữ liệu cá nhân bất thường
```

**Nguyên tắc:** job kiểm tra **phát hiện và cảnh báo**, không tự động sửa. Tự sửa có thể che giấu lỗi thật.

Ngoại lệ: giải phóng reservation quá hạn được tự động — hành động an toàn và có tính lặp lại.

---

## 10. Tài liệu liên quan

- [deployment.md](deployment.md)
- [../05-data/consistency.md](../05-data/consistency.md) mục 10
- [../05-data/audit.md](../05-data/audit.md)
- [../01-business/kpi.md](../01-business/kpi.md)
