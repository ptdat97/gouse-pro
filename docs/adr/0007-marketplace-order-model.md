# ADR-0007: Mô hình Offer và tách Order/FulfillmentOrder

**Trạng thái:** Accepted

---

## Context

Nền tảng vận hành đồng thời hai mô hình cung ứng:

```text
Own brand (1P)   — nền tảng sở hữu hàng, sản xuất, kiểm soát giá
Marketplace (3P) — seller sở hữu hàng, tự đặt giá
```

Hai đặc điểm nghiệp vụ tạo ra vấn đề mô hình hóa:

**Vấn đề 1 — Nhiều nhà bán, cùng một sản phẩm:**

```text
Product: Áo thun cotton mã 450251
├── Seller A → 299.000đ, còn 12, giao 2 ngày
├── Seller B → 289.000đ, còn 3,  giao 4 ngày
└── Seller C → 310.000đ, còn 50, giao 1 ngày

Khách nhìn thấy MỘT trang sản phẩm, nhiều lựa chọn nhà bán.
```

**Vấn đề 2 — Một đơn, nhiều nguồn hàng:**

```text
Giỏ hàng:
├── Áo own brand   (kho nền tảng, Hà Nội)
├── Giày Seller A  (kho seller A, TP.HCM)
└── Túi Seller B   (kho seller B, Đà Nẵng)

Ba món KHÔNG THỂ đóng chung một gói.
```

---

## Decision

### Quyết định 1: Tách `Offer` thành khái niệm riêng

```text
Product   — sản phẩm (thông tin chung, ảnh, mô tả)
   ↓
Variant   — tổ hợp thuộc tính (màu Trắng, size M)
   ↓
SKU       — đơn vị lưu kho, định danh hàng hóa CHUNG
   ↓
Offer     — lời chào bán của MỘT seller cho SKU đó
   ↓
Inventory — tồn kho tại điểm lưu kho của chủ sở hữu đó
```

**Phân bổ thuộc tính:**

| Thuộc tính | Thuộc về | Lý do |
|---|---|---|
| Tên, mô tả, ảnh | Product | Chung cho mọi seller |
| Màu, size | Variant | Chung |
| Mã định danh hàng hóa | SKU | Chung — để biết "cùng một hàng" |
| **Giá** | **Offer** | Mỗi seller khác nhau |
| **Thời gian giao** | **Offer** | Phụ thuộc vị trí seller |
| **Chính sách đổi trả** | **Offer** | Có thể khác nhau |
| **Tồn kho** | **Inventory** | Nhiều kho, nhiều trạng thái |

**Quan trọng:** `Offer` thuộc **Commerce context**, không thuộc Marketplace context. Own brand cũng có Offer. Nhờ đó mọi thứ bán được đều đi qua một đường duy nhất.

### Quyết định 2: Tách `Order` và `FulfillmentOrder`

```text
Khách nhìn thấy:
    Order #FC-2026-08-001234 — 1.250.000đ

Hệ thống thực thi:
    Order #FC-2026-08-001234
    ├── FulfillmentOrder ...-A  (own brand, kho HN)
    ├── FulfillmentOrder ...-B  (Seller A, TP.HCM)
    └── FulfillmentOrder ...-C  (Seller B, Đà Nẵng)
```

```text
Order            = HỢP ĐỒNG VỚI KHÁCH HÀNG
FulfillmentOrder = ĐƠN VỊ CÔNG VIỆC VẬN HÀNH
```

---

## Alternatives cho Quyết định 1

### A. Mỗi seller tạo product riêng — **bị loại**

```text
Nhược (quyết định):
    − Trang sản phẩm trùng lặp hàng loạt
    − Khách tìm "áo thun Uniqlo U" thấy 40 kết quả giống hệt
    − Không so sánh được giá
    − Trải nghiệm tệ, SEO tệ (nội dung trùng lặp)
```

### B. Gắn giá + tồn kho vào SKU, thêm cột `seller_id` — **bị loại**

```text
Nhược (quyết định):
    − SKU mất ý nghĩa "định danh hàng hóa"
    − Không trả lời được "có bao nhiêu nơi bán mã hàng này"
    − Gom nhóm để so sánh thành truy vấn phức tạp
      thay vì quan hệ tự nhiên
```

### C. Chỉ làm Offer khi thật sự có nhiều seller — **bị loại**

```text
Đây là cái bẫy phổ biến nhất.

Nhược (quyết định):
    − Đến lúc cần thì `price` và `stock` đã nằm rải rác
      trong hàng chục truy vấn, API, màn hình
    − Tách ra trở thành DỰ ÁN DI TRÚ LỚN
```

**Kết luận:** làm Offer từ ngày đầu, kể cả khi chỉ có own brand bán. Ban đầu mỗi SKU có đúng một offer. Chi phí thêm rất nhỏ; chi phí không làm thì rất lớn.

---

## Alternatives cho Quyết định 2

### A. Một Order duy nhất với một trạng thái — **bị loại**

```text
Ưu:
    + Đơn giản nhất

Nhược (quyết định — năm lý do):

1. Chủ sở hữu khác nhau
   Order thuộc khách · FulfillmentOrder thuộc seller/kho

2. Vòng đời khác nhau
   Order gần như bất biến sau khi đặt
   FulfillmentOrder thay đổi liên tục theo tiến trình
   Một Order "đang xử lý" trong khi ba FO ở ba trạng thái khác nhau

3. RÀNG BUỘC BẢO MẬT
   Seller được xem phần của mình
   Seller KHÔNG được xem Order (chứa hàng seller khác)

4. TRANH CHẤP GHI
   Ba seller cập nhật đồng thời sẽ tranh chấp
   trên cùng một bản ghi Order

5. Không hỗ trợ xử lý từng phần
   Giao/hủy/hoàn từng phần không khả thi
```

**Lý do 3 và 4 là quyết định** — chúng không giải quyết được nếu gộp.

### B. Chia thành nhiều Order riêng cho khách — **bị loại**

```text
Ví dụ: khách đặt 1 lần, nhận 3 mã đơn riêng

Nhược:
    − Khách bối rối: "sao tôi mua một lần lại có 3 đơn?"
    − Thanh toán một lần nhưng 3 đơn → khó đối chiếu
    − Áp dụng mã giảm giá cho cả giỏ trở nên phức tạp
    − Hủy/hoàn phức tạp hơn
```

---

## Consequences

### Tích cực

```text
✓ Một trang sản phẩm, nhiều nhà bán — khách so sánh được
✓ Own brand và marketplace dùng CHUNG một luồng đơn hàng
✓ Hỗ trợ giao/hủy/hoàn từng phần
✓ Seller chỉ thấy phần của mình — ranh giới nằm trong CẤU TRÚC DỮ LIỆU,
  không phụ thuộc vào việc nhớ lọc ở tầng hiển thị
✓ Không tranh chấp ghi giữa các seller
✓ Thêm own brand thứ hai không tốn công
```

### Điểm quan trọng về bảo mật

```text
Nếu seller truy cập Order:
    → phải lọc dữ liệu ở tầng hiển thị
    → quên MỘT lần là rò rỉ dữ liệu đối thủ

Với FulfillmentOrder:
    → ranh giới nằm sẵn trong cấu trúc
    → seller.FindByID(foID) tự nhiên chỉ trả phần của họ
```

### Tiêu cực

```text
− Mô hình phức tạp hơn (5 tầng thay vì 3)
− Cần logic tách đơn khi thanh toán thành công
− Trạng thái Order phải suy ra từ các FulfillmentOrder
− Nhiều bảng hơn, nhiều truy vấn hơn
```

---

## Hệ quả bắt buộc kèm theo

### 1. Đóng băng dữ liệu trong OrderLine

```text
OrderLine lưu:
    product_name          ← ĐÓNG BĂNG
    variant_description   ← ĐÓNG BĂNG
    unit_price            ← ĐÓNG BĂNG
    commission_rate       ← ĐÓNG BĂNG
    creator_commission_rate ← ĐÓNG BĂNG
```

Vì offer có thể đổi giá, seller có thể đổi tên sản phẩm, chính sách hoa hồng có thể thay đổi.

### 2. Trạng thái Order là dữ liệu suy ra

```text
Tất cả FO đã giao       → DELIVERED
Một số FO đã giao       → PARTIALLY_DELIVERED
Tất cả FO hoàn tất      → COMPLETED
```

`order` lắng nghe event từ `fulfillment` và tính lại — không hỏi ngược (tránh phụ thuộc vòng).

### 3. Offer không chứa số lượng tồn kho

```text
offer.status = OUT_OF_STOCK là dữ liệu DẪN XUẤT, cập nhật qua event.
Nguồn sự thật là inventory_item.quantity_available.
```

Lý do: một offer có thể phục vụ từ nhiều kho; tồn kho thay đổi tần suất rất cao.

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Mô hình 5 tầng phức tạp hơn | Hỗ trợ marketplace thật sự |
| Logic tách đơn | Giao/hủy/hoàn từng phần |
| Trạng thái Order là dữ liệu suy ra | Không tranh chấp ghi |
| Nhiều bảng, nhiều truy vấn | Ranh giới bảo mật trong cấu trúc |
| Làm Offer từ đầu dù chỉ 1 seller | Tránh dự án di trú sau này |

---

## Tài liệu liên quan

- [../01-business/marketplace.md](../01-business/marketplace.md) mục 2
- [../01-business/fulfillment.md](../01-business/fulfillment.md) mục 3
- [../02-domain/aggregates.md](../02-domain/aggregates.md) mục 3.1–3.2
- [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md)
