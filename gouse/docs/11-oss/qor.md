# QOR

| | |
|---|---|
| Repository | `github.com/qor/qor` |
| License | MIT |
| Sao / Fork | 5.343 / 687 |
| Cập nhật cuối | 2026-07-27 (đang hoạt động) |
| Vai trò | Tham chiếu **quản trị, workflow, media** — **KHÔNG** phải kiến trúc thương mại lõi |

---

## 1. QOR là gì và không là gì

QOR là bộ công cụ xây dựng **ứng dụng nghiệp vụ** trong Go, không phải nền tảng thương mại điện tử. Nó mạnh ở tầng quản trị và các mối quan tâm cắt ngang.

```text
Admin         sinh giao diện quản trị từ struct
Transition    máy trạng thái
Publish2      nháp/xuất bản có lịch
Media         thư viện media, xử lý ảnh
Worker        tác vụ nền
Roles         phân quyền
L10n / I18n   đa vùng, đa ngôn ngữ
Validations   kiểm tra dữ liệu
Activity      nhật ký hoạt động
```

**Ràng buộc quan trọng:** QOR gắn chặt với **GORM**. Mọi module giả định model là struct GORM với tag. Đây là lý do chính khiến chúng ta không lấy QOR làm nền.

---

## Năng lực: Transition (máy trạng thái)

### Cách OSS làm

Định nghĩa trạng thái và chuyển đổi trên model GORM:

```text
State("draft"), State("published")
Event("publish").To("published").From("draft")
```

Có hook `Before`/`After` cho mỗi chuyển đổi. Trạng thái lưu vào cột của model.

### Điểm mạnh

- Chuyển đổi tường minh — không thể nhảy trạng thái tùy tiện
- Hook cho phép gắn tác dụng phụ (gửi thông báo, ghi log)
- Ghi lại lịch sử chuyển đổi

### Điểm yếu

- Gắn với GORM, không dùng độc lập được
- Một model = một máy trạng thái. Không mô hình hóa được nhiều mối quan tâm song song

### Yêu cầu của chúng ta

Chúng ta có nhiều thực thể vòng đời phức tạp: `Order`, `FulfillmentOrder`, `ReturnRequest`, `Seller`, `ProductDevelopment`, `ProductionOrder`.

Nhưng **không phải tất cả** đều cần máy trạng thái hình thức. Nguyên tắc P16 nói: thà rõ ràng còn hơn trừu tượng sớm.

### Adopt

**Ý tưởng chuyển đổi tường minh có kiểm tra.** Trạng thái không được đổi bằng phép gán trực tiếp; phải qua phương thức domain kiểm tra chuyển đổi hợp lệ:

```go
// Đúng — kiểm tra được
func (o *Order) Cancel(reason CancelReason) error {
    if o.status == StatusShipped || o.status == StatusDelivered {
        return ErrCannotCancelShippedOrder
    }
    ...
}

// Sai — không kiểm soát được
order.Status = "CANCELLED"
```

Mẫu này đã có trong thiết kế của chúng ta.

### Adapt

**Không dùng thư viện máy trạng thái.** Cài đặt bằng phương thức domain thuần Go, vì:

- Không phụ thuộc GORM
- Domain layer phải sạch (quy tắc R2 của archcheck)
- Số lượng chuyển đổi của chúng ta vừa phải, viết tay dễ đọc hơn cấu hình

**Chỉ dùng máy trạng thái hình thức khi vòng đời thật sự phức tạp.** Xem quyết định chi tiết ở [sylius.md](sylius.md) — Sylius có mô hình tốt hơn QOR cho việc này.

### Reject

Thư viện `qor/transition` — gắn GORM, không phù hợp domain layer sạch.

---

## Năng lực: Publish2 (nháp/xuất bản có lịch)

### Cách OSS làm

Model có hai phiên bản: bản nháp và bản đang xuất bản. Hỗ trợ lên lịch: đặt trước ngày giờ xuất bản, hệ thống tự chuyển.

### Điểm mạnh

Giải đúng một vấn đề thật của thương mại thời trang: **bộ sưu tập ra mắt theo lịch**. Nội dung, giá, sản phẩm được chuẩn bị trước, công bố đồng loạt vào một thời điểm.

### Điểm yếu

Nhân đôi mọi bảng dữ liệu. Với dữ liệu lớn, chi phí lưu trữ và độ phức tạp truy vấn tăng gấp đôi.

### Yêu cầu của chúng ta

`Collection` có `launch_date`. Bộ sưu tập "Thu Đông 2026" phải sẵn sàng trước, hiển thị đúng giờ ra mắt.

Nội dung creator cũng cần: chuẩn bị trước chiến dịch, xuất bản đồng loạt.

### Adapt

**Không nhân đôi bảng.** Dùng trường trạng thái + thời điểm:

```text
Collection {
    status       PLANNING | ACTIVE | ENDING | ARCHIVED
    launch_date  thời điểm chuyển sang ACTIVE
}

Content {
    status        DRAFT | SCHEDULED | PUBLISHED
    publish_at    thời điểm tự động xuất bản
}
```

Job định kỳ trong `cmd/worker` chuyển trạng thái khi tới giờ.

Lý do khác QOR: chúng ta không cần **sửa song song** bản nháp và bản đang chạy — chỉ cần **hoãn công bố**. Bài toán đơn giản hơn nên giải pháp cũng nên đơn giản hơn.

### Quyết định cuối

```text
✓ Trạng thái + thời điểm công bố, job định kỳ chuyển trạng thái
✗ Không nhân đôi bảng theo mô hình Publish2
```

---

## Năng lực: Media Library

### Cách OSS làm

Trừu tượng lưu trữ (filesystem, S3), xử lý ảnh (crop, resize theo kích thước định sẵn), gắn media vào model qua tag.

### Điểm mạnh

- Tách lưu trữ khỏi domain
- Nhiều kích thước ảnh sinh tự động
- Có khái niệm "thư viện" — media dùng lại được ở nhiều nơi

### Yêu cầu của chúng ta

Thương mại thời trang là **ngành nặng về hình ảnh**:

```text
3–8 ảnh mỗi sản phẩm
Ảnh đổi theo màu variant
Ảnh chi tiết chất liệu
Ảnh trên người mẫu có số đo
Video creator, lookbook, UGC
```

Đồng thời ảnh nặng làm chậm trang → giảm tỷ lệ chuyển đổi. Đây là đánh đổi trực tiếp.

### Adopt

**Trừu tượng lưu trữ sau interface** (nguyên tắc P13):

```go
type MediaStorage interface {
    Put(ctx, key string, r io.Reader, meta Metadata) (URL, error)
    Delete(ctx, key string) error
    SignedURL(ctx, key string, ttl time.Duration) (URL, error)
}
```

**Media là thực thể độc lập, dùng lại được** — một ảnh có thể xuất hiện ở sản phẩm, outfit, và chiến dịch.

### Adapt

**Không tự xử lý ảnh trong tiến trình API.** QOR xử lý ảnh khi tải lên; chúng ta chuyển sang bất đồng bộ:

```text
Tải lên → lưu bản gốc → phát event media.uploaded
    → worker sinh các biến thể kích thước
    → CDN phục vụ
```

Lý do: xử lý ảnh tốn CPU, không được làm chậm request. Đây cũng là ứng viên tách service **nhóm 1** theo [ADR-0009](../adr/0009-service-extraction.md).

### Quyết định cuối

```text
✓ MediaStorage sau interface
✓ Media là thực thể độc lập, quan hệ nhiều-nhiều với product/content
✓ Xử lý ảnh bất đồng bộ trong worker
✗ Không dùng qor/media (gắn GORM)
```

---

## Năng lực: Admin sinh tự động

### Cách OSS làm

Sinh giao diện CRUD từ struct GORM. Cấu hình bằng code: trường nào hiển thị, lọc thế nào, quyền ai được sửa.

### Điểm mạnh

Rất nhanh cho CRUD thuần. Tiết kiệm hàng tuần công sức cho màn hình quản trị đơn giản.

### Điểm yếu

- Gắn chặt GORM và cấu trúc bảng
- Màn hình phức tạp (đề xuất bổ sung hàng, đối soát) không sinh tự động được
- Giao diện sinh ra phản ánh **cấu trúc dữ liệu**, không phản ánh **quy trình công việc**

### Yêu cầu của chúng ta

Xem [08-frontend/admin.md](../08-frontend/admin.md): màn hình quan trọng nhất của admin là **hỗ trợ ra quyết định**, không phải CRUD.

Ví dụ màn hình đề xuất bổ sung hàng: hiển thị tín hiệu nhu cầu, mâu thuẫn MOQ, ước tính tài chính từng phương án. Không công cụ sinh tự động nào làm được.

### Reject

**Không dùng admin sinh tự động.** Admin của chúng ta là ứng dụng Next.js riêng, gọi API như mọi client khác.

Lý do gốc: nguyên tắc P3 (API First). Nếu admin truy cập database trực tiếp qua công cụ sinh tự động, nó bỏ qua toàn bộ tầng phân quyền, audit log, và quy tắc nghiệp vụ.

### Quyết định cuối

```text
✗ REJECT qor/admin
✓ Admin là ứng dụng Next.js, gọi /api/v1/admin/... như mọi client
✓ Giữ nguyên thiết kế trong 08-frontend/admin.md
```

---

## 2. Tổng kết QOR

| Hạng mục | Quyết định |
|---|---|
| Chuyển đổi trạng thái tường minh | **ADOPT** ý tưởng, tự cài |
| Nháp/xuất bản có lịch | **ADAPT** — trạng thái + thời điểm, không nhân đôi bảng |
| Trừu tượng lưu trữ media | **ADOPT** |
| Media là thực thể dùng lại | **ADOPT** |
| Xử lý ảnh | **ADAPT** — bất đồng bộ, không đồng bộ |
| `qor/admin` sinh tự động | **REJECT** — vi phạm API First |
| `qor/transition` (thư viện) | **REJECT** — gắn GORM |
| `qor/media` (thư viện) | **REJECT** — gắn GORM |
| Toàn bộ QOR làm nền | **REJECT** — gắn GORM, không phải kiến trúc thương mại |

**Nhận xét cuối:** QOR có ý tưởng tốt ở tầng quản trị nhưng **mọi thư viện đều gắn GORM**. Vì chúng ta chọn sqlc (xem [ADR-0010](../adr/0010-database-layer.md)), không thư viện QOR nào dùng được trực tiếp. Giá trị của QOR với chúng ta là **ý tưởng**, không phải code.

---

## 3. Tài liệu liên quan

- [../08-frontend/admin.md](../08-frontend/admin.md)
- [../adr/0010-database-layer.md](../adr/0010-database-layer.md)
- [synthesis.md](synthesis.md)
