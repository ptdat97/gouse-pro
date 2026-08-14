# Medusa

| | |
|---|---|
| Repository | `github.com/medusajs/medusa` |
| License | **MIT + phần Enterprise theo giấy phép riêng** |
| Sao / Fork | 35.728 / 5.062 |
| Ngôn ngữ | TypeScript |
| Cập nhật cuối | 2026-08-11 (rất tích cực) |
| Vai trò | **Nguồn ý tưởng quan trọng nhất trong nhóm non-Go** |

---

## 1. Lưu ý license trước

Repository là MIT **trừ** các phần Enterprise Edition được nêu trong `ENTERPRISE-LICENSE.md`. Trước khi sao chép bất kỳ đoạn code nào, phải kiểm tra file đó thuộc phần nào.

Với chúng ta, điều này ít ảnh hưởng: giá trị của Medusa là **ý tưởng kiến trúc**, không phải code TypeScript (chúng ta viết Go).

---

## Năng lực: Module Links — liên kết dữ liệu không dùng khóa ngoại

### Cách OSS làm

Medusa cấm module tham chiếu trực tiếp bảng của module khác. Thay vào đó, hai model ở hai module được liên kết qua **link table** — bảng trung gian **chỉ chứa hai định danh, không có ràng buộc khóa ngoại**.

Tài liệu Medusa nêu rõ lý do: giữ module cô lập để có thể tách thành service độc lập sau này mà không phải tái cấu trúc lớn.

### Điểm mạnh

Đây là **lời giải trực tiếp cho một vấn đề mà tài liệu của chúng ta nêu nhưng chưa giải quyết đầy đủ.**

[05-data/data-model.md](../05-data/data-model.md) mục 3 của chúng ta nói:

```text
"Vượt module: chỉ lưu định danh, KHÔNG khai báo khóa ngoại"
```

Đúng, nhưng chưa trả lời: **quan hệ nhiều-nhiều vượt module thì để bảng liên kết ở đâu?**

Ví dụ cụ thể trong domain của chúng ta:

```text
Content (module content) ←→ Product (module product)
    một nội dung gắn nhiều sản phẩm
    một sản phẩm xuất hiện trong nhiều nội dung

Bảng product_tag thuộc module nào?
```

Cách cũ (chưa rõ ràng): đặt vào `content`, nhưng nó chứa `product_id` — có vẻ như một quan hệ vượt module trá hình.

Cách Medusa: đây là **link table**, một loại bảng riêng có quy tắc riêng.

### Điểm yếu

- Không có ràng buộc toàn vẹn ở database → có thể trỏ tới bản ghi đã xóa
- Truy vấn phải nối thủ công, không JOIN được tự nhiên
- Thêm một loại bảng phải hiểu và quản lý

### Yêu cầu của chúng ta

Có ít nhất bốn quan hệ nhiều-nhiều vượt module:

```text
content ←→ product        (product_tag)
outfit  ←→ product        (outfit_item)
campaign ←→ creator       (campaign_participant)
brand   ←→ seller         (brand_authorization)
```

### Adopt

**Khái niệm link table tường minh**, với ba quy tắc:

```text
1. Link table CHỈ chứa hai định danh + metadata của chính quan hệ
   (ví dụ: vị trí tag trên ảnh, vai trò trong outfit)

2. KHÔNG có ràng buộc khóa ngoại vượt module

3. Link table thuộc về module SỞ HỮU Ý NGHĨA của quan hệ
   → product_tag thuộc content (vì "gắn sản phẩm vào nội dung"
     là khái niệm của content, không phải của product)
```

### Adapt

Medusa sinh link table tự động qua khai báo. Chúng ta viết SQL tường minh — không cần cơ chế sinh tự động.

Bổ sung điều Medusa không có: **job đối chiếu định kỳ** phát hiện link trỏ tới bản ghi không tồn tại. Đã có trong [05-data/consistency.md](../05-data/consistency.md) mục 10.

### Quyết định cuối

```text
✓ ADOPT khái niệm link table, ghi rõ ba quy tắc trên
→ Cập nhật docs/05-data/data-model.md
```

---

## Năng lực: Workflows với logic bù trừ

### Cách OSS làm

`Workflow` là chuỗi bước; mỗi `step` có thể khai báo **compensation function** — hàm hoàn tác nếu bước sau thất bại. Khi có lỗi, Medusa chạy các hàm bù trừ theo thứ tự ngược.

### Điểm mạnh

Giải bài toán thật: một thao tác nghiệp vụ chạm nhiều module, không có giao dịch database bao trùm.

Ví dụ đặt hàng: giữ tồn kho → tạo payment intent → tạo đơn. Nếu bước 3 thất bại, hai bước trước phải được hoàn tác.

### Điểm yếu

**Bù trừ chủ động cũng có thể thất bại.** Nếu hàm hoàn tác lỗi, phải bù trừ cho việc bù trừ — chuỗi này không có điểm dừng tự nhiên.

Ngoài ra thêm một khái niệm lớn (workflow engine) mà lập trình viên phải học.

### Yêu cầu của chúng ta

[05-data/consistency.md](../05-data/consistency.md) mục 7 đã nêu mẫu bù trừ, với một nguyên tắc quan trọng:

> Ưu tiên **bù trừ thụ động** (tự hết hạn) hơn **bù trừ chủ động** (phải gọi ngược).

### Adapt

**Lấy ý tưởng bù trừ, không lấy workflow engine.**

So sánh cách xử lý cùng một tình huống:

```text
Medusa:     workflow.step(reserve).compensate(release)
            → nếu bước sau lỗi, gọi release()

Chúng ta:   reservation có TTL 15 phút
            → nếu bước sau lỗi, KHÔNG LÀM GÌ CẢ
            → reservation tự hết hạn
```

Cách của chúng ta đơn giản hơn và **không thể thất bại** — vì không có hành động nào để thất bại.

Bảng chiến lược đầy đủ:

| Thao tác | Bù trừ | Kiểu |
|---|---|---|
| Giữ tồn kho | TTL tự hết hạn | Thụ động |
| Tạo payment intent | Tự hết hạn | Thụ động |
| Tạo đơn hàng | Hủy + hoàn tiền | Chủ động |
| Ghi bút toán | Bút toán đảo ngược | Chủ động |

### Reject

**Workflow engine.** Với số lượng luồng hiện tại và ưu tiên bù trừ thụ động, chi phí học và vận hành một workflow engine không tương xứng lợi ích.

Cân nhắc lại nếu: số luồng nhiều bước tăng đáng kể, hoặc cần hiển thị trạng thái workflow cho người vận hành.

### Quyết định cuối

```text
✓ ADOPT tư duy "mỗi bước phải có cách hoàn tác"
✓ Ưu tiên bù trừ thụ động (TTL) — đã có trong thiết kế
✗ REJECT workflow engine ở giai đoạn này
```

---

## Năng lực: Module isolation

### Cách OSS làm

Mỗi module có data model riêng, service riêng, đăng ký vào container. Module không import trực tiếp module khác.

### Điểm mạnh

Cùng triết lý với [ADR-0001](../adr/0001-modular-monolith.md) và [ADR-0005](../adr/0005-module-boundaries.md) của chúng ta — đây là xác nhận độc lập rằng hướng đi đúng.

Medusa là dự án 35k sao đang phát triển tích cực, chọn **modular monolith với ranh giới nghiêm ngặt** thay vì microservices. Đó là bằng chứng mạnh.

### Điểm yếu

Medusa cưỡng chế ranh giới bằng **quy ước và container**, không bằng công cụ phân tích tĩnh. TypeScript cho phép import bất kỳ đâu.

### Adopt

Triết lý — đã có.

### Adapt

Chúng ta đi **xa hơn Medusa** ở điểm cưỡng chế: `cmd/archcheck` làm CI thất bại khi vi phạm. Medusa dựa vào kỷ luật.

Đây là lợi thế của Go: `internal/` chặn ở tầng trình biên dịch, và phân tích AST dễ viết.

### Quyết định cuối

```text
✓ Xác nhận hướng modular monolith
✓ Giữ archcheck — mạnh hơn cách Medusa làm
```

---

## 2. Tổng kết Medusa

| Hạng mục | Quyết định |
|---|---|
| **Link table không khóa ngoại** | **ADOPT** — cập nhật data-model.md |
| Tư duy bù trừ cho mỗi bước | **ADOPT** |
| Ưu tiên bù trừ thụ động | **ADAPT** — chúng ta làm khác và tốt hơn cho trường hợp này |
| Module isolation | **ADOPT** — xác nhận thiết kế |
| Workflow engine | **REJECT** — chưa tương xứng chi phí |
| Sales Channel làm marketplace | **REJECT** — xem [vendure.md](vendure.md) |
| Code TypeScript | Không áp dụng (khác ngôn ngữ) |

**Nhận xét cuối:** Medusa đóng góp **ý tưởng có giá trị nhất** trong toàn bộ nghiên cứu non-Go — link table. Nó lấp một khoảng trống thật trong tài liệu của chúng ta, không phải xác nhận điều đã biết.

---

## 3. Tài liệu liên quan

- [../05-data/data-model.md](../05-data/data-model.md) — đã cập nhật theo phát hiện này
- [../05-data/consistency.md](../05-data/consistency.md) mục 7
- [../adr/0005-module-boundaries.md](../adr/0005-module-boundaries.md)
