# ADR-0003: Go cho backend

**Trạng thái:** Accepted

---

## Context

Cần chọn ngôn ngữ cho backend — nơi chứa toàn bộ domain logic, xử lý giao dịch tài chính, và điều phối nghiệp vụ.

Yêu cầu cụ thể của hệ thống:

```text
- Xử lý đồng thời cao (đặc biệt tồn kho khi live commerce)
- Độ trễ thấp và ổn định (ảnh hưởng tỷ lệ chuyển đổi)
- Tính đúng đắn tài chính tuyệt đối
- Codebase lớn, nhiều người, nhiều năm
- Cần ranh giới module thực thi được bằng công cụ
- Triển khai đơn giản
```

---

## Decision

**Dùng Go cho backend.**

---

## Alternatives

### A. Go — **được chọn**

```text
Ưu:
    + Mô hình đồng thời (goroutine) phù hợp với xử lý nhiều request
      và tranh chấp tồn kho cao
    + Độ trễ ổn định, thời gian GC ngắn và dự đoán được
    + Biên dịch ra một file nhị phân → triển khai cực đơn giản
    + Kiểu tĩnh → bắt lỗi sớm, refactor an toàn trong codebase lớn
    + `internal/` chặn import ở mức TRÌNH BIÊN DỊCH
      → hỗ trợ trực tiếp ranh giới module
    + Ngôn ngữ đơn giản, ít cách viết → code nhất quán giữa nhiều người
    + Công cụ phân tích tĩnh tốt → dễ viết kiểm tra ranh giới tùy chỉnh
    + Thư viện chuẩn mạnh cho HTTP, database

Nhược:
    − Ít trừu tượng hóa hơn các ngôn ngữ khác (nhiều code lặp)
    − Hệ sinh thái ORM và framework kém phong phú hơn
    − Xử lý lỗi dài dòng
```

**Ưu điểm quyết định:** `internal/` và công cụ phân tích tĩnh cho phép **thực thi ranh giới module bằng công cụ**, không chỉ bằng kỷ luật. Đây là điều kiện sống còn cho modular monolith (xem [ADR-0001](0001-modular-monolith.md)).

### B. Java/Kotlin — **bị loại**

```text
Ưu:
    + Hệ sinh thái rất mạnh cho hệ thống doanh nghiệp
    + ORM và framework trưởng thành
    + Công cụ kiểm tra kiến trúc có sẵn

Nhược:
    − Thời gian khởi động chậm hơn
    − Tiêu thụ bộ nhớ cao hơn
    − GC có thể gây độ trễ đột biến (quan trọng với live commerce)
    − Triển khai phức tạp hơn (JVM, cấu hình)
```

Không phải lựa chọn sai — chỉ là Go phù hợp hơn với ưu tiên độ trễ ổn định và triển khai đơn giản.

### C. Node.js/TypeScript — **bị loại**

```text
Ưu:
    + Cùng ngôn ngữ với frontend → chia sẻ kiểu dữ liệu
    + Hệ sinh thái lớn
    + Tuyển dụng dễ

Nhược (quyết định):
    − Đơn luồng → xử lý tính toán nặng kém
    − Số dấu chấm động là mặc định → RỦI RO CAO với tiền
    − Kiểu động ở runtime (TypeScript chỉ kiểm tra lúc biên dịch)
    − Khó thực thi ranh giới module bằng công cụ
```

**Lý do loại chính:** rủi ro tính toán tiền. Với JavaScript, `0.1 + 0.2 !== 0.3`. Dù có thể dùng thư viện số thập phân, nguy cơ ai đó vô tình dùng `number` cho tiền là rất cao trong codebase lớn.

Hệ thống này giữ tiền hộ seller và creator — độ lệch đối soát phải bằng 0.

### D. Python — **bị loại**

```text
Ưu:
    + Phát triển nhanh
    + Mạnh cho phân tích dữ liệu và học máy

Nhược:
    − Hiệu năng thấp hơn đáng kể
    − GIL hạn chế đồng thời
    − Kiểu động → khó bảo trì codebase lớn
```

**Lưu ý:** Python vẫn có thể dùng cho **dịch vụ gợi ý/ML tách riêng** ở Phase 4 — đó là lý do chính đáng để tách service (chuyên biệt công nghệ). Xem [ADR-0009](0009-service-extraction.md).

---

## Consequences

### Tích cực

```text
✓ Ranh giới module thực thi được bằng trình biên dịch + công cụ
✓ Độ trễ ổn định, phù hợp yêu cầu thương mại
✓ Xử lý tốt tranh chấp tồn kho cao (live commerce)
✓ Triển khai một file nhị phân — đơn giản, ít lỗi cấu hình
✓ Refactor an toàn nhờ kiểu tĩnh
✓ Code nhất quán giữa nhiều lập trình viên
```

### Tiêu cực

```text
− Nhiều code lặp hơn (xử lý lỗi, chuyển đổi kiểu)
− Cần tự viết nhiều thứ mà framework khác có sẵn
− Tuyển dụng khó hơn Node.js/Java ở một số thị trường
```

### Biện pháp

```text
Chấp nhận code lặp — nguyên tắc P16: thà rõ ràng còn hơn ngắn gọn.
Với codebase nhiều người, nhiều năm, code tường minh dễ bảo trì hơn
trừu tượng hóa thông minh.
```

---

## Quyết định kèm theo: Money là kiểu riêng

```go
type Money struct {
    amount   int64    // đơn vị nhỏ nhất
    currency Currency
}
```

**Cấm tuyệt đối dùng `float64` cho tiền.** Kiểm tra bằng lint trong CI.

Xem [../02-domain/value-objects.md](../02-domain/value-objects.md) mục 2.

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Code dài dòng hơn | Rõ ràng, dễ bảo trì lâu dài |
| Hệ sinh thái framework kém phong phú | Kiểm soát, ít phụ thuộc bất ngờ |
| Không chia sẻ kiểu với frontend trực tiếp | Sinh kiểu TypeScript từ OpenAPI thay thế |
| Tuyển dụng hẹp hơn | Chất lượng code nhất quán |

---

## Tài liệu liên quan

- [ADR-0001](0001-modular-monolith.md) — Go hỗ trợ ranh giới module
- [ADR-0005](0005-module-boundaries.md)
- [../03-architecture/architecture.md](../03-architecture/architecture.md)
