# Luồng: Đơn hàng nhiều nhà bán

## 1. Tình huống

```text
Giỏ hàng của khách:
├── Áo sơ mi own brand      (kho nền tảng, Hà Nội)     299.000đ
├── Giày từ Seller A         (kho seller A, TP.HCM)     650.000đ
└── Túi từ Seller B          (kho seller B, Đà Nẵng)    301.000đ
                                                    ─────────────
                                          Tổng:      1.250.000đ
```

Ba món này **không thể** đóng chung một gói. Chúng ở ba nơi, do ba bên xử lý.

---

## 2. Mô hình dữ liệu

```text
Khách nhìn thấy:
    Order #FC-2026-08-001234 — 1.250.000đ

Hệ thống thực thi:
    Order #FC-2026-08-001234
    ├── FulfillmentOrder ...-A  (own brand)  → giao 11/08
    ├── FulfillmentOrder ...-B  (Seller A)   → giao 13/08
    └── FulfillmentOrder ...-C  (Seller B)   → giao 14/08
```

---

## 3. Sequence diagram — tạo đơn và tách

```mermaid
sequenceDiagram
    autonumber
    participant Chk as checkout
    participant Mkt as marketplace
    participant Aff as affiliate
    participant Ord as order
    participant Bus as Event Bus
    participant Ful as fulfillment
    participant Inv as inventory
    participant Pay as payment

    Chk->>Ord: PlaceOrder(checkout_id)

    loop Với mỗi dòng hàng
        Ord->>Mkt: GetCommissionRate(seller, category)
        Mkt-->>Ord: tỷ lệ (ví dụ 10%)
        Ord->>Aff: ResolveAttribution(offer, session)
        Aff-->>Ord: creator_id + tỷ lệ (nếu có)
    end

    Note over Ord: ĐÓNG BĂNG vào OrderLine:<br/>giá, tên SP, hoa hồng NT,<br/>hoa hồng creator

    Ord->>Ord: GIAO DỊCH: order + order_line + outbox
    Ord->>Bus: order.placed

    Bus->>Inv: Reserved → Committed
    Bus->>Pay: ghi bút toán
    Bus->>Aff: xác nhận quy kết

    Bus->>Ful: order.paid
    Note over Ful: Nhóm OrderLine theo<br/>(seller_id, nguồn hàng)
    Ful->>Ful: Tạo FO-A (own brand)
    Ful->>Ful: Tạo FO-B (Seller A)
    Ful->>Ful: Tạo FO-C (Seller B)
    Ful->>Bus: fulfillment_order.created ×3
    Note over Ful: Việc tách phải IDEMPOTENT —<br/>kiểm tra đã có FO cho order_id chưa
```

---

## 4. Bút toán tài chính

Đơn 1.250.000đ với ba nguồn hàng:

```text
LedgerEntry: ORDER_REVENUE, ref=Order#FC-2026-08-001234

Phần own brand (299.000đ):
  DEBIT   PLATFORM_CASH                    299.000
  CREDIT  PLATFORM_REVENUE                 299.000

  (bút toán riêng cho COGS)
  DEBIT   COGS                             120.000
  CREDIT  INVENTORY_ASSET                  120.000

Phần Seller A (650.000đ, hoa hồng 8%):
  DEBIT   PLATFORM_CASH                    650.000
  CREDIT  SELLER_PAYABLE (A)               598.000
  CREDIT  PLATFORM_REVENUE                  52.000

Phần Seller B (301.000đ, hoa hồng 12%, creator 5%):
  DEBIT   PLATFORM_CASH                    301.000
  CREDIT  SELLER_PAYABLE (B)               249.880
  CREDIT  PLATFORM_REVENUE                  36.120
  CREDIT  CREATOR_PAYABLE                   15.050
```

**Quan sát quan trọng:**

```text
GMV       = 1.250.000đ
Doanh thu nền tảng = 299.000 (own brand toàn phần)
                   + 52.000  (hoa hồng A)
                   + 36.120  (hoa hồng B)
                   = 387.120đ

GMV ≠ Doanh thu. Phân biệt này nằm ở TẦNG GHI SỔ,
không phải xử lý sau bằng báo cáo.
```

---

## 5. Xử lý từng phần — điều chỉ khả thi khi tách Order/FO

### 5.1 Giao hàng từng phần

```mermaid
sequenceDiagram
    participant FoA as FO-A (own brand)
    participant FoB as FO-B (Seller A)
    participant FoC as FO-C (Seller B)
    participant Ord as order
    participant Bus as Event Bus

    FoA->>Bus: fulfillment_order.delivered
    Bus->>Ord: tính lại trạng thái tổng hợp
    Note over Ord: 1/3 đã giao → PARTIALLY_DELIVERED

    FoB->>Bus: fulfillment_order.delivered
    Bus->>Ord: PARTIALLY_DELIVERED (2/3)

    FoC->>Bus: fulfillment_order.delivered
    Bus->>Ord: DELIVERED (3/3)
```

Khách nhận món A trước, không phải chờ cả ba.

### 5.2 Hủy từng phần — Seller B hết hàng

```mermaid
sequenceDiagram
    actor SB as Seller B
    participant Ful as fulfillment
    participant Ord as order
    participant Bus as Event Bus
    participant Inv as inventory
    participant Pay as payment
    participant Aff as affiliate

    SB->>Ful: Không thể giao (hết hàng)
    Ful->>Ord: CancelOrderLine(oln_C, lý do)
    Ord->>Ord: OrderLine C → CANCELLED
    Note over Ord: Order → PARTIALLY_CANCELLED<br/>A và B VẪN GIAO BÌNH THƯỜNG
    Ord->>Bus: order.line_cancelled

    Bus->>Inv: giải phóng hàng phần C
    Bus->>Pay: hoàn tiền phần C (301.000đ)
    Bus->>Aff: đảo ngược hoa hồng creator phần C
    Note over Pay: Đảo ngược ĐỦ chuỗi:<br/>tiền khách, hoa hồng NT,<br/>số dư seller, hoa hồng creator
```

**Điểm mấu chốt:** đơn hàng **không** bị hủy toàn bộ. Nếu mô hình chỉ có một `Order` với một trạng thái, không làm được điều này — phải hủy cả đơn, khách mất luôn hai món còn lại.

### 5.3 Vấn đề phí vận chuyển khi hủy một phần

```text
Khách mua 1.250.000đ, miễn phí ship (ngưỡng 500.000đ)
Hủy món 301.000đ → còn 949.000đ, vẫn đạt ngưỡng → không vấn đề

Nhưng nếu ngưỡng là 1.000.000đ:
    → sau khi hủy còn 949.000đ, không còn đạt ngưỡng

Câu hỏi: có thu lại phí ship không?
```

**Quyết định: KHÔNG thu lại.**

Lý do: chi phí xử lý tranh chấp và tổn hại trải nghiệm lớn hơn số tiền thu về. Khách bị phạt vì lỗi của seller là trải nghiệm rất tệ.

Quyết định này phải được ghi vào quy tắc nghiệp vụ, không để mỗi trường hợp xử lý một kiểu.

---

## 6. Ranh giới dữ liệu giữa các seller

```mermaid
sequenceDiagram
    actor SA as Seller A
    participant API as Seller API
    participant Ful as fulfillment

    SA->>API: GET /seller/fulfillment-orders/{FO-B}
    API->>Ful: lấy FO, LỌC theo seller_id trong truy vấn
    Ful-->>API: chỉ FO-B, chỉ hàng của Seller A
    API-->>SA: dữ liệu FO-B

    Note over SA: KHÔNG thấy:<br/>order_id · tổng tiền đơn<br/>FO-A, FO-C · tên seller khác<br/>lịch sử mua của khách
```

**Đây là lý do kỹ thuật thứ hai cho việc tách Order/FulfillmentOrder** (bên cạnh lý do tranh chấp ghi).

Nếu seller truy cập `Order`, phải lọc dữ liệu ở tầng hiển thị — chỉ cần quên một lần là rò rỉ dữ liệu đối thủ. Với `FulfillmentOrder`, ranh giới nằm sẵn trong cấu trúc dữ liệu.

---

## 7. Đối soát và chi trả

```mermaid
sequenceDiagram
    participant Ful as fulfillment
    participant Bus as Event Bus
    participant Pay as payment
    participant Job as Job đối soát
    actor Adm as Nhân viên tài chính
    participant Bank as Ngân hàng

    Ful->>Bus: fulfillment_order.delivered
    Note over Pay: Bắt đầu đếm hạn đổi trả (7 ngày)

    Note over Ful: Hết hạn đổi trả, không có yêu cầu trả
    Ful->>Bus: fulfillment_order.completed
    Bus->>Pay: chuyển số dư Pending → Available

    Job->>Pay: Đến kỳ đối soát (thứ Ba hàng tuần)
    Pay->>Pay: Tổng hợp: doanh thu − hoa hồng<br/>− phí − hoàn tiền ± điều chỉnh
    Pay->>Pay: Tạo Settlement

    Adm->>Pay: Duyệt payout (2FA + Idempotency-Key)
    Pay->>Pay: Kiểm tra trạng thái TRƯỚC khi gọi ngân hàng
    Pay->>Bank: Chuyển tiền
    Bank-->>Pay: Xác nhận
    Pay->>Pay: Ghi bút toán:<br/>DEBIT SELLER_PAYABLE<br/>CREDIT PLATFORM_CASH
    Pay->>Bus: payout.executed
```

### Vì sao chỉ chi trả sau khi hết hạn đổi trả

```text
Nếu trả tiền ngay khi giao hàng:
    → khách hoàn hàng
    → nền tảng phải ĐÒI LẠI tiền từ seller
    → rất khó thu hồi, đặc biệt với seller nhỏ

Nếu chờ hết hạn đổi trả:
    → phần lớn trường hợp hoàn hàng xảy ra TRƯỚC khi chi tiền
    → chỉ cần trừ vào số dư, không phải đòi
```

### Xử lý số dư âm

```text
Seller bị hoàn 2 triệu, chỉ bán được 1 triệu trong kỳ
    → Số dư kỳ này = −1.000.000đ

Xử lý:
    1. KHÔNG chuyển tiền âm
    2. Chuyển khoản âm sang kỳ sau
    3. Nếu âm kéo dài → yêu cầu nộp bù
    4. Nếu seller ngừng hoạt động → khoản phải thu khó đòi

Phòng ngừa: giữ reserve với seller mới hoặc tỷ lệ hoàn cao
```

---

## 8. Điểm cần giám sát

| Chỉ báo | Ngưỡng |
|---|---|
| Đơn không tách được thành FO | 0 |
| Σ FulfillmentLine ≠ Σ OrderLine | 0 |
| Bút toán không cân bằng | 0 |
| Payout thất bại | Cảnh báo ngay |
| Seller có số dư âm | Theo dõi danh sách |
| Tỷ lệ hủy do seller hết hàng | < 3% |

Ba chỉ báo đầu phải luôn bằng 0.

---

## 9. Tài liệu liên quan

- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md)
- [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md)
- [../04-modules/fulfillment.md](../04-modules/fulfillment.md), [../04-modules/payment.md](../04-modules/payment.md)
- [../01-business/monetization.md](../01-business/monetization.md)
