# Architecture Decision Records (ADR)

## Mục đích

ADR ghi lại **vì sao** một quyết định kiến trúc được đưa ra, không chỉ **quyết định là gì**.

Giá trị thật của ADR xuất hiện sau 1–2 năm, khi có người hỏi "sao lại làm thế này?" — và người quyết định ban đầu đã không còn ở dự án.

## Khung chuẩn

Mỗi ADR gồm:

```text
Context      — bối cảnh, vấn đề cần giải quyết
Decision     — quyết định
Alternatives — các phương án đã cân nhắc và vì sao bị loại
Consequences — hệ quả (cả tốt và xấu)
Trade-offs   — đánh đổi chấp nhận
```

## Trạng thái

```text
Proposed   — đang đề xuất
Accepted   — đã chấp nhận, đang áp dụng
Superseded — đã bị thay thế bởi ADR khác
Deprecated — không còn áp dụng
```

## Chỉ mục

| # | Quyết định | Trạng thái | Ảnh hưởng |
|---|---|---|---|
| [0001](0001-modular-monolith.md) | Bắt đầu bằng Modular Monolith | Accepted | Toàn hệ thống |
| [0002](0002-api-first.md) | API First | Accepted | Toàn hệ thống |
| [0003](0003-go-backend.md) | Go cho backend | Accepted | Backend |
| [0004](0004-nextjs-frontend.md) | Next.js cho frontend | Accepted | Frontend |
| [0005](0005-module-boundaries.md) | Ranh giới module tường minh | Accepted | Toàn hệ thống |
| [0006](0006-internal-events.md) | Domain event nội bộ + Outbox | Accepted | Toàn hệ thống |
| [0007](0007-marketplace-order-model.md) | Offer và tách Order/FulfillmentOrder | Accepted | Commerce, Marketplace |
| [0008](0008-financial-ledger.md) | Sổ cái bất biến | Accepted | Financial |
| [0009](0009-service-extraction.md) | Hoãn tách service | Accepted | Toàn hệ thống |
| [0010](0010-database-layer.md) | PostgreSQL + sqlc cho tầng dữ liệu | Accepted | Toàn hệ thống |
| [0011](0011-audit-log.md) | Audit log là năng lực platform | Accepted | Toàn hệ thống |

## Quy trình thêm ADR mới

```text
1. Đánh số tiếp theo
2. Viết theo khung chuẩn
3. Rà soát với đội
4. Cập nhật chỉ mục này
5. Nếu thay thế ADR cũ → đánh dấu ADR cũ là Superseded, liên kết hai chiều
```

**Khi nào cần ADR:** quyết định khó đảo ngược, ảnh hưởng nhiều module, hoặc có phương án thay thế hợp lý bị loại. Không cần ADR cho quyết định nhỏ, dễ đổi.
