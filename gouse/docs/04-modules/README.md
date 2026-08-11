# Đặc tả Module

## Cách đọc

Mỗi module được mô tả theo cùng một khung:

```text
1. Trách nhiệm            — module này làm gì
2. Không thuộc trách nhiệm — làm rõ ranh giới
3. Khái niệm domain        — aggregate, entity chính
4. Dữ liệu sở hữu          — bảng
5. Interface công khai     — module khác gọi được gì
6. Use case                — thao tác nghiệp vụ
7. Event phát / nghe
8. Phụ thuộc
9. Quy tắc nghiệp vụ quan trọng
10. Giai đoạn triển khai
```

Mục 2 ("không thuộc trách nhiệm") quan trọng ngang mục 1 — phần lớn lỗi kiến trúc đến từ việc module ôm việc không phải của mình.

---

## Danh sách module theo tầng

### Tầng nền

| Module | Context | Phân loại | Giai đoạn |
|---|---|---|---|
| [identity](identity.md) | Platform | Generic | MVP |
| [notification](notification.md) | Platform | Generic | MVP |
| [analytics](analytics.md) | Platform | Supporting | MVP (cơ bản) |

### Tầng dữ liệu chính

| Module | Context | Phân loại | Giai đoạn |
|---|---|---|---|
| [customer](customer.md) | Commerce | Supporting | MVP |
| [catalog](catalog.md) | Commerce | **Core** | MVP |
| [product](product.md) | Commerce | **Core** | MVP |
| [pricing](pricing.md) | Commerce | Supporting | MVP |
| [inventory](inventory.md) | Inventory | Supporting | MVP |

### Tầng nghiệp vụ

| Module | Context | Phân loại | Giai đoạn |
|---|---|---|---|
| [marketplace](marketplace.md) | Marketplace | **Core** | MVP |
| [seller](seller.md) | Marketplace | Supporting | MVP |
| [promotion](promotion.md) | Commerce | Supporting | MVP |
| [creator](creator.md) | Growth | **Core** | Phase 2 |
| [content](content.md) | Growth | **Core** | Phase 2 |
| [affiliate](affiliate.md) | Growth | **Core** | Phase 2 |
| [campaign](campaign.md) | Growth | Supporting | Phase 2 |
| [recommendation](recommendation.md) | Growth | Supporting | Phase 2 |
| [loyalty](loyalty.md) | Growth | Supporting | Phase 3 |
| [quality](quality.md) | Supply Chain | Supporting | Phase 3 |
| [warehouse](warehouse.md) | Supply Chain | Supporting | Phase 2 |

### Tầng giao dịch

| Module | Context | Phân loại | Giai đoạn |
|---|---|---|---|
| [cart](cart.md) | Commerce | Supporting | MVP |
| [order](order.md) | Commerce | **Core** | MVP |
| [payment](payment.md) | Financial | **Core** | MVP |
| [return](return.md) | Commerce | Supporting | Phase 2 |
| [procurement](procurement.md) | Supply Chain | Supporting | Phase 3 |
| [manufacturing](manufacturing.md) | Supply Chain | **Core** | Phase 3 |

### Tầng điều phối

| Module | Context | Phân loại | Giai đoạn |
|---|---|---|---|
| [checkout](checkout.md) | Commerce | Supporting | MVP |
| [fulfillment](fulfillment.md) | Commerce | **Core** | MVP |
| [supply-chain](supply-chain.md) | Supply Chain | **Core** | Phase 3 (ghi tín hiệu từ MVP) |

---

## Đồ thị phụ thuộc tóm tắt

```text
checkout · fulfillment · supply-chain          ← tầng điều phối
        │
cart · order · payment · return                ← tầng giao dịch
procurement · manufacturing
        │
marketplace · seller · creator · content       ← tầng nghiệp vụ
affiliate · campaign · promotion
warehouse · quality · loyalty · recommendation
        │
catalog · product · pricing                    ← tầng dữ liệu chính
inventory · customer
        │
identity · notification · analytics            ← tầng nền

Phụ thuộc CHỈ đi từ trên xuống.
Từ dưới lên: CHỈ qua event.
```

Chi tiết: [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md).

---

## Module theo giai đoạn

### MVP (14 module)

```text
identity · notification · analytics (cơ bản)
customer · catalog · product · pricing · inventory
marketplace · seller · promotion (cơ bản)
cart · checkout · order · payment · fulfillment
```

Cộng thêm: ghi nhận `demand_signal` (thuộc supply-chain) dù chưa dùng — lý do tại [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 9.

### Phase 2 (+7 module)

```text
creator · content · affiliate · campaign
recommendation · return · warehouse
```

### Phase 3 (+5 module)

```text
supply-chain (đầy đủ) · procurement · manufacturing
quality · loyalty
```

### Phase 4

Không thêm module mới. Nâng cấp chiều sâu:

```text
recommendation  → cá nhân hóa nâng cao
supply-chain    → dự báo nâng cao
analytics       → phân tích chuyên sâu
marketplace     → retail media
```

Xem [../10-roadmap/](../10-roadmap/).
