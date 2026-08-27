# ADR-0010: PostgreSQL + SQL viết tay cho tầng dữ liệu

**Trạng thái:** Accepted — **đã sửa đổi sau triển khai** (xem mục cuối)
**Ngày:** 12/08/2026 · **Sửa đổi:** 14/08/2026

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

**PostgreSQL + SQL viết tay, KHÔNG dùng ORM.**

```text
migrations/*.sql              → schema (golang-migrate)
infrastructure/postgres/*.go  → SQL viết tay + ánh xạ tường minh sang domain
```

Quyết định ban đầu chọn `sqlc` để sinh code Go từ SQL. Sau khi triển khai
mười module, cách làm thực tế là **pgx với SQL viết trực tiếp trong
`infrastructure/postgres/`**. Lý do và đánh giá lại nằm ở mục
"[Sửa đổi sau triển khai](#sửa-đổi-sau-triển-khai)" cuối tài liệu này.

**Phần KHÔNG đổi** — và đây mới là phần quan trọng của ADR này:

```text
✓ PostgreSQL, không phải database khác
✓ SQL thật, viết tay — không ORM, không query builder che khuất SQL
✓ Không khóa ngoại vượt ranh giới module
✓ Struct của tầng lưu trữ KHÔNG BAO GIỜ rời khỏi infrastructure/
✓ Ánh xạ tường minh sang thực thể domain
✓ Ràng buộc CHECK, chỉ mục UNIQUE có điều kiện, trigger dùng đầy đủ
```

Toàn bộ lập luận bác bỏ ORM ở phần dưới vẫn nguyên giá trị.

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

### D. sqlc — **được chọn ban đầu, sau đó thay bằng SQL viết tay**

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

### E. pgx + SQL viết trực tiếp — **đang dùng**

Giữ mọi ưu điểm của D trừ kiểm tra kiểu lúc biên dịch, bỏ được bước sinh code.

```text
Ưu:
    + Mọi ưu điểm của D về SQL thật và ranh giới module
    + Không có bước sinh code trong quy trình
    + Truy vấn động viết tự nhiên (xây WHERE theo bộ lọc)
    + Ánh xạ sang domain ở đúng một chỗ, không qua struct trung gian

Nhược:
    − Sai tên cột / sai thứ tự Scan chỉ lộ ra lúc chạy
    → bù bằng test tích hợp trên database THẬT ở mọi module
```

Xem "[Sửa đổi sau triển khai](#sửa-đổi-sau-triển-khai)" để biết vì sao
chuyển từ D sang E.

---

## Xử lý nhược điểm của SQL viết tay

> Mục này viết cho phương án sqlc. Sau khi triển khai, cách làm là SQL viết
> trực tiếp — nhưng ba nhược điểm bên dưới **vẫn đúng** vì chúng thuộc về
> việc "viết SQL tay", không thuộc về công cụ sinh code.

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

### Nhược điểm 3: không có kiểm tra kiểu lúc biên dịch

Đây là nhược điểm **thật** của cách làm hiện tại, và là thứ sqlc lẽ ra giải
được. Sai tên cột hoặc sai thứ tự `Scan` chỉ lộ ra lúc chạy.

**Cách bù đắp:** mọi module có kho lưu trữ PostgreSQL đều có test tích hợp
chạy trên **database thật**, không phải bản giả. Một cột sai làm test đỏ ở
lần chạy đầu tiên.

Đây là đánh đổi có ý thức: chấp nhận phát hiện muộn hơn vài giây, đổi lấy
việc không có bước sinh code trong quy trình. Xem
"[Sửa đổi sau triển khai](#sửa-đổi-sau-triển-khai)".

---

## Consequences

### Tích cực

```text
✓ Domain layer sạch — ánh xạ tường minh sang thực thể domain
✓ Không có khóa ngoại ORM vượt ranh giới module
✓ Sai schema lộ ra ở test tích hợp chạy trên database thật
✓ Truy vấn phân tích phức tạp viết tự nhiên
✓ Ràng buộc CHECK và RULE ở database dùng được đầy đủ
✓ Không phụ thuộc công cụ nào ngoài driver database
```

Điểm cuối quan trọng: đây là khác biệt then chốt so với ORM. Một ORM ngừng
bảo trì kéo theo toàn bộ tầng dữ liệu. Ở đây, thứ duy nhất phụ thuộc là
driver `pgx` — và SQL thì không phụ thuộc gì cả.

### Tiêu cực

```text
− Viết nhiều SQL hơn
− Không có kiểm tra kiểu lúc biên dịch (bù bằng test tích hợp thật)
− Đội cần biết SQL tốt (không phải nhược điểm với hệ thống thương mại)
```

---

## Ánh xạ sang domain — quy tắc bắt buộc

Dữ liệu đọc từ database **không phải** thực thể domain. Phải ánh xạ tường minh.

```text
infrastructure/postgres/store.go   ← rows.Scan vào biến cục bộ
        ↓ ánh xạ tường minh
domain.RestoreOrder(params)        ← thực thể domain với hành vi và bất biến
```

Quy tắc:

```text
✓ Kiểu của tầng lưu trữ CHỈ dùng trong infrastructure/
✗ KHÔNG trả struct khớp-bảng ra khỏi repository
✗ KHÔNG dùng kiểu của pgx làm tham số của use case
✓ Repository nhận và trả thực thể domain
```

**Cách cài đặt hiện tại:** `rows.Scan` đổ thẳng vào biến cục bộ, rồi gọi
`domain.Restore<Entity>(Restore<Entity>Params{...})`. Không có struct trung
gian khớp bảng nào tồn tại — nên cũng không có gì để vô tình rò rỉ ra ngoài.

Hàm `Restore*` nhận kiểu của **domain** (`money.Money`, `types.BasisPoints`),
không phải kiểu SQL. Đó là chỗ chuyển đổi thật, và nó luôn phải viết tay dù
dùng công cụ sinh code nào.

`cmd/archcheck` R2 cưỡng chế ranh giới: `domain/` không import được `infrastructure/`.

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Viết SQL cho mọi truy vấn | Kiểm soát hoàn toàn, kiểm tra kiểu lúc biên dịch |
| Không kiểm tra kiểu lúc biên dịch | Không có bước sinh code trong quy trình |
| Truy vấn động phức tạp hơn | Không có tầng trừu tượng che khuất SQL |
| Ánh xạ thủ công sang domain | Domain layer sạch, tách được service sau này |

---

## Quyết định kèm theo

```text
Database:   PostgreSQL 16+
Driver:     jackc/pgx (MIT, 14k sao, cập nhật 2026-08)
Sinh code:  KHÔNG dùng — SQL viết trực tiếp trong infrastructure/postgres/
Migration:  golang-migrate/migrate (MIT, 18k sao)
            → file SQL đánh số, KHÔNG auto-migrate
Schema:     mỗi bounded context một schema PostgreSQL
```

Lý do chọn PostgreSQL đã ghi trong [05-data/database.md](../05-data/database.md) mục 1.

---

## Điều kiện xem lại quyết định

```text
Xem lại (đưa sqlc hoặc công cụ sinh code vào) nếu:
  - Lỗi sai cột / sai kiểu lọt qua review đủ nhiều để ĐO ĐƯỢC
  - Người mới vào đội mắc lỗi ánh xạ lặp lại
  - Xuất hiện nhu cầu đa database (khó xảy ra)

KHÔNG xem lại vì:
  - "Viết SQL mệt quá"
  - "ORM nhanh hơn khi prototype"
  - "Tài liệu ban đầu nói dùng sqlc"   ← nguyên tắc P17
```

---

## Tài liệu liên quan

- [0005-module-boundaries.md](0005-module-boundaries.md) — ràng buộc khóa ngoại
- [0008-financial-ledger.md](0008-financial-ledger.md) — cần RULE ở database
- [../05-data/database.md](../05-data/database.md)
- [../11-oss/goshop.md](../11-oss/goshop.md) — phân tích vấn đề GORM
- [../11-oss/dependency-registry.md](../11-oss/dependency-registry.md)


---

## Sửa đổi sau triển khai

**Ngày:** 14/08/2026 · **Bối cảnh:** sau khi triển khai 10 module (giai đoạn 1–4)

### Điều gì đã đổi

Quyết định ban đầu: `sqlc` sinh code Go từ file `queries/*.sql`.
Thực tế triển khai: **`pgx` với SQL viết trực tiếp** trong
`internal/modules/<tên>/infrastructure/postgres/store.go`.

Không có `sqlc.yaml`, không có thư mục `queries/`, không có code sinh tự
động. Mười module — catalog, product, pricing, inventory, seller,
marketplace, payment, order, cart, checkout — đều theo cách này.

### Vì sao giả định ban đầu sai

Ba lý do, phát hiện trong lúc viết code chứ không phải lúc thiết kế:

**1. Giá trị lớn nhất của sqlc — kiểm tra kiểu lúc biên dịch — trùng lặp
với thứ đã có.** Ràng buộc thật của hệ thống này nằm ở `CHECK`, chỉ mục
`UNIQUE` có điều kiện, và trigger, chứ không ở kiểu cột. Một `sai tên cột`
bị bắt ngay ở test tích hợp đầu tiên vì mọi module đều có test chạy trên
PostgreSQL thật. sqlc bắt sớm hơn vài giây, không sớm hơn vài ngày.

**2. Bước sinh code tạo ra một tầng gián tiếp không trả đủ giá.** Truy vấn
của hệ thống này gắn chặt với ánh xạ sang domain: `RestoreOrderParams`
nhận 19 trường, trong đó có `Money` và `BasisPoints` là kiểu của kernel,
không phải kiểu SQL. Dù dùng sqlc thì vẫn phải viết tay toàn bộ hàm ánh xạ
đó — sqlc chỉ tiết kiệm phần `rows.Scan`, phần rẻ nhất.

**3. Truy vấn động nhiều hơn dự kiến.** `ListBySeller` trong module order
xây mệnh đề `WHERE` theo bộ lọc trạng thái; `findOne` dùng chung một câu
`SELECT` với nhiều mệnh đề `WHERE` khác nhau. ADR gốc đã lường trước điểm
này ("nhược điểm 2") và đề xuất viết tay ở `infrastructure/` — thực tế thì
gần như mọi kho lưu trữ đều rơi vào trường hợp đó.

### Vì sao KHÔNG viết lại theo sqlc

Áp dụng nguyên tắc P17: code đang chạy, có test tích hợp trên database
thật, và giữ đúng mọi ràng buộc mà ADR này quan tâm. Viết lại 15 file kho
lưu trữ để khớp một câu trong tài liệu là chi phí thật đổi lấy sự nhất
quán hình thức.

### Điều gì được giữ nguyên và được cưỡng chế

Điều quan trọng của ADR này chưa bao giờ là công cụ, mà là **ranh giới**:

| Ràng buộc | Cách cưỡng chế hiện tại |
|---|---|
| Domain layer không import hạ tầng | `cmd/archcheck` R2 — chạy trong CI |
| Struct lưu trữ không rời `infrastructure/` | Không có struct trung gian: `rows.Scan` đổ thẳng vào biến cục bộ rồi gọi `domain.Restore*` |
| Không khóa ngoại vượt module | Rà tay khi review migration; mọi `REFERENCES` đều trong cùng module |
| SQL thật, không ORM | Không có phụ thuộc ORM nào trong `go.mod` |

### Khi nào xét lại

Đưa sqlc vào nếu **một trong hai** điều sau xảy ra:

```text
1. Số lỗi "sai tên cột / sai kiểu" lọt qua review đủ nhiều để đo được
2. Có người mới vào đội và mắc lỗi ánh xạ lặp lại
```

Cả hai đều là tín hiệu đo được, không phải cảm tính. Chưa xảy ra thì không đổi.
