# ADR-0010: PostgreSQL + sqlc cho tầng dữ liệu

**Trạng thái:** Accepted
**Ngày:** 12/08/2026

---

## Context

Cần chọn cách truy cập dữ liệu cho toàn bộ hệ thống. Đây là quyết định khó đảo ngược: đổi tầng dữ liệu sau khi có 20 module là dự án viết lại.

### Ràng buộc kiến trúc đã có

Ba quyết định trước đó **giới hạn** lựa chọn:

```text
ADR-0005 (ranh giới module)
    → KHÔNG khóa ngoại vượt ranh giới module
    → mỗi bảng thuộc đúng một module

archcheck R2 (domain layer sạch)
    → domain/ chỉ import thư viện chuẩn và kernel/
    → nếu xóa infrastructure/, domain vẫn phải biên dịch được

ADR-0008 (ledger bất biến)
    → cần RULE ở tầng database chặn UPDATE/DELETE
    → cần ràng buộc CHECK cho bất biến
```

### Đặc điểm truy vấn của hệ thống

Không phải CRUD đơn thuần. Ví dụ thật từ [06-api/admin-api.md](../06-api/admin-api.md):

```text
Đề xuất bổ sung hàng cần:
  tồn kho hiện tại theo SKU
  + tốc độ bán 30 ngày
  + tín hiệu nhu cầu (hết hàng, tìm không ra kết quả, đăng ký nhận tin)
  + MOQ và lead time của nhà cung cấp
  + số tuần còn lại của mùa
  → tính reorder point, phát hiện mâu thuẫn MOQ/dự báo
```

Truy vấn kiểu này cần SQL thật: cửa sổ, tổng hợp có điều kiện, CTE.

---

## Decision

**PostgreSQL + sqlc.** SQL viết tay, sinh code Go có kiểu từ SQL.

```text
migrations/*.sql   → schema (golang-migrate)
queries/*.sql      → truy vấn viết tay
    ↓ sqlc generate
infrastructure/db/ → code Go có kiểu, sinh tự động
```

---

## Alternatives

### A. GORM — **bị loại**

```text
Ưu:
    + Hệ sinh thái Go lớn nhất, nhiều người biết
    + CRUD nhanh, ít code lặp
    + Có migration tự động, hook, soft delete sẵn

Nhược (QUYẾT ĐỊNH):
    − Model GORM TRỞ THÀNH model domain
    − Khóa ngoại GORM tạo quan hệ vượt ranh giới module
    − Truy vấn phức tạp phải rơi về SQL thô — mất hết lợi ích
    − Kiểu yếu: tên trường là chuỗi, lỗi lộ ra lúc chạy
    − Vấn đề N+1 ẩn trong Preload
```

**Lý do loại chính — mâu thuẫn kiến trúc trực tiếp:**

```go
// Kiểu GORM
type Order struct {
    gorm.Model
    UserID uint   `gorm:"index"`
    Items  []Item `gorm:"foreignKey:OrderID"`
}
```

Struct này vừa là thực thể domain vừa là ánh xạ bảng. Hệ quả:

```text
✗ domain/ phải import gorm → VI PHẠM archcheck R2
✗ foreignKey giữa các module → VI PHẠM ADR-0005
✗ không test được domain mà không có GORM
```

Đây không phải sở thích. `cmd/archcheck` sẽ **làm CI thất bại** nếu `domain/` import GORM.

Có thể tách model GORM khỏi model domain và ánh xạ thủ công — nhưng khi đó ta mất gần hết tiện lợi của GORM mà vẫn giữ chi phí của nó.

### B. ent — **bị loại**

```text
Ưu:
    + Kiểu tĩnh mạnh, sinh code
    + Xử lý quan hệ phức tạp tốt
    + Schema tường minh

Nhược:
    − Schema định nghĩa bằng DSL Go riêng → schema thật nằm ở công cụ,
      không phải ở SQL
    − Entity sinh ra có xu hướng trở thành model domain (như GORM)
    − Khó viết truy vấn phân tích phức tạp
    − Migration do ent quản lý → khó viết migration ba bước
      (mở rộng → di trú → thu hẹp) mà ADR về triển khai yêu cầu
```

ent tốt hơn GORM về kiểu, nhưng vẫn đặt một tầng trừu tượng giữa ta và SQL — chính thứ ta cần kiểm soát.

### C. Raw SQL với `database/sql` — **bị loại một phần**

```text
Ưu:
    + Kiểm soát hoàn toàn
    + Không phụ thuộc

Nhược:
    − Quét kết quả thủ công, rất nhiều code lặp
    − Không kiểm tra được kiểu lúc biên dịch: sai tên cột lộ ra lúc chạy
    − Đổi schema không làm code báo lỗi
```

Đây chính là vấn đề mà sqlc giải: giữ SQL thật, thêm kiểm tra kiểu.

### D. sqlc — **được chọn**

```text
Ưu:
    + SQL là SQL — không học ngôn ngữ truy vấn mới
    + Kiểm tra kiểu LÚC BIÊN DỊCH: sqlc phân tích schema, sai cột → lỗi build
    + Code sinh ra là Go thuần, không phụ thuộc runtime
    + Truy vấn phức tạp viết tự nhiên
    + Không có khóa ngoại ORM → không vô tình vượt ranh giới module
    + Model sinh ra là struct thuần → ánh xạ sang domain rõ ràng

Nhược:
    − Phải viết SQL cho mọi truy vấn, kể cả CRUD đơn giản
    − Thêm bước sinh code vào quy trình
    − Truy vấn động (bộ lọc tùy chọn) khó hơn query builder
```

---

## Xử lý nhược điểm của sqlc

### Nhược điểm 1: nhiều SQL cho CRUD đơn giản

Chấp nhận. Nguyên tắc P16: thà rõ ràng còn hơn ngắn gọn.

Với codebase nhiều người, nhiều năm, SQL tường minh dễ rà soát hơn truy vấn sinh tự động. Khi có vấn đề hiệu năng, biết chính xác truy vấn nào chạy.

### Nhược điểm 2: truy vấn động

Đây là hạn chế thật. Ví dụ: danh sách sản phẩm có 8 bộ lọc tùy chọn.

Hai cách xử lý:

```sql
-- Cách 1: điều kiện có thể bỏ qua (dùng cho số ít bộ lọc)
WHERE (@category_id::uuid IS NULL OR category_id = @category_id)
  AND (@brand_id::uuid   IS NULL OR brand_id   = @brand_id)
```

```text
-- Cách 2: với truy vấn thật sự động (tìm kiếm nâng cao)
   → xây SQL bằng tay ở infrastructure/, có kiểm thử riêng
   → hoặc chuyển sang chỉ mục tìm kiếm (Phase 2)
```

Cách 1 đủ cho đa số. Cách 2 dùng khi số tổ hợp bộ lọc quá lớn.

### Nhược điểm 3: bước sinh code

Thêm vào Makefile và CI:

```makefile
sqlc-gen:   ## Sinh code từ SQL
	sqlc generate

sqlc-check: ## Kiểm tra code sinh ra đã cập nhật (CI)
	sqlc diff
```

CI thất bại nếu code sinh ra không khớp SQL — giống cách chúng ta kiểm tra đặc tả OpenAPI.

---

## Consequences

### Tích cực

```text
✓ Domain layer sạch — model sinh ra là struct thuần, ánh xạ tường minh
✓ Không có khóa ngoại ORM vượt ranh giới module
✓ Sai schema lộ ra lúc BIÊN DỊCH, không phải lúc chạy
✓ Truy vấn phân tích phức tạp viết tự nhiên
✓ Ràng buộc CHECK và RULE ở database dùng được đầy đủ
✓ Nếu sqlc ngừng phát triển, code đã sinh vẫn chạy
```

Điểm cuối quan trọng: đây là khác biệt then chốt so với ORM. Một ORM ngừng bảo trì kéo theo toàn bộ tầng dữ liệu; sqlc chỉ là công cụ sinh code, sản phẩm của nó là Go thuần.

### Tiêu cực

```text
− Viết nhiều SQL hơn
− Thêm bước sinh code
− Truy vấn động cần xử lý riêng
− Đội cần biết SQL tốt (không phải nhược điểm với hệ thống thương mại)
```

---

## Ánh xạ sang domain — quy tắc bắt buộc

sqlc sinh struct từ bảng. Những struct này **không phải** thực thể domain.

```text
infrastructure/db/models.go     ← sqlc sinh: struct khớp bảng
        ↓ ánh xạ tường minh
domain/order.go                 ← thực thể domain với hành vi và bất biến
```

Quy tắc:

```text
✓ Struct sqlc CHỈ dùng trong infrastructure/
✗ KHÔNG trả struct sqlc ra khỏi repository
✗ KHÔNG dùng struct sqlc làm tham số của use case
✓ Repository nhận và trả thực thể domain
```

`cmd/archcheck` R2 cưỡng chế điều này: `domain/` không import được `infrastructure/`.

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Viết SQL cho mọi truy vấn | Kiểm soát hoàn toàn, kiểm tra kiểu lúc biên dịch |
| Thêm bước sinh code | Sai schema lộ ra khi build |
| Truy vấn động phức tạp hơn | Không có tầng trừu tượng che khuất SQL |
| Ánh xạ thủ công sang domain | Domain layer sạch, tách được service sau này |

---

## Quyết định kèm theo

```text
Database:   PostgreSQL 16+
Driver:     jackc/pgx (MIT, 14k sao, cập nhật 2026-08)
Sinh code:  sqlc-dev/sqlc (MIT, 18k sao, cập nhật 2026-08)
Migration:  golang-migrate/migrate (MIT, 18k sao)
            → file SQL đánh số, KHÔNG auto-migrate
Schema:     mỗi bounded context một schema PostgreSQL
```

Lý do chọn PostgreSQL đã ghi trong [05-data/database.md](../05-data/database.md) mục 1.

---

## Điều kiện xem lại quyết định

```text
Xem lại nếu:
  - Truy vấn động chiếm > 30% tổng số truy vấn
  - sqlc ngừng phát triển > 18 tháng
  - Xuất hiện nhu cầu đa database (khó xảy ra)

KHÔNG xem lại vì:
  - "Viết SQL mệt quá"
  - "ORM nhanh hơn khi prototype"
```

---

## Tài liệu liên quan

- [0005-module-boundaries.md](0005-module-boundaries.md) — ràng buộc khóa ngoại
- [0008-financial-ledger.md](0008-financial-ledger.md) — cần RULE ở database
- [../05-data/database.md](../05-data/database.md)
- [../11-oss/goshop.md](../11-oss/goshop.md) — phân tích vấn đề GORM
- [../11-oss/dependency-registry.md](../11-oss/dependency-registry.md)
