# ADR-0011: Audit log là năng lực platform, không phải module

**Trạng thái:** Accepted
**Ngày:** 14/08/2026

---

## Context

Tài liệu yêu cầu ghi nhật ký thao tác ở ba chỗ khác nhau, nhưng
**không chỗ nào tồn tại trong code** tính tới 14/08/2026 — không package,
không migration, không bảng.

### Ba yêu cầu đã có trong tài liệu

```text
06-api/admin-api.md mục 2
    → 7 endpoint bắt buộc trường `reason`, ghi vào audit_log:
      ledger.adjust · inventory.adjust · seller.suspend ·
      creator.suspend · content.take-down · order.cancel · refund

06-api/admin-api.md mục 6
    → MỌI lần đọc dữ liệu cá nhân khách hàng đều ghi audit

08-frontend/admin.md mục 7
    → trang nhật ký thao tác, CHỈ ĐỌC
```

### Vì sao đây là vấn đề triển khai thật

`docs/README.md` (Architecture Freeze) cấm thêm module mới ngoài 28 module
đã liệt kê trừ khi có ADR, và đặt ra thước đo:

> Vấn đề cụ thể nào trong code hiện tại buộc phải thay đổi này?

Câu trả lời cụ thể: Admin UI đợt đầu gồm ba nhóm trang — sellers, audit log,
orders. **Hai trong ba** không dựng được nếu không có audit log:

```text
Trang audit log   → không có nguồn dữ liệu
Trang orders      → admin.md mục 6 bắt buộc chặn bằng lý do truy cập
                    trước khi xem dữ liệu khách; không có nơi ghi lý do
```

Đây không phải "sẽ cần sau này". Đây là hai màn hình đã chốt phạm vi mà
không có audit log thì không tồn tại được.

### Ràng buộc kiến trúc đã có

```text
archcheck R3
    → platform KHÔNG được import module nghiệp vụ

archcheck R5
    → không phụ thuộc vòng

ADR-0005
    → mỗi bảng thuộc đúng một module,
      không khóa ngoại vượt ranh giới module
```

---

## Decision

**Audit log đặt ở `internal/platform/audit`, không phải module nghiệp vụ.**

```text
internal/platform/audit/
    audit.go        — Recorder: Write + Query
    postgres.go     — cài đặt PostgreSQL

migrations/
    NNNN_audit_log.up.sql
```

Ba ràng buộc bắt buộc:

```text
1. CHỈ có Write và Query. KHÔNG có Update, KHÔNG có Delete.
2. Nhận resource_type và action dạng CHUỖI THUẦN.
   Không import module nào — nếu không sẽ vi phạm R3.
3. Không khóa ngoại tới bảng của bất kỳ module nào (ADR-0005).
```

### Vì sao platform, không phải module

Audit log **bị mọi module gọi** và **không sở hữu khái niệm nghiệp vụ nào**.
Đó chính là định nghĩa của platform trong `cmd/archcheck/rules.go`: hạ tầng
trung lập với domain, giống `logger` và `eventbus`.

Nếu làm thành module, mọi module cần ghi audit sẽ phải import nó:

```text
seller  → audit
order   → audit
payment → audit
...
```

Điều này biến audit thành điểm phụ thuộc chung của toàn hệ thống — đúng thứ
mà nguyên tắc P12 và `forbiddenDirs` trong archcheck sinh ra để ngăn. Nó
cũng tạo rủi ro phụ thuộc vòng (R5) ngay khi audit cần bất kỳ khái niệm nào
từ module mà module đó cũng ghi audit.

### Vì sao không có Update và Delete

Nêu ở `08-frontend/admin.md` mục 7: nếu sửa được, audit log **mất hết giá
trị**. Một bản ghi kiểm toán chỉ đáng tin khi không ai — kể cả người có
quyền cao nhất — sửa được nó sau khi sự việc xảy ra.

Vì thế đây là ràng buộc ở **tầng API của package**, không phải quy ước: không
có hàm nào để gọi thì không có đường nào để lạm dụng. Tăng cường thêm ở tầng
database bằng RULE chặn `UPDATE`/`DELETE`, cùng cách ADR-0008 làm với sổ cái.

---

## Alternatives

### A. Module `audit` trong `internal/modules/` — **bị loại**

Nghe hợp lý vì audit log có bảng riêng và vòng đời riêng.

Bị loại vì audit không có **quy tắc nghiệp vụ** nào. Nó không có máy trạng
thái, không có bất biến domain, không có quyết định nào để đưa ra. Một module
chỉ gồm một bảng và hai thao tác ghi/đọc là module rỗng — và nó kéo theo cái
giá thật: mọi module khác phải import nó.

### B. Ghi audit qua eventbus — **bị loại cho MVP**

Hấp dẫn: module phát event, một bộ xử lý ghi vào bảng audit. Không module nào
phải biết audit tồn tại.

Bị loại vì **audit phải nằm trong cùng giao dịch với hành động**. Outbox đảm
bảo event cuối cùng sẽ được phát, nhưng "cuối cùng" là không đủ với kiểm toán:
nếu tiến trình chết giữa lúc commit hành động và lúc dispatcher chạy, hành
động đã xảy ra mà vết kiểm toán tới muộn — hoặc, khi event hỏng và vào dead
letter sau 5 lần, **không bao giờ tới**.

`todo.md` đã ghi nhận cơ chế dead letter này hoạt động đúng như thiết kế. Với
event nghiệp vụ, mất một event là sự cố khôi phục được. Với audit log của
thao tác điều chỉnh sổ cái, mất vết là mất khả năng trả lời "ai đã làm việc
này" — vĩnh viễn.

Có thể xem lại sau nếu khối lượng ghi trở thành vấn đề hiệu năng thật.

### C. Ghi vào logger có sẵn — **bị loại**

`platform/logger` đã tồn tại và đã che dữ liệu nhạy cảm.

Bị loại vì log ứng dụng và vết kiểm toán có yêu cầu khác nhau về bản chất:

```text
Log ứng dụng    xoay vòng và bị xóa; tối ưu cho gỡ lỗi;
                không truy vấn được theo resource_id
Vết kiểm toán   giữ nhiều năm; phải truy vấn được theo
                người thao tác, tài nguyên, khoảng thời gian
```

Trang audit log ở `admin.md` mục 7 cần lọc theo `resource_type` và `action`
trên khoảng ngày. Đó là truy vấn database, không phải tìm kiếm văn bản log.

---

## Consequences

### Tốt

```text
✓ Hai trang Admin UI trong phạm vi đợt đầu dựng được
✓ Không module nào phải import audit — R3 giữ platform trung lập
✓ Ghi audit cùng giao dịch với hành động: không mất vết
✓ Không có đường sửa hay xóa, ở cả tầng code lẫn database
```

### Xấu

```text
✗ Mỗi lời gọi ghi audit là một lần ghi database trong giao dịch nghiệp vụ
✗ Bảng audit_log lớn dần không giới hạn — cần chính sách lưu trữ,
  nhưng CHƯA làm bây giờ: chưa có dữ liệu thật để biết tốc độ tăng
✗ Truyền resource_type dạng chuỗi mất kiểm tra kiểu lúc biên dịch;
  gõ sai "SELER" thay vì "SELLER" chỉ lộ ra khi truy vấn không thấy gì
```

Điểm xấu thứ ba là **cái giá trực tiếp của việc giữ R3**. Giảm nhẹ bằng hằng
số khai báo trong `platform/audit` — chuỗi thuần, không import module.

---

## Trade-offs

Chấp nhận **một lần ghi database thêm trong mỗi thao tác nhạy cảm** để đổi
lấy vết kiểm toán không mất được.

Đây là đánh đổi đúng vì các thao tác cần audit đều là thao tác hiếm do con
người thực hiện — duyệt seller, điều chỉnh sổ cái, xem hồ sơ khách — không
phải đường đi nóng của khách mua hàng. Chi phí rơi vào đúng chỗ chịu được nó.

Chấp nhận **mất an toàn kiểu** ở tham số `resource_type` để giữ platform
trung lập với domain. Đảo lại lựa chọn này — cho audit biết về các loại tài
nguyên nghiệp vụ — sẽ phá R3 và biến platform thành nơi mọi module phải sửa
mỗi lần thêm một loại tài nguyên.

---

## Tài liệu liên quan

- [0005-module-boundaries.md](0005-module-boundaries.md) — R3, R5
- [0008-financial-ledger.md](0008-financial-ledger.md) — tiền lệ bất biến ở tầng database
- [../06-api/admin-api.md](../06-api/admin-api.md) mục 2, 6, 7
- [../08-frontend/admin.md](../08-frontend/admin.md) mục 7
- [../10-roadmap/admin-ui-plan.md](../10-roadmap/admin-ui-plan.md)
