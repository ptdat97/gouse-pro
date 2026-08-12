# Sylius

| | |
|---|---|
| Repository | `github.com/Sylius/Sylius` |
| License | MIT |
| Sao / Fork | 8.513 / 2.160 |
| Ngôn ngữ | PHP |
| Cập nhật cuối | 2026-08-12 (tích cực) |
| Vai trò | **Nguồn hai phát hiện làm thay đổi thiết kế của chúng ta** |

---

## 1. Vì sao Sylius quan trọng hơn số sao của nó

Sylius không phải dự án lớn nhất trong nhóm, nhưng nó mô hình hóa **hai thứ mà mọi dự án khác làm sơ sài**:

```text
1. Adjustment  — mọi khoản tiền cộng/trừ là thực thể hạng nhất
2. Nhiều state machine tách biệt — checkout, payment, shipping độc lập
```

Cả hai đều trực tiếp giải quyết vấn đề đã nêu trong tài liệu của chúng ta nhưng chưa có lời giải đầy đủ.

---

## Năng lực: Adjustment — khoản tiền là thực thể, không phải trường

### Cách OSS làm

Mọi khoản cộng/trừ vào giá trị đơn hàng là một `Adjustment` gắn vào `Order` hoặc `OrderItem`:

```text
Adjustment {
    type    ORDER_PROMOTION | SHIPPING | TAX | ...
    label   nhãn hiển thị
    amount  số tiền (âm hoặc dương)
    origin  nguồn gốc — quy tắc khuyến mãi nào tạo ra nó
}
```

Phí vận chuyển là adjustment. Thuế là adjustment. Giảm giá là adjustment.

### Điểm mạnh

**Truy vết được nguồn gốc từng đồng.** Khi khách hỏi "sao tôi bị tính 30.000đ này?", hệ thống trả lời được chính xác khoản nào, do quy tắc nào.

**Tính lại được.** Khi giỏ hàng đổi, xóa hết adjustment và tính lại — không sợ sót hay tính hai lần.

**Hoàn tiền từng phần chính xác.** Adjustment gắn vào từng dòng hàng, nên hoàn một món biết chính xác phần thuế và giảm giá tương ứng.

### Điểm yếu

Nhiều bản ghi hơn một trường số. Truy vấn tổng phải cộng adjustment thay vì đọc một cột.

### Yêu cầu của chúng ta — và khoảng trống đã có

[07-workflows/return.md](../07-workflows/return.md) mục 5 của chúng ta nêu vấn đề:

```text
Đơn: 3 món, tổng 500.000đ, giảm 50.000đ (10%)
Khách trả món C (100.000đ)
    SAI:  hoàn 100.000đ (giá niêm yết)
    ĐÚNG: hoàn  90.000đ (giá thực trả)
```

Và kết luận: "giảm giá phải được **phân bổ theo tỷ lệ xuống từng dòng hàng và lưu lại**".

Nhưng tài liệu **không nói lưu ở đâu và dưới dạng gì.** `OrderLine` chỉ có `unit_price` và `line_total`.

Sylius trả lời câu hỏi này: lưu thành `Adjustment` gắn vào dòng hàng.

### Adopt

**Adjustment là thực thể hạng nhất.**

```text
OrderLineAdjustment {
    id
    order_line_id
    type          PROMOTION | TAX | SHIPPING | COMMISSION | FEE
    label         "Giảm giá THUDONG20"
    amount        Money (âm = giảm, dương = tăng)
    source_type   PROMOTION | TAX_RULE | SHIPPING_METHOD
    source_id     định danh nguồn
    cost_bearer   PLATFORM | SELLER | SHARED   ← đặc thù marketplace
}
```

Trường `cost_bearer` là bổ sung của chúng ta — Sylius không cần vì không phải marketplace.

Lợi ích trực tiếp:

```text
✓ Hoàn tiền từng phần tính đúng
✓ Đối soát seller biết chính xác khoản nào trừ vào ai
✓ Giải thích được cho khách từng dòng tiền
✓ Tính lại giỏ hàng an toàn
```

### Quyết định cuối

```text
✓ ADOPT — bổ sung Adjustment vào mô hình Order
→ Cập nhật docs/02-domain/entities.md
→ Cập nhật docs/04-modules/order.md
→ Cập nhật docs/04-modules/promotion.md
```

---

## Năng lực: Nhiều state machine tách biệt

### Cách OSS làm

Sylius dùng Symfony Workflow với **nhiều máy trạng thái độc lập** cho cùng một đơn hàng:

```text
checkout state:  cart → addressed → shipping_selected
                 → payment_selected → completed

payment state:   mỗi Payment có state machine RIÊNG
                 (một đơn có thể nhiều payment)

shipping state:  mỗi Shipment có state machine RIÊNG
                 (một đơn có thể nhiều shipment)
```

Mỗi `Shipment` có phương thức giao và vòng đời riêng — cho phép chia một đơn thành nhiều lô giao.

### Điểm mạnh

**Tách mối quan tâm đúng chỗ.** Thanh toán và giao hàng là hai tiến trình độc lập; ép chúng vào một trạng thái tuyến tính làm mất thông tin.

Ví dụ tình huống thật: đơn đã thanh toán xong, lô A đã giao, lô B đang vận chuyển, lô C bị hủy. Một trạng thái duy nhất không diễn đạt được.

### So sánh với thiết kế hiện tại của chúng ta

Chúng ta **đã đi đúng hướng** nhưng chưa đặt tên rõ:

| Sylius | Chúng ta | Trạng thái |
|---|---|---|
| checkout state | `Checkout.status` | Đã có |
| payment state (nhiều) | `Payment.status` | Có, nhưng chưa hỗ trợ nhiều payment/đơn |
| shipment state (nhiều) | `FulfillmentOrder.status` | **Đã có — tương đương** |
| order state | `Order.status` (suy ra) | Đã có |

Phát hiện: `FulfillmentOrder` của chúng ta **chính là** `Shipment` của Sylius về mặt khái niệm. Sylius xác nhận cách tách này đúng.

Khoảng trống duy nhất: chúng ta chưa hỗ trợ **nhiều Payment cho một Order**.

### Adopt

**Nguyên tắc: mỗi tiến trình độc lập có vòng đời riêng.**

Áp dụng: một `Order` có thể có nhiều `Payment` — cần cho:

```text
Thanh toán một phần bằng điểm thưởng + phần còn lại bằng thẻ
Hoàn tiền một phần khi hủy từng phần
Thử lại thanh toán sau khi thất bại
```

### Adapt

**Không dùng thư viện workflow engine.** Cài đặt bằng phương thức domain thuần Go, vì:

- Domain layer phải sạch (quy tắc R2 của archcheck)
- Số chuyển đổi vừa phải, viết tay đọc dễ hơn cấu hình
- Không thêm phụ thuộc

Quy tắc quyết định khi nào cần state machine hình thức:

```text
CẦN khi:
  ✓ > 5 trạng thái
  ✓ Chuyển đổi có điều kiện phức tạp
  ✓ Cần hiển thị vòng đời cho người vận hành
  ✓ Sai trạng thái gây hậu quả tài chính

Áp dụng cho: Order, FulfillmentOrder, ReturnRequest,
             ProductionOrder, Seller

KHÔNG CẦN với: Cart, Content, Category — chỉ vài trạng thái đơn giản
```

### Quyết định cuối

```text
✓ Xác nhận tách Order/FulfillmentOrder đúng
✓ Bổ sung: một Order có nhiều Payment (Phase 2)
✓ State machine bằng phương thức domain, không dùng thư viện
✗ Không áp dụng state machine cho mọi thực thể
→ Cập nhật docs/04-modules/payment.md
```

---

## Năng lực: Resource pattern

### Cách OSS làm

Mọi thực thể là "resource" với CRUD, route, form, grid sinh tự động từ cấu hình.

### Điểm mạnh

Rất nhanh cho màn hình quản trị chuẩn.

### Điểm yếu với chúng ta

Cùng vấn đề với QOR admin: giao diện sinh ra phản ánh **cấu trúc dữ liệu**, không phản ánh **quy trình công việc**.

Màn hình quan trọng nhất của chúng ta (đề xuất bổ sung hàng, đối soát seller) là hỗ trợ ra quyết định, không phải CRUD.

### Reject

Xem [qor.md](qor.md) — cùng lý do.

---

## 2. Tổng kết Sylius

| Hạng mục | Quyết định |
|---|---|
| **Adjustment là thực thể hạng nhất** | **ADOPT** — lấp khoảng trống thật |
| **Nhiều state machine tách biệt** | **ADOPT** — xác nhận + bổ sung nhiều Payment |
| Shipment có vòng đời riêng | **ADOPT** — đã có qua FulfillmentOrder |
| Thư viện workflow engine | **ADAPT** — tự cài bằng phương thức domain |
| Resource pattern / admin sinh tự động | **REJECT** — như QOR |
| Kiến trúc PHP/Symfony | Không áp dụng |

**Nhận xét cuối:** Sylius đóng góp nhiều nhất trong nhóm non-Go về **mô hình domain thương mại**. Hai phát hiện của nó không phải "ý tưởng hay để tham khảo" mà là **lời giải cho vấn đề đã biết nhưng chưa giải quyết** trong tài liệu của chúng ta.

---

## 3. Tài liệu liên quan

- [../07-workflows/return.md](../07-workflows/return.md) mục 5 — vấn đề mà Adjustment giải quyết
- [../02-domain/entities.md](../02-domain/entities.md) — đã cập nhật
- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md)

## 4. Nguồn

- [Sylius State Machine](https://docs.sylius.com/the-book/architecture/state-machine)
- [Sylius Orders](https://docs.sylius.com/the-book/carts-and-orders/orders)
