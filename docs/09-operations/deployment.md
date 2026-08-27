# Triển khai

## 1. Nguyên tắc

> Hạ tầng phải **đơn giản nhất có thể** cho quy mô hiện tại. Thêm độ phức tạp khi **đo được** nhu cầu, không phải khi phỏng đoán.

Đây là nguyên tắc P15 áp dụng cho vận hành.

---

## 2. Kiến trúc triển khai giai đoạn MVP

```text
        Người dùng
            │
       ┌────▼────┐
       │   CDN   │  ← ảnh, tài nguyên tĩnh
       └────┬────┘
            │
    ┌───────▼────────┐
    │  Load Balancer │
    └───┬────────┬───┘
        │        │
   ┌────▼───┐ ┌──▼─────┐
   │Next.js │ │Next.js │  ← storefront (nhiều bản)
   └────┬───┘ └──┬─────┘
        │        │
   ┌────▼────────▼───┐
   │   Go API (n)    │  ← cmd/api, nhiều bản
   └────┬────────────┘
        │
   ┌────▼────────────┐
   │  Go Worker (1)  │  ← cmd/worker: outbox, job định kỳ
   └────┬────────────┘
        │
   ┌────▼───────────┐
   │  PostgreSQL    │  ← một database, có bản sao dự phòng
   └────────────────┘
        │
   ┌────▼───────────┐
   │ Object Storage │  ← ảnh, video
   └────────────────┘
```

### Vì sao không dùng Kubernetes ở MVP

| Lý do | Giải thích |
|---|---|
| Chưa có nhiều service | Kubernetes giải bài toán điều phối nhiều service; ta có 2 tiến trình |
| Chi phí vận hành cao | Cần người hiểu sâu để vận hành an toàn |
| Chậm khắc phục sự cố | Thêm một tầng phải gỡ lỗi khi có vấn đề |
| Có lựa chọn đơn giản hơn | Nền tảng chạy container quản lý sẵn đủ dùng |

**Khi nào cân nhắc:** khi có nhiều service được tách ra (Phase 3+), có đội vận hành đủ năng lực, và có nhu cầu điều phối thật sự.

---

## 3. Hai tiến trình, một codebase

```text
cmd/api      — phục vụ HTTP request
cmd/worker   — xử lý outbox, job định kỳ, tác vụ nền

Chung: codebase, phiên bản, database, cấu hình module
Khác:  điểm khởi chạy, cách mở rộng quy mô
```

### Vì sao tách

```text
Tác vụ nền nặng (tổng hợp tín hiệu nhu cầu, tạo báo cáo)
KHÔNG được làm chậm request của khách hàng.

Tách tiến trình cho phép:
    - Mở rộng API độc lập với worker
    - Worker lỗi không làm sập API
    - Giới hạn tài nguyên riêng cho mỗi loại
```

**Lưu ý:** đây **không phải** microservices. Cùng codebase, cùng phiên bản, triển khai đồng thời.

### Worker — số lượng bản chạy

```text
Outbox publisher: PHẢI đảm bảo không xử lý trùng
    → dùng khóa ở tầng database (SELECT ... FOR UPDATE SKIP LOCKED)
    → hoặc chỉ chạy MỘT bản

Job định kỳ: dùng khóa để tránh chạy trùng
    → nhiều bản worker nhưng chỉ một bản thực thi mỗi job
```

---

## 4. Quy trình triển khai

```text
Push code
    ↓
CI: build · unit test · integration test
    · kiểm tra ranh giới module · kiểm tra hợp đồng API
    ↓
Tạo artifact (container image), gắn phiên bản
    ↓
Triển khai môi trường staging
    ↓
Kiểm thử tự động trên staging
    ↓
Triển khai production (rolling update)
    ↓
Theo dõi chỉ số 30 phút
    ↓
Nếu bất thường → quay lui
```

### Kiểm tra bắt buộc trong CI

```text
✓ Biên dịch thành công
✓ Unit test domain (phải rất nhanh — domain không phụ thuộc gì)
✓ Integration test (database thật)
✓ Kiểm tra ranh giới module — VI PHẠM LÀM CI THẤT BẠI
✓ Kiểm tra không có chu trình phụ thuộc
✓ Kiểm tra sở hữu bảng
✓ Đặc tả OpenAPI khớp cài đặt
✓ Không có thư mục bị cấm (common/, utils/, helpers/, services/)
```

Chi tiết: [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md) mục 9.

**Nguyên tắc:** vi phạm ranh giới làm CI **thất bại**, không phải cảnh báo. Cảnh báo sẽ bị bỏ qua.

---

## 5. Migration database

```text
Nguyên tắc: migration phải TƯƠNG THÍCH NGƯỢC trong thời gian triển khai.

Lý do: trong lúc rolling update, phiên bản cũ và mới chạy đồng thời.
       Cả hai phải hoạt động được với cùng schema.
```

### Quy trình ba bước cho thay đổi phá vỡ

```text
Bước 1 — MỞ RỘNG (triển khai 1)
    Thêm cột/bảng mới
    Code ghi vào CẢ cũ và mới, đọc từ cũ

Bước 2 — DI TRÚ (triển khai 2)
    Chuyển dữ liệu cũ sang mới
    Code đọc từ mới

Bước 3 — THU HẸP (triển khai 3)
    Xóa cột/bảng cũ
```

Mỗi bước là một lần triển khai riêng, có thể quay lui độc lập.

### Migration trên bảng lớn

```text
✗ KHÔNG: ALTER TABLE khóa bảng hàng triệu dòng
✓ CÓ:    thêm cột nullable (nhanh), backfill theo lô, thêm ràng buộc sau
✓ CÓ:    tạo chỉ mục CONCURRENTLY
```

---

## 6. Cấu hình và bí mật

```text
Cấu hình:  biến môi trường
Bí mật:    dịch vụ quản lý bí mật, KHÔNG trong biến môi trường thường,
           KHÔNG trong code, KHÔNG trong repository

Bí mật cần bảo vệ:
    - Thông tin kết nối database
    - Khóa API cổng thanh toán
    - Khóa ký token
    - Khóa ký webhook
    - Thông tin truy cập object storage
```

**Quy tắc:** bí mật production chỉ người vận hành có quyền truy cập. Lập trình viên dùng bí mật của môi trường phát triển.

---

## 7. Môi trường

```text
Development  — máy lập trình viên, database cục bộ
Staging      — giống production, dữ liệu ẩn danh hóa
Production   — thật
```

### Về dữ liệu staging

```text
✗ KHÔNG sao chép dữ liệu production nguyên vẹn sang staging
   → dữ liệu cá nhân khách hàng bị phát tán ra môi trường ít bảo mật hơn

✓ Sao chép rồi ẨN DANH HÓA:
   - Thay tên, email, số điện thoại bằng dữ liệu giả
   - Thay địa chỉ
   - Giữ nguyên cấu trúc và khối lượng để test hiệu năng đúng
```

---

## 8. Rolling update và quay lui

```text
Rolling update:
    Thay từng bản một, kiểm tra sức khỏe trước khi chuyển tiếp
    → không có thời gian ngừng dịch vụ

Điều kiện: phiên bản cũ và mới phải cùng tồn tại được
    → API tương thích ngược
    → schema tương thích ngược
```

### Kế hoạch quay lui

```text
Mọi lần triển khai phải trả lời được: "quay lui thế nào?"

Code:      triển khai lại artifact phiên bản trước
Schema:    KHÔNG quay lui migration đã chạy
           → đó là lý do migration phải tương thích ngược
Dữ liệu:   nếu đã ghi dữ liệu sai → cần kịch bản sửa riêng
```

---

## 9. Kiểm tra sức khỏe

```text
GET /health/live    — tiến trình còn sống không
                      → không kiểm tra phụ thuộc

GET /health/ready   — sẵn sàng nhận request chưa
                      → kiểm tra kết nối database
                      → kiểm tra migration đã chạy
```

**Phân biệt quan trọng:** nếu `live` cũng kiểm tra database, một sự cố database ngắn sẽ khiến toàn bộ tiến trình bị khởi động lại — làm sự cố tệ hơn.

---

## 10. Lộ trình hạ tầng

| Giai đoạn | Bổ sung | Điều kiện kích hoạt |
|---|---|---|
| MVP | API, worker, PostgreSQL, object storage, CDN | — |
| Phase 2 | Cache, chỉ mục tìm kiếm, bản sao chỉ đọc | Có điểm nghẽn đo được |
| Phase 3 | Lưu trữ chuỗi thời gian cho analytics | Ghi analytics ảnh hưởng database chính |
| Phase 3+ | Service tách ra | Có lý do trong sáu lý do hợp lệ |

Xem [../03-architecture/evolution-to-services.md](../03-architecture/evolution-to-services.md) mục 5.

---

## 11. Tài liệu liên quan

- [observability.md](observability.md) — giám sát sau triển khai
- [backup.md](backup.md), [disaster-recovery.md](disaster-recovery.md)
- [../05-data/database.md](../05-data/database.md)
- [../10-roadmap/scale.md](../10-roadmap/scale.md)
