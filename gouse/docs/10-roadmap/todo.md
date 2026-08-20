# Tiến độ triển khai — việc ĐÃ LÀM

> **Đây KHÔNG phải backlog.** Việc còn phải làm nằm ở
> **[backlog.md](backlog.md)** — backlog duy nhất của dự án.
>
> File này là **hồ sơ hồi cứu**: đã làm những gì, và kiểm chứng bằng cách
> nào. Giá trị của nó nằm ở phần bằng chứng — mỗi bất biến đều được xác
> nhận bằng cách phá code sản xuất rồi chạy lại test.

**Cập nhật:** 15/08/2026 · **Giai đoạn:** Implementation Completion

**17/17 module MVP đã có logic nghiệp vụ.** Việc còn lại lớn nhất là
**tầng HTTP**: 20/71 endpoint đã cài đặt, 7/17 module có tầng HTTP.

Ký hiệu: `[x]` xong và đã kiểm chứng · `[~]` đang làm · `[ ]` chưa bắt đầu

---

## 1. Tình hình hiện tại

Đo ngày **20/08/2026**:

```text
Tài liệu     128 file · 37.107 dòng                  ✓ đồng bộ với code
Đặc tả API   12 file YAML · 75 thao tác · 0 lỗi lint
Code Go      282 file · 84.117 dòng · 740 hàm test
Migration    25 file SQL · đảo được
Giao diện    3 app Next.js (kho riêng) · 5 test trình duyệt · 10 test đơn vị

Module MVP có logic:   17/17
Module có tầng HTTP:   12/17
Thao tác có route:     51/75   (24 còn lại: 20 thuộc Phase 2/3)

Bảy luồng nghiệm thu MVP:  7/7 chạy được
```

**Giai đoạn 1–5 xong. Giai đoạn 6 xong một phần. Giai đoạn 7 xong.**
Phase hiện tại là **Production Hardening** — mục 12.

Kiểm chứng lần cuối (13/08/2026):

```text
✓ gofmt        không có file cần định dạng lại
✓ go vet       không có cảnh báo
✓ archcheck    OK — 191 file, không vi phạm ranh giới
✓ go test      toàn bộ package pass, CÓ database thật
✓ chạy thật    api chạy đủ 10 module trên PostgreSQL; worker chạy 3 job
               (phát event 5s, dọn giữ hàng 30s, dọn phiên 60s)
✓ bền vững     dữ liệu sống qua khởi động lại, seed tự bỏ qua lần 2
✓ migration    15/15 áp dụng được, đảo được
✓ tranh chấp   20 khách mua 1 sản phẩm → ĐÚNG 1 người thắng,
               19 xung đột phiên bản được phát hiện và từ chối
✓ cách ly      seller A không đọc/ghi được đơn của seller B dù biết id
✓ idempotency  10 request song song cùng khóa → ĐÚNG 1 đơn hàng
✓ đóng băng    đối soát ra cùng con số sau khi giá và chính sách đổi
✓ giỏ hàng     10 tab cùng mở → ĐÚNG 1 giỏ; giá theo giá hiện tại;
               món hết hàng được đánh dấu chứ không bị xóa
✓ tranh chấp   10 khách checkout, kho còn 5 → ĐÚNG 5 người giữ được
✓ giá đóng băng seller đổi giá giữa chừng → đơn vẫn ghi giá khách đã thấy
✓ nhả hàng     hết hạn và hủy phiên đều trả hàng về kho, đếm khớp
✓ outbox       giao dịch rollback → event KHÔNG phát; phát lại → bên
               nhận xử lý ĐÚNG 1 lần; event hỏng → dead letter sau 5 lần
✓ commit kho   đặt hàng xong → 7/3/0 thành 7/0/3 qua event thật
✓ tín hiệu     thêm giỏ và đặt hàng sinh demand_signal qua event;
               phát lại KHÔNG ghi hai lần
✓ tách đơn     3 nguồn hàng → 3 đơn thực hiện; seller không thấy đơn
               của seller khác dù biết định danh
✓ email        10 request song song → ĐÚNG 1 email; thiếu địa chỉ thì
               ghi SKIPPED chứ không báo lỗi
```

**Cách kiểm chứng:** mọi mục trong bảng trên đều được xác nhận bằng cách
**phá code sản xuất rồi chạy lại test** — nếu test vẫn xanh sau khi bất
biến bị phá, nó không kiểm chứng được gì. Xem mục 5 để biết đã phá những gì.

---

## 2. Giai đoạn 1 — Nền tảng ✓

> Mục 14 deliverables: *platform · kernel · identity · kiểm tra ranh giới trong CI*

### 2.1 Kiểm tra ranh giới module — **LÀM NGAY** ✓

Tài liệu yêu cầu làm việc này **trước module đầu tiên**; đã tuân thủ.

- [x] `cmd/archcheck` — 7 quy tắc R1–R8 (893 dòng)
- [x] R1 chỉ import `public.go` của module khác
- [x] R2 tầng domain sạch (không import hạ tầng)
- [x] R3 platform trung lập với domain
- [x] R4 kernel tối thiểu
- [x] R5 không phụ thuộc vòng
- [x] R7 cấm thư mục `common/` `utils/` `helpers/` `services/`
- [x] R8 hướng phụ thuộc giữa các tầng
- [x] Test tích hợp tạo vi phạm THẬT trong thư mục tạm để xác nhận công cụ bắt được
- [x] Quét cả file `_test.go`, không miễn trừ — đã bắt vi phạm thật trong test của chính mình (12/08)

### 2.2 Kernel ✓

- [x] `kernel/money` — số nguyên, `Allocate()` chia không mất đồng nào (631 dòng)
- [x] `kernel/ids` — ULID có tiền tố loại, khớp mẫu regex trong OpenAPI (410 dòng)
- [x] `kernel/types` — kiểu dùng chung, `BasisPoints` (190 dòng)

### 2.3 Platform ✓

- [x] `platform/config` — kiểm tra cấu hình, trả TẤT CẢ lỗi cùng lúc (393 dòng)
- [x] `platform/logger` — che dữ liệu nhạy cảm: mật khẩu, token, thẻ, **số đo cơ thể** (304 dòng)
- [x] `platform/apierror` — khớp `common.yaml#/schemas/Error`, không rò rỉ chi tiết nội bộ (516 dòng)
- [x] `platform/httpserver` — middleware, health check tách `live`/`ready`, tắt êm (796 dòng)
- [x] `cmd/api` · `cmd/worker` — hai điểm khởi chạy, chung codebase

### 2.4 Chưa làm ở giai đoạn 1

- [x] `platform/database` — **đã cài đặt** (pool pgx, health check, ping lúc khởi động)
- [x] `platform/eventbus` — **đã cài đặt** (outbox, dispatcher, event_processed)
- [x] `identity` — **đã cài đặt** (đăng ký · đăng nhập · phiên thu hồi được ·
      vai trò + phạm vi). Chưa có: xác thực hai lớp, đăng nhập mạng xã hội,
      đặt lại mật khẩu qua email — đều SAU MVP.

### 2.5 CI ✓

`.github/workflows/ci.yml` — 5 job:

- [x] `architecture` — chạy **RIÊNG và ĐẦU TIÊN**, vi phạm ranh giới làm CI thất bại
- [x] Test chính công cụ archcheck (công cụ bỏ sót vi phạm nguy hiểm hơn không có công cụ)
- [x] `quality` — gofmt · go vet · `go mod tidy` không tạo thay đổi
- [x] `test` — kèm **race detector** (cần cho khóa lạc quan của `inventory`) + báo cáo độ phủ
- [x] `build` — kiểm chứng tiến trình **thật sự khởi động** và trả lời health check, không chỉ biên dịch
- [x] `api-spec` — lint đặc tả + sinh kiểu TypeScript để xác nhận đặc tả dùng được

---

## 3. Giai đoạn 2 — Danh mục ✓

> Thứ tự: `catalog` → `product` → `pricing`

### 3.1 Module `catalog` ✓

Đối chiếu [../04-modules/catalog.md](../04-modules/catalog.md).

**Domain** (1.598 dòng, 9 file)

- [x] `Brand` — 3 mức bảo vệ `OPEN` / `VERIFIED_ONLY` / `RESTRICTED`
- [x] `BrandAuthorization` — link table có hiệu lực theo thời gian, `ExpiresWithin()`
- [x] `Collection` — máy trạng thái + lịch ra mắt, `WeeksRemaining()` cho quyết định bổ sung hàng
- [x] `Category` — cây phân cấp
- [x] `SizeChart` — số đo THỰC TẾ theo `(brand, product_type)`, không chỉ ký hiệu S/M/L
- [x] Port repository thiết kế theo LÔ (`FindByIDs` nhận danh sách) để tránh N+1

**Application** (988 dòng)

- [x] `CanSellerSellBrand()` — quy tắc **chống hàng giả**, trả cả lý do và hành động cần làm
- [x] `ProcessScheduledCollections()` — ra mắt/kết thúc mùa theo lịch, idempotent
- [x] `Clock` tiêm được để test kiểm soát thời gian
- [x] `NewInMemoryService()` — hàm dựng cho test, đặt ở `application` để giữ đúng hướng phụ thuộc (R8)

**Infrastructure** (485 dòng)

- [x] Kho in-memory — lưu bản sao, không lưu con trỏ (giống database thật)
- [x] Kho PostgreSQL — **đã cài đặt**, cùng port với in-memory

**Giao diện công khai** (820 dòng)

- [x] `public.go` — interface `API`, chỉ trả DTO, không trả domain object
- [x] `module.go` — chuyển đổi domain → DTO, `var _ API = (*Module)(nil)` bảo đảm lúc biên dịch
- [x] `RegisterRoutes()` — module tự đăng ký route; `cmd/api` **không** cầm được `*application.Service`
- [x] `seed.go` — dữ liệu mẫu cho kho in-memory, trả về ID vừa tạo

**HTTP** (561 dòng) — khớp `api/paths/storefront.yaml`

- [x] `GET /api/v1/brands/{brand_id}`
- [x] `GET /api/v1/collections/{collection_id}`
- [x] `GET /api/v1/categories`

**Kiểm chứng**

- [x] Chạy server THẬT, không chỉ biên dịch — cây danh mục 2 cấp, brand 200, collection có `launch_date` dạng `YYYY-MM-DD`
- [x] Bộ sưu tập chưa ra mắt **không lộ** qua endpoint công khai
- [x] Bộ sưu tập chưa ra mắt trả **404 chứ không phải 403** (403 xác nhận tài nguyên tồn tại)
- [x] Hai quy tắc trên đã kiểm chứng ngược bằng cách **phá code**: cả hai test đều fail đúng chỗ
- [x] `MODULES_STORAGE=memory` bị **chặn ở production** (mất dữ liệu khi khởi động lại)

### 3.2 Module `product` ✓

Đối chiếu [../04-modules/product.md](../04-modules/product.md).

**Domain** (1.823 dòng)

- [x] `Product` — aggregate root, máy trạng thái 5 trạng thái (mục 6)
- [x] `Variant` — tổ hợp thuộc tính, ảnh riêng theo màu
- [x] `SKU` — **định danh hàng hóa chung**, không thuộc seller nào (nền tảng của mô hình Offer)
- [x] Ba trường đặc thù thời trang bắt buộc trước khi duyệt: chất liệu, bảng size, mô tả
- [x] Túi và phụ kiện **không** bắt buộc bảng size — bắt buộc mọi loại sẽ chặn hàng hợp lệ
- [x] `ARCHIVED` là trạng thái cuối, không có đường quay lại

**Application** (942 dòng)

- [x] `CatalogPort` — interface do **bên gọi** định nghĩa, chỉ 3 năng lực product thực dùng
- [x] Kiểm tra quyền bán **trước khi tạo** và **kiểm tra lại trước khi xuất bản**
- [x] Tự tra bảng size theo (thương hiệu, loại sản phẩm) nếu không chỉ định
- [x] `ListSellerProducts` trả lỗi khi thiếu `sellerID` — không âm thầm trả toàn sàn

**Infrastructure** (804 dòng)

- [x] Kho in-memory, bản chụp đi **sâu** qua Variant và SKU
- [x] Mô phỏng ràng buộc UNIQUE: slug và mã SKU toàn hệ thống
- [x] Dọn ánh xạ SKU cũ khi cập nhật — không thì mã bị chiếm vĩnh viễn
- [x] Kho PostgreSQL — **đã cài đặt**, cùng port với in-memory

**Giao diện công khai** (623 dòng)

- [x] `public.go` — interface `API` khớp đặc tả mục 9, chỉ trả DTO
- [x] `catalogAdapter` — chỗ **duy nhất** trong product biết tới catalog
- [x] `seed.go` — nạp cả sản phẩm đã duyệt lẫn hàng nháp để kiểm chứng được

**HTTP** (762 dòng)

- [x] `GET /api/v1/products/{product_id}`
- [x] `GET /api/v1/products` — lọc theo brand, category, **collection**

**Kiểm chứng**

- [x] Chạy server THẬT: trả đủ 3 biến thể, mỗi biến thể đúng size của mình
- [x] Sản phẩm chưa duyệt trả **404 chứ không phải 403**, thông báo lỗi không lộ trạng thái
- [x] `?status=DRAFT` **không** lộ được hàng chưa duyệt
- [x] Ba quy tắc trên đã kiểm chứng ngược bằng cách **phá code**: test fail đúng chỗ
- [x] Không trả trường của module chưa có (`price_from`, `available`, `buy_box_offer`)

**Ghi chú về `collection.products`:** đặc tả khai báo trường này trong
`GET /api/v1/collections/{id}`, nhưng **catalog không được gọi product** —
[../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md)
mục 3 quy định chiều phụ thuộc là `product → catalog`. Gọi ngược sẽ tạo phụ
thuộc vòng. Thay vào đó dùng `GET /api/v1/products?collection_id=...` —
đã cài đặt và kiểm chứng.

### 3.3 Module `pricing` ✓

Đối chiếu [../04-modules/pricing.md](../04-modules/pricing.md). Phạm vi MVP
theo mục 10 của đặc tả: **giá cơ bản, giá gạch ngang, khung giá**.

**Domain** (1.696 dòng)

- [x] `Price` — 5 loại giá, thứ tự ưu tiên `Flash > Campaign > Clearance > Member > Base`
- [x] `SelectBest` — quy tắc 2: **chỉ một** giá được áp dụng, không cộng dồn
- [x] Cùng loại thì chọn giá **thấp hơn** — cấu hình trùng do lỗi vận hành thì khách được lợi
- [x] Giá flash/chiến dịch **bắt buộc có thời hạn** — quên tắt là bán lỗ vô hạn
- [x] `PriceConstraint` — khung giá chống bán phá giá **và** chống lỗi nhập liệu
- [x] `PricePoint` — lịch sử **bất biến**, không có phương thức sửa/xóa
- [x] `LowestIn` — "giá thấp nhất 30 ngày qua", con số bắt buộc công bố ở một số thị trường
- [x] Mức giảm tính theo **phần vạn**, không dùng số thực

**Application** (781 dòng)

- [x] `SetPrice` ghi lịch sử **trong** use case — để bên gọi tự nhớ thì sẽ có chỗ quên
- [x] Ghi lịch sử **trước**, lưu giá **sau** — vết thừa dễ đối chiếu hơn thiếu vết
- [x] `GetPrices` theo lô, cho **cùng kết quả** với tra từng cái
- [x] SKU chưa có khung giá: chấp nhận giá dương (chặn hết sẽ khiến không ai đăng bán được)

**Infrastructure** (598 dòng)

- [x] Ba kho: giá, khung giá, lịch sử
- [x] Kho trả **mọi** mức giá — việc chọn là quyết định nghiệp vụ của domain
- [x] `HistoryStore` **chỉ có `Append`** — không có Update/Delete
- [x] Kho PostgreSQL — **đã cài đặt**, cùng port với in-memory

**Giao diện công khai** (825 dòng)

- [x] `public.go` khớp đặc tả mục 7, dùng `Amount{Value, Currency}` thay vì số thực
- [x] Đơn vị tiền tệ không hợp lệ **bị chặn**, không âm thầm coi là hợp lệ

**Kiểm chứng**

- [x] Chạy server thật: giá gắn đúng `sku_id` có thật từ module product
- [x] Ba quy tắc kiểm chứng ngược bằng cách **phá code**: bỏ kiểm tra hạng thành viên,
      bỏ sắp xếp, bỏ lưu ngưỡng cảnh báo — cả ba test đều fail đúng chỗ

**Chưa làm — đúng phạm vi giai đoạn sau:**

| Hạng mục | Giai đoạn | Ghi chú |
|---|---|---|
| `Adjustment` + `cost_bearer` | 4 (`order`) | Gắn với `OrderLine`, không phải bảng giá — xem [../02-domain/entities.md](../02-domain/entities.md) mục 2.10 |
| Giá theo chiến dịch, giá thành viên | Phase 2 | Domain đã hỗ trợ, chưa có module `campaign`/`loyalty` để cấu hình |
| Quy tắc giảm giá theo mùa | Phase 3 | Cần `inventory` để biết sell-through |
| Định giá động theo nhu cầu | Phase 4 | Nguyên tắc P14: MVP dùng quy tắc theo lịch |

**Ghi chú:** module pricing **không có endpoint HTTP riêng** ở MVP. Giá hiển
thị qua trang sản phẩm; khi có `marketplace`, giá đi kèm `Offer`. Tạo endpoint
`/api/v1/prices` lúc này sẽ là API không ai gọi.

---

## 3.4 Tầng dữ liệu PostgreSQL ✓

Theo [../adr/0010-database-layer.md](../adr/0010-database-layer.md).

**Kết nối** — `internal/platform/database`

- [x] Pool `pgx`, giới hạn kết nối, tuổi thọ kết nối (cần khi có failover)
- [x] **Ping lúc khởi động**: pgxpool tạo kết nối lười, không ping thì tiến trình
      khởi động "thành công" rồi đổ vỡ ở request đầu tiên của khách
- [x] Database chỉ nằm ở health check `ready`, **không** ở `live` — sự cố ngắn
      không được khiến bộ điều phối khởi động lại toàn bộ tiến trình
- [x] DSN chứa mật khẩu nên **không** bọc vào thông báo lỗi trả ra ngoài

**Migration** — 6 file, `golang-migrate`

- [x] Ba migration: catalog · product · pricing
- [x] **Đảo được**: `down -all` rồi `up` lại thành công
- [x] Lệnh `make migrate-up` · `migrate-reset` · `migrate-new`

**Ràng buộc THẬT — điều in-memory chỉ mô phỏng được**

Đã kiểm chứng từng cái bằng cách cố tình ghi dữ liệu sai:

| Ràng buộc | Kết quả |
|---|---|
| `sku_code` UNIQUE **toàn hệ thống** (quy tắc 1) | chặn, kể cả giữa hai sản phẩm khác nhau |
| Không trùng tổ hợp thuộc tính trong một Product (quy tắc 2) | chặn |
| Một ủy quyền `APPROVED` cho mỗi (brand, seller) | chặn; bản ghi `PENDING`/`REJECTED` vẫn thêm được |
| Một bảng size cho mỗi (brand, product_type) | chặn |
| Sản phẩm `ACTIVE` phải có mô tả + chất liệu (quy tắc 3, 4) | chặn |
| Giá ≤ 0, giá gạch ngang thấp hơn giá bán | chặn |
| Giá `FLASH`/`CAMPAIGN` không có hạn kết thúc | chặn |
| Khung giá `min > max`, khung giá rỗng | chặn |
| Định danh sai tiền tố, trạng thái ngoài enum | chặn |

**Lịch sử giá BẤT BIẾN** — `price_history`

- [x] Trigger **từ chối thi hành** `UPDATE` và `DELETE`, kể cả bằng tài khoản quản trị
- [x] Báo lỗi tường minh thay vì im lặng bỏ qua — im lặng khiến người sửa
      tưởng đã thành công và đi tiếp với giả định sai
- [x] `INSERT` vẫn hoạt động bình thường
- [x] Điểm lịch sử **thiếu lý do bị chặn** — rà soát thao túng giá cần biết
      VÌ SAO mỗi lần đổi, không chỉ biết giá mới

Đây là điều kho in-memory **không thể** chứng minh: nó chỉ "không cung cấp
phương thức sửa", còn database "không cho phép hành động sửa".

**Sai lệch có chủ đích so với [../05-data/data-model.md](../05-data/data-model.md) mục 12:**
định danh lưu `TEXT` chứ không phải `UUID`, vì định danh của hệ thống có
**tiền tố loại** (`brd_`, `sku_`). Tiền tố là thứ ngăn truyền nhầm `brand_id`
vào chỗ cần `category_id` — lỗi mà `UUID` thuần không bắt được, và đặc tả
OpenAPI cũng đã công bố định dạng này ra ngoài. Đánh đổi: tốn 30 byte thay
vì 16 mỗi định danh.

**Lỗi phát hiện được nhờ dựng schema:** `BrandAuthorization` đang dùng lại
tiền tố `brd_` của thương hiệu — id ủy quyền và id thương hiệu không phân
biệt được, đúng thứ mà tiền tố sinh ra để ngăn. Đã thêm `aut_`.

**CI** — job `test` giờ có PostgreSQL thật

- [x] `services: postgres:18` + chạy migration trước khi test
- [x] Kiểm tra migration đảo được ngay trong CI
- [x] Không có `DATABASE_URL` thì test tầng kho lưu trữ **tự bỏ qua** —
      nên CI **bắt buộc** phải có, nếu không bộ test báo xanh mà không
      kiểm chứng gì

---

## 4. Giai đoạn 3 — Tồn kho và bán hàng ✓

### 4.1 Module `inventory` ✓

Đối chiếu [../04-modules/inventory.md](../04-modules/inventory.md).

**Domain** (1.376 dòng)

- [x] `Quantities` — value object BẤT BIẾN giữ bất biến sáu trạng thái
- [x] Mọi chuyển đổi đi qua `move()` nên tổng được bảo toàn theo **cấu trúc**,
      không phải nhờ người viết nhớ kiểm tra
- [x] Chỉ `Receive` và `Ship` làm đổi tổng — vì hàng thật sự vào/ra khỏi kho
- [x] `InventoryItem` — khóa nghiệp vụ `(sku, địa điểm, chủ sở hữu)`
- [x] Tách `owner` khỏi `location`: hàng seller gửi kho nền tảng **vẫn thuộc
      sở hữu seller**, không được ghi nhận là tài sản nền tảng
- [x] `Reservation` — TTL bắt buộc, giới hạn 3 lần gia hạn
- [x] `InventoryMovement` — nhật ký bất biến, số lượng luôn dương
- [x] **Test theo tính chất**: 200 chuỗi 40 thao tác ngẫu nhiên, bất biến
      giữ vững sau mỗi bước

**Infrastructure** (928 dòng) — **CHỈ PostgreSQL, không có bản in-memory**

- [x] `ApplyChange` — `UPDATE ... WHERE id = ? AND version = ?`
- [x] 0 dòng bị ảnh hưởng → `ErrVersionConflict`, phân biệt rõ với hết hàng
- [x] `UnitOfWork` — đổi số lượng và ghi nhật ký trong **một giao dịch**
- [x] Ràng buộc `CHECK >= 0` ở tầng database — lớp bảo vệ cuối cùng
- [x] Trigger chặn `UPDATE`/`DELETE` trên nhật ký biến động

**Application** (558 dòng)

- [x] Thử lại **đúng loại lỗi**: xung đột phiên bản → thử lại tối đa 3 lần
      với khoảng chờ **ngẫu nhiên**; hết hàng → trả lỗi ngay
- [x] Chờ ngẫu nhiên tránh "bầy đàn đồng bộ" — cùng chờ một khoảng rồi
      cùng thử lại thì lại xung đột tiếp
- [x] `ExpireReservations` dọn **từng cái** trong giao dịch riêng: một bản
      ghi hỏng không được làm kẹt toàn bộ cơ chế
- [x] `CountExpiredPending` — chỉ báo giám sát, cảnh báo khi > 100

**KIỂM CHỨNG TRANH CHẤP THẬT** — điều in-memory không thể chứng minh

| Kịch bản | Kết quả |
|---|---|
| 20 khách mua 1 sản phẩm | **đúng 1 thắng**, 19 xung đột bị từ chối |
| 30 khách tranh 10 sản phẩm | **đúng 10 thắng**, `available+reserved = 10` |
| Vô hiệu khóa lạc quan | **20 người mua 1 món**, **30 người mua 10 món** |

Dòng cuối là bằng chứng test có giá trị: bỏ điều kiện `version` khỏi `WHERE`
thì hệ thống bán quá số lượng ngay lập tức, và cả hai test đều fail.

**Không có kho in-memory** là quyết định có chủ đích: khóa lạc quan cần hai
giao dịch database thật chạy song song trên cùng một dòng. Một bản in-memory
sẽ tạo **cảm giác an toàn giả** — test xanh mà không chứng minh được điều
quan trọng nhất. Module từ chối khởi tạo nếu không có PostgreSQL.

**Chưa làm — đúng phạm vi giai đoạn sau** (mục 14 của đặc tả):

| Hạng mục | Giai đoạn |
|---|---|
| Nhiều địa điểm, chuyển kho | Phase 2 |
| Truy vết theo lô sản xuất | Phase 3 |
| Chia ô tồn kho cho tranh chấp cực cao | Phase 4 |
| Tiến trình nền tự động dọn reservation | cần `cmd/worker` |

**Lưu ý:** `ExpireReservations` đã có và đã kiểm chứng, nhưng **chưa có
tiến trình nền gọi nó định kỳ**. Hiện phải gọi thủ công. Đây là việc của
`cmd/worker` — nếu quên, hàng sẽ bị khóa dần và cuối cùng không bán được gì.

### 4.2 `cmd/worker` — tiến trình nền ✓

Khoảng trống đã nêu ở đợt trước, nay đã đóng.

- [x] Job dọn giữ hàng quá hạn, nhịp 30 giây theo khuyến nghị mục 6.3
- [x] Chạy NGAY một lượt lúc khởi động, không chờ hết nhịp đầu — worker vừa
      khởi động lại sau sự cố có thể đang có tồn đọng
- [x] Xử lý theo lô (200 bản ghi) để tồn đọng lớn không tạo giao dịch khổng lồ
- [x] Lỗi một lượt **ghi log chứ không làm chết tiến trình** — lượt sau có
      thể thành công
- [x] Cảnh báo khi tồn đọng > 100 (mục 13)
- [x] Tắt êm: chờ job hoàn tất lượt đang chạy, không cắt ngang giao dịch
- [x] Kiểm chứng đầu-cuối: 3 khách bỏ checkout → khả dụng 10→4 → worker
      chạy → **quay lại 10**, tồn đọng 0

### 4.3 Module `seller` ✓

Đối chiếu [../04-modules/seller.md](../04-modules/seller.md). (1.505 dòng)

- [x] Vòng đời 8 trạng thái, `TERMINATED` là trạng thái **cuối**
- [x] **Quy tắc 1**: ACTIVE phải có tài khoản ngân hàng đã xác minh —
      cưỡng chế ở cả domain lẫn `CHECK` của database
- [x] **Own brand là seller INTERNAL**, không phải đường đi riêng: đơn lẫn
      own brand và hàng seller đi CHUNG một luồng
- [x] Own brand hoạt động ngay, không cần duyệt, **không chịu hoa hồng**
      (nền tảng không thu của chính mình)
- [x] Đình chỉ/từ chối/chấm dứt **bắt buộc nêu lý do** — ràng buộc ở database
- [x] Đình chỉ làm **ẩn offer** nhưng KHÔNG hủy đơn đang xử lý
- [x] `EnsureInternalSeller` idempotent

**Không lưu số dư** (quy tắc 4): muốn biết seller còn bao nhiêu tiền → gọi
`payment.GetBalance()`. Hai nơi cùng lưu một sự thật thì sớm muộn chúng
lệch nhau, và khi lệch thì không biết bên nào đúng.

### 4.4 Module `marketplace` ✓ — **Offer, 1 trong 4 thứ phải làm đúng ngay**

Đối chiếu [../04-modules/marketplace.md](../04-modules/marketplace.md). (2.560 dòng)

**Offer** — đơn vị khách THỰC SỰ mua

- [x] **Quy tắc 1**: một seller chỉ MỘT offer ACTIVE cho một SKU —
      `UNIQUE INDEX ... WHERE status = 'ACTIVE'` ở database
- [x] Offer **KHÔNG lưu số lượng tồn kho**: nguồn sự thật là `InventoryItem`,
      `OUT_OF_STOCK` là dữ liệu dẫn xuất
- [x] **Quy tắc 5**: lịch sử giá bất biến, trigger chặn UPDATE/DELETE

**Chống hàng giả** (mục 5) — rủi ro SỐNG CÒN của marketplace thời trang

- [x] Kiểm tra quyền bán thương hiệu **trước khi tạo** offer
- [x] **Kiểm tra lại trước khi đưa lên bán**: giữa hai thời điểm có thể đã
      nhiều ngày và giấy ủy quyền có thể đã hết hạn
- [x] Kiểm chứng ngược bằng cách **bỏ qua kết quả kiểm tra** → cả hai test fail

**Buy box** — công thức CÔNG KHAI (mục 4)

- [x] Trọng số **giá 40% · giao hàng 30% · hiệu suất 30%**
- [x] Ràng buộc bắt buộc: offer ACTIVE, seller ACTIVE, **còn hàng**
- [x] Trả kèm **điểm số** để seller hiểu vì sao mình không thắng
- [x] Kết quả **ổn định** giữa các lần gọi
- [x] Buy box theo lô khớp với tra từng cái

**Một điều chỉnh nhờ test bắt được:** trọng số ban đầu là 50/25/25. Test
phát hiện với giá chiếm một nửa, một offer rẻ hơn **10%** thắng được offer
kém nhất ở CẢ HAI tiêu chí còn lại (52 điểm so với 50) — đúng cuộc đua giảm
giá mà đặc tả cảnh báo. Đã đổi sang **40/30/30**: chất lượng phục vụ (60%)
đủ sức thắng khoảng chênh giá nhỏ, nhưng giá vẫn là yếu tố đơn lẻ nặng nhất.

**Hoa hồng** — phân vai rõ ràng (quy tắc 8)

```text
marketplace → ĐỊNH NGHĨA quy tắc
order       → ĐÓNG BĂNG vào OrderLine
payment     → GHI SỔ vào ledger
```

`GetCommissionRate` chỉ trả **tỷ lệ**, không tính số tiền — nếu tính luôn,
sẽ có hai nơi cùng tính một con số.

---

## 5. Giai đoạn 4 — Giao dịch `[x]`

- [x] `cart` — **không giữ tồn kho**, giá cập nhật động
- [x] `checkout` — **giữ hàng**, **đóng băng giá**
- [x] `demand_signal` — ghi tín hiệu nhu cầu từ MVP (thứ 4 trong nhóm "sửa sau là viết lại")
- [x] `order` — đóng băng dữ liệu, **tách Order/FulfillmentOrder**
- [x] `payment` — **sổ cái bất biến**, trigger chặn UPDATE/DELETE ở database
- [x] Bất biến Σ DEBIT = Σ CREDIT kiểm tra trong constructor

**Hai thứ khó nhất của giai đoạn 4 đã xong trước** — có chủ ý: cả hai đều
thuộc nhóm "sửa sau là viết lại" ở mục 9. `cart` và `checkout` xây trên
chúng, không ngược lại.

### `cart` — ranh giới với `order` là toàn bộ lý do nó tồn tại riêng

|  | `cart` | `order` |
|---|---|---|
| Bản chất | Ý ĐỊNH mua | HỢP ĐỒNG |
| Giá | Cập nhật động | ĐÓNG BĂNG |
| Giữ tồn kho | **KHÔNG** | (checkout giữ) |
| Thời gian sống | 30 ngày | vĩnh viễn |
| Món không hợp lệ | Đánh dấu, khách tự xóa | Không áp dụng |

Hai bảng `cart_item` và `order_line` trông gần giống nhau nhưng ý nghĩa
ngược nhau: sửa giá trong `order_line` là làm sai hóa đơn cũ, còn sửa giá
trong `cart_item` là hành vi ĐÚNG và diễn ra thường xuyên.

**Vì sao giỏ không giữ tồn kho:** khách thêm rồi bỏ quên hai tuần thì hàng
khóa hai tuần. Với hàng khan hiếm, vài trăm giỏ bỏ quên = hết hàng ảo,
không bán được cho khách thật sự muốn mua. Hệ quả phải chấp nhận là khách
có thể tới lúc checkout mới biết hết hàng — đánh đổi đúng, vì số lượng
hiển thị ở giỏ là **thông tin tham khảo**, không phải cam kết.

Kiểm chứng ngược, bốn phép phá:

| Phá gì | Test bắt được |
|---|---|
| `Sync` tự giảm số lượng về mức tồn kho | Hệ thống quyết thay khách: chọn 10 thành 3 |
| `Sync` không cập nhật giá (đóng băng như order) | Giỏ hiện 299.000 sau khi seller giảm còn 249.000 |
| Gộp giỏ cắt số lượng mà không cảnh báo | Khách đăng nhập xong thấy ít hàng hơn, không biết vì sao |
| Bỏ UNIQUE có điều kiện trên `cart` | 10 tab → **3 giỏ ACTIVE** cho cùng một khách |

**Một lỗ hổng trong bộ test đã được sửa nhờ kiểm chứng ngược.** Phép phá
thứ hai ban đầu KHÔNG bị bắt: `GetCart` tự đồng bộ trước khi trả, nên giá
mới luôn xuất hiện kể cả khi nó không hề được ghi xuống database. Hai lần
gọi `GetCart` không phân biệt được "đã lưu" với "tính lại mỗi lần đọc".

Hậu quả thật nếu để lọt: khách luôn thấy giá đúng, nhưng mọi thứ đọc thẳng
bảng `cart_item` — job nhắc giỏ bỏ quên, phân tích, tín hiệu nhu cầu — sẽ
thấy giá cũ mãi mãi. Test đã sửa để truy vấn trực tiếp thay vì đi qua
service.

### `order` — đã kiểm chứng những gì

Bốn bất biến quan trọng nhất đều **kiểm chứng ngược**: cố tình phá code sản
xuất để xác nhận test thật sự bắt được, không phải pass vì may.

| Bất biến | Cách phá | Test bắt được |
|---|---|---|
| Cách ly seller | Bỏ lọc `seller_id` trong SQL | Lớp domain `BelongsTo` vẫn chặn — **hai lớp hoạt động độc lập**; phá cả hai thì test báo rò rỉ thật |
| Idempotency | Bỏ nhánh xử lý `ErrDuplicateOrder` | 9/10 request song song đụng UNIQUE — **kiểm tra trước khi ghi không chặn được gì** |
| Đóng băng | Tính hoa hồng lúc đọc theo tỷ lệ mới | Đối soát ra 35.880đ thay vì 29.900đ — đúng kịch bản đặc tả cảnh báo |
| Trạng thái tổng hợp | Xét "tất cả đã xuất" trước "một số đã giao" | Đơn báo SHIPPED khi một gói đã giao — hiển thị **lùi** so với thực tế |

Điều đáng ghi lại nhất là hàng thứ hai: dưới 10 request song song cùng khóa,
**chín cái đi tới tận ràng buộc UNIQUE của database**. Kiểm tra khóa ở tầng
ứng dụng chỉ tiết kiệm một lần ghi trong trường hợp thường, nó không phải
cơ chế bảo vệ. Nếu chỉ có nó, khách bấm hai lần sẽ thành hai đơn.

### `checkout` — nơi giá chuyển từ động sang tĩnh

Ba bảng đọc liền nhau thì thấy toàn bộ thiết kế của luồng mua hàng:

| Bảng | Giá | Khóa hàng | Sống |
|---|---|---|---|
| `cart_item` | ĐỘNG | không | 30 ngày |
| `checkout_line` | **ĐÓNG BĂNG** | **CÓ** | 15 phút |
| `order_line` | ĐÓNG BĂNG | (đã chuyển sang đơn) | vĩnh viễn |

Cột `reservation_id` là thứ `cart_item` không có và `order_line` không cần:
nó chỉ tồn tại trong 15 phút giữa lúc khách bấm "Thanh toán" và lúc đơn
được tạo. Đó là toàn bộ lý do checkout là aggregate riêng.

**`checkout` gần như không sở hữu luật nghiệp vụ nào** — nó gọi bốn module
và làm đúng hai việc không module nào khác làm: giữ tồn kho và đóng băng
giá. Việc khó nhất không phải logic mà là **xử lý thất bại giữa chừng**.

Kiểm chứng ngược, năm phép phá:

| Phá gì | Test bắt được |
|---|---|
| Không nhả hàng khi thất bại giữa chừng | 5 sản phẩm bị khóa 15 phút cho một phiên **chưa từng tồn tại** |
| `ExpireStale` không nhả hàng | Hàng khóa vĩnh viễn cho phiên đã chết — không tiến trình nào tìm nó nữa |
| Bỏ lớp idempotency thứ nhất | Không tạo đơn thừa (lớp 2 chặn) nhưng khách nhận **lỗi cho một đơn đã đặt thành công** |
| `mutable` chỉ nhìn trạng thái, bỏ kiểm tra đồng hồ | Phiên đã quá hạn vẫn thanh toán được, dù hàng có thể đã bị nhả |
| Bỏ giới hạn gia hạn | Khóa hàng vô hạn — đúng thứ việc tách checkout khỏi giỏ sinh ra để tránh |

Phép thứ ba đáng ghi lại: hai lớp idempotency phục vụ **hai mục đích khác
nhau**. Lớp 2 (ràng buộc UNIQUE ở `order`) ngăn đơn trùng; lớp 1 (kiểm tra
trạng thái phiên) khiến lần gọi thứ hai trả về **đơn cũ** thay vì lỗi. Bỏ
lớp 1 thì dữ liệu vẫn đúng nhưng khách thấy thông báo thất bại cho giao
dịch đã thành công.

### Một data race có thật, phát hiện nhờ test tranh chấp

Test "10 khách tranh 5 sản phẩm" làm lộ một lỗi **có sẵn từ giai đoạn 3**
trong `inventory`: `withRetry` dùng chung một `*rand.Rand` không khóa để
sinh khoảng chờ giữa các lần thử lại. `math/rand.Rand` giữ trạng thái nội
bộ và không an toàn khi nhiều goroutine gọi cùng lúc.

Đây đúng là hàm bị gọi song song nhiều nhất hệ thống — mọi tranh chấp tồn
kho đều đi qua nó — nhưng không test nào trước đó gọi `Reserve` song song
qua nhiều `Service`, nên `-race` chưa từng thấy. Đã chuyển sang bộ sinh
toàn cục của `math/rand/v2`.

### Vì sao `order` và `payment` từ chối chạy khi không có PostgreSQL

Cùng một lý do ở hai chỗ khác nhau:

- `payment` cần **trigger** chặn UPDATE/DELETE trên sổ cái
- `order` cần **UNIQUE** trên khóa idempotency và **SEQUENCE** cho mã đơn

Cả hai đều là thứ chỉ database cưỡng chế được dưới tải song song. Bản
in-memory chỉ "không cung cấp phương thức sửa" và "kiểm tra trước khi ghi" —
với tiền và đơn hàng, khác biệt đó quá lớn để chấp nhận.

---

## 6. Giai đoạn 5 — Thực hiện đơn `[x]`

- [x] `fulfillment` — tách đơn theo nguồn hàng, vòng đời giao hàng đầy đủ
- [x] `notification` — email giao dịch, chống gửi trùng

### `notification` — ràng buộc quan trọng nhất là thứ nó KHÔNG làm

```text
Module này KHÔNG GỌI bất kỳ module nghiệp vụ nào.
```

Nếu nó gọi `order` để lấy tên sản phẩm và `customer` để lấy email, nó phụ
thuộc toàn hệ thống — và một module lỗi sẽ làm hỏng việc gửi **mọi loại**
thông báo, kể cả mã OTP.

**Hệ quả buộc phải nhận:** payload event phải mang đủ dữ liệu. Đã bổ sung:

| Event | Thêm gì | Vì sao |
|---|---|---|
| `checkout.completed` | `guest_email`, `product_name` | notification cần để soạn email |
| `fulfillment.progress_changed` | `email`, `tracking_number`, `fo_number` | như trên |
| `fulfillment_order` (bảng) | `notify_email`, `customer_id` | fulfillment không biết email nếu không lưu |

Cột `notify_email` là **nhân bản có chủ ý**, cùng loại với việc đóng băng
giá ở `order_line`. Ba lựa chọn và lý do chọn cái thứ ba:

```text
1. notification gọi order       → vi phạm quy tắc 1
2. fulfillment gọi order        → phụ thuộc VÒNG (order đã nghe event từ đây)
3. Lưu email lúc tách đơn       → nhân bản, nhưng KHÔNG phụ thuộc gì  ← CHỌN
```

### SKIPPED khác FAILED

```text
SKIPPED  quyết định CÓ CHỦ Ý không gửi (thiếu địa chỉ, chưa có đồng ý)
FAILED   sự cố thật, cần xem và thử lại
```

Gộp chung sẽ làm cảnh báo vận hành kêu vì những việc hoàn toàn bình
thường — và rồi không ai đọc cảnh báo nữa. Cả hai đều **được ghi log**:
khách hỏi "sao tôi không nhận được email" thì phải trả lời được.

### Bộ gửi ghi-log ở MVP

Nền tảng chưa ký hợp đồng với nhà cung cấp dịch vụ email, nhưng luồng phải
chạy được đầu-cuối ngay — nếu không, việc soạn nội dung, chống gửi trùng và
ghi nhật ký sẽ không được kiểm chứng cho tới tận lúc tích hợp thật.

Đổi sang nhà cung cấp thật: viết một package cài `domain.Sender`, đổi một
dòng ở nơi khởi tạo. Module không đổi gì (nguyên tắc P13).

Kiểm chứng ngược, ba phép phá:

| Phá gì | Test bắt được |
|---|---|
| Gửi trước, ghi log sau | **11 email cho 10 request song song** — thứ tự "ghi trước" là thứ chặn |
| Thiếu địa chỉ thì trả lỗi | Event bị thử lại vô ích rồi rơi vào dead letter |
| Gửi email cho mọi bước tiến độ | 4 email cho các bước nội bộ của seller — làm phiền khách |

Phép đầu là kết quả rõ nhất: chỉ mục UNIQUE chỉ chặn được nếu bản ghi vào
**trước** khi gửi.

### Một ranh giới đặt sai đã được sửa

Giai đoạn 4 cài `fulfillment_order` **trong module `order`**, vì lúc đó
module `fulfillment` chưa tồn tại. Nhưng
[module-boundaries.md](../03-architecture/module-boundaries.md) mục 3 và
[ADR-0007](../adr/0007-marketplace-order-model.md) đều ghi rõ nó thuộc
`fulfillment`.

```text
order        = HỢP ĐỒNG với khách   — "khách mua gì, giá bao nhiêu"
fulfillment  = ĐƠN VỊ CÔNG VIỆC     — "ai giao, đến đâu rồi"
```

**Đã chuyển sang đúng chỗ.** Hệ quả nếu để nguyên: khi tách service, module
`order` sẽ mang theo dữ liệu vận hành của seller — đúng thứ ADR-0007 tách
ra để tránh.

Việc chuyển kéo theo hai thay đổi cắt phụ thuộc ngược:

| Trước | Sau |
|---|---|
| `SplitIntoFulfillmentOrders(*Order)` | `SplitIntoFulfillmentOrders(SplitInput)` — dữ liệu thuần |
| `order.RecalculateStatus([]*FulfillmentOrder)` | `RecalculateStatus([]FulfillmentProgress)` — dữ liệu thuần |

Nhờ vậy `order` không import `fulfillment` và ngược lại. Chiều thông tin từ
fulfillment về order đi bằng **event**, một chiều.

### Vòng đời giao hàng đầy đủ

```text
PENDING → ALLOCATED → CONFIRMED → PICKING → PACKED
        → HANDED_OVER → IN_TRANSIT → DELIVERED → COMPLETED

IN_TRANSIT → DELIVERY_FAILED → IN_TRANSIT  (giao lại)
                             → CANCELLED   (trả về người gửi)
```

**`COMPLETED` có ý nghĩa TÀI CHÍNH, không phải bước vận hành:**

```text
DELIVERED  → số dư seller vẫn Pending
COMPLETED  → số dư chuyển Available, seller được chi trả
```

Đây là cơ chế bảo vệ nền tảng khỏi rủi ro hoàn hàng sau khi đã trả tiền.
Worker có job chuyển trạng thái này sau 7 ngày.

Kiểm chứng ngược, ba phép phá:

| Phá gì | Test bắt được |
|---|---|
| Bỏ lọc `seller_id` + bỏ `BelongsTo` | Seller A đọc và thao tác được đơn của seller B |
| Không phát event tiến độ | Hàng đã giao mà đơn vẫn báo `PENDING_PAYMENT` |
| Bỏ kiểm tra idempotency khi tách đơn | **Không bắt được** — chỉ mục `UNIQUE (order_id, seller_id)` vẫn chặn. Đã bổ sung test riêng cho lớp thứ hai này |

Phép thứ ba là kết quả đáng ghi: hai lớp bảo vệ hoạt động độc lập, và test
ban đầu không phân biệt được lớp nào đang làm việc.

### Một lỗi mất dữ liệu âm thầm

`withLineIDs` dựng lại thực thể qua `RestoreFOParams`. Khi thêm bảy trường
mới cho giai đoạn 5, tôi quên liệt kê chúng ở đó — nên mã vận đơn và mốc
hoàn tất bị **xóa trắng khi đọc lại**, dù đã ghi đúng xuống database.

Lỗi này không lộ ra lúc ghi, chỉ lộ ở test đọc-lại-sau-khi-ghi. Đây là rủi
ro cố hữu của mẫu "dựng lại từ đầu": trường nào quên là trường đó biến mất
im lặng.

---

## 7. Giai đoạn 6 — Marketplace hoàn chỉnh `[~]`

- [x] **Nhà bán tự quản offer** — tạo, đổi giá, ngừng bán
      (`listMyOffers` · `createOffer` · `updateOffer`)
- [x] **Nhà bán tự kiểm kê** (`updateInventory`, con số tuyệt đối)
- [x] **Nhà bán tự thực hiện đơn** — xem việc, bàn giao vận chuyển
      (`listMyFulfillmentOrders` · `getMyFulfillmentOrder` · `shipFulfillmentOrder`)
- [x] **Cô lập giữa các nhà bán** — phòng vệ hai lớp: `WHERE seller_id`
      ở SQL và `BelongsTo` ở domain
- [x] **Trung tâm người bán** (giao diện, cổng 3002)
- [ ] Đăng ký seller (`applyAsSeller` — đặc tả có, chưa có route)
- [ ] Đối soát (`getMyBalance` · `getMySettlement`)
- [ ] Chi trả (`executePayouts`)
- [ ] Điểm hiệu suất nhà bán (`getMyPerformance`)

Bốn việc còn lại đều thuộc nhóm TIỀN và điểm số — chúng không chặn luồng
bán hàng, và được hoãn lại sau Production Hardening theo nguyên tắc không
mở rộng miền trong phase này.

---

## 8. Giai đoạn 7 — Hoàn thiện MVP ✓

- [x] `promotion` — làm sớm trước giai đoạn 6
- [x] `analytics` cơ bản — đã nối vào bus event
- [x] **`demand_signal`** — xong ở giai đoạn 5, kiểm chứng end-to-end
- [x] `customer` — hồ sơ, địa chỉ, wishlist, đồng ý
      (preference/dữ liệu size là Phase 2 theo customer.md mục 11)
- [x] `identity` — **cả module lẫn tầng HTTP**: đăng nhập, làm mới,
      đăng xuất, `/admin/me`
- [x] **Đăng ký và đăng nhập cho KHÁCH** — mở khóa 6 endpoint tài khoản
- [x] **Gộp giỏ khi đăng nhập** (`POST /api/v1/cart/merge`)

**Tầng HTTP đã hết là chỗ nghẽn.** Mô tả cũ của mục này ("chỉ 5 endpoint
nghiệp vụ, 15 module chưa có tầng interfaces") là tình hình đầu tháng 8;
nay 12/17 module có tầng HTTP và 51/75 thao tác có route.

Năm module không có tầng HTTP — `analytics`, `notification`, `pricing`,
`promotion`, `supplychain` — là CỐ Ý: chúng phục vụ module khác qua Go và
qua event, không có người dùng gọi thẳng.

---

## 9. Bốn thứ phải làm đúng ngay ở MVP

> [mvp.md](mvp.md) mục 2 — *sửa sau là viết lại*

| # | Hạng mục | Trạng thái | Ghi chú |
|---|---|---|---|
| 1 | Mô hình `Offer` | `[x]` | **Xong** — giai đoạn 3, kèm chống hàng giả và buy box |
| 2 | Tách `Order`/`FulfillmentOrder` | `[x]` | **Xong** — giai đoạn 4, kiểm chứng ngược cách ly seller |
| 3 | Sổ cái bất biến | `[x]` | **Xong** — giai đoạn 4, trigger chặn UPDATE/DELETE |
| 4 | Ghi `demand_signal` | `[x]` | **Xong** — `ADD_TO_CART` và `ORDER` qua event; ba loại "nhu cầu chưa đáp ứng" có mô hình, chờ nguồn phát |

**4/4 đã xong.** Bốn quyết định "sửa sau là viết lại" đều đã cài và kiểm
chứng ngược.

Riêng `demand_signal` đáng ghi lại: nó là thứ **duy nhất** trong nhóm mà
làm muộn thì không có gì để viết lại — ba cái kia sai thì sửa code, cái này
thiếu thì dữ liệu mất vĩnh viễn. Bắt đầu ghi từ MVP nghĩa là tới Phase 3 đã
có lịch sử thật để dự báo.

---

## 10. Tiêu chí hoàn thành MVP

Đối chiếu [mvp.md](mvp.md) mục 7.

### Kiến trúc

| Tiêu chí | Trạng thái |
|---|---|
| Không có phụ thuộc vòng giữa module | `[x]` archcheck R5 |
| Không có thư mục `common/` `utils/` `helpers/` `services/` | `[x]` archcheck R7 |
| Kiểm tra ranh giới module trong CI đều xanh | `[x]` job `architecture` chạy đầu tiên, thất bại = chặn merge |
| Không có JOIN vượt ranh giới module | `[x]` không bảng nào có `REFERENCES` vượt module — `order_line.offer_id` chỉ giữ định danh |
| Mọi lệnh ghi API đều idempotent | `[~]` header BẮT BUỘC ở mọi đường ghi, nhưng chỉ `order` và `payment` có ràng buộc `UNIQUE` ở database — chi tiết ở [backlog mục 2.7](backlog.md) |
| Outbox hoạt động, không có event kẹt | `[x]` `platform/eventbus`: rollback thì KHÔNG phát, phát lại xử lý đúng một lần, event hỏng vào dead letter sau 5 lần |

### Chức năng · Chất lượng

| Tiêu chí | Trạng thái |
|---|---|
| Bảy luồng nghiệm thu MVP chạy được | `[x]` cả 7 |
| Luồng mua hàng chạy trên giao diện thật | `[x]` cửa hàng + trung tâm người bán, 5 test trình duyệt |
| E2E xuyên module trên PostgreSQL | `[~]` 8/12 kịch bản — xem [backlog mục 2.5](backlog.md) |
| Không oversell dưới thanh toán đồng thời | `[x]` chứng minh ở TOÀN CHUỖI: 10 khách tranh 3 món qua `StartCheckout` thật → đúng 3 người thắng |
| API p95 < 300ms · LCP < 2,5s | `[ ]` cần môi trường có tải thật — [backlog mục 2.12](backlog.md) |

**Hai dòng `[~]` là điều kiện chặn của phase Production Hardening**, không
phải việc "làm sau nếu còn thời gian".

---

## 11. Việc tiếp theo

> **Việc tiếp theo nay là PRODUCTION HARDENING — xem mục 12.**
>
> Mục này giữ lại lịch sử từng giai đoạn vì nó ghi cách kiểm chứng của
> mỗi giai đoạn, và cách kiểm chứng là thứ dùng lại được.

**Giai đoạn 4 đã xong.** Luồng mua hàng chạy đầu-cuối trên PostgreSQL thật:

```text
cart.AddItem → checkout.StartCheckout → inventory.Reserve
             → checkout.CompleteCheckout → order.PlaceOrder
             → cart.MarkConverted
```

**Giai đoạn 5 đã xong.** Luồng chạy đầu-cuối từ giỏ hàng tới email xác nhận:

```text
cart → checkout → inventory.Reserve → order.PlaceOrder
                → checkout.completed (event)
                     ├──→ inventory     : Reserved → Committed
                     ├──→ fulfillment   : tách đơn theo nguồn hàng
                     ├──→ supply-chain  : ghi tín hiệu ORDER
                     └──→ notification  : email xác nhận đơn

seller giao hàng → fulfillment.progress_changed (event)
                     ├──→ order         : tính trạng thái tổng hợp
                     ├──→ notification  : email "đã gửi" / "đã giao"
                     └──→ analytics     : mốc DELIVERED
```

Sáu bên nhận, và **không module nào biết bên nào nghe mình** — đó là giá
trị thật của kiến trúc event, giờ đã kiểm chứng được bằng luồng chạy thật.

Thêm `analytics` vào không sửa một dòng nào của `checkout` hay
`fulfillment`: chỉ thêm một `bus.Subscribe` ở `cmd/worker`.

Tiếp theo là **giai đoạn 6 — Marketplace hoàn chỉnh**. Bốn module MVP còn
thiếu (`identity`, `customer`, `promotion`, `analytics`) ĐÃ XONG cả bốn.

**Vì sao làm chúng trước giai đoạn 6:** giai đoạn 6 xây tính năng cho seller
và creator, mà mọi tính năng đó đều bắt đầu bằng câu hỏi "ai đang gọi, và họ
được xem dữ liệu của gian hàng nào". Xây tính năng trước rồi gắn phân quyền
sau nghĩa là phải sửa lại từng truy vấn — và bỏ sót một truy vấn là seller
đọc được đơn của đối thủ.

**Nợ kỹ thuật ưu tiên cao — chặn giai đoạn 5:**

| # | Nợ | Trạng thái |
|---|---|---|
| 1 | `Reserved → Committed` chưa tự động | **XONG** — `inventory` nghe `checkout.completed` |
| 2 | `platform/eventbus` rỗng, chưa có outbox | **XONG** — Transactional Outbox + dispatcher, worker phát mỗi 5 giây |
| 3 | `demand_signal` chưa được ghi | **XONG** — `supplychain` nghe `cart.item_added` và `checkout.completed` |

**Cả ba nợ chặn giai đoạn 5 đã trả.** Xem
[ADR-0006](../adr/0006-internal-events.md) mục "Trạng thái triển khai".

### `demand_signal` — vì sao ghi từ MVP dù chưa ai dùng

Đây là 1 trong 4 thứ "sửa sau là viết lại" ở mục 9, và là thứ cuối cùng
được hoàn thành.

```text
DỮ LIỆU LỊCH SỬ KHÔNG TẠO NGƯỢC ĐƯỢC.
```

Tới Phase 3 mà thiếu dữ liệu hành vi của 12 tháng trước thì không dự báo
được gì. Không có cách nào dựng lại "tháng 3 có bao nhiêu người tìm áo
khoác dạ mà không thấy".

**Sai lầm mà module này sinh ra để tránh:**

```text
Chỉ nhìn doanh số:  "Áo khoác bán 200 chiếc" → nhu cầu là 200

Thực tế:            bán 200, HẾT HÀNG từ tuần 3
                    1.500 lượt tìm sau khi hết
                    400 lượt đăng ký báo có hàng
                    → nhu cầu thật gần 800
```

Lập kế hoạch chỉ dựa vào doanh số sẽ **liên tục sản xuất thiếu** đúng những
mặt hàng bán chạy nhất — vì chúng hết hàng sớm nên số đơn thấp hơn nhu cầu.

**Ba loại tín hiệu giá trị nhất** — `SEARCH_NO_RESULT`, `STOCKOUT`,
`NOTIFY_REQUEST` — đo nhu cầu KHÔNG được đáp ứng, thứ không bao giờ xuất
hiện trong dữ liệu bán hàng.

**Đã cài ở MVP:** `ADD_TO_CART` và `ORDER` qua event. Ba loại "nhu cầu chưa
đáp ứng" có mô hình và bảng, nhưng chưa có nguồn phát — cần module search
(chưa có) và event `inventory.depleted` (chưa phát).

Kiểm chứng ngược, bốn phép phá:

| Phá gì | Test bắt được |
|---|---|
| Handler bỏ qua `cart.item_added` | 0 tín hiệu thay vì 1 |
| Bỏ nguồn giới thiệu khỏi metadata | Mất khả năng đo "nội dung nào tạo nhu cầu thật" |
| Bỏ ràng buộc "tín hiệu phải có đối tượng" ở domain | **Ràng buộc `CHECK` ở database vẫn chặn** — hai lớp hoạt động độc lập |
| Dùng thời điểm ghi thay thời điểm nghiệp vụ | Tín hiệu 3 ngày trước bị đẩy sang hôm nay; lọc theo kỳ ra 0 |

**Nợ kỹ thuật khác đã ghi nhận:**

- `checkout` chọn kho theo quy tắc "kho đầu tiên còn đủ hàng". Chọn kho gần
  khách nhất sẽ giảm phí ship và thời gian giao, nhưng cần địa chỉ — thứ
  khách chưa nhập tại thời điểm giữ hàng. Đổi tiêu chí chỉ ảnh hưởng một
  hàm (`pickStockItem`).
- Phí vận chuyển hiện do bên gọi truyền vào; `fulfillment.EstimateShipping`
  chưa có.

Điểm cuối là ràng buộc quan trọng nhất khi viết `checkout`: giá đưa vào
`PlaceOrder` phải là giá **khách đã nhìn thấy** ở màn hình thanh toán, không
phải giá tại thời điểm ghi database. `order` cố ý không có đường nào tự tra
giá, nên chỗ này không thể làm sai một cách im lặng.

**Còn thiếu ở tầng nền:** `platform/eventbus` vẫn rỗng. Cần Outbox để phát
`order.placed` cho inventory chuyển Reserved → Committed. Hiện `checkout`
sẽ phải gọi đồng bộ — chấp nhận được ở MVP, nhưng là nợ kỹ thuật đã biết.

**Lưu ý về thứ tự:** ba module dữ liệu chính (`catalog` · `product` · `pricing`)
đang được kiểm chứng bằng kho in-memory theo đúng khuyến nghị của ADR-0010 —
xác nhận mô hình domain trước khi dựng schema. Nhưng không nên để chậm quá:
`inventory` ở giai đoạn 3 cần khóa lạc quan, thứ **không kiểm chứng được**
bằng kho in-memory.

**Nợ kỹ thuật đã trả xong (12/08):** ba module giờ có **hai** cài đặt kho
lưu trữ cùng thỏa mãn một bộ port. Ba trong bốn ràng buộc từng chỉ được mô
phỏng nay đã kiểm chứng thật:

```text
✓ UNIQUE toàn cục trên sku_code     — product (quy tắc 1)
✓ Trigger chặn UPDATE/DELETE        — pricing.price_history, inventory_movement
✓ UNIQUE có điều kiện               — brand_authorization (chỉ áp cho APPROVED)
✓ CHECK số lượng >= 0               — inventory_item, sáu trạng thái
✓ Khóa lạc quan có tranh chấp thật  — 20 khách → đúng 1 người thắng
```

**Cả năm đã kiểm chứng thật.** Không còn ràng buộc quan trọng nào chỉ được
mô phỏng.

Việc chuyển kho lưu trữ **không sửa một dòng nào** trong `domain/` và
`application/` của cả ba module — đây là chỗ kiến trúc ports & adapters trả
lại giá trị đã bỏ ra.

---

## 12. PHASE HIỆN TẠI — Production Hardening

Danh sách việc chi tiết ở [backlog.md mục 2](backlog.md). Mục này ghi
**cái gì đã thực sự xong**, kèm bằng chứng — không đánh dấu xong nếu chưa
có code VÀ test chứng minh.

### 12.1 Commerce core — đã xong

| Phần | Bằng chứng |
|---|---|
| Catalog · Product · SKU | `product` 74 test · `catalog` 54 test · route công khai chạy |
| Offer · Pricing | `marketplace` 23 test · `pricing` 67 test |
| Seller · Marketplace | `seller` 23 test · 3 endpoint offer của nhà bán |
| Tồn kho của NHÀ BÁN | `inventory` 33 test · `FindOwnedItem` lọc theo chủ |
| Tồn kho NỀN TẢNG / Own Brand | `inventory.OwnerForSeller` · `TestOwnBrandLayHangCuaNenTang` |
| Cart | `cart` 33 test · gộp giỏ khi đăng nhập |
| Checkout | `checkout` 37 test · giữ hàng 15 phút · đóng băng giá |
| Customer · tài khoản · địa chỉ · wishlist | `customer` 41 test · 6 endpoint |
| Order | `order` 33 test · mã đơn qua SEQUENCE · idempotency `UNIQUE` |
| Payment | `payment` 30 test · sổ cái hai vế · idempotency `UNIQUE` |
| Fulfillment | `fulfillment` 26 test · vòng đời đầy đủ tới DELIVERED |
| Trung tâm người bán (MVP) | `apps/seller` cổng 3002 · 2 test trình duyệt |
| Nền móng Demand Signal | bảng `demand_signal` · `supplychain` 9 test · kiểm chứng 3 loại tín hiệu |
| Hợp đồng OpenAPI + TypeScript sinh ra | `npm run types:check` chặn ở CI |
| Outbox + Event Dispatcher | `platform/eventbus` · rollback không phát · dead letter sau 5 lần |
| Tách đơn nhiều nhà bán | `TestTachDonBaNguonHangQuaEvent` · hậu tố cơ số 26 |
| Cô lập fulfillment giữa nhà bán | `TestSellerKhongThayDuocPhanCuaSellerKhac` · phòng vệ hai lớp |
| **Phân giải chủ sở hữu tồn kho** | P3-18 · 3 test hồi quy, mỗi test xác nhận đỏ khi phá |
| E2E thương mại trên PostgreSQL | `internal/e2e` · 6 test, 8/12 kịch bản |
| Own Brand + Marketplace trong CÙNG một đơn | cùng test trên: 2 đơn thực hiện, tiền và tồn kho đúng từng bên |
| Đồng bộ tài liệu / README | 20/08 · mọi liên kết đã kiểm |

### 12.2 Chưa xong — và vì sao nó chặn production

| Khoảng trống | Hậu quả nếu bỏ qua |
|---|---|
| E2E còn thiếu thực hiện/giao hàng từng phần, hủy đơn, rollback toàn chuỗi | Lỗi ở KHOẢNG GIỮA module không ai thấy — P3-18 đã chứng minh |
| Idempotency thiếu ràng buộc DB ở inventory và fulfillment | Hai request song song cùng khóa vẫn lọt |
| Event versioning chỉ có TRƯỜNG, chưa có quy trình | Đã xảy ra: worker cũ nuốt event mới, bỏ trường mới, không lỗi |
| Không có metrics, không có tracing | Sự cố production chỉ biết khi khách báo |
| Correlation ID gần như luôn rỗng | Không lần được một đơn qua nhiều tiến trình |
| Rate limit chỉ áp cho đăng ký, và đếm trong bộ nhớ | Đăng nhập bị dò; N bản sao = N lần hạn mức |
| Webhook thanh toán và vận chuyển chưa có route | Đơn không bao giờ rời `PENDING_PAYMENT` bằng đường thật |
| `MarkOutOfStock` là code chết (P3-23) | Trạng thái offer trong database sai; mới vá ở tầng đọc |

### 12.3 Điều kiện kết thúc phase

Không chuyển phase cho tới khi chuỗi đầy đủ chạy E2E ổn định với **sáu
kịch bản** — own brand · 1 nhà bán · nhiều nhà bán · đơn trộn · thực hiện
từng phần · lỗi và thử lại · thanh toán đồng thời — và mọi bất biến về
ownership, authorization, inventory, transaction, idempotency đều có test
hồi quy tự động.

### 12.4 Nguyên tắc của phase

```text
Correctness > Consistency > Security > Reliability > Performance > Feature velocity
```

- Không thêm trừu tượng khi chưa có nhu cầu thật.
- Không thêm miền chỉ để "đủ tính năng" — danh sách khóa ở backlog mục 6.
- Không đánh dấu xong khi chưa có cài đặt VÀ test.
- **Mỗi lỗi kiểu production phải thành một test hồi quy.**
- Mỗi bất biến quan trọng phải có mặt ở cả ba nơi: code, test, tài liệu.

---

## 13. Tài liệu liên quan

- [deliverables.md](deliverables.md) — thứ tự triển khai, ma trận, rủi ro
- [mvp.md](mvp.md) — phạm vi và tiêu chí hoàn thành MVP
- [../adr/README.md](../adr/README.md) — chỉ mục ADR
- [../04-modules/README.md](../04-modules/README.md) — đặc tả từng module
