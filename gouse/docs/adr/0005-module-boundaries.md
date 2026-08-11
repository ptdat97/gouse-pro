# ADR-0005: Ranh giới module tường minh

**Trạng thái:** Accepted

---

## Context

[ADR-0001](0001-modular-monolith.md) quyết định dùng Modular Monolith. Nhưng "module hóa" là từ mơ hồ — nhiều dự án tự nhận là module hóa nhưng thực chất là monolith rối.

Câu hỏi: **ranh giới module được định nghĩa và thực thi như thế nào?**

Rủi ro cụ thể:

```text
- Không có ranh giới rõ ràng → sau 2 năm không tách được
- Kỷ luật con người không đủ — người mới không biết quy tắc,
  áp lực deadline làm người ta đi đường tắt
- Vi phạm nhỏ tích lũy thành không thể sửa
```

---

## Decision

**Ranh giới module được định nghĩa bằng bốn quy tắc và thực thi bằng công cụ tự động.**

### Quy tắc 1: Interface công khai là điểm vào duy nhất

```text
internal/modules/inventory/
├── public.go              ← CHỈ file này được module khác import
├── domain/                ← nội bộ
├── application/           ← nội bộ
├── infrastructure/        ← nội bộ
└── interfaces/            ← nội bộ
```

Interface công khai **không trả về domain object** — chỉ trả DTO chỉ đọc. Lý do: thay đổi nội bộ domain không phá vỡ module khác.

### Quy tắc 2: Mỗi bảng thuộc đúng một module

```text
✓ Module order đọc/ghi bảng order
✗ Module order SELECT trực tiếp từ bảng inventory_item
✗ JOIN order với inventory_item
✗ Khóa ngoại vượt ranh giới module
```

### Quy tắc 3: Không phụ thuộc vòng

Đồ thị phụ thuộc phải là DAG. Khi A cần B và B cần A:

```text
1. Đảo một chiều bằng domain event (thường dùng nhất)
2. Trích xuất phần chung xuống tầng thấp hơn
3. Xem lại ranh giới — có thể A và B là một module
```

### Quy tắc 4: Không có tầng dùng chung toàn cục

```text
CẤM: common/ · utils/ · helpers/ · services/

Code dùng chung phải phân loại rõ:
    kernel/    — khái niệm domain mọi module hiểu giống nhau
                 (Money, Percentage, ID types)
                 → rất nhỏ, thay đổi rất hiếm
    platform/  — hạ tầng TRUNG LẬP domain
                 (database, event bus, HTTP, log, metric)
                 → nếu nó nhắc tới "order" hay "seller" = đặt sai chỗ
    pkg/       — tiện ích kỹ thuật thuần
```

---

## Thực thi bằng công cụ — phần quan trọng nhất

```text
Kiểm tra trong CI, VI PHẠM LÀM CI THẤT BẠI:

1. modules/*/... chỉ import modules/*/public.go
2. modules/*/domain/ chỉ import: thư viện chuẩn, kernel/, chính nó
3. platform/... không import modules/...
4. kernel/... chỉ import thư viện chuẩn
5. Đồ thị phụ thuộc module là DAG
6. File SQL trong module X chỉ nhắc tới bảng thuộc X
7. Không tồn tại thư mục common/, utils/, helpers/, services/
```

**Nguyên tắc:** thất bại CI, **không phải cảnh báo**. Cảnh báo sẽ bị bỏ qua.

Kiểm tra phải chạy nhanh (< 30 giây) để không ai muốn bỏ qua.

---

## Alternatives

### A. Chỉ dựa vào kỷ luật và rà soát code — **bị loại**

```text
Ưu:
    + Không tốn công xây công cụ
    + Linh hoạt

Nhược (quyết định):
    − Người mới không biết quy tắc
    − Áp lực deadline làm người ta đi đường tắt
    − Người rà soát bỏ sót
    − Vi phạm nhỏ tích lũy → sau 2 năm không sửa được
```

**Kinh nghiệm chung:** kiến trúc chỉ tồn tại nếu được thực thi bằng công cụ.

### B. Tách repository cho mỗi module — **bị loại**

```text
Ưu:
    + Ranh giới vật lý tuyệt đối

Nhược:
    − Thay đổi xuyên module cần nhiều pull request
    − Quản lý phiên bản phức tạp
    − Chậm phát triển đáng kể
    − Chưa cần thiết ở quy mô hiện tại
```

### C. Dùng schema database riêng cho mỗi module — **được chọn một phần**

```text
Áp dụng: mỗi bounded context một schema PostgreSQL

Lợi ích:
    + Ranh giới hiển thị ngay trong database
    + Truy vấn JOIN qua schema khác nhìn thấy ngay
    + Khi tách service: chuyển cả schema, dễ hơn nhiều

Không áp dụng ngay: phân quyền database theo module
    → phức tạp khi vận hành monolith một kết nối
    → có thể thêm sau nếu cần
```

---

## Consequences

### Tích cực

```text
✓ Ranh giới không thể vi phạm âm thầm
✓ Người mới học được quy tắc qua lỗi CI
✓ Tách service sau này chỉ là thay cài đặt interface
✓ Module kiểm thử độc lập được
✓ Thay đổi không lan truyền ngoài kiểm soát
```

### Tiêu cực

```text
− Đầu tư ban đầu xây công cụ kiểm tra
− Một số truy vấn cần nhiều lời gọi thay vì một câu SQL
− Đôi khi cảm giác "rườm rà" khi chỉ cần đọc một trường nhỏ
```

### Xử lý vấn đề N+1

Cấm JOIN có chi phí thật. Giải pháp:

```text
1. Interface công khai LUÔN có phiên bản theo lô
   GetProductsByIDs(ids []string)  — không phải GetProduct(id) gọi 50 lần

2. Đa số trường hợp: chấp nhận, gọi hàm trong tiến trình rất nhanh

3. Báo cáo cần join phức tạp: mô hình đọc riêng, đồng bộ qua event
   → đây là chỗ CQRS có lý do chính đáng
```

---

## Quy trình phá quy tắc

Đôi khi có lý do chính đáng để vi phạm:

```text
1. Ghi lý do trong một ADR
2. Đánh dấu ngoại lệ TƯỜNG MINH trong cấu hình lint
3. Ghi điều kiện để gỡ bỏ ngoại lệ
4. Rà soát ngoại lệ mỗi quý

KHÔNG được: tắt kiểm tra toàn cục, hoặc thêm ngoại lệ không ghi lý do
```

Một ngoại lệ không ghi chép sẽ trở thành tiền lệ cho mười ngoại lệ sau.

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Đầu tư xây công cụ kiểm tra | Ranh giới thực sự tồn tại |
| Nhiều lời gọi thay vì JOIN | Khả năng tách service |
| Cảm giác rườm rà | Thay đổi an toàn trong codebase lớn |
| Interface công khai phải duy trì | Module độc lập, kiểm thử được |

---

## Dấu hiệu cần rà soát lại ranh giới

```text
⚠ Interface công khai > 30 phương thức     → module quá lớn
⚠ Hai module luôn sửa cùng nhau            → ranh giới sai
⚠ Nhiều truy vấn "chỉ đọc chút xíu"        → kỷ luật đang lỏng
⚠ kernel/ ngày càng phình                   → đang thành bãi rác
⚠ Nhiều event chỉ để đồng bộ hai module    → có thể là một module
⚠ Sửa 5 module cho một tính năng           → cắt sai trục
```

Rà soát đồ thị phụ thuộc **mỗi quý**. Kiến trúc module không tự duy trì.

---

## Tài liệu liên quan

- [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md)
- [../03-architecture/module-boundaries.md](../03-architecture/module-boundaries.md)
- [ADR-0001](0001-modular-monolith.md), [ADR-0006](0006-internal-events.md)
