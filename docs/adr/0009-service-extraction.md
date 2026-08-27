# ADR-0009: Hoãn tách service

**Trạng thái:** Accepted

---

## Context

[ADR-0001](0001-modular-monolith.md) quyết định bắt đầu bằng Modular Monolith, đồng thời giữ mở khả năng tách service sau này.

Câu hỏi tiếp theo: **khi nào tách, tách cái gì, và ai quyết định?**

Rủi ro nếu không có quy tắc rõ ràng:

```text
- Tách quá sớm → độ phức tạp không cần thiết, giao dịch phân tán
- Tách vì lý do sai → "microservices là chuẩn hiện đại"
- Tách sai module → tạo hệ thống rối hơn monolith
- Không bao giờ tách → bỏ lỡ khi thật sự cần
```

---

## Decision

**Không định nghĩa microservices từ trước. Mọi lần tách phải có lý do được ghi chép bằng một ADR riêng.**

### Nguyên tắc nền tảng

> Modular Monolith **không phải giai đoạn tạm bợ** chờ ngày lên microservices. Nó là kiến trúc hợp lệ và duy trì được lâu dài.
>
> Tách service là **giải pháp cho một vấn đề cụ thể**, không phải mục tiêu tự thân.

### Năm câu hỏi bắt buộc trả lời trước khi tách

```text
1. Vấn đề cụ thể đang gặp là gì?
   → phải có SỐ LIỆU, không phải cảm giác
   → "API chậm" KHÔNG đủ
   → "p99 của endpoint X là 3s, do truy vấn Y" MỚI đủ

2. Đã thử các cách rẻ hơn chưa?
   → thêm chỉ mục · tối ưu truy vấn · thêm cache
   → tách tiến trình worker · mở rộng theo chiều dọc
   → nếu chưa thử, THỬ TRƯỚC

3. Module này có thật sự độc lập không?
   → interface công khai ổn định chưa?
   → bao nhiêu module gọi nó?
   → có bảng nào bị module khác cần join không?

4. Ai vận hành nó sau khi tách?
   → có đội chịu trách nhiệm không?
   → có năng lực giám sát, gỡ lỗi phân tán không?

5. Chấp nhận được cái giá gì?
   → độ trễ mạng · nhất quán cuối · triển khai phức tạp
   → gỡ lỗi khó · xử lý lỗi mạng ở mọi lời gọi
```

**Nếu không trả lời được câu 1 bằng số liệu, không tách.**

### Sáu lý do chính đáng

| # | Lý do | Ví dụ |
|---|---|---|
| 1 | Quy mô | Module chiếm phần lớn tài nguyên, cần mở rộng riêng |
| 2 | Triển khai độc lập | Module thay đổi rất thường xuyên |
| 3 | Tải dữ liệu khác biệt | Analytics ghi hàng triệu bản ghi/giờ |
| 4 | Sở hữu theo đội | Đội độc lập cần tự chủ hoàn toàn |
| 5 | Cách ly vận hành | Lỗi module này không được làm sập phần còn lại |
| 6 | Chuyên biệt công nghệ | Cần ngôn ngữ khác (ví dụ Python cho ML) |

### Lý do KHÔNG chính đáng

```text
✗ "Microservices là chuẩn hiện đại"
✗ "Monolith nghe cũ"
✗ "Để dễ mở rộng sau này" (chưa có vấn đề)
✗ "Đội muốn thử công nghệ mới"
✗ "Codebase to quá" (giải bằng module hóa tốt hơn)
```

---

## Thứ tự ứng viên tách

### Nhóm 1 — Dễ, rủi ro thấp

| Module | Lý do | Điều kiện kích hoạt |
|---|---|---|
| Media Processing | Tốn CPU, tải biến động, không có trạng thái nghiệp vụ | Xử lý video ảnh hưởng API |
| Search | Hạ tầng riêng, mô hình dữ liệu riêng | Chỉ mục lớn, truy vấn nhiều |
| Notification | Không có trạng thái nghiệp vụ, chỉ nhận event | Khối lượng gửi lớn |
| Analytics | Tải ghi rất lớn, không cần nhất quán tức thời | Ảnh hưởng database chính |

**Vì sao dễ:** chúng ở tầng nền của đồ thị phụ thuộc, không module nào gọi ngược lại.

### Nhóm 2 — Cần cân nhắc

| Module | Rủi ro |
|---|---|
| Recommendation | Cần dữ liệu từ nhiều module |
| Content | Ghép nối với affiliate |
| **Inventory** | **Rất nhiều module phụ thuộc** |

**Về Inventory:** hấp dẫn (tải ghi cao, tranh chấp lớn) nhưng rủi ro nhất — `cart`, `checkout`, `order`, `fulfillment`, `warehouse` đều gọi nó. Tách biến mọi lời gọi trong tiến trình thành lời gọi mạng.

### Không nên tách

```text
order            — trung tâm mọi luồng, ghép nối chặt
cart, checkout   — vòng đời ngắn, gắn liền order
catalog, product — được đọc bởi hầu hết module
identity         — tách ra tạo độ trễ cho MỌI request
```

### Không bao giờ tách khỏi nhau

```text
order + order_line                 — cùng aggregate
ledger_entry + ledger_line         — bất biến Σ debit = Σ credit
inventory_item + reservation       — tranh chấp cần khóa cùng nơi
payment + ledger                   — ghi nhận và ghi sổ phải nguyên tử
```

Tách những thứ này buộc phải dùng giao dịch phân tán — độ phức tạp tăng vọt, rủi ro sai sót tài chính rất cao.

---

## Quy trình tách (bảy bước)

```text
1. Củng cố ranh giới
   Rà soát interface công khai, xóa truy cập dữ liệu trực tiếp còn sót,
   viết kiểm thử hợp đồng đầy đủ

2. Tách dữ liệu
   Chuyển bảng sang schema riêng, xóa khóa ngoại vượt ranh giới

3. Đo lường TRƯỚC khi tách
   Ghi nhận số liệu hiện tại làm cơ sở so sánh

4. Thêm lớp truyền tải
   GIỮ NGUYÊN interface công khai
   Thêm cài đặt gọi qua mạng bên cạnh cài đặt trong tiến trình
   Chuyển đổi bằng cờ cấu hình

5. Chạy song song
   Cho một phần lưu lượng qua service mới, so sánh kết quả

6. Chuyển hoàn toàn
   Tăng dần lưu lượng, sẵn sàng quay lui

7. Dọn dẹp
   Xóa code cũ, cập nhật tài liệu
```

**Điểm mấu chốt ở bước 4:** vì mọi module đã giao tiếp qua interface, đổi từ gọi hàm sang gọi mạng chỉ là **thay cài đặt interface** — không sửa logic nghiệp vụ.

Đây chính là lý do phải giữ kỷ luật ranh giới từ ngày đầu ([ADR-0005](0005-module-boundaries.md)).

---

## Alternatives

### A. Định nghĩa sẵn kiến trúc microservices mục tiêu — **bị loại**

```text
Ưu:
    + Có đích rõ ràng
    + Đội biết hướng đi

Nhược (quyết định):
    − Ranh giới domain chưa ổn định → đích có thể sai
    − Tạo áp lực tách dù chưa cần
    − Quyết định trước khi có thông tin
```

### B. Không bao giờ tách — **bị loại**

```text
Nhược: bỏ lỡ khi thật sự có vấn đề quy mô hoặc cần
       chuyên biệt công nghệ (ví dụ ML)
```

### C. Tách theo lịch trình định sẵn — **bị loại**

```text
Ví dụ: "Phase 3 tách search và notification"

Nhược: tách theo lịch, không theo nhu cầu
       → có thể tách khi chưa cần, hoặc chưa tách khi đã cần
```

---

## Consequences

### Tích cực

```text
✓ Không tốn công tách khi chưa cần
✓ Mỗi lần tách đều có lý do rõ ràng, đo được
✓ Ranh giới domain được hiểu rõ hơn trước khi cố định
✓ Giữ được sự đơn giản càng lâu càng tốt
```

### Tiêu cực

```text
− Có thể tách muộn hơn lý tưởng trong một số trường hợp
− Cần kỷ luật từ chối áp lực "làm microservices"
```

---

## Chi phí phải hiểu rõ trước

| Chi phí | Trong monolith | Sau khi tách |
|---|---|---|
| Gọi module khác | ~microsecond | ~milisecond + có thể lỗi |
| Giao dịch | Một giao dịch database | Nhất quán cuối, cần bù trừ |
| Gỡ lỗi | Một stack trace | Truy vết phân tán |
| Triển khai | Một artifact | Nhiều artifact, quản lý phiên bản |
| Kiểm thử | Trong bộ nhớ | Cần môi trường tích hợp |
| Xử lý lỗi | Lỗi hoặc không | Timeout, thử lại, ngắt mạch |

**Ví dụ cụ thể:** trang sản phẩm cần dữ liệu từ `product`, `marketplace`, `inventory`, `content`.

```text
Monolith:      4 lời gọi hàm  ≈ 0,1ms tổng
Sau khi tách:  4 lời gọi mạng ≈ 20–40ms + 4 điểm có thể hỏng
```

---

## Chuẩn bị đã làm sẵn

Những việc này có giá trị **ngay lập tức** trong monolith, không chỉ để chuẩn bị tương lai:

```text
✓ Interface công khai cho mọi module
✓ Không join vượt ranh giới module
✓ Event contract thiết kế như thể vượt tiến trình
✓ Transactional Outbox
✓ Không khóa ngoại vượt module
✓ Định danh ULID/UUID
✓ Idempotency ở mọi lệnh ghi
✓ Distributed tracing từ đầu
✓ Kiểm thử hợp đồng
```

Đây là lý do chúng đáng làm — không phải đầu cơ vào tương lai chưa chắc xảy ra.

---

## Dấu hiệu KHÔNG nên tách

```text
✗ Module cần tách phải gọi ngược monolith nhiều lần cho một thao tác
   → ranh giới sai, tách sẽ tạo hệ thống rối hơn

✗ Không tách được dữ liệu vì nhiều truy vấn join
   → sửa ranh giới dữ liệu trước

✗ Interface công khai thay đổi liên tục
   → hợp đồng chưa ổn định

✗ Không ai muốn nhận trách nhiệm vận hành
   → sẽ thành service mồ côi
```

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Có thể tách muộn hơn lý tưởng | Không tách sai, không tách thừa |
| Cần kỷ luật từ chối áp lực | Giữ sự đơn giản |
| Phải đo lường trước khi quyết định | Quyết định dựa trên dữ liệu |

---

## Mẫu ADR cho mỗi lần tách

```text
Module nào được tách
Vấn đề cụ thể, kèm SỐ LIỆU
Các cách rẻ hơn đã thử và kết quả
Lý do thuộc nhóm nào trong sáu lý do
Chi phí chấp nhận
Kế hoạch quay lui
Cách đo lường thành công
```

---

## Tài liệu liên quan

- [../03-architecture/evolution-to-services.md](../03-architecture/evolution-to-services.md)
- [ADR-0001](0001-modular-monolith.md), [ADR-0005](0005-module-boundaries.md)
- [../10-roadmap/scale.md](../10-roadmap/scale.md)
