# Nghiệp vụ: Hoàn tất đơn hàng (Fulfillment)

## 1. Fulfillment là gì

Fulfillment là toàn bộ quá trình từ khi đơn hàng được xác nhận đến khi hàng nằm trong tay khách.

```text
Đơn được xác nhận
    ↓
Phân bổ nguồn hàng (chọn kho / seller nào xuất)
    ↓
Lấy hàng (Picking)
    ↓
Đóng gói (Packing)
    ↓
Bàn giao vận chuyển (Handover)
    ↓
Vận chuyển (Shipping)
    ↓
Giao thành công (Delivered)
```

---

## 2. Ba mô hình fulfillment

Nền tảng vận hành đồng thời ba mô hình. Đây là lý do `FulfillmentOrder` phải tách khỏi `Order`.

### 2.1 Nền tảng tự thực hiện (1P Fulfillment)

```text
Kho nền tảng → Đóng gói → Đối tác vận chuyển → Khách
```

Áp dụng cho: own brand.

Đặc điểm: kiểm soát hoàn toàn chất lượng đóng gói, thời gian, trải nghiệm mở hộp (quan trọng với thời trang).

### 2.2 Seller tự thực hiện (Seller Fulfilled)

```text
Kho seller → Seller đóng gói → Đối tác vận chuyển → Khách
```

Áp dụng cho: đa số seller marketplace.

Đặc điểm: nền tảng không kiểm soát chất lượng đóng gói và thời gian xử lý. Rủi ro chính về trải nghiệm khách hàng.

Cơ chế kiểm soát: cam kết thời gian xử lý (SLA), theo dõi hiệu suất, quy chuẩn đóng gói bắt buộc.

### 2.3 Nền tảng thực hiện hộ seller (Platform Fulfilled Service)

```text
Seller gửi hàng vào kho nền tảng → Nền tảng lưu kho, đóng gói, giao → Khách
```

Áp dụng cho: seller muốn dùng dịch vụ, thường là seller có doanh số cao.

Đặc điểm: nền tảng thu phí dịch vụ, kiểm soát được trải nghiệm, seller vẫn sở hữu hàng.

**Điểm phức tạp kế toán:** hàng nằm trong kho nền tảng nhưng **quyền sở hữu thuộc seller**. Không được ghi nhận là tài sản của nền tảng. Đây là lý do `Inventory` cần trường `inventory_owner` tách biệt với `stock_location`.

```text
Inventory {
  sku_id
  stock_location_id    ← ở đâu (kho nền tảng)
  inventory_owner_id   ← của ai (seller)
  quantity_by_state
}
```

Đây là mô hình duy nhất xử lý được cả ba trường hợp mà không cần bảng riêng.

---

## 3. Tách Order và Fulfillment Order

### Vấn đề

Một đơn hàng của khách có thể chứa hàng từ nhiều nguồn:

```text
Giỏ hàng của khách:
├── Áo sơ mi own brand      (kho nền tảng, Hà Nội)
├── Giày từ Seller A         (kho seller A, TP.HCM)
└── Túi từ Seller B          (kho seller B, Đà Nẵng)
```

Ba món này **không thể** đóng chung một gói. Chúng ở ba nơi khác nhau, do ba bên khác nhau xử lý.

### Giải pháp

```text
Khách nhìn thấy:
    Order #1000 — tổng 1.250.000đ, đặt ngày 10/08

Hệ thống xử lý:
    Order #1000
    ├── FulfillmentOrder #1000-A  (own brand, kho HN)   → giao 11/08
    ├── FulfillmentOrder #1000-B  (Seller A, TP.HCM)    → giao 13/08
    └── FulfillmentOrder #1000-C  (Seller B, Đà Nẵng)   → giao 14/08
```

### Điều gì thuộc về đâu

| Thông tin | Order | FulfillmentOrder |
|---|---|---|
| Khách hàng | Có | Tham chiếu |
| Địa chỉ giao | Có | Sao chép |
| Tổng tiền | Có | Phần của mình |
| Thanh toán | Có | Không |
| Mã đơn khách thấy | Có | Mã phụ |
| Người thực hiện | Không | Seller / kho |
| Trạng thái vận chuyển | Tổng hợp | Chi tiết |
| Mã vận đơn | Không | Có |
| Đối soát tiền cho seller | Không | Có |

**Nguyên tắc:** `Order` là **hợp đồng với khách hàng**. `FulfillmentOrder` là **đơn vị công việc vận hành**.

Xem [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md).

---

## 4. Trạng thái phải hỗ trợ

Việc tách hai khái niệm cho phép xử lý các tình huống mà mô hình một-đơn-một-trạng-thái không làm được:

### 4.1 Giao hàng từng phần (Partial Fulfillment)

```text
Order #1000: đang xử lý
├── FO-A: đã giao      ✓
├── FO-B: đang giao
└── FO-C: chờ xử lý
```

Khách nhận được món A trước, không phải chờ cả ba.

### 4.2 Hủy từng phần (Partial Cancellation)

```text
Seller B hết hàng → FO-C bị hủy
├── FO-A: đã giao
├── FO-B: đang giao
└── FO-C: đã hủy → hoàn tiền phần của FO-C
```

Đơn hàng **không** bị hủy toàn bộ. Chỉ phần liên quan.

### 4.3 Hoàn tiền từng phần (Partial Refund)

Hoàn đúng số tiền của phần bị hủy hoặc trả lại, cộng phần phí vận chuyển tương ứng nếu có.

**Điểm phức tạp:** nếu khách được miễn phí vận chuyển do đạt ngưỡng đơn hàng, mà sau đó hủy một phần khiến đơn không còn đạt ngưỡng — có thu lại phí vận chuyển không? Đây là quyết định chính sách phải được ghi rõ, không để tùy ý xử lý.

**Khuyến nghị:** không thu lại. Chi phí xử lý tranh chấp và tổn hại trải nghiệm lớn hơn số tiền thu về.

### 4.4 Trả hàng từng phần (Partial Return)

Khách nhận ba món, chỉ trả một món (thường vì không vừa size).

Hệ quả: chỉ đảo ngược phần tiền và hoa hồng liên quan tới món đó.

---

## 5. Phân bổ nguồn hàng (Sourcing)

Khi own brand có nhiều kho, phải quyết định kho nào xuất hàng.

```text
Tiêu chí:
  1. Có đủ hàng không
  2. Khoảng cách tới khách (thời gian và chi phí giao)
  3. Cân bằng tồn kho giữa các kho
  4. Ưu tiên giải phóng hàng sắp hết mùa
```

**Nguyên tắc P14:** bắt đầu bằng quy tắc đơn giản — kho gần nhất có đủ hàng. Tối ưu phức tạp hơn (chia đơn để giảm tổng chi phí) chỉ cần khi có nhiều kho và khối lượng lớn.

**Cảnh báo:** chia một đơn thành nhiều gói làm tăng chi phí vận chuyển và giảm trải nghiệm. Chỉ chia khi thật sự cần.

---

## 6. Trạng thái FulfillmentOrder

```text
   Pending (Chờ xử lý)
      │
      ▼
   Allocated (Đã phân bổ nguồn hàng, tồn kho đã committed)
      │
      ▼
   Picking (Đang lấy hàng)
      │
      ▼
   Packed (Đã đóng gói)
      │
      ▼
   Handed Over (Đã bàn giao vận chuyển)
      │
      ▼
   In Transit (Đang vận chuyển)
      │
      ├──→ Delivery Failed (Giao không thành công)
      │        │
      │        ├──→ thử lại → In Transit
      │        └──→ Returned to Sender (Hoàn về người gửi)
      │
      ▼
   Delivered (Đã giao)
      │
      ▼
   Completed (Hoàn tất — hết hạn đổi trả)

Nhánh hủy:
   Pending / Allocated ──→ Cancelled (giải phóng tồn kho)
   Sau Packed          ──→ chỉ hủy được với chi phí, cần xử lý riêng
```

**Điểm quan trọng:** trạng thái `Completed` khác `Delivered`. Giao hàng xong chưa phải kết thúc — còn thời hạn đổi trả. Payout cho seller chỉ diễn ra sau `Completed`.

---

## 7. Giao hàng không thành công

Tình huống phổ biến hơn nhiều so với dự kiến, đặc biệt với hình thức thanh toán khi nhận hàng.

```text
Nguyên nhân:
  - Khách không có nhà
  - Sai địa chỉ / số điện thoại
  - Khách từ chối nhận
  - Khách đổi ý

Xử lý:
  Lần 1 → liên hệ khách, hẹn lại
  Lần 2 → liên hệ, hẹn lại
  Lần 3 → hoàn về người gửi

Hệ quả tài chính:
  - Chi phí vận chuyển hai chiều
  - Hàng phải nhập lại kho, kiểm tra lại
  - Nếu thanh toán trước → hoàn tiền
```

**Hệ quả kiến trúc:** hàng hoàn về phải đi qua quy trình kiểm định trước khi trở lại trạng thái `Available`. Không được tự động cộng lại tồn kho — hàng có thể đã hư hỏng trong quá trình vận chuyển hai chiều.

---

## 8. Đặc thù đóng gói thời trang

| Yêu cầu | Lý do |
|---|---|
| Không gấp gây nếp với hàng cao cấp | Ảnh hưởng cảm nhận chất lượng |
| Túi/hộp có thương hiệu | Trải nghiệm mở hộp là điểm chạm marketing |
| Kèm phiếu đổi trả | Giảm ma sát khi khách cần đổi size |
| Bảo vệ chống ẩm | Vải bị ẩm mốc trong vận chuyển |
| Đóng gói nhiều món cùng đơn | Giảm chi phí, tốt cho môi trường |

**Trải nghiệm mở hộp** là điểm khác biệt thật sự của own brand so với marketplace. Đây là lý do chiến lược để giữ fulfillment own brand tự làm thay vì thuê ngoài hoàn toàn.

---

## 9. Tích hợp đối tác vận chuyển

**Nguyên tắc P13:** vận chuyển là năng lực thay thế được, phải nằm sau interface.

```text
Domain định nghĩa:

  ShippingProvider interface {
      Quote(from, to, package) → giá, thời gian dự kiến
      CreateShipment(...)      → mã vận đơn
      Track(trackingNumber)    → trạng thái
      Cancel(trackingNumber)   → hủy
  }

Hạ tầng cài đặt:
  - Đối tác A
  - Đối tác B
  - Đối tác C
```

Module `fulfillment` **không** biết tên nhà vận chuyển cụ thể trong domain logic. Việc chọn nhà vận chuyển nào là cấu hình và quy tắc, không phải mã cứng.

**Lý do thực tế:** giá và chất lượng dịch vụ của các đối tác vận chuyển thay đổi thường xuyên. Nền tảng cần đổi hoặc dùng đồng thời nhiều đối tác mà không sửa domain.

---

## 10. Chỉ số fulfillment

| Chỉ số | Ý nghĩa | Mục tiêu tham khảo |
|---|---|---|
| Order processing time | Từ xác nhận đến bàn giao vận chuyển | < 24 giờ |
| On-time delivery rate | Giao đúng cam kết | > 95% |
| Delivery success rate | Giao thành công lần đầu | > 90% |
| Perfect order rate | Đơn đúng, đủ, đúng hạn, không hư hỏng | > 95% |
| Split shipment rate | Tỷ lệ đơn bị chia nhiều gói | Càng thấp càng tốt |
| Cost per shipment | Chi phí trung bình mỗi lần giao | Theo dõi xu hướng |
| Return-to-sender rate | Tỷ lệ hoàn về người gửi | < 3% |

---

## 11. Tài liệu liên quan

- [../04-modules/fulfillment.md](../04-modules/fulfillment.md) — đặc tả module
- [../04-modules/warehouse.md](../04-modules/warehouse.md) — vận hành kho
- [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) — luồng đơn nhiều nhà bán
- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md) — quyết định tách Order/FulfillmentOrder
