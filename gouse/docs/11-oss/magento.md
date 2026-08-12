# Magento / Adobe Commerce

| | |
|---|---|
| Repository | `github.com/magento/magento2` |
| License | **OSL-3.0** (copyleft) |
| Sao / Fork | 12.166 / 9.351 |
| Ngôn ngữ | PHP |
| Cập nhật cuối | 2026-08-11 (tích cực) |
| Vai trò | Tham chiếu **MSI (tồn kho nhiều nguồn)** — **CHỈ ĐỌC Ý TƯỞNG** |

---

## 1. Cảnh báo license — nghiêm trọng

**OSL-3.0 là copyleft mạnh**, thậm chí chặt hơn GPL ở một điểm: điều khoản "External Deployment" kích hoạt nghĩa vụ mở nguồn **ngay cả khi chỉ cung cấp qua mạng**, không cần phân phối phần mềm.

```text
✗ KHÔNG sao chép code Magento
✗ KHÔNG sao chép cấu trúc lớp một cách trực tiếp
✓ ĐƯỢC đọc để hiểu ý tưởng, rồi tự cài đặt từ đầu
```

Toàn bộ ghi chép dưới đây là **mô tả khái niệm bằng lời**, không trích code.

---

## Năng lực: Multi-Source Inventory — tách tồn vật lý khỏi số lượng khả bán

Đây là mô hình tồn kho hoàn chỉnh nhất trong toàn bộ nhóm nghiên cứu.

### Cách OSS làm

Ba khái niệm tách biệt:

```text
Source        địa điểm vật lý có hàng (kho, cửa hàng)
              → số lượng THẬT SỰ nằm ở đó

Stock         nhóm nguồn phục vụ một kênh bán
              → một Stock có thể gộp nhiều Source

Reservation   bản ghi tạm khi đặt hàng, LÀM GIẢM số lượng khả bán
              nhưng KHÔNG đụng vào số lượng nguồn
```

Công thức:

```text
Salable Quantity = Σ (số lượng các Source) − Σ (reservation đang hoạt động) − ngưỡng
```

Điểm mấu chốt: khi khách đặt hàng, Magento **không trừ ngay** số lượng ở nguồn. Nó tạo một `Reservation` làm giảm số khả bán. Số lượng nguồn chỉ đổi khi **tạo lô giao hàng**.

### Điểm mạnh

**Tách đúng hai câu hỏi khác nhau:**

```text
"Trong kho có bao nhiêu?"    → số lượng Source (vật lý)
"Khách mua được bao nhiêu?"  → Salable Quantity (khả bán)
```

Hai con số này khác nhau và nhầm lẫn chúng gây bán quá số lượng.

**Reservation cho phép hủy đơn không cần đụng tới tồn kho vật lý** — chỉ xóa reservation.

**Có công cụ phát hiện sai lệch.** Magento có lệnh liệt kê reservation không nhất quán, chạy định kỳ để phát hiện rò rỉ.

### Điểm yếu

Phức tạp đáng kể. Với cửa hàng một kho, ba khái niệm là thừa.

Tính `Salable Quantity` phải tổng hợp nhiều bảng — cần bảng chỉ mục và có thể lệch nếu chỉ mục không được cập nhật.

### So sánh với thiết kế hiện tại của chúng ta

Chúng ta **đã có cùng nguyên lý** nhưng cài đặt khác:

| Magento MSI | Chúng ta | Ghi chú |
|---|---|---|
| Source | `stock_location_id` | Đã có |
| Stock (nhóm nguồn) | — | **Chưa có** |
| Reservation | `Reservation` + `quantity_reserved` | Đã có |
| Salable Quantity | `quantity_available` | Đã có, tính khác |

Khác biệt về cách tính:

```text
Magento:   Salable = Σ Source − Σ Reservation  (TÍNH TOÁN)
Chúng ta:  available và reserved là HAI CỘT riêng, cập nhật nguyên tử

Bất biến của chúng ta:
    available + reserved + committed + in_transit + damaged + returned
        = tổng số lượng vật lý
```

### Đánh giá: cách nào tốt hơn cho chúng ta?

**Cách của chúng ta phù hợp hơn** ở giai đoạn này, vì:

```text
✓ Kiểm tra và cập nhật NGUYÊN TỬ trong một câu lệnh UPDATE
  → chống bán quá số lượng ngay ở tầng database
✓ Không cần bảng chỉ mục, không có độ trễ tính toán
✓ Ràng buộc CHECK ở database bảo vệ bất biến
✓ Chịu được tranh chấp cao (live commerce) bằng khóa lạc quan
```

Cách Magento cần bảng chỉ mục và có nguy cơ lệch — chính họ phải cung cấp công cụ dò sai lệch.

### Adopt

**Ba ý tưởng đáng lấy:**

**1. Phân biệt rõ "tồn vật lý" và "khả bán" trong ngôn ngữ nghiệp vụ.**

Đã có trong thiết kế nhưng nên nói rõ hơn trong tài liệu: `quantity_available` là **khả bán**, không phải "số lượng trong kho".

**2. Reservation không đụng tồn kho vật lý cho tới khi xuất hàng.**

Đây chính là mô hình của chúng ta: `Reserved` → `Committed` → `Ship` (rời khỏi tồn kho).

**3. Công cụ dò sai lệch định kỳ.**

Magento có lệnh này vì kinh nghiệm thực tế cho thấy reservation **sẽ** rò rỉ. Chúng ta đã có trong [09-operations/observability.md](../09-operations/observability.md) mục 9:

```text
Hàng ngày: Reservation quá hạn chưa xử lý?
           Có inventory_item âm?
```

Magento xác nhận đây không phải cẩn thận thừa.

### Adapt

**Khái niệm Stock (nhóm nguồn) — hoãn tới khi có nhiều kho.**

Ở MVP một kho, nó là trừu tượng hóa sớm. Khi có nhiều kho (Phase 2), cân nhắc lại: bài toán "kênh bán X được phục vụ bởi kho A và B" sẽ xuất hiện.

### Reject

```text
✗ Tính Salable Quantity bằng tổng hợp + bảng chỉ mục
  → dùng cột riêng cập nhật nguyên tử, an toàn hơn
✗ Sao chép code (OSL-3.0)
```

### Quyết định cuối

```text
✓ Giữ mô hình 6 trạng thái hiện có — an toàn hơn cho tranh chấp cao
✓ Làm rõ trong tài liệu: available = KHẢ BÁN, không phải tồn vật lý
✓ Giữ job dò sai lệch — Magento xác nhận là cần thiết
→ Cân nhắc khái niệm Stock ở Phase 2 khi có nhiều kho
```

---

## Năng lực: Catalog phức tạp — sản phẩm cấu hình được

### Cách OSS làm

Nhiều loại sản phẩm: simple, configurable, grouped, bundle, virtual, downloadable.

`Configurable product` là sản phẩm cha có nhiều biến thể theo thuộc tính — khách chọn màu và size, hệ thống ánh xạ sang sản phẩm con.

### Điểm mạnh

Bao phủ gần như mọi mô hình bán hàng.

### Điểm yếu

**Độ phức tạp rất cao.** Sáu loại sản phẩm với hành vi khác nhau ở mọi khâu: hiển thị, giá, tồn kho, giỏ hàng, đơn hàng.

Đây là nguồn gốc nổi tiếng của độ khó khi vận hành Magento.

### Yêu cầu của chúng ta

Thời trang cần **một** mô hình: Product → Variant (màu) → SKU (màu + size).

Có thể cần thêm **bundle** trong tương lai cho outfit ("mua cả bộ") — nhưng đó là tính năng bán hàng, không phải loại sản phẩm mới.

### Reject

```text
✗ Nhiều loại sản phẩm với hành vi khác nhau
```

Chúng ta có **một** mô hình sản phẩm, xử lý được mọi mặt hàng thời trang.

Bán theo bộ (outfit) được mô hình hóa ở tầng **nội dung và khuyến mãi**, không phải loại sản phẩm:

```text
Outfit là Content có nhiều ProductTag
"Mua cả bộ" = thêm nhiều Offer vào giỏ cùng lúc
Giảm giá combo = Promotion có điều kiện "mua đủ N món trong outfit"
```

Cách này giữ mô hình sản phẩm đơn giản và vẫn đạt mục tiêu kinh doanh.

---

## Năng lực: Service contracts

### Cách OSS làm

Mỗi module định nghĩa interface công khai (`Api/` namespace); module khác chỉ được dùng interface đó, không dùng cài đặt cụ thể.

### Điểm mạnh

Cùng nguyên lý với `public.go` của chúng ta.

### Adopt

Xác nhận [ADR-0005](../adr/0005-module-boundaries.md). Magento là hệ thống lớn nhất trong nhóm và họ đi tới cùng kết luận: **module phải giao tiếp qua interface công khai**.

Khác biệt: Magento cưỡng chế bằng quy ước; chúng ta cưỡng chế bằng `cmd/archcheck` làm CI thất bại.

---

## 2. Tổng kết Magento

| Hạng mục | Quyết định |
|---|---|
| Phân biệt tồn vật lý / khả bán | **ADOPT** — làm rõ trong tài liệu |
| Reservation không đụng tồn vật lý | **ADOPT** — đã có |
| Job dò sai lệch reservation | **ADOPT** — đã có, được xác nhận |
| Khái niệm Stock (nhóm nguồn) | **ADAPT** — hoãn tới Phase 2 |
| Tính khả bán bằng tổng hợp + chỉ mục | **REJECT** — cột riêng an toàn hơn |
| Nhiều loại sản phẩm | **REJECT** — một mô hình duy nhất |
| Service contracts | **ADOPT** — xác nhận ADR-0005 |
| Sao chép code | **CẤM** — OSL-3.0 |

**Nhận xét cuối:** Magento có mô hình tồn kho hoàn chỉnh nhất và catalog phức tạp nhất. Chúng ta lấy **nguyên lý** của cái thứ nhất, **từ chối** cái thứ hai.

Bài học lớn hơn: Magento là ví dụ điển hình của việc **bao phủ mọi trường hợp dẫn tới độ phức tạp không kiểm soát được**. Nguyên tắc P15 của chúng ta (mỗi thứ phải giải thích được vì sao cần cho *chính* nghiệp vụ này) chính là để tránh kết cục đó.

---

## 3. Tài liệu liên quan

- [../04-modules/inventory.md](../04-modules/inventory.md)
- [../09-operations/observability.md](../09-operations/observability.md) mục 9
- [../adr/0005-module-boundaries.md](../adr/0005-module-boundaries.md)

## 4. Nguồn

- [Magento MSI Overview](https://github.com/magento/inventory/wiki/Overview)
