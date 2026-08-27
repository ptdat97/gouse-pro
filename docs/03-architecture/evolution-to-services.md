# Chiến lược tiến hóa sang dịch vụ

## 1. Nguyên tắc nền tảng

> **Không định nghĩa microservices từ trước. Mọi lần tách phải có lý do được ghi chép.**

Modular monolith không phải giai đoạn tạm bợ chờ ngày lên microservices. Nó là kiến trúc **hợp lệ và có thể duy trì lâu dài**. Nhiều hệ thống quy mô lớn chạy tốt như monolith được module hóa.

Việc tách service là **giải pháp cho một vấn đề cụ thể**, không phải mục tiêu tự thân.

---

## 2. Ba giai đoạn kiến trúc

### Giai đoạn 1 — MVP

```text
        Next.js
           │
           ▼
    Go Modular Monolith
           │
           ▼
       PostgreSQL
```

Một tiến trình API. Một database. Đơn giản nhất có thể.

### Giai đoạn 2 — Tách theo tải công việc

```text
        Next.js
           │
           ▼
    ┌──────────────────────────────┐
    │   Go Modular Monolith        │
    │   (cùng codebase)            │
    │                              │
    │   cmd/api      — HTTP        │
    │   cmd/worker   — tác vụ nền  │
    └──────────────────────────────┘
           │           │
           ▼           ▼
      PostgreSQL   ┌─────────────┐
                   │ Search index│
                   │ Object store│
                   │ Cache       │
                   └─────────────┘
```

**Quan trọng:** đây **không phải** microservices. Cùng codebase, cùng phiên bản, chỉ khác điểm khởi chạy.

Lý do tách tiến trình:

```text
- Tác vụ nền nặng không được làm chậm request của khách
- Xử lý outbox cần chạy liên tục, độc lập với lưu lượng HTTP
- Job định kỳ (tổng hợp tín hiệu nhu cầu, tạo báo cáo) chạy theo lịch
```

Thêm hạ tầng chuyên biệt:

| Hạ tầng | Khi nào cần | Vì sao |
|---|---|---|
| Chỉ mục tìm kiếm | Khi tìm kiếm bằng SQL chậm hoặc thiếu tính năng | Tìm kiếm toàn văn, lọc theo nhiều thuộc tính |
| Object storage | Ngay từ đầu | Ảnh và video không lưu trong database |
| Cache | Khi có điểm nóng đo được | Giảm tải database |
| Xử lý media | Khi có nhiều video | Chuyển mã video tốn CPU |

### Giai đoạn 3 — Trích xuất có chọn lọc

```text
        Next.js
           │
           ▼
    ┌──────────────┐
    │  API Gateway │ (nếu cần)
    └──────────────┘
       │        │
       ▼        ▼
  ┌─────────┐  ┌──────────────┐
  │ Modular │  │ Dịch vụ được │
  │Monolith │◄─┤ trích xuất   │
  │ (lõi)   │  │              │
  └─────────┘  └──────────────┘
```

Chỉ tách những module có lý do rõ ràng.

---

## 3. Ứng viên tách theo thứ tự ưu tiên

### Nhóm 1 — Dễ tách nhất, ít rủi ro

| Module | Lý do tách | Điều kiện kích hoạt |
|---|---|---|
| **Media Processing** | Tốn CPU, tải biến động mạnh, không có trạng thái nghiệp vụ | Khi xử lý video làm ảnh hưởng API |
| **Search** | Yêu cầu hạ tầng riêng, mô hình dữ liệu riêng | Khi chỉ mục lớn và truy vấn nhiều |
| **Notification** | Không có trạng thái nghiệp vụ, chỉ nhận event | Khi khối lượng gửi lớn |
| **Analytics** | Tải ghi rất lớn, không cần nhất quán tức thời | Khi ghi analytics ảnh hưởng database chính |

**Vì sao nhóm này dễ:** chúng ở tầng nền của đồ thị phụ thuộc, không module nào gọi ngược lại chúng. Tách chúng gần như không ảnh hưởng phần còn lại.

### Nhóm 2 — Cần cân nhắc

| Module | Lý do tách | Rủi ro |
|---|---|---|
| **Recommendation** | Có thể cần hạ tầng ML, ngôn ngữ khác | Cần dữ liệu từ nhiều module |
| **Content** | Tải đọc lớn, mô hình dữ liệu khác biệt | Ghép nối với affiliate |
| **Inventory** | Tải ghi cao, cần mở rộng riêng | **Rất nhiều module phụ thuộc** |

**Về Inventory:** đây là ứng viên hấp dẫn (tải ghi cao, tranh chấp lớn) nhưng cũng rủi ro nhất, vì `cart`, `checkout`, `order`, `fulfillment`, `warehouse` đều gọi nó. Tách nó biến mọi lời gọi trong tiến trình thành lời gọi mạng — thêm độ trễ và điểm hỏng.

### Nhóm 3 — Chỉ tách khi thật sự cần

| Module | Điều kiện |
|---|---|
| **Supply Chain** | Khi có đội riêng và mô hình dữ liệu đủ độc lập |
| **Financial/Ledger** | Khi có yêu cầu tuân thủ đòi hỏi cách ly |

### Không nên tách

| Module | Vì sao |
|---|---|
| `order` | Trung tâm của mọi luồng, ghép nối chặt với nhiều module |
| `cart`, `checkout` | Vòng đời ngắn, gắn liền với order |
| `catalog`, `product` | Được đọc bởi hầu hết module |
| `identity` | Tách ra tạo độ trễ cho mọi request |

---

## 4. Điều kiện bắt buộc trước khi tách bất kỳ module nào

Mọi lần tách phải trả lời được **tất cả** câu hỏi sau:

```text
1. Vấn đề cụ thể đang gặp là gì?
   → phải có SỐ LIỆU, không phải cảm giác
   → "API chậm" không đủ. "p99 của endpoint X là 3s, do truy vấn Y" mới đủ.

2. Đã thử các cách rẻ hơn chưa?
   → thêm chỉ mục database
   → tối ưu truy vấn
   → thêm cache
   → tách tiến trình worker
   → mở rộng theo chiều dọc (máy mạnh hơn)
   → nếu chưa thử, THỬ TRƯỚC

3. Module này có thật sự độc lập không?
   → interface công khai ổn định chưa?
   → có bao nhiêu module gọi nó?
   → có bảng nào bị module khác cần join không?

4. Ai vận hành nó sau khi tách?
   → có đội chịu trách nhiệm không?
   → có năng lực giám sát, gỡ lỗi phân tán không?

5. Chấp nhận được cái giá gì?
   → độ trễ mạng
   → nhất quán cuối thay vì tức thời
   → triển khai phức tạp hơn
   → gỡ lỗi khó hơn
   → cần xử lý lỗi mạng ở mọi lời gọi
```

**Nếu không trả lời được câu 1 bằng số liệu, không tách.**

---

## 5. Sáu lý do chính đáng để tách

Mọi ADR về trích xuất service phải nêu rõ lý do thuộc nhóm nào:

| # | Lý do | Ví dụ cụ thể |
|---|---|---|
| 1 | **Quy mô** | Module chiếm phần lớn tài nguyên, cần mở rộng riêng |
| 2 | **Triển khai độc lập** | Module thay đổi rất thường xuyên, cần phát hành riêng |
| 3 | **Tải dữ liệu khác biệt** | Analytics ghi hàng triệu bản ghi/giờ, làm chậm database giao dịch |
| 4 | **Sở hữu theo đội** | Một đội độc lập cần tự chủ hoàn toàn |
| 5 | **Cách ly vận hành** | Lỗi ở module này không được làm sập phần còn lại |
| 6 | **Chuyên biệt công nghệ** | Cần ngôn ngữ/runtime khác (ví dụ Python cho ML) |

**Lý do KHÔNG chính đáng:**

```text
✗ "Microservices là chuẩn hiện đại"
✗ "Monolith nghe cũ"
✗ "Để dễ mở rộng sau này" (chưa có vấn đề)
✗ "Đội muốn thử công nghệ mới"
✗ "Codebase to quá" (giải bằng module hóa tốt hơn, không phải tách)
```

---

## 6. Quy trình tách một module

Khi đã quyết định tách, làm theo bảy bước:

```text
Bước 1 — Củng cố ranh giới
    - Rà soát interface công khai, đảm bảo đầy đủ
    - Xóa mọi truy cập dữ liệu trực tiếp còn sót
    - Viết kiểm thử hợp đồng đầy đủ

Bước 2 — Tách dữ liệu
    - Chuyển bảng của module sang schema riêng
    - Xóa mọi khóa ngoại vượt ranh giới
    - Thay join bằng lời gọi interface

Bước 3 — Đo lường trước khi tách
    - Ghi nhận số liệu hiệu năng hiện tại
    - Để có cơ sở so sánh sau khi tách

Bước 4 — Thêm lớp truyền tải
    - Giữ nguyên interface công khai
    - Thêm cài đặt gọi qua HTTP/gRPC bên cạnh cài đặt trong tiến trình
    - Chuyển đổi bằng cờ cấu hình

Bước 5 — Chạy song song
    - Cho một phần lưu lượng đi qua service mới
    - So sánh kết quả
    - Theo dõi độ trễ, tỷ lệ lỗi

Bước 6 — Chuyển hoàn toàn
    - Tăng dần lưu lượng
    - Sẵn sàng quay lui

Bước 7 — Dọn dẹp
    - Xóa code cũ trong monolith
    - Cập nhật tài liệu
```

**Điểm mấu chốt ở bước 4:** vì mọi module đã giao tiếp qua interface, việc đổi từ gọi hàm sang gọi mạng chỉ là **thay cài đặt interface** — không sửa logic nghiệp vụ.

Đây chính là lý do phải giữ kỷ luật ranh giới nghiêm ngặt từ ngày đầu.

---

## 7. Cái giá phải trả — cần hiểu rõ trước

| Chi phí | Trong monolith | Sau khi tách |
|---|---|---|
| Gọi module khác | ~microsecond | ~milisecond + có thể lỗi |
| Giao dịch | Một giao dịch database | Nhất quán cuối, cần bù trừ |
| Gỡ lỗi | Một stack trace | Truy vết phân tán |
| Triển khai | Một artifact | Nhiều artifact, cần quản lý phiên bản |
| Kiểm thử | Chạy toàn bộ trong bộ nhớ | Cần môi trường tích hợp |
| Xử lý lỗi | Lỗi hoặc không lỗi | Timeout, thử lại, ngắt mạch |
| Thay đổi schema | Một migration | Điều phối nhiều service |

**Ví dụ cụ thể về chi phí:** trang sản phẩm cần dữ liệu từ `product`, `marketplace`, `inventory`, `content`.

```text
Trong monolith:  4 lời gọi hàm  ≈ 0,1ms tổng
Sau khi tách:    4 lời gọi mạng ≈ 20–40ms tổng, và 4 điểm có thể hỏng
```

Với trang có lưu lượng cao, đây là khác biệt lớn. Cần cân nhắc kỹ.

---

## 8. Cái gì KHÔNG BAO GIỜ được tách khỏi nhau

Một số nhóm phải ở cùng nhau vì cần nhất quán giao dịch:

```text
KHÔNG tách:
    order + order_line
    → cùng aggregate

    ledger_entry + ledger_line
    → bất biến Σ debit = Σ credit phải giữ trong một giao dịch

    inventory_item + reservation
    → tranh chấp đồng thời cần khóa cùng nơi

    payment + ledger
    → ghi nhận thanh toán và ghi sổ phải nguyên tử
```

Nếu tách những thứ này, phải dùng giao dịch phân tán — độ phức tạp tăng vọt và rủi ro sai sót tài chính rất cao.

---

## 9. Chuẩn bị sẵn từ bây giờ

Những việc làm ngay ở giai đoạn monolith để việc tách sau này dễ dàng:

| Việc | Đã có trong thiết kế |
|---|---|
| Interface công khai cho mọi module | [module-boundaries.md](module-boundaries.md) |
| Không join qua ranh giới module | [dependency-rules.md](dependency-rules.md) |
| Event contract như thể vượt tiến trình | [../02-domain/domain-events.md](../02-domain/domain-events.md) |
| Outbox pattern | [../02-domain/domain-events.md](../02-domain/domain-events.md) mục 7 |
| Không khóa ngoại vượt module | [../05-data/data-model.md](../05-data/data-model.md) |
| Định danh dùng ULID/UUID | [../02-domain/entities.md](../02-domain/entities.md) mục 7 |
| Idempotency ở mọi lệnh ghi | [../05-data/idempotency.md](../05-data/idempotency.md) |
| Truy vết phân tán từ đầu | [../09-operations/observability.md](../09-operations/observability.md) |
| Kiểm thử hợp đồng | [modular-monolith.md](modular-monolith.md) mục 9 |

**Nhận xét quan trọng:** mọi việc trong bảng trên đều có giá trị **ngay lập tức** trong monolith, không chỉ để chuẩn bị cho tương lai. Đây là lý do chúng đáng làm — không phải đầu cơ vào một tương lai chưa chắc xảy ra.

---

## 10. Dấu hiệu KHÔNG nên tách

Nếu thấy những điều sau, dừng lại và xem lại:

```text
✗ Module cần tách phải gọi ngược lại monolith nhiều lần cho một thao tác
   → ranh giới sai, tách sẽ tạo hệ thống rối hơn

✗ Không tách được dữ liệu vì có nhiều truy vấn join
   → chưa đủ điều kiện, sửa ranh giới dữ liệu trước

✗ Interface công khai thay đổi liên tục
   → hợp đồng chưa ổn định, chờ thêm

✗ Không ai muốn nhận trách nhiệm vận hành nó
   → sẽ thành service mồ côi

✗ Lý do duy nhất là "codebase to quá"
   → giải bằng tổ chức code tốt hơn
```

---

## 11. Ghi chép quyết định

Mỗi lần tách phải tạo một ADR mới theo mẫu [../adr/0009-service-extraction.md](../adr/0009-service-extraction.md), nêu rõ:

```text
- Module nào được tách
- Vấn đề cụ thể, kèm SỐ LIỆU
- Các cách rẻ hơn đã thử và kết quả
- Lý do thuộc nhóm nào trong sáu lý do ở mục 5
- Chi phí chấp nhận
- Kế hoạch quay lui
- Cách đo lường thành công
```

---

## 12. Tài liệu liên quan

- [../adr/0001-modular-monolith.md](../adr/0001-modular-monolith.md) — vì sao bắt đầu bằng monolith
- [../adr/0009-service-extraction.md](../adr/0009-service-extraction.md) — quyết định hoãn tách service
- [modular-monolith.md](modular-monolith.md) — cấu trúc module hiện tại
- [../10-roadmap/scale.md](../10-roadmap/scale.md) — chiến lược mở rộng quy mô
