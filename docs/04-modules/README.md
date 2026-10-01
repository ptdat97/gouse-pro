# Đặc tả Module

> **Về các đoạn SQL trong tài liệu module:** chúng là **minh họa cấu trúc**,
> không phải DDL thật. Nhiều đoạn viết `id UUID PRIMARY KEY` cho ngắn gọn;
> code thật dùng `TEXT` + ULID có tiền tố loại kèm ràng buộc `CHECK`. Xem
> [../05-data/data-model.md](../05-data/data-model.md) mục 1.1 — đó là nguồn
> sự thật về định danh. `migrations/*.sql` trong repo là DDL thật.


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

## Hai trục phân loại, và chúng KHÔNG mâu thuẫn

Mỗi tài liệu module có hai hàng dễ nhìn tưởng là nói ngược nhau:

```text
| Phân loại           | Core                  |   ← trục GIÁ TRỊ KINH DOANH
| Tầng (microkernel)  | EXTENSION — đã dựng   |   ← trục ĐỘ ỔN ĐỊNH
```

Ví dụ trên là `affiliate` thật. Nó **Core** và **EXTENSION** cùng lúc, và cả
hai đều đúng:

| Trục | Câu hỏi | Giá trị |
|---|---|---|
| `Phân loại` | Nó có phải lợi thế cạnh tranh của doanh nghiệp? | Core · Supporting · Generic |
| `Tầng (microkernel)` | Tháo nó ra thì khách còn mua được hàng? | KERNEL · EXTENSION · CHƯA XẾP ĐƯỢC |

Hoa hồng creator **là** lợi thế cạnh tranh (Core), và tháo nó ra thì khách
**vẫn** mua được hàng (EXTENSION). Trộn hai trục lại sẽ dẫn tới một trong hai
sai lầm:

```text
coi Core là KERNEL     → nhồi logic hoa hồng creator vào order
                         (đã chặn: P3-83 bỏ order_line.creator_commission_rate)

coi EXTENSION là phụ   → làm nó cẩu thả, dù nó là chỗ sinh tiền
```

Một `Phân loại: Supporting` nằm trong KERNEL cũng bình thường: `cart` không
phải lợi thế cạnh tranh, nhưng tháo nó ra thì không ai mua được gì.

Hàng `Tầng (microkernel)` là **nguồn sự thật** của phân loại ấy — `cmd/archcheck`
R10 đọc chính nó, cùng cách R6 đọc mục "Dữ liệu sở hữu". Không có sổ phân
loại nào trong mã.

Hệ quả, và nó là chủ ý:

```text
tài liệu MẤT hàng ấy            archcheck LỖI khởi động, không phải vi phạm
tài liệu ghi giá trị LẠ         archcheck LỖI khởi động
tài liệu khai KERNEL mà module  R10c ĐỎ — đó là hình dạng một extension
  ngoài đường mua hàng và
  không ai import
tài liệu khai EXTENSION mà      R10a ĐỎ — tháo nó ra sẽ làm bên kia
  có module import nó             không biên dịch được
```

Nghĩa là **thêm một module mà không trả lời "tháo nó ra thì mất gì" thì CI
không chạy qua được.**

Xem [../03-architecture/microkernel.md](../03-architecture/microkernel.md) và
[ADR-0022](../adr/0022-microkernel-kernel-on-dinh.md).

### Bảng tra nhanh theo tầng

```text
KERNEL — đường mua hàng (12)
    cart · catalog · checkout · fulfillment · inventory
    marketplace · order · payment · pricing · product
    promotion · seller

KERNEL — năng lực cắt ngang (2)
    identity · customer

EXTENSION — đã dựng (6)
    affiliate · return · notification · analytics
    recommendation · supply-chain

EXTENSION — chưa dựng (8)
    campaign · content · creator · loyalty
    manufacturing · procurement · quality · warehouse

CHƯA XẾP ĐƯỢC (0)
    `pricing` từng ở đây; giải 01/10/2026 bằng ADR-0023.
    Trạng thái này vẫn HỢP LỆ — xếp sai tệ hơn để trống.
```

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

### MVP (16 module)

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
