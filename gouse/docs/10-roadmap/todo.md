# Tiến độ triển khai

Theo dõi việc đã làm và việc còn lại, bám theo thứ tự triển khai ở
[deliverables.md](deliverables.md) mục 14 và phạm vi MVP ở [mvp.md](mvp.md) mục 3.

**Cập nhật:** 13/08/2026 · **Trạng thái:** Giai đoạn 3 **hoàn thành** (3/3 module)

Ký hiệu: `[x]` xong và đã kiểm chứng · `[~]` đang làm · `[ ]` chưa bắt đầu

---

## 1. Tình hình hiện tại

```text
Tài liệu     125 file · 32.367 dòng · 13 thư mục      ✓ hoàn thành
Đặc tả API   11 file YAML · 62 operation · 0 lỗi lint ✓ hoàn thành
             (còn 4 cảnh báo redocly, không chặn)
Code Go      117 file · 28.868 dòng · 337 hàm test    → Hết giai đoạn 3/7
Migration    12 file SQL · 6 module · đảo được

Module MVP đã có code:  6/14  (catalog · product · pricing ·
                               inventory · seller · marketplace)
Kho lưu trữ:            in-memory VÀ PostgreSQL, cùng port
Endpoint đã cài đặt:    5/62  (brand, collection, categories,
                               product detail, product list)
```

Kiểm chứng lần cuối (12/08/2026):

```text
✓ gofmt        không có file cần định dạng lại
✓ go vet       không có cảnh báo
✓ archcheck    OK — 117 file, không vi phạm ranh giới
✓ go test      28/28 package pass, có -race, CÓ database thật
✓ chạy thật    server chạy trên PostgreSQL: brand kèm bộ sưu tập,
               sản phẩm đủ 3 biến thể + SKU, hàng nháp trả 404
✓ bền vững     dữ liệu sống qua khởi động lại, seed tự bỏ qua lần 2
✓ migration    down -all rồi up lại thành công (đảo được)
✓ tranh chấp   20 khách mua 1 sản phẩm → ĐÚNG 1 người thắng,
               19 xung đột phiên bản được phát hiện và từ chối
```

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
- [ ] `platform/eventbus` — **thư mục rỗng**, cần cho Outbox (ADR-0006)
- [ ] `identity` — module MVP, chưa bắt đầu

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

## 5. Giai đoạn 4 — Giao dịch `[ ]`

- [ ] `cart` — không giữ tồn kho
- [ ] `checkout` — giữ hàng, đóng băng giá
- [ ] `order` — đóng băng dữ liệu, **tách Order/FulfillmentOrder**
- [ ] `payment` — **sổ cái bất biến**, RULE chặn UPDATE/DELETE ở database
- [ ] Bất biến Σ DEBIT = Σ CREDIT kiểm tra trong constructor

---

## 6. Giai đoạn 5 — Thực hiện đơn `[ ]`

- [ ] `fulfillment` — tách đơn theo nguồn hàng
- [ ] `notification`

---

## 7. Giai đoạn 6 — Marketplace hoàn chỉnh `[ ]`

- [ ] Đăng ký seller
- [ ] Đối soát
- [ ] Chi trả

---

## 8. Giai đoạn 7 — Hoàn thiện MVP `[ ]`

- [ ] `promotion`
- [ ] `analytics` cơ bản
- [ ] **`demand_signal`** — ghi từ MVP dù chưa dùng
- [ ] `customer` — wishlist, preference

---

## 9. Bốn thứ phải làm đúng ngay ở MVP

> [mvp.md](mvp.md) mục 2 — *sửa sau là viết lại*

| # | Hạng mục | Trạng thái | Ghi chú |
|---|---|---|---|
| 1 | Mô hình `Offer` | `[x]` | **Xong** — giai đoạn 3, kèm chống hàng giả và buy box |
| 2 | Tách `Order`/`FulfillmentOrder` | `[ ]` | Giai đoạn 4 |
| 3 | Sổ cái bất biến | `[ ]` | Giai đoạn 4 |
| 4 | Ghi `demand_signal` | `[ ]` | Giai đoạn 7 — **dữ liệu lịch sử không tạo ngược được** |

**1/4 đã xong.** Ba cái còn lại vẫn là rủi ro cần theo dõi: hai cái ở giai
đoạn 4 (ngay tiếp theo), riêng `demand_signal` ở giai đoạn 7 là đáng lo
nhất — dữ liệu lịch sử không tạo ngược được, bỏ qua thì Phase 3 chậm gần
một năm.

---

## 10. Tiêu chí hoàn thành MVP

Đối chiếu [mvp.md](mvp.md) mục 7.

### Kiến trúc

| Tiêu chí | Trạng thái |
|---|---|
| Không có phụ thuộc vòng giữa module | `[x]` archcheck R5 |
| Không có thư mục `common/` `utils/` `helpers/` `services/` | `[x]` archcheck R7 |
| Kiểm tra ranh giới module trong CI đều xanh | `[x]` job `architecture` chạy đầu tiên, thất bại = chặn merge |
| Không có JOIN vượt ranh giới module | `[ ]` chưa có database |
| Mọi lệnh ghi API đều idempotent | `[ ]` chưa có endpoint ghi |
| Outbox hoạt động, không có event kẹt | `[ ]` `platform/eventbus` còn rỗng |

### Chức năng · Chất lượng

Chưa đánh giá được — cần đủ module giao dịch. Riêng "API p95 < 300ms" và
"LCP < 2,5s" cần môi trường có tải thật.

---

## 11. Việc tiếp theo

Bắt đầu **giai đoạn 4 — Giao dịch**:

```text
1. cart      — không giữ tồn kho
2. checkout  — giữ hàng (gọi inventory.Reserve), đóng băng giá
3. order     — đóng băng dữ liệu, TÁCH Order/FulfillmentOrder
4. payment   — sổ cái bất biến
```

**Hai trong bốn thứ "phải làm đúng ngay ở MVP" nằm ở giai đoạn này.** Sửa
sau là viết lại, nên không được rút gọn.

**Nền đã sẵn:** `checkout` gọi được `inventory.Reserve` với khóa lạc quan
đã kiểm chứng; `order` lấy được tỷ lệ hoa hồng từ `marketplace.GetCommissionRate`
để đóng băng vào `OrderLine`.

**Còn thiếu ở tầng nền:** `platform/eventbus` vẫn rỗng. Giai đoạn 4 cần
Outbox để phát `order.placed` cho inventory chuyển Reserved → Committed.
Hiện các module chỉ gọi đồng bộ được.

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

## 12. Tài liệu liên quan

- [deliverables.md](deliverables.md) — thứ tự triển khai, ma trận, rủi ro
- [mvp.md](mvp.md) — phạm vi và tiêu chí hoàn thành MVP
- [../adr/README.md](../adr/README.md) — chỉ mục ADR
- [../04-modules/README.md](../04-modules/README.md) — đặc tả từng module
