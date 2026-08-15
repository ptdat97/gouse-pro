# Admin UI — ghi chép quyết định thiết kế

> **Đây KHÔNG phải backlog.** Việc còn phải làm nằm ở
> **[backlog.md](backlog.md)** (mục P2-3, P2-4, P3-5).
>
> File này giữ **lý do đằng sau các quyết định** của lát cắt admin/frontend
> và bằng chứng kiểm chứng — thứ không thuộc về một danh sách việc.

**Ngày lập:** 14/08/2026 · **Trạng thái:** đang thực hiện

**Đã xong:** ADR-0011 · sửa đặc tả OpenAPI · **A1 middleware xác thực** ·
**A2 endpoint xác thực** · **A3 audit log**. Đặc tả từ 62 lên **71
operation**, 0 lỗi lint. Endpoint đã cài: 5 → **10**.

**Đang làm:** A4 — seller list + approve/suspend.

Phạm vi đợt đầu: **auth + 3 nhóm trang** (sellers · audit log · orders).
Quyết định theo tinh thần Architecture Freeze — chỉ làm phần có vấn đề thật,
không dựng trang cho API chưa tồn tại.

---

## 1. Vì sao backend phải đi trước

Đối chiếu code ngày 14/08/2026:

| Thứ Admin UI cần | Trạng thái backend |
|---|---|
| Đăng nhập | `identity` có `Login`/`Refresh`/`Logout`/`Authenticate` — **chưa có endpoint HTTP** |
| Middleware xác thực | **Không tồn tại** — `httpserver` chỉ có RequestID · Recover · Logging · MaxBytes · SecurityHeaders |
| Idempotency-Key | **Không tồn tại** ở tầng HTTP dù `api-guidelines.md` mục 10 bắt buộc |
| Danh sách seller chờ duyệt | `seller.API` **không có** `ListSellers` |
| Audit log | **Không tồn tại**: không code, không migration, không bảng |
| Chi tiết đơn cho admin | `order.API` có `GetOrder`/`GetOrderByNumber`, chưa có endpoint |

Ba khoảng trống đầu chặn **mọi** trang. Không có chúng thì Admin UI không
đăng nhập được, và mọi thứ xây bên trên là giao diện chạy trên dữ liệu giả.

### Quyết định: audit log là module mới hay không?

Tài liệu `docs/README.md` cấm thêm module ngoài 28 module đã liệt kê **trừ khi
có ADR**. Audit log không nằm trong danh sách 28 module, nhưng:

- `admin.md` mục 7 yêu cầu trang audit log
- `admin-api.md` mục 2 yêu cầu 7 endpoint ghi `reason` vào `audit_log`
- `admin-api.md` mục 6 yêu cầu ghi log **mọi** lần đọc dữ liệu khách

Đây là vấn đề triển khai THẬT, đúng thước đo ở `docs/README.md`. Đề xuất:
**viết ADR-0011 "Audit log là năng lực platform, không phải module"** —
đặt ở `internal/platform/audit`, vì nó bị mọi module gọi và không sở hữu
khái niệm nghiệp vụ nào. Đặt làm module sẽ tạo phụ thuộc từ mọi module tới
nó, vi phạm R5 (không phụ thuộc vòng) khi audit cần đọc ngược.

**Cần chốt trước khi code.**

---

## 2. Giai đoạn A — Nền tảng HTTP (backend)

Không có UI nào trong giai đoạn này. Mục tiêu: một request có token gọi được
endpoint admin và bị chặn đúng khi thiếu quyền.

### A1. Middleware xác thực — `internal/platform/httpserver` ✓

- [x] `Auth(TokenVerifier)` — đọc `Authorization: Bearer`, gắn `AuthContext`
      vào request context (`auth.go`)
- [x] `RequireRole(roles ...string)` — chặn theo vai trò, trả **403**
- [x] Phân biệt đúng **401 vs 403** theo `api-guidelines.md` mục 4
- [x] `RequireIdempotencyKey()` — bắt buộc header với POST/PATCH
      (`idempotency.go`)

**Hai quyết định thiết kế cần ghi lại:**

`TokenVerifier` là interface do **platform khai báo**, module identity cài
đặt. Platform không được import module nghiệp vụ (R3), nên `AuthContext` ở
đây là kiểu riêng, trùng hình dạng với `identity.AuthContext` nhưng độc lập.
Cái giá là phải chuyển đổi thủ công ở `cmd/api`; cái được là identity thay
đổi không kéo theo platform.

`RequireIdempotencyKey` **chỉ kiểm tra định dạng**, không lưu khóa và không
phát lại response cũ. Việc chống trùng lặp thật đã nằm ở các module, dựa trên
ràng buộc UNIQUE của cột `idempotency_key` — xem `migrations/000007_payment`
và `000008_order`. Đó là chỗ duy nhất làm đúng được, vì chỉ ở đó việc kiểm
tra khóa và việc ghi dữ liệu mới nằm trong **cùng một giao dịch**. Một bộ nhớ
đệm ở tầng HTTP sẽ nằm ngoài giao dịch và tạo ra đúng lỗi nó định ngăn: ghi
khóa xong rồi giao dịch rollback → lần thử lại bị coi là trùng và trả về
"thành công" cho thao tác chưa bao giờ xảy ra.

**Kiểm chứng bằng cách phá code** — cả 4 lần test đều fail đúng chỗ:

| Bất biến bị phá | Test bắt được |
|---|---|
| `RequireRole` trả 401 thay vì 403 | `TestAuthDistinguishes401From403` |
| `RequireRole` đi tiếp khi thiếu `AuthContext` | `TestRequireRoleWithoutAuthFailsClosed` |
| `HasAnyRole()` danh sách rỗng trả true | `TestHasAnyRoleEmptyListDeniesAccess` |
| Khóa idempotency rỗng đi lọt | `TestIdempotencyKeyRequiredForWrites` |

`gofmt` · `go vet` · `archcheck` (235 file) · `go test ./...` (45 package) ·
`go test -race` — tất cả pass.

### A2a. Phát hành access token — `internal/platform/token` ✓

`identity` cố ý **không** phát hành access token: nó chỉ trả refresh token và
`AccessTokenTTL` khuyến nghị, để tầng interfaces tự phát hành. Chỗ trống đó
nay do `platform/token` lấp, dùng `golang-jwt/jwt/v5` (phụ thuộc ngoài thứ
ba của dự án, sau `pgx` và `golang.org/x/crypto`).

- [x] `Issuer.Issue` / `Issuer.Verify`, HS256, TTL mặc định 15 phút khớp
      `identity/domain.AccessTokenTTL`
- [x] Khóa bí mật ngắn hơn 32 ký tự bị từ chối **lúc khởi tạo**
- [x] Chỉ chấp nhận ĐÚNG HS256 — kể cả token ký bằng HS512 với đúng khóa
- [x] Token không mang email, tên, số điện thoại

**Một test hóa ra không kiểm chứng được gì.** `TestAlgNoneRejected` vẫn xanh
sau khi bỏ hết lớp bảo vệ thuật toán, vì `golang-jwt/v5` tự chặn `none` ở
tầng thư viện. Bài học đúng như `todo.md` mục 5 cảnh báo: test xanh sau khi
phá bất biến là test không kiểm chứng gì.

Đã thay bằng `TestOtherSigningAlgorithmRejected` — ký token bằng **HS512 với
đúng khóa bí mật**. Đó là lỗ hổng thật mà code của chúng ta chặn, và test này
fail đúng khi bỏ `WithValidMethods`. Test `alg=none` giữ lại nhưng ghi rõ nó
chốt hành vi của thư viện, không phải của ta.

**Kiểm chứng bằng cách phá code** — cả 3 lần test fail đúng chỗ:

| Bất biến bị phá | Test bắt được |
|---|---|
| Bỏ khóa cứng thuật toán | `TestOtherSigningAlgorithmRejected` |
| Bỏ kiểm tra độ dài khóa bí mật | `TestShortSecretRejectedAtStartup` |
| Bỏ kiểm tra `Subject` rỗng | `TestMissingSubjectRejected` |

### A2b. Endpoint xác thực ✓

- [x] `POST /api/v1/auth/login` — refresh token vào **httpOnly cookie**,
      access token trong body
- [x] `POST /api/v1/auth/refresh` — xoay token, thu hồi token cũ
- [x] `POST /api/v1/auth/logout` — idempotent
- [x] `GET /api/v1/admin/me` — vai trò + phạm vi + `requires_two_factor`
- [x] `AUTH_JWT_SECRET` bắt buộc ở production, tối thiểu 32 ký tự
- [x] `SecureCookie` tắt CHỈ ở development (localhost chạy HTTP)

Cookie đặt `Path=/api/v1/auth` để nó không đi kèm mọi request — chỉ tới
endpoint xác thực. Cùng với `HttpOnly`, `Secure`, `SameSite=Strict`, mỗi
thuộc tính chặn một đường tấn công khác nhau.

**Kiểm chứng bằng cách CHẠY SERVER THẬT** trên PostgreSQL, không chỉ biên
dịch — 14 phép thử, tất cả đúng như thiết kế:

| Kiểm chứng | Kết quả |
|---|---|
| `/admin/me` không token | 401 |
| `/admin/me` token hợp lệ | 200, `requires_two_factor: true` cho OPS_FINANCE |
| Token rác, sai scheme (`Basic`) | 401 |
| Đăng nhập đúng | 200, cookie `#HttpOnly_`, Path `/api/v1/auth`, `expires_in: 900` |
| Refresh | 200 và refresh token **đã xoay** |
| Refresh bằng token CŨ sau khi xoay | 401 — token cũ đã thu hồi |
| Logout, logout lần hai, logout không cookie | 204 cả ba — idempotent |
| Refresh sau logout | 401 |
| Email có thật vs không tồn tại | **thông báo giống hệt nhau** |

Phép thử cuối là quan trọng nhất: hai thông báo khác nhau dù chỉ một chữ
cũng đủ biến đường đăng nhập thành công cụ dò danh sách email có thật.

**Một lỗi ranh giới bị archcheck bắt tại chỗ.** Handler ban đầu import
`identity/infrastructure/crypto` cho `HashIP` — vi phạm R8 (interfaces không
được import infrastructure). Đã đổi sang `platform/privacy`; hai hàm cho
cùng kết quả vì `crypto.HashIP` chỉ là wrapper gọi thẳng `privacy.HashIP`.

### A3. Audit log — `internal/platform/audit` ✓

- [x] Migration 000020: bảng `audit_log`, 4 chỉ mục theo đúng đường tra cứu
      của trang audit, **đảo được** (kiểm chứng down rồi up lại)
- [x] `Recorder` chỉ có `Write`, `WriteTx`, `WriteSensitive`, `Query` —
      **không có** `Update`, **không có** `Delete`
- [x] Trigger database chặn UPDATE/DELETE, theo tiền lệ sổ cái (ADR-0008)
- [x] `WriteTx` ghi bằng CHÍNH giao dịch của bên gọi
- [x] `ValidateReason` — tối thiểu 20 ký tự, chặn lý do rác
- [x] `GET /api/v1/admin/audit-log` — lọc theo resource_type · resource_id ·
      action · actor_id · khoảng ngày, phân trang cursor, **chỉ ADMIN**

**Ba quyết định thiết kế:**

*Không khóa ngoại tới bảng nào.* Ngoài ADR-0005, còn một lý do mạnh hơn:
audit log phải sống lâu hơn thứ nó ghi lại. Xóa một seller mà vết "ai đã
đình chỉ seller đó" biến mất theo thì audit log vô dụng đúng lúc cần nhất.

*`resource_type` là chuỗi thuần, không phải enum database.* Thêm loại tài
nguyên mới không được buộc phải chạy migration. Cái giá là mất an toàn kiểu
— giảm nhẹ bằng hằng số khai báo trong package.

*Phân trang theo ID, không theo `occurred_at`.* ULID tăng dần theo thời gian
nên thứ tự giống nhau, nhưng ID là DUY NHẤT. Phân trang theo mốc thời gian
sẽ bỏ sót hoặc lặp bản ghi khi nhiều bản ghi cùng dấu thời gian.

**Một giới hạn đã biết, được ghi lại chứ không giấu.** Trigger `FOR EACH ROW`
không chạy với `TRUNCATE`, nên ai có quyền TRUNCATE vẫn xóa sạch được. Không
chặn là có chủ ý: test cần TRUNCATE để dọn dữ liệu, và quyền này ở production
được kiểm soát ở tầng phân quyền database. `TestTruncateStillPossible` tồn
tại để giới hạn đó không bị phát hiện muộn.

**Kiểm chứng bằng cách phá code** — cả 3 lần test fail đúng chỗ:

| Bất biến bị phá | Test bắt được |
|---|---|
| Gỡ trigger chặn UPDATE khỏi database | `TestUpdateBlockedByDatabase` |
| Bỏ kiểm tra lý do rác | `TestValidateReason` (3 case) |
| `WriteTx` ghi ngoài giao dịch của bên gọi | `TestWriteTxRollbackLeavesNoTrace` |

**Kiểm chứng trên server thật** — 13 phép thử:

| Kiểm chứng | Kết quả |
|---|---|
| Không token / OPS_FINANCE / ADMIN | 401 / **403** / 200 |
| Lọc theo resource_type, action, actor_id | đúng số bản ghi từng loại |
| `from=to=2026-08-14` | trả **cả 4** bản ghi trong ngày |
| Phân trang `limit=2` + cursor | 2+2 bản ghi, **không lặp**, `has_more` đúng |
| `limit=abc`, `from=14-08-2026` | 400 cả hai |

Phép thử khoảng ngày quan trọng hơn vẻ ngoài: không bao trọn ngày cuối thì
nhân viên lọc theo tháng mất sạch bản ghi ngày 31 mà không hề biết.

**Phát hiện ngoài lề: test suite có một chỗ chập chờn CÓ SẴN.**
`TestTrangThaiDonSuyRaTuTienDoQuaEvent` (module fulfillment) thỉnh thoảng
fail khi chạy `go test ./...` với database thật. Nguyên nhân: `order` và
`fulfillment` cùng `TRUNCATE "order" CASCADE` trong hàm dựng test, mà Go
chạy các package test SONG SONG — package này xóa dữ liệu package kia đang
dùng.

Không liên quan tới audit (bảng `audit_log` không package nào khác đụng
vào) và đã có trước thay đổi này: chạy riêng pass 3/3, chạy đầy đủ 3 lần
liên tiếp cũng pass. Ghi lại ở đây để không bị bỏ quên — cần sửa bằng cách
cho mỗi package test một schema riêng, hoặc đánh dấu các package đụng
database là không chạy song song.

### A4. Seller — bổ sung năng lực còn thiếu

- [ ] Thêm `ListSellers(ctx, filter)` vào `seller.API` — lọc theo status,
      phân trang cursor. Trang duyệt hồ sơ không có cái này thì không tồn tại
- [ ] `ApproveSeller` nhận thêm `commission_policy_id`, `reserve_rate`,
      `reserve_hold_days` theo `admin-api.md` mục 3
- [ ] `SuspendSeller` trả `effects.offers_hidden` và
      `pending_fulfillment_orders` — UI phải hiện con số này trước khi xác nhận
- [ ] `GET /api/v1/admin/sellers`, `GET /api/v1/admin/sellers/{id}`
- [ ] `POST .../approve`, `POST .../suspend` — bắt buộc `reason`

**Quy tắc nghiệp vụ phải giữ:** đình chỉ seller **không hủy** đơn khách đã
trả tiền (`admin-api.md` mục 3).

### A5. Orders cho admin

- [ ] `GET /api/v1/admin/orders` — danh sách, lọc theo trạng thái/ngày
- [ ] `GET /api/v1/admin/orders/{id}` — **ghi audit log mỗi lần gọi**, bắt
      buộc tham số `reason`
- [ ] `POST /api/v1/admin/orders/{id}/cancel` — bắt buộc `reason`

Kiểm tra `reason`: có độ dài tối thiểu, từ chối giá trị rác như "test",
"fix" (`admin-api.md` mục 2). Lý do trống làm audit log vô giá trị.

---

## 3. Giai đoạn B — Khung monorepo

```text
/apps/admin                  — Next.js 15, App Router, TS strict
/packages
    /types                   — sinh từ OpenAPI, KHÔNG viết tay
    /api-client              — fetch wrapper + xử lý lỗi
    /design-tokens           — màu, khoảng cách, typography
    /ui                      — component trên Radix primitives
```

Không tạo `ui-commerce` — `design-system.md` mục 2 ghi rõ admin gần như
không dùng component thương mại.

- [ ] pnpm workspace + Turborepo
- [ ] `packages/types` sinh bằng `make api-types` (target **đã có sẵn** trong
      Makefile)
- [ ] CI job: sinh lại types và fail nếu lệch bản đã commit — backend đổi hợp
      đồng thì frontend phải đỏ ngay, đúng ý `frontend-architecture.md` mục 6

---

## 4. Giai đoạn C — api-client và design system

### C1. `packages/api-client`

- [ ] Tự gắn `Authorization`, `X-Request-ID`, `Accept-Language: vi-VN`
- [ ] Tự sinh `Idempotency-Key` (ULID) cho POST/PATCH
- [ ] `isApiError` + xử lý theo `code`, **không parse `message`** — message
      đổi được và có thể đa ngôn ngữ (`frontend-architecture.md` mục 6)
- [ ] 401 → thử refresh một lần rồi retry; 403 → **không** retry
- [ ] 429 → chờ theo `X-RateLimit-Reset`
- [ ] Cursor pagination theo `api-guidelines.md` mục 8
- [ ] Enum lạ **không được làm crash UI** — yêu cầu bắt buộc trong openapi.yaml

### C2. `packages/design-tokens`

Chỉ CSS variable. Component **không bao giờ** dùng màu/khoảng cách trực tiếp
(`design-system.md` mục 3). Thang khoảng cách: 4, 8, 12, 16, 24, 32, 48, 64.
Kiểm tra độ tương phản ngay ở tầng token.

### C3. `packages/ui` — trên Radix primitives

Button · Input · Select · Modal · DataTable · Toast · Badge · Tabs

Ràng buộc từ `design-system.md` mục 9:
- Không chứa logic nghiệp vụ
- **Không gọi API trực tiếp** — nhận dữ liệu qua props
- A11y là mặc định: mọi input có nhãn, modal bẫy focus + đóng bằng Escape,
  thông báo lỗi liên kết với input tương ứng
- Chấp nhận lặp lại đến **lần thứ ba** mới trừu tượng hóa (P16)

---

## 5. Giai đoạn D — Vỏ ứng dụng admin

- [ ] Trang đăng nhập; access token giữ **trong memory**, không localStorage
      (chống XSS — `frontend-architecture.md` mục 8)
- [ ] Refresh ngầm trước khi access token hết hạn (15 phút)
- [ ] Sidebar lọc theo 6 vai trò vận hành. **Chỉ là trải nghiệm** — backend
      luôn kiểm tra lại
- [ ] Chỗ trống cho 2FA: `identity` đã có `RequiresTwoFactor()` trả true cho
      `ADMIN`, `OPS_FINANCE`, `SELLER_OWNER`, nhưng luồng 2FA **chưa cài đặt**
      (`todo.md` mục 2.4 ghi rõ là sau MVP). UI thiết kế sẵn bước này

### Ba primitive dùng lại khắp app

Phản ánh nguyên tắc ở `admin.md` mục 8:

| Primitive | Việc |
|---|---|
| `ReasonDialog` | Cảnh báo không hoàn tác + lý do tối thiểu 20 ký tự + chặn giá trị rác |
| `ImpactPreview` | Hiện tác động trước khi xác nhận: "sẽ ẩn 142 offer" |
| `AuditNotice` | Báo trước khi truy cập dữ liệu cá nhân, bắt nhập lý do |

Kiểm tra lý do ở frontend là **trải nghiệm**, không phải bảo mật. Backend
vẫn phải từ chối lý do rác.

---

## 6. Giai đoạn E — Ba nhóm trang

### E1. Sellers

- Danh sách, lọc theo trạng thái, ưu tiên **hồ sơ chờ duyệt** lên đầu
  ("nhân viên vào để làm việc" — `admin.md` mục 8)
- Trang duyệt theo `admin.md` mục 4: giấy phép, mã số thuế, và
  **xác minh tài khoản ngân hàng** — sai tài khoản là chuyển tiền nhầm
  người, rất khó thu hồi
- Chính sách áp dụng: loại seller, chính sách hoa hồng, tỷ lệ giữ bảo đảm
- Sau khi duyệt: hiện `side_effects` từ response (cấp vai trò, tạo tài khoản
  tài chính, gửi email) — các tác động này chạy bất đồng bộ qua event nên
  người vận hành cần thấy
- Đình chỉ: `ImpactPreview` hiện số offer sẽ ẩn và số đơn đang xử lý, kèm
  ghi chú đơn đang xử lý **không** bị hủy

### E2. Audit log

- Chỉ đọc. **Không có** nút sửa hay xóa ở bất kỳ đâu
- Lọc theo resource_type · action · khoảng ngày
- Hiện đầy đủ: thời điểm · người thao tác · hành động · tài nguyên · lý do

### E3. Orders (hỗ trợ khách)

- Tra cứu theo mã đơn
- **`AuditNotice` chặn trước**: nhập lý do truy cập rồi mới xem được
- Chi tiết đơn liên kết chéo theo `admin.md` mục 6: đơn → lô giao →
  bút toán → lịch sử thao tác. Nhân viên cần toàn cảnh để trả lời khách
- Đánh dấu nổi bật lô quá hạn SLA
- Hủy lô: qua `ReasonDialog`

**Không làm đợt này:** finance/ledger, payouts, supply chain, warehouse,
products, brands, content, campaigns, analytics. Backend chưa có endpoint,
và dựng UI cho API chưa tồn tại là vi phạm chính điều Architecture Freeze cấm.

---

## 7. Giai đoạn F — Chất lượng

- [ ] MSW mock sinh từ OpenAPI — cho phép làm UI song song backend
- [ ] Unit test component; integration test luồng nhạy cảm
- [ ] E2E Playwright: đình chỉ seller, truy cập dữ liệu khách có audit
- [ ] CI: typecheck · lint · test · kiểm tra lệch types
- [ ] Kiểm chứng ngược bằng cách **phá code**: bỏ kiểm tra vai trò ở sidebar,
      bỏ bắt buộc lý do — test phải fail. Test vẫn xanh sau khi phá bất biến
      thì nó không kiểm chứng được gì

---

## 8. Thứ tự thực hiện

```text
A1 Middleware auth + idempotency     ← chặn mọi thứ
A2 Endpoint login/refresh/me         ← chặn mọi thứ
ADR-0011 audit log                   ← cần chốt trước A3
A3 Audit log platform
A4 Seller list + approve/suspend
A5 Orders admin
B  Monorepo + sinh types
C  api-client + tokens + ui
D  Vỏ app + 3 primitive
E1 Sellers → E2 Audit log → E3 Orders
F  Test và CI
```

A1 và A2 làm được song song với B. Từ C trở đi cần A xong để gọi API thật.

---

## 9. Các điểm đã chốt

### 9.1 Audit log → `internal/platform/audit` ✓

Xem [../adr/0011-audit-log.md](../adr/0011-audit-log.md).

### 9.2 Sửa OpenAPI trước khi viết code ✓

Đã thêm `api/paths/auth.yaml` (login · refresh · logout · admin/me) và bổ
sung vào `admin.yaml`: `sellers`, `seller_detail`, `orders`, `order_detail`,
`order_cancel`, schema `AdminSellerSummary`.

### 9.3 2FA — chặn phát hành, không chặn phát triển

Xây A→F không có 2FA, nhưng **2FA nằm trong định nghĩa "xong"** của Admin UI.

Lý do: `admin-api.md` mục 1 ghi đây là yêu cầu bắt buộc với `ADMIN` và
`OPS_FINANCE`, và một admin console có quyền điều chỉnh sổ cái mà chỉ cần
mật khẩu là rủi ro thật. Chấp nhận tạm khi đang phát triển thì được; phát
hành cho nhân viên dùng thật thì không.

Vì thế `GET /api/v1/admin/me` trả sẵn `requires_two_factor` — giao diện biết
tài khoản nào sẽ cần, và trường này là chỗ nối khi luồng 2FA được cài.

**Tiêu chí phát hành Admin UI:**

```text
[ ] Luồng 2FA hoạt động cho ADMIN và OPS_FINANCE
[ ] Ba nhóm trang chạy trên API thật, không phải mock
[ ] Mọi thao tác nhạy cảm ghi audit log, đã kiểm chứng bằng cách phá code
```

---

## 10. Tài liệu liên quan

- [../08-frontend/admin.md](../08-frontend/admin.md)
- [../08-frontend/frontend-architecture.md](../08-frontend/frontend-architecture.md)
- [../08-frontend/design-system.md](../08-frontend/design-system.md)
- [../06-api/admin-api.md](../06-api/admin-api.md)
- [../06-api/api-guidelines.md](../06-api/api-guidelines.md)
- [todo.md](todo.md)
