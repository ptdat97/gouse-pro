# Tài liệu Kiến trúc — Fashion Commerce Platform

Đây là bộ tài liệu kiến trúc cho một **Nền tảng Thương mại Thời trang** (Fashion Commerce Platform), không phải một website thương mại điện tử thông thường.

## Nguyên tắc đọc tài liệu

Tài liệu được xây dựng theo thứ tự **nghiệp vụ trước, kỹ thuật sau**. Nếu bạn đọc lần đầu, hãy đi theo thứ tự sau:

1. `00-overview/` — Tầm nhìn, mô hình kinh doanh, từ điển thuật ngữ, nguyên tắc
2. `01-business/` — Các tác nhân nghiệp vụ và mô hình doanh thu
3. `02-domain/` — Bản đồ miền, bounded context, aggregate, entity, domain event
4. `03-architecture/` — Kiến trúc tổng thể, modular monolith, quy tắc phụ thuộc
5. `04-modules/` — Đặc tả từng module
6. `05-data/` — Kiến trúc dữ liệu, nhất quán, idempotency, audit
7. `06-api/` — Chuẩn API và các nhóm API theo đối tượng
8. `07-workflows/` — Luồng nghiệp vụ quan trọng (sequence diagram)
9. `08-frontend/` — Kiến trúc Next.js
10. `09-operations/` — Vận hành, bảo mật, quan sát hệ thống
11. `10-roadmap/` — Lộ trình MVP → Phase 4
12. `11-oss/` — Nghiên cứu mã nguồn mở và tổng hợp kiến trúc
13. `adr/` — Các quyết định kiến trúc và lý do

## Kiến trúc tổng thể (một hình duy nhất)

```text
                      NGƯỜI DÙNG
                          │
                          ▼
                    Next.js UI
              (Chỉ trình bày — Presentation only)
                          │
                    HTTP / API
                          │
                          ▼
                  ┌───────────────┐
                  │  Go Backend   │
                  │               │
                  │ Business Core │
                  │ Domain Logic  │
                  │ Application   │
                  │ API           │
                  │ Workflows     │
                  └───────────────┘
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
          Database     Storage    External APIs
```

## Tài liệu nói gì, code nói gì

> **Tài liệu mô tả kiến trúc dự định và các ranh giới. Code, test và hành vi
> thật trên production là thẩm quyền cuối cùng. Khi triển khai cho thấy một
> giả định là sai, hãy cập nhật ADR và tài liệu — đừng ép code chạy theo
> tài liệu.**

Kiến trúc phục vụ việc triển khai, không phải ngược lại. Xem nguyên tắc P17
tại [00-overview/principles.md](00-overview/principles.md).

Điều này KHÔNG có nghĩa là tài liệu tùy tiện sửa cho khớp code. Khi sửa,
phải ghi lại **vì sao** giả định ban đầu sai — đó mới là thứ có giá trị cho
người đọc sau.

**Quy tắc bất biến:**

- Next.js **không** chứa logic nghiệp vụ. Không truy cập database trực tiếp.
- Go backend là nơi duy nhất sở hữu domain logic, dữ liệu và giao dịch.
- Bắt đầu bằng **Modular Monolith**, không phải microservices.
- Mọi năng lực đều được lộ ra qua API trước (API First).

## Architecture Freeze — 14/08/2026

**Kiến trúc hiện tại ĐỦ để triển khai. Dừng thêm trừu tượng hóa.**

Từ thời điểm này, tài liệu kiến trúc chuyển sang chế độ **chỉ sửa khi
triển khai chứng minh là cần**. Quy trình bắt buộc:

```text
Architecture  →  Implementation  →  Test  →  Vấn đề THẬT  →  ADR  →  Cập nhật architecture
```

Không phải:

```text
Architecture  →  Architecture  →  Architecture  →  ...
```

### Điều KHÔNG được làm nữa

```text
✗ Thêm tầng trừu tượng khi chưa có vấn đề triển khai cụ thể
✗ Thêm module mới ngoài 28 module đã liệt kê, trừ khi có ADR
✗ Thêm mẫu kiến trúc vì "OSS khác có" (P15)
✗ Viết thêm tài liệu kiến trúc mà không có code tương ứng
```

### Điều VẪN nên làm

```text
✓ Cập nhật tài liệu khi triển khai cho thấy giả định sai (P17)
✓ Ghi ADR mới khi gặp quyết định kiến trúc THẬT trong lúc code
✓ Bổ sung chi tiết còn thiếu của module chưa triển khai
✓ Sửa mâu thuẫn giữa tài liệu và code
```

### Thước đo một thay đổi kiến trúc có chính đáng không

Trả lời được câu này thì làm, không thì đừng:

> **Vấn đề cụ thể nào trong code hiện tại buộc phải thay đổi này?**

"Sẽ cần sau này", "chuẩn hơn", "OSS khác làm thế" — đều không phải câu trả lời.

---

## Trạng thái tài liệu

| Nhóm | Trạng thái | Ghi chú |
|---|---|---|
| 00-overview | Hoàn thành | Nền tảng, cần đọc trước |
| 01-business | Hoàn thành | Định nghĩa tác nhân nghiệp vụ |
| 02-domain | Hoàn thành | Là "hợp đồng" của toàn hệ thống |
| 03-architecture | Hoàn thành | Quy tắc bắt buộc khi code |
| 04-modules | Hoàn thành | 28 module (MVP 16 · Phase 2 +7 · Phase 3 +5) |
| 05-data | Hoàn thành | |
| 06-api | Hoàn thành | |
| 07-workflows | Hoàn thành | |
| 08-frontend | Hoàn thành | |
| 09-operations | Hoàn thành | |
| 10-roadmap | Hoàn thành | |
| 11-oss | Hoàn thành | 12 dự án + 2 mô hình kinh doanh |
| adr | Hoàn thành | 10 ADR — ADR-0006 và ADR-0010 có mục "trạng thái triển khai" |

**Đối chiếu với code (14/08/2026):** 10/16 module MVP đã triển khai
(catalog · product · pricing · inventory · seller · marketplace · payment ·
order · cart · checkout). Chi tiết ở
[10-roadmap/todo.md](10-roadmap/todo.md).

Ba chỗ tài liệu đã được sửa cho khớp code thật, kèm lý do:

| Tài liệu | Giả định ban đầu | Thực tế | Ghi ở |
|---|---|---|---|
| ADR-0010 | Dùng `sqlc` sinh code | `pgx` + SQL viết tay | ADR-0010 mục cuối |
| 05-data | Định danh kiểu `UUID` | `TEXT` + ULID có tiền tố | data-model.md mục 1.1 |
| 05-data | Tách 7 schema PostgreSQL | Dùng `public`, hoãn tới khi tách service | database.md mục 3 |

## Đặc tả OpenAPI

Hợp đồng API nằm ở [`/api/openapi.yaml`](../api/README.md) — **nguồn sự thật duy nhất**, được cập nhật cùng pull request với thay đổi code.

```text
62 operation · 0 lỗi lint · sinh kiểu TypeScript thành công
```

## Nghiên cứu mã nguồn mở

Xem [11-oss/synthesis.md](11-oss/synthesis.md) — tổng hợp sau khi nghiên cứu 12 dự án OSS và 2 mô hình kinh doanh. Nghiên cứu **xác nhận** phần lớn thiết kế và **thay đổi ba điểm**: link table, Adjustment, cơ sở tính hoa hồng creator.

## Tổng hợp bàn giao

Xem [10-roadmap/deliverables.md](10-roadmap/deliverables.md) — tổng hợp toàn bộ bản đồ, ma trận, rủi ro và thứ tự triển khai đề xuất, kèm kết quả rà soát tính nhất quán của tài liệu.

## Tiến độ triển khai

Xem [10-roadmap/todo.md](10-roadmap/todo.md) — việc đã làm và việc còn lại theo 7 giai đoạn, đối chiếu với tiêu chí hoàn thành MVP. Cập nhật sau mỗi module.
