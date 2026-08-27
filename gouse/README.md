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

Với `MODULES_STORAGE=postgres`, lần khởi động đầu nạp dữ liệu mẫu **mua
được**: thương hiệu → sản phẩm → SKU → giá → **offer** → **tồn kho**. Định
danh ghi ra log lúc khởi động (ULID sinh mới mỗi lần nạp nên không đoán được).

```bash
go run ./cmd/taotaikhoan   # tạo tài khoản để THỬ GIAO DIỆN (chỉ development)
```

Tạo `khach@` · `admin@` · `vanhanh@gouse.test`, mật khẩu `Gouse@Test2026`.
Chạy lại nhiều lần không sao — nó nhận ra tài khoản đã có và xác minh mật
khẩu vẫn đúng. Từ chối chạy khi `APP_ENV` khác `development`.

```bash
make test-db          # MỘT LẦN: tạo database khuôn cho test
make check            # chạy toàn bộ kiểm tra như CI
make help             # xem tất cả lệnh
```

Test chạy trên database **riêng**, không phải database phát triển. Mỗi gói
test tự sao một bản từ khuôn `gouse_test`, nên chúng chạy song song được và
không gói nào xóa dữ liệu của gói nào. Chi tiết:
[internal/platform/testdb](internal/platform/testdb/testdb.go).

Chưa chạy `make test-db` thì test cần database **tự bỏ qua** — bộ test vẫn
xanh nhưng KHÔNG kiểm chứng tầng kho lưu trữ. CI phải chạy nó.

---

## Trạng thái hiện tại

Đang ở **Giai đoạn 8 — Production Hardening** theo [lộ trình triển khai](../docs/10-roadmap/deliverables.md#14-thứ-tự-triển-khai-đề-xuất).

Commerce core đã dựng xong và có test. Phase này **không thêm tính năng** —
nó chứng minh những gì đã có chịu được điều kiện thật: đồng thời, lỗi, thử
lại, và kẻ tấn công. Ưu tiên:

```text
Correctness > Consistency > Security > Reliability > Performance > Feature velocity
```

Việc chi tiết ở [backlog mục 2](../docs/10-roadmap/backlog.md); trạng thái
từng phần kèm bằng chứng ở [todo mục 12](../docs/10-roadmap/todo.md).

| Thành phần | Trạng thái |
|---|---|
| `internal/kernel` — Money, ID, BasisPoints, Quantity | Xong |
| `cmd/archcheck` — thực thi ranh giới module | Xong |
| `internal/platform` — config, logger, apierror, httpserver, database, eventbus | Xong |
| `cmd/api`, `cmd/worker` | Xong |
| 17 module nghiệp vụ | Xong |
| 27 migration | Xong |
| Giao diện: quản trị · cửa hàng · trung tâm người bán | Xong (kho riêng) |
| `internal/e2e` — luồng đi qua nhiều module | 12/12 kịch bản |
| Idempotency có ràng buộc ở database | 5/5 đường ghi |
| Event versioning — quy tắc + test tương thích | Xong |
| Vận hành: metrics (Prometheus) | Xong — `/metrics` ở API và worker |
| Vận hành: tracing, chính sách lưu trữ audit | Chưa |

**Bảy luồng nghiệm thu MVP đều chạy được**, kể cả luồng nhà bán đăng sản
phẩm và luồng đơn hàng → giao hàng.

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
  modules/        17 module nghiệp vụ, mỗi module một `public.go` duy nhất
  e2e/            luồng đi QUA NHIỀU module, bằng module thật
                  → bắt loại lỗi mà test dùng bản giả không thể thấy

api/              đặc tả OpenAPI — nguồn sự thật duy nhất về hợp đồng API
docs/             tài liệu kiến trúc (128 file)
migrations/       25 file SQL migration
```

Giao diện nằm ở **kho riêng** (`../gouse-web`), một monorepo npm workspaces
với ba ứng dụng Next.js. Tách kho có chủ ý: giao diện chỉ TRÌNH BÀY, và
ranh giới giữa hai bên là `api/openapi.yaml` chứ không phải lời gọi hàm.

| Ứng dụng | Cổng | Dành cho |
|---|---|---|
| `apps/admin` | 3000 | vận hành nội bộ |
| `apps/storefront` | 3001 | khách mua hàng |
| `apps/seller` | 3002 | nhà bán |

Cả ba đều gọi API ở cổng 8080. Ở môi trường phát triển, ba origin này nằm
sẵn trong danh sách trắng CORS — thiếu một cái thì lỗi CHỈ hiện ở console
trình duyệt, log máy chủ hoàn toàn sạch.

**Cấm tuyệt đối** các thư mục `common/`, `utils/`, `helpers/`, `services/` — chúng trở thành bãi rác phụ thuộc và phá hủy tính module một cách âm thầm. `archcheck` sẽ chặn.

---

## Ranh giới module

Bảy quy tắc được **thực thi bằng công cụ**, không dựa vào kỷ luật con người
(R6 đã bỏ; đánh số giữ nguyên để tài liệu và ADR cũ không lệch):

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

Xem [ADR-0005](../docs/adr/0005-module-boundaries.md).

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
| [docs/README.md](../docs/README.md) | Điều hướng toàn bộ tài liệu |
| [docs/00-overview/principles.md](../docs/00-overview/principles.md) | 16 nguyên tắc kiến trúc bắt buộc |
| [docs/10-roadmap/deliverables.md](../docs/10-roadmap/deliverables.md) | Tổng hợp bàn giao, rủi ro, thứ tự triển khai |
| [docs/adr/](../docs/adr/) | 13 quyết định kiến trúc kèm lý do |

---

## Yêu cầu môi trường

- Go 1.26+
- Node.js 20+ (chỉ để kiểm tra đặc tả OpenAPI)
- PostgreSQL 16+ — BẮT BUỘC: module `order`, `payment`, `checkout` và
  `inventory` từ chối chạy nếu không có, vì bất biến của chúng cần ràng
  buộc ở tầng database chứ không phải kiểm tra trước khi ghi

Biến môi trường: xem [internal/platform/config/config.go](internal/platform/config/config.go). Ở `development` mọi biến đều có giá trị mặc định hợp lý; ở `production` thiếu `DATABASE_URL` là lỗi khởi động.

| Biến | Dùng ở đâu |
|---|---|
| `DATABASE_URL` | Máy chủ API và worker |
| `TEST_DATABASE_URL` | Database KHUÔN cho test |

Hai biến này **không được trỏ cùng một database**: test dọn dữ liệu bằng
`TRUNCATE`, nên dùng chung là một lần `go test ./...` mất sạch dữ liệu phát
triển. `internal/platform/testdb` dừng hẳn test nếu phát hiện trùng.
