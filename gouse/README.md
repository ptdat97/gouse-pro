# Fashion Commerce Platform

Nền tảng thương mại thời trang: own brand + marketplace + creator commerce + chuỗi cung ứng.

**Kiến trúc:** Modular Monolith bằng Go, API First, frontend Next.js chỉ làm trình bày.

---

## Bắt đầu nhanh

```bash
make run              # chạy API ở cổng 8080
curl localhost:8080/health/live
curl localhost:8080/version
```

```bash
make check            # chạy toàn bộ kiểm tra như CI
make help             # xem tất cả lệnh
```

---

## Trạng thái hiện tại

Đang ở **Giai đoạn 1 — Nền tảng** theo [lộ trình triển khai](docs/10-roadmap/deliverables.md#14-thứ-tự-triển-khai-đề-xuất).

| Thành phần | Trạng thái |
|---|---|
| `internal/kernel` — Money, ID, BasisPoints, Quantity | Xong |
| `cmd/archcheck` — thực thi ranh giới module | Xong |
| `internal/platform` — config, logger, apierror, httpserver | Xong |
| `cmd/api`, `cmd/worker` — chạy được | Xong |
| CI: ranh giới module, race detector, lint OpenAPI | Xong |
| `internal/platform/database` + migration | Chưa |
| `internal/platform/eventbus` + outbox | Chưa |
| Module nghiệp vụ đầu tiên (`catalog`) | Chưa |

---

## Cấu trúc thư mục

```text
cmd/
  api/            tiến trình HTTP
  worker/         tiến trình tác vụ nền (outbox, job định kỳ)
  archcheck/      công cụ thực thi ranh giới module

internal/
  kernel/         khái niệm domain MỌI module đều hiểu giống nhau
                  → rất nhỏ, thay đổi rất hiếm
  platform/       hạ tầng TRUNG LẬP với domain
                  → nếu nó nhắc tới "order" hay "seller" thì đặt sai chỗ
  modules/        module nghiệp vụ (chưa có)

api/              đặc tả OpenAPI — nguồn sự thật duy nhất về hợp đồng API
docs/             tài liệu kiến trúc (105 file)
migrations/       file SQL migration
```

**Cấm tuyệt đối** các thư mục `common/`, `utils/`, `helpers/`, `services/` — chúng trở thành bãi rác phụ thuộc và phá hủy tính module một cách âm thầm. `archcheck` sẽ chặn.

---

## Ranh giới module

Bảy quy tắc được **thực thi bằng công cụ**, không dựa vào kỷ luật con người:

| Mã | Quy tắc |
|---|---|
| R1 | Chỉ import `public.go` của module khác, không import sâu |
| R2 | `domain/` chỉ phụ thuộc chính nó và `kernel/` |
| R3 | `platform/` không biết khái niệm nghiệp vụ |
| R4 | `kernel/` chỉ dùng thư viện chuẩn |
| R5 | Không có phụ thuộc vòng giữa các module |
| R7 | Cấm thư mục `common/`, `utils/`, `helpers/`, `services/` |
| R8 | Chiều tầng: `interfaces → application → domain ← infrastructure` |

```bash
make arch      # kiểm tra thủ công
```

Trong CI, vi phạm làm **build thất bại**, không phải cảnh báo — cảnh báo sẽ bị bỏ qua và vi phạm nhỏ tích lũy tới mức không tách service được nữa.

Xem [ADR-0005](docs/adr/0005-module-boundaries.md).

---

## Ba nguyên tắc quan trọng nhất khi viết code

**1. Tiền là số nguyên, luôn kèm đơn vị**

```go
price := money.MustNew(299_000, money.VND)   // 299.000đ
```

Không bao giờ dùng `float` cho tiền — `0.1 + 0.2 = 0.30000000000000004`, và độ lệch đối soát phải bằng 0. Chia tiền dùng `Allocate()` để không mất đồng nào.

**2. Domain layer không phụ thuộc gì**

Nếu xóa toàn bộ `infrastructure/` và `interfaces/`, code trong `domain/` vẫn phải biên dịch được. Đây là điều kiện để kiểm thử quy tắc nghiệp vụ mà không cần database.

**3. Lỗi API theo đúng đặc tả**

```go
apierror.New(apierror.CodeInsufficientInventory, "Sản phẩm không đủ số lượng").
    WithDetails(map[string]any{"available": 1, "requested": 2})
```

Client xử lý theo `code`, không parse `message`. Chi tiết đủ để hiển thị hữu ích — "chỉ còn 1 sản phẩm" tốt hơn "hết hàng".

---

## Đặc tả API

```bash
make api-lint      # kiểm tra đặc tả hợp lệ
make api-types     # sinh kiểu TypeScript cho frontend
```

`api/openapi.yaml` là **nguồn sự thật duy nhất**, cập nhật trong cùng pull request với thay đổi code. Xem [api/README.md](api/README.md).

---

## Tài liệu

| Bắt đầu từ | Nội dung |
|---|---|
| [docs/README.md](docs/README.md) | Điều hướng toàn bộ tài liệu |
| [docs/00-overview/principles.md](docs/00-overview/principles.md) | 16 nguyên tắc kiến trúc bắt buộc |
| [docs/10-roadmap/deliverables.md](docs/10-roadmap/deliverables.md) | Tổng hợp bàn giao, rủi ro, thứ tự triển khai |
| [docs/adr/](docs/adr/) | 9 quyết định kiến trúc kèm lý do |

---

## Yêu cầu môi trường

- Go 1.26+
- Node.js 20+ (chỉ để kiểm tra đặc tả OpenAPI)
- PostgreSQL 16+ (khi có module dùng database)

Biến môi trường: xem [internal/platform/config/config.go](internal/platform/config/config.go). Ở `development` mọi biến đều có giá trị mặc định hợp lý; ở `production` thiếu `DATABASE_URL` là lỗi khởi động.
