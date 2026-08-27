# GoShop

| | |
|---|---|
| Repository | `github.com/quangdangfit/goshop` |
| License | MIT |
| Sao / Fork | 395 / 89 |
| Cập nhật cuối | 2026-05-22 (đang hoạt động) |
| Vai trò | Tham chiếu **cấu trúc ứng dụng Go thực dụng** |

---

## 1. GoShop là gì

Ứng dụng thương mại điện tử Go quy mô vừa, có cấu trúc rõ ràng. Giá trị với chúng ta không phải mô hình domain (khá đơn giản) mà là **cách tổ chức một dự án Go production**.

```text
internal/{domain}/
├── model/       thực thể GORM
├── dto/         request/response
├── repository/  truy cập dữ liệu
├── service/     logic nghiệp vụ
└── port/
    ├── http/    REST (Gin)
    └── grpc/    gRPC
```

Chạy song song hai server: REST (8888) và gRPC (8889).

Công nghệ: Gin, GORM, PostgreSQL, Redis, JWT, golang-migrate, testify + mockery, testcontainers, Swagger.

---

## Năng lực: Cấu trúc thư mục theo domain

### Cách OSS làm

Mỗi domain có đủ 5 tầng trong một thư mục. Domain: user, product, order, payment, notification.

### Điểm mạnh

- Code liên quan nằm gần nhau — dễ tìm
- Ranh giới domain hiện rõ trong cây thư mục
- Cấu trúc nhất quán giữa các domain

### Điểm yếu

**Không có gì cưỡng chế ranh giới.** Package `order/service` có thể import thẳng `product/repository` mà không ai chặn.

Đây chính là vấn đề mà [ADR-0005](../adr/0005-module-boundaries.md) nêu: kỷ luật con người không đủ.

### Yêu cầu của chúng ta

Ranh giới module phải được **cưỡng chế bằng công cụ**, vi phạm làm CI thất bại.

### Adopt

**Cấu trúc thư mục theo domain, các tầng nhất quán.** Chúng ta đã dùng cấu trúc tương tự nhưng đổi tên tầng cho đúng DDD:

```text
GoShop:      model / dto / repository / service / port
Chúng ta:    domain / application / infrastructure / interfaces + public.go
```

Khác biệt quan trọng: chúng ta có **`public.go`** — điểm vào duy nhất của module. GoShop không có khái niệm này.

### Adapt

Thêm hai thứ GoShop thiếu:

```text
1. public.go — interface công khai, điểm vào duy nhất
2. cmd/archcheck — cưỡng chế ranh giới trong CI
```

### Quyết định cuối

```text
✓ Cấu trúc theo domain, tầng nhất quán — đã áp dụng
✓ Bổ sung public.go và archcheck — đã làm
```

---

## Năng lực: Migration có phiên bản

### Cách OSS làm

Dùng `golang-migrate` với file SQL đánh số, **không dùng auto-migrate của GORM**.

### Điểm mạnh

Đây là quyết định đúng và đáng lưu ý. Auto-migrate của ORM có ba vấn đề:

```text
✗ Không tái lập được — chạy hai lần có thể ra hai kết quả
✗ Không kiểm soát được thứ tự và cách thực hiện
✗ Không viết được migration ba bước (mở rộng → di trú → thu hẹp)
```

Migration có phiên bản cho phép: rà soát trong pull request, chạy giống hệt ở mọi môi trường, quay lui có kiểm soát.

### Yêu cầu của chúng ta

[05-data/data-model.md](../05-data/data-model.md) mục 14 yêu cầu migration tương thích ngược và quy trình ba bước cho thay đổi phá vỡ.

### Adopt

**Migration SQL có phiên bản.** Chúng ta dùng cùng công cụ (`golang-migrate`) hoặc tương đương.

Với sqlc (xem [ADR-0010](../adr/0010-database-layer.md)), viết SQL trực tiếp là mặc định — không có auto-migrate để mà cám dỗ.

### Quyết định cuối

```text
✓ golang-migrate với file SQL đánh số trong /migrations
✗ Không bao giờ dùng auto-migrate
```

---

## Năng lực: Kiểm thử với testcontainers

### Cách OSS làm

Unit test dùng mock (mockery); integration test dùng **testcontainers** khởi động PostgreSQL và Redis thật trong Docker.

### Điểm mạnh

Integration test chạy trên database **thật**, bắt được lỗi mà mock bỏ sót:

```text
Ràng buộc CHECK có hoạt động không?
Chỉ mục duy nhất có chặn đúng không?
Giao dịch có cô lập đúng không?
Truy vấn có chạy đúng trên PostgreSQL thật không?
```

Với chúng ta, điều này đặc biệt quan trọng vì nhiều bất biến được cưỡng chế **ở tầng database**:

```sql
CHECK (quantity_available >= 0)
CREATE RULE ledger_entry_no_update ... DO INSTEAD NOTHING
CREATE UNIQUE INDEX ... WHERE status = 'ACTIVE'
```

Mock không kiểm chứng được những thứ này.

### Điểm yếu

Chậm hơn unit test đáng kể; cần Docker trong môi trường CI và máy phát triển.

### Yêu cầu của chúng ta

Máy phát triển hiện tại **chưa có Docker**. Nhưng CI (GitHub Actions) có.

### Adopt

**Ba tầng kiểm thử:**

```text
1. Unit test domain      — không phụ thuộc gì, chạy trong bộ nhớ, rất nhanh
2. Test application      — repository in-memory (mẫu từ Flamingo)
3. Integration test      — testcontainers + PostgreSQL thật
```

Tầng 3 chạy trong CI và trên máy có Docker; đánh dấu bằng build tag để bỏ qua khi không có Docker:

```go
//go:build integration
```

### Quyết định cuối

```text
✓ testcontainers cho integration test
✓ Build tag để chạy được cả khi không có Docker
✓ Bắt buộc integration test cho: ràng buộc CHECK, RULE bất biến ledger,
  chỉ mục duy nhất, khóa lạc quan
```

---

## Năng lực: REST + gRPC song song

### Cách OSS làm

Hai server chạy đồng thời, cùng gọi vào tầng service.

### Điểm mạnh

Chứng minh tầng service **độc lập với giao thức** — đây là kiểm chứng tốt cho kiến trúc.

### Điểm yếu

Duy trì hai bề mặt API tốn công gấp đôi: hai định nghĩa schema, hai bộ test, hai tài liệu.

GoShop cũng không nhất quán — chỉ 3/5 domain có gRPC.

### Yêu cầu của chúng ta

Sáu loại client: storefront, seller center, creator center, admin, app di động, đối tác. Tất cả đều dùng được REST + JSON.

### Reject

**Không làm gRPC ở giai đoạn này.**

```text
Lý do (nguyên tắc P15 — phải giải thích được vì sao cần cho nghiệp vụ này):
  ✗ Không client nào của chúng ta cần gRPC
  ✗ Trình duyệt không gọi gRPC trực tiếp
  ✗ Thêm bề mặt API thứ hai = gấp đôi công duy trì
  ✓ REST + OpenAPI đã sinh được kiểu TypeScript cho frontend
```

Cân nhắc lại **chỉ khi** tách service và cần giao tiếp nội bộ hiệu năng cao — nhưng đó là bài toán của Phase 3+.

### Adopt

**Ý tưởng tầng application độc lập giao thức.** Đây là điều archcheck quy tắc R8 cưỡng chế: `interfaces` gọi `application`, không ngược lại.

### Quyết định cuối

```text
✓ Tầng application độc lập giao thức (đã có)
✗ Không gRPC ở MVP/Phase 2
```

---

## Năng lực: GORM

### Cách OSS làm

GORM cho toàn bộ truy cập dữ liệu. Model là struct có tag GORM.

### Điểm yếu với kiến trúc của chúng ta

**Model GORM trở thành model domain.** Đây là vi phạm ranh giới nghiêm trọng:

```go
// Kiểu GoShop — domain biết về database
type Order struct {
    gorm.Model
    UserID  uint   `gorm:"index"`
    Items   []Item `gorm:"foreignKey:OrderID"`
}
```

Struct này vừa là thực thể domain vừa là ánh xạ bảng. Hệ quả:

```text
✗ Domain layer phụ thuộc thư viện database → vi phạm quy tắc R2 của archcheck
✗ Không test được domain mà không có GORM
✗ Đổi schema database làm đổi domain model
✗ Khóa ngoại GORM tạo quan hệ vượt ranh giới module
```

Điểm cuối đặc biệt nguy hiểm: `foreignKey` giữa các module phá vỡ chính thứ mà [ADR-0005](../adr/0005-module-boundaries.md) bảo vệ.

### Reject

**Không dùng GORM.** Quyết định đầy đủ và các phương án đã cân nhắc: [ADR-0010](../adr/0010-database-layer.md).

---

## 2. Tổng kết GoShop

| Hạng mục | Quyết định |
|---|---|
| Cấu trúc thư mục theo domain | **ADOPT** — đã áp dụng |
| Migration SQL có phiên bản | **ADOPT** |
| Không dùng auto-migrate | **ADOPT** |
| testcontainers cho integration test | **ADOPT** |
| Tầng application độc lập giao thức | **ADOPT** |
| Repository sau interface | **ADOPT** |
| GORM | **REJECT** — xem ADR-0010 |
| gRPC song song REST | **REJECT** ở giai đoạn này |
| Giỏ hàng lưu ở trình duyệt | **REJECT** — chúng ta cần giỏ ở server để quy kết creator |
| Không có event/bất đồng bộ | **REJECT** — chúng ta cần outbox từ đầu |

**Nhận xét cuối:** GoShop là tham chiếu tốt về **cách tổ chức dự án Go**, không phải về mô hình domain thương mại. Ba thứ đáng lấy nhất: cấu trúc thư mục, migration có phiên bản, và testcontainers.

---

## 3. Tài liệu liên quan

- [../adr/0010-database-layer.md](../adr/0010-database-layer.md)
- [../adr/0005-module-boundaries.md](../adr/0005-module-boundaries.md)
- [../09-operations/deployment.md](../09-operations/deployment.md)
