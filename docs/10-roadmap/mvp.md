# MVP

## 1. Mục tiêu MVP

> Bán được hàng own brand và hàng của một số seller đầu tiên, với mô hình dữ liệu **đúng ngay từ đầu** cho những phần khó thay đổi sau này.

MVP **không phải** phiên bản đơn giản hóa mọi thứ. Nó là phiên bản có **ít tính năng** nhưng **mô hình đúng**.

---

## 2. Nguyên tắc phân định phạm vi

```text
CORE DOMAIN     → mô hình phải ĐÚNG ngay, dù tính năng còn ít
SUPPORTING      → làm đơn giản, mở rộng sau
GENERIC         → dùng dịch vụ có sẵn, không tự xây
```

### Bốn thứ phải làm đúng ngay từ MVP

Đây là những quyết định mà sửa sau rất tốn kém:

```text
1. Mô hình Offer (tách khỏi Product)
   → dù ban đầu mỗi SKU chỉ có một offer của own brand
   → nếu để sau: price và stock đã nằm rải rác trong hàng chục
     truy vấn, API, màn hình → dự án di trú lớn

2. Tách Order / FulfillmentOrder
   → dù ban đầu chỉ có một nguồn hàng
   → nếu để sau: phải viết lại module đơn hàng

3. Sổ cái bất biến
   → dù ban đầu chưa phải chia tiền cho ai nhiều
   → nếu để sau: không tái dựng được lịch sử tài chính

4. Ghi nhận tín hiệu nhu cầu (demand_signal)
   → dù chuỗi cung ứng tới Phase 3 mới làm
   → DỮ LIỆU LỊCH SỬ KHÔNG TẠO NGƯỢC ĐƯỢC
```

Chi phí làm bốn thứ này ở MVP là nhỏ. Chi phí không làm, ở Phase 2–3, là viết lại.

---

## 3. Module trong MVP (16 module)

### Tầng nền

| Module | Phạm vi MVP |
|---|---|
| `identity` | Đăng ký, đăng nhập, vai trò cơ bản, token |
| `notification` | Email giao dịch (xác nhận đơn, giao hàng) |
| `analytics` | Ghi sự kiện cơ bản, chỉ số cốt lõi |

### Tầng dữ liệu chính

| Module | Phạm vi MVP |
|---|---|
| `customer` | Hồ sơ, địa chỉ, wishlist, đồng ý cơ bản |
| `catalog` | Danh mục, thương hiệu, **bảng size** |
| `product` | Product/Variant/SKU, xuất bản cơ bản |
| `pricing` | Giá cơ bản, giá gạch ngang, khung giá |
| `inventory` | Một địa điểm, **6 trạng thái đầy đủ**, reservation, khóa lạc quan |

### Tầng nghiệp vụ

| Module | Phạm vi MVP |
|---|---|
| `marketplace` | **Offer**, buy box đơn giản, hoa hồng theo ngành hàng |
| `seller` | Đăng ký, duyệt thủ công, **own brand là seller nội bộ** |
| `promotion` | Mã giảm giá cơ bản, miễn phí ship theo ngưỡng |

### Tầng giao dịch và điều phối

| Module | Phạm vi MVP |
|---|---|
| `cart` | Giỏ nhiều seller, gộp giỏ khi đăng nhập |
| `checkout` | Giữ tồn kho, **đóng băng giá**, một cổng thanh toán |
| `order` | Tạo đơn, hủy toàn phần, **đóng băng dữ liệu giao dịch** |
| `payment` | Một cổng, **ledger đầy đủ**, đối soát thủ công |
| `fulfillment` | **Tách đơn theo seller**, một kho, một đối tác vận chuyển |

Cộng thêm: bảng `demand_signal` và việc ghi tín hiệu (thuộc `supply-chain`, chưa có giao diện).

---

## 4. Phạm vi chi tiết theo tính năng

### Có trong MVP

```text
Khách hàng:
    ✓ Duyệt, tìm kiếm (SQL cơ bản), lọc theo danh mục/size/màu/giá
    ✓ Xem sản phẩm với bảng size và chất liệu
    ✓ Giỏ hàng nhiều seller
    ✓ Checkout, thanh toán một cổng
    ✓ Khách vãng lai đặt hàng được
    ✓ Xem đơn hàng, theo dõi vận chuyển
    ✓ Wishlist

Seller:
    ✓ Đăng ký, chờ duyệt thủ công
    ✓ Tạo offer trên sản phẩm có sẵn
    ✓ Tạo sản phẩm mới (có duyệt)
    ✓ Quản lý tồn kho
    ✓ Xử lý đơn (xác nhận, đóng gói, giao)
    ✓ Xem số dư và đối soát

Vận hành:
    ✓ Duyệt seller, duyệt sản phẩm
    ✓ Quản lý danh mục, thương hiệu, bảng size
    ✓ Nhập kho own brand thủ công
    ✓ Xem đơn hàng, hỗ trợ khách
    ✓ Sổ cái, đối soát, chi trả (bán tự động)
```

### KHÔNG có trong MVP

```text
✗ Creator, nội dung, affiliate        → Phase 2
✗ Trả hàng (xử lý thủ công ngoài hệ thống) → Phase 2
✗ Gợi ý sản phẩm cá nhân hóa          → Phase 2
✗ Nhiều kho                            → Phase 2
✗ Chuỗi cung ứng (mua, sản xuất, QC)  → Phase 3
✗ Điểm thưởng, hạng thành viên        → Phase 3
✗ Live commerce                        → Phase 4
✗ Retail media                         → Phase 4
```

---

## 5. Quyết định gây tranh cãi và lý do

### Vì sao Return không có trong MVP?

```text
Lập luận phản đối: tỷ lệ hoàn hàng thời trang rất cao,
                   không thể không có

Quyết định: xử lý THỦ CÔNG ở MVP
    - Khối lượng đơn ban đầu nhỏ, xử lý tay được
    - Return cần quy trình QC, kho, và chuỗi đảo ngược tài chính
      → phức tạp, cần làm đúng chứ không làm vội

NHƯNG bắt buộc chuẩn bị sẵn:
    ✓ Ledger hỗ trợ bút toán đảo ngược
    ✓ Inventory có trạng thái Returned và Damaged
    ✓ Order có trạng thái dòng hàng RETURNED
    ✓ Danh sách lý do hoàn chuẩn hóa (dùng khi nhập tay)

→ Phase 2 chỉ cần thêm quy trình và giao diện, không sửa mô hình
```

### Vì sao ghi demand_signal ở MVP dù chưa dùng?

```text
Dữ liệu lịch sử KHÔNG THỂ tạo ngược.

Nếu đến Phase 3 mới bắt đầu ghi:
    → chờ thêm nhiều tháng mới đủ dữ liệu lập kế hoạch
    → mất gần một năm trước khi bánh đà bắt đầu quay

Chi phí ở MVP: một bảng + ghi event bất đồng bộ. Rất nhỏ.
```

Đây là một trong số ít trường hợp "làm sớm dù chưa cần" là đúng — vì dữ liệu có tính tích lũy.

### Vì sao mô hình Offer ở MVP dù chỉ own brand bán?

```text
Đây là cái bẫy phổ biến nhất của nền tảng marketplace.

Nếu để sau:
    price và stock đã nằm rải rác trong hàng chục truy vấn,
    API, và màn hình → tách ra là dự án di trú lớn

Chi phí ở MVP: một bảng thêm, một tầng thêm trong truy vấn.
```

Xem [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md).

---

## 6. Hạ tầng MVP

```text
✓ Go API (nhiều bản) + Go Worker (outbox, job định kỳ)
✓ PostgreSQL (một database, có replica dự phòng)
✓ Object storage (ảnh, video)
✓ CDN
✓ Cổng thanh toán (tích hợp một nhà cung cấp)
✓ Dịch vụ gửi email
✓ Đối tác vận chuyển (tích hợp một nhà cung cấp)

✗ KHÔNG có: Kubernetes, message broker riêng, cache riêng,
            chỉ mục tìm kiếm riêng, kho dữ liệu
```

Xem [../09-operations/deployment.md](../09-operations/deployment.md).

---

## 7. Tiêu chí hoàn thành MVP

> **Đối chiếu thực tế 20/08/2026.** Phần CHỨC NĂNG đã đạt, trừ đối soát:
> `getMySettlement` và `getMyBalance` mới có đặc tả, chưa có route. Phần
> CHẤT LƯỢNG chưa đo được vì cần môi trường có tải thật — đó là một trong
> các việc của phase Production Hardening
> ([backlog mục 2.12](backlog.md)).
>
> Trạng thái từng phần kèm bằng chứng: [todo.md mục 12](todo.md).

### Chức năng

```text
✓ Khách đặt được đơn có hàng own brand + hàng seller trong cùng giỏ
✓ Đơn tự động tách thành FulfillmentOrder theo từng nguồn
✓ Seller xử lý được đơn của mình, KHÔNG thấy dữ liệu seller khác
✓ Sổ cái ghi đúng: doanh thu, hoa hồng, số dư seller
✓ Đối soát ra đúng số tiền phải trả seller
✓ Khách vãng lai đặt hàng được
```

### Chất lượng

```text
✓ Độ lệch đối soát = 0
✓ Bút toán không cân bằng = 0
✓ Tồn kho âm = 0
✓ API p95 < 300ms
✓ LCP trang sản phẩm < 2,5s
✓ Kiểm tra ranh giới module trong CI đều xanh
```

### Kiến trúc

```text
✓ Không có phụ thuộc vòng giữa module
✓ Không có JOIN vượt ranh giới module
✓ Không có thư mục common/ utils/ helpers/ services/
✓ Mọi lệnh ghi API đều idempotent
✓ Outbox hoạt động, không có event kẹt
```

---

## 8. Thứ tự triển khai đề xuất

```text
Giai đoạn 1 — Nền tảng
    platform (database, event bus, HTTP, log)
    kernel (Money, ID types)
    identity
    → chưa có gì để demo, nhưng bắt buộc làm trước

Giai đoạn 2 — Danh mục
    catalog → product → pricing
    → có thể xem sản phẩm

Giai đoạn 3 — Tồn kho và bán hàng
    inventory → marketplace (Offer) → seller
    → có thể bán own brand

Giai đoạn 4 — Giao dịch
    cart → checkout → order → payment (ledger)
    → có thể đặt hàng và thanh toán

Giai đoạn 5 — Thực hiện đơn
    fulfillment → notification
    → hoàn chỉnh luồng mua hàng

Giai đoạn 6 — Marketplace
    seller onboarding đầy đủ → đối soát → chi trả
    → seller thật bán được hàng

Giai đoạn 7 — Hoàn thiện
    promotion, analytics cơ bản, demand_signal
    customer (wishlist, preference)
```

**Lý do thứ tự này:** mỗi giai đoạn xây trên giai đoạn trước theo đúng đồ thị phụ thuộc module. Không có giai đoạn nào phải chờ module chưa làm.

---

## 9. Rủi ro chính của MVP

| Rủi ro | Giảm thiểu |
|---|---|
| Ledger sai từ đầu | Kiểm tra tính toàn vẹn hàng ngày ngay từ ngày đầu |
| Ranh giới module lỏng dần | Kiểm tra tự động trong CI, thất bại = chặn merge |
| Tồn kho sai do đồng thời | Khóa lạc quan + ràng buộc CHECK ở database |
| Bỏ qua demand_signal vì "chưa cần" | Đưa vào tiêu chí hoàn thành MVP |
| Làm Offer sau vì "phức tạp quá" | Đưa vào tiêu chí hoàn thành MVP |

Hai rủi ro cuối là rủi ro **tổ chức**, không phải kỹ thuật — áp lực deadline dễ khiến người ta bỏ qua.

---

## 10. Tài liệu liên quan

- [future-phases.md](future-phases.md) — Phase 2, 3, 4 (FUTURE), [scale.md](scale.md)
- [backlog.md](backlog.md) — việc đang làm
- [deliverables.md](deliverables.md) — tổng hợp bàn giao
- [../02-domain/domain-map.md](../02-domain/domain-map.md) — phân loại core/supporting/generic
