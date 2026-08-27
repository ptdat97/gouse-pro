# Saleor

| | |
|---|---|
| Repository | `github.com/saleor/saleor` |
| License | BSD-3-Clause (cho phép dùng thương mại) |
| Sao / Fork | 23.218 / 6.106 |
| Ngôn ngữ | Python |
| Cập nhật cuối | 2026-08-11 (rất tích cực) |
| Vai trò | Tham chiếu **tách catalog khỏi giá/tồn theo kênh**, API-first |

---

## Năng lực: Channel Listing — tách dữ liệu chung khỏi dữ liệu theo ngữ cảnh

### Cách OSS làm

Product và Variant là thực thể **toàn cục**. Dữ liệu phụ thuộc ngữ cảnh bán nằm ở bảng riêng:

```text
Product          tên, mô tả, ảnh, thuộc tính  (dùng chung)
    ↓
ProductChannelListing
    → hiển thị, khả dụng, ngày xuất bản, tìm kiếm được (theo channel)

ProductVariant   màu, size, mã SKU             (dùng chung)
    ↓
ProductVariantChannelListing
    → giá, tồn kho khả dụng                    (theo channel)
```

### Điểm mạnh

Tránh nhân bản sản phẩm. Một danh mục duy nhất, nhiều cấu hình bán.

Quan trọng hơn: nó đặt câu hỏi đúng — **thuộc tính nào là của bản thân sản phẩm, thuộc tính nào phụ thuộc ngữ cảnh bán?**

### So sánh với mô hình của chúng ta

Đây là xác nhận mạnh cho quyết định tách `Offer`:

| Saleor | Chúng ta | Vai trò |
|---|---|---|
| Product | Product | Thông tin trình bày, dùng chung |
| ProductVariant | Variant + SKU | Tổ hợp thuộc tính, định danh hàng hóa |
| ProductVariantChannelListing | **Offer** | Giá + khả dụng theo ngữ cảnh |
| Channel | (seller) | Ngữ cảnh bán |

Cùng một nguyên lý, khác ở chiều phân tách:

```text
Saleor:    ngữ cảnh = thị trường/kênh bán
Chúng ta:  ngữ cảnh = NHÀ BÁN
```

Vì bài toán của chúng ta là nhiều nhà bán cạnh tranh, không phải nhiều thị trường.

### Adopt

**Nguyên lý: thuộc tính phụ thuộc ngữ cảnh phải tách ra bảng riêng.**

Áp dụng kiểm tra này cho mọi trường trong `Product`:

```text
Câu hỏi: trường này có đổi theo NGƯỜI BÁN không?
    Không → thuộc Product/Variant/SKU
    Có    → thuộc Offer

Ví dụ:
  tên sản phẩm, chất liệu, bảng size     → Product (chung)
  màu, size                              → Variant/SKU (chung)
  giá, thời gian giao, chính sách đổi trả → Offer (theo seller)
```

Đây chính là bảng phân bổ thuộc tính đã có trong [01-business/marketplace.md](../01-business/marketplace.md) mục 2 — Saleor xác nhận nó đúng.

### Adapt

Saleor cho phép **giá theo nhiều tiền tệ** trong một channel. Chúng ta chưa cần, nhưng mô hình `Money` đã có `currency` nên mở rộng được.

### Quyết định cuối

```text
✓ Xác nhận mô hình Offer đúng nguyên lý
✓ Giữ bảng phân bổ thuộc tính hiện có
→ Bổ sung: khi thêm trường mới vào Product, phải trả lời
  "trường này có đổi theo người bán không?"
```

---

## Năng lực: API-first với GraphQL

### Cách OSS làm

Toàn bộ chức năng qua GraphQL. Không có API REST. Storefront và dashboard đều là client.

### Điểm mạnh

- Client lấy đúng dữ liệu cần, tránh over-fetching
- Một schema duy nhất, tự mô tả
- Tách frontend/backend triệt để

### Điểm yếu với chúng ta

Đã phân tích trong [ADR-0002](../adr/0002-api-first.md) khi loại GraphQL:

```text
✗ Phân quyền phức tạp hơn nhiều — phải kiểm tra ở TỪNG TRƯỜNG
✗ Cache khó — không dùng được cache HTTP thông thường
✗ Rủi ro truy vấn tốn kém từ client
✗ Đội chưa có kinh nghiệm vận hành
```

Điểm đầu tiên đặc biệt quan trọng với chúng ta. Ràng buộc bảo mật nghiêm ngặt nhất là **seller không thấy dữ liệu seller khác**. Với REST, điều kiện lọc nằm trong truy vấn của endpoint. Với GraphQL, client tự do ghép trường — phải kiểm tra quyền ở mọi resolver.

### Adopt

**Triết lý API-first** — đã có.

**Schema là nguồn sự thật, sinh code từ schema** — chúng ta làm điều tương tự với OpenAPI:

```bash
make api-types    # sinh kiểu TypeScript từ api/openapi.yaml
```

### Reject

GraphQL cho giai đoạn này. Cân nhắc lại nếu có nhiều client với nhu cầu dữ liệu rất khác nhau.

---

## Năng lực: Metadata mở rộng

### Cách OSS làm

Hầu hết thực thể có trường `metadata` (công khai) và `privateMetadata` (chỉ nội bộ) dạng khóa-giá trị.

### Điểm mạnh

Mở rộng không cần migration. Tích hợp bên thứ ba lưu dữ liệu riêng.

### Điểm yếu

Dễ trở thành bãi rác — dữ liệu quan trọng nằm trong metadata không có schema, không kiểm tra được, không truy vấn hiệu quả.

### Yêu cầu của chúng ta

[05-data/data-model.md](../05-data/data-model.md) mục 13 đã có quy tắc về JSONB:

```text
NÊN:    thuộc tính sản phẩm khác nhau theo loại, metadata event
KHÔNG:  dữ liệu cần truy vấn thường xuyên, bất cứ thứ gì liên quan tiền
```

### Adapt

Lấy ý tưởng **tách metadata công khai / nội bộ** — đây là phân biệt hữu ích chúng ta chưa có:

```text
Ví dụ với Seller:
  metadata công khai:  thông tin hiển thị cho khách
  metadata nội bộ:     ghi chú của nhân viên, điểm rủi ro
```

Ràng buộc: nếu một trường trong metadata được truy vấn thường xuyên, nó phải trở thành cột thật.

### Quyết định cuối

```text
✓ Phân biệt metadata công khai / nội bộ khi cần
✓ Giữ quy tắc JSONB hiện có — không nới lỏng
```

---

## 2. Tổng kết Saleor

| Hạng mục | Quyết định |
|---|---|
| Tách thuộc tính chung / theo ngữ cảnh | **ADOPT** — xác nhận mô hình Offer |
| Kiểm tra "trường này có đổi theo người bán không" | **ADOPT** |
| API-first, sinh code từ schema | **ADOPT** — qua OpenAPI |
| Metadata công khai / nội bộ | **ADAPT** |
| GraphQL | **REJECT** — phân quyền theo trường quá phức tạp |
| Channel làm ngữ cảnh bán | **ADAPT** — Phase 4 cho đa thị trường |

**Nhận xét cuối:** Saleor cung cấp **nguyên lý** rõ ràng nhất về việc tách dữ liệu chung khỏi dữ liệu theo ngữ cảnh. Nó không cho ý tưởng mới nhưng làm sắc nét hơn quyết định đã có.

---

## 3. Tài liệu liên quan

- [../01-business/marketplace.md](../01-business/marketplace.md) mục 2
- [../adr/0002-api-first.md](../adr/0002-api-first.md)
- [../05-data/data-model.md](../05-data/data-model.md) mục 13
