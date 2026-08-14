# Sổ đăng ký phụ thuộc OSS

Mọi thư viện và dự án được xét đều phải có mặt ở đây trước khi đưa vào dự án.

Dữ liệu kiểm tra ngày **12/08/2026** qua GitHub API.

---

## 1. Phân loại

```text
Core Dependency        Phụ thuộc cốt lõi — hệ thống không chạy được nếu thiếu
Optional Dependency    Dùng cho một phần, thay thế được
Reference Only         CHỈ đọc để học, không đưa vào code
Potential Future       Có thể dùng sau, chưa cần
Rejected               Đã xét và loại
```

---

## 2. Phụ thuộc cốt lõi

| Thư viện | Mục đích | Sao | Cập nhật | License | Trạng thái |
|---|---|---|---|---|---|
| `sqlc-dev/sqlc` | Sinh code Go từ SQL | 18.158 | 2026-08-11 | MIT | **KHÔNG dùng** — xem [ADR-0010](../adr/0010-database-layer.md) mục sửa đổi |
| `jackc/pgx` | Driver PostgreSQL | 14.129 | 2026-08-01 | MIT | **Kế hoạch** |
| `golang-migrate/migrate` | Migration có phiên bản | 18.810 | 2026-07-05 | MIT | **Kế hoạch** |

### Đánh giá rủi ro

Cả ba đều: cập nhật trong 2 tháng gần đây, license cho phép thương mại, cộng đồng lớn.

**Rủi ro tập trung:** ba thư viện này là nền của toàn bộ tầng dữ liệu. Nếu một trong ba ngừng phát triển:

```text
pgx      → driver thuần, SQL không phụ thuộc gì; thay driver là việc cục bộ
           → rủi ro THẤP, có thể duy trì code sinh ra thủ công
pgx      → cần đổi driver, ảnh hưởng rộng
           → rủi ro TRUNG BÌNH, nhưng pgx là driver chuẩn de facto
migrate  → dễ thay nhất, file SQL vẫn dùng được với công cụ khác
           → rủi ro THẤP
```

Đây là lý do chọn **SQL viết tay thay vì ORM**: thứ duy nhất phụ thuộc là driver, còn SQL thì không phụ thuộc gì. Một ORM ngừng bảo trì kéo theo toàn bộ tầng dữ liệu.

---

## 3. Phụ thuộc tùy chọn

| Thư viện | Mục đích | Sao | Cập nhật | License | Trạng thái |
|---|---|---|---|---|---|
| `testcontainers/testcontainers-go` | Integration test với DB thật | 4.946 | 2026-08-11 | MIT | **Kế hoạch** |
| `stretchr/testify` | Tiện ích test | 26.156 | 2026-07-21 | MIT | Cân nhắc |

### Ghi chú

**testify:** hiện tại chúng ta dùng thư viện chuẩn `testing` thuần, không có phụ thuộc. 87 test đã viết không cần testify.

Chỉ thêm nếu thấy rõ lợi ích — nguyên tắc P16 (thà rõ ràng còn hơn ngắn gọn) nghiêng về việc giữ thư viện chuẩn.

**testcontainers:** chỉ dùng trong test, không vào binary production. Cần Docker — đánh dấu bằng build tag để test chạy được cả khi không có.

---

## 4. Chỉ tham chiếu — không đưa vào code

| Dự án | Học được gì | License | Được sao chép code? |
|---|---|---|---|
| Flamingo Commerce | Chia tiền, ports/adapters, adapter giả | MIT | Có (giữ bản quyền) |
| QOR | Nháp/xuất bản, media, trạng thái | MIT | Có, nhưng gắn GORM → không dùng |
| Digota | Tiền số nguyên, nhận diện tranh chấp | MIT | Không nên — ngừng bảo trì 2021 |
| GoShop | Cấu trúc dự án, migration, testcontainers | MIT | Có |
| GoCommerce | Headless, thuế là adjustment | MIT | Có |
| Medusa | **Link table**, workflow bù trừ | MIT + Enterprise | Có (phần MIT) |
| Saleor | Tách thuộc tính theo ngữ cảnh | BSD-3 | Có (giữ bản quyền) |
| Shopware | Điều kiện là dữ liệu | MIT | Có |
| Sylius | **Adjustment**, nhiều state machine | MIT | Có |
| **Vendure** | Channel, event bus | **GPLv3** | **KHÔNG** |
| **Magento** | **MSI reservation**, service contracts | **OSL-3.0** | **KHÔNG** |
| **go-saas/commerce** | (không có gì độc đáo) | **Không có** | **KHÔNG** |

### Hai dự án cấm sao chép code

**Vendure (GPLv3)** và **Magento (OSL-3.0)** là copyleft. Sao chép code vào sản phẩm độc quyền của chúng ta sẽ buộc phải mở nguồn toàn bộ.

```text
Được phép: đọc, hiểu ý tưởng, mô tả bằng lời, tự cài đặt từ đầu
Bị cấm:    sao chép code, sao chép cấu trúc lớp trực tiếp
```

Toàn bộ ghi chép trong [vendure.md](vendure.md) và [magento.md](magento.md) là mô tả khái niệm, không trích code.

---

## 5. Có thể dùng trong tương lai

| Thư viện | Mục đích | Khi nào cần | Điều kiện |
|---|---|---|---|
| `go-chi/chi` | HTTP router | Khi `net/http` mux không đủ | Hiện tại Go 1.22+ mux đã đủ |
| `oklog/ulid` | Sinh ULID | Nếu tự cài có vấn đề | Đã tự cài trong `kernel/ids` |
| Redis client | Cache | Khi có điểm nóng **đo được** | Không thêm khi chưa đo |
| OpenTelemetry SDK | Truy vết phân tán | Phase 2 | Đã thiết kế sẵn request_id |
| Chỉ mục tìm kiếm | Tìm kiếm nâng cao | Khi SQL không đủ | Đo trước |

### Vì sao tự cài ULID thay vì dùng `oklog/ulid`

```text
✓ Chỉ ~150 dòng, không phức tạp
✓ Cần tiền tố theo loại thực thể — thư viện không có
✓ Cần khớp CHÍNH XÁC pattern trong đặc tả OpenAPI
✓ Giảm một phụ thuộc trong kernel (kernel phải tối thiểu — quy tắc R4)
```

Đây là áp dụng nguyên tắc: thư viện phải **tiết kiệm nhiều hơn chi phí phụ thuộc**.

---

## 6. Đã loại

| Ứng viên | Lý do loại |
|---|---|
| **GORM** | Model ORM = model domain → vi phạm R2; khóa ngoại vượt module |
| **ent** | Sinh entity + schema riêng → domain phụ thuộc công cụ |
| **go-saas/commerce** | **Không có license** — rào cản pháp lý tuyệt đối |
| Thư viện `qor/*` | Gắn GORM |
| Thư viện workflow engine | Chưa tương xứng chi phí |
| Thư viện state machine | Domain layer phải sạch, tự cài đơn giản hơn |
| MongoDB driver | Cần ACID cho tài chính |
| gRPC | Không client nào cần |

Chi tiết quyết định database: [ADR-0010](../adr/0010-database-layer.md).

---

## 7. Trạng thái hiện tại của dự án

```bash
$ cat go.mod
module github.com/fashion-commerce/platform
go 1.26.5
```

**Không có phụ thuộc bên ngoài nào.** 4.400 dòng Go, 87 test, chỉ dùng thư viện chuẩn.

Đây không phải mục tiêu tự thân — nhưng nó cho thấy phần lớn nền tảng không cần thư viện ngoài. Ba phụ thuộc cốt lõi sẽ được thêm khi làm tầng database.

---

## 8. Mẫu ghi nhận cho phụ thuộc mới

Khi đề xuất thêm bất kỳ thư viện nào, điền đủ:

```text
Thư viện:
Repository:
License:                    ← không có license = DỪNG
Sao / Fork:
Cập nhật cuối:              ← > 2 năm = rủi ro cao
Số phụ thuộc kéo theo:
Vấn đề cụ thể cần giải:
Tự viết mất bao lâu:
Nếu ngừng bảo trì thì sao:
Phân loại:
Dùng ở module nào:
```

---

## 9. Tài liệu liên quan

- [adoption-policy.md](adoption-policy.md) mục 6 — quy trình xét
- [../adr/0010-database-layer.md](../adr/0010-database-layer.md)
- [go-saas-commerce.md](go-saas-commerce.md) — ví dụ loại ở bước 1
