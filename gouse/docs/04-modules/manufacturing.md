# Module: Manufacturing

| | |
|---|---|
| **Bounded Context** | Supply Chain |
| **Phân loại** | **Core** |
| **Giai đoạn** | Phase 3 |

---

## 1. Trách nhiệm

- Quản lý đơn sản xuất gửi nhà máy
- Quản lý **lô sản xuất** (`ProductionBatch`) — đơn vị truy vết
- Quản lý định mức nguyên phụ liệu
- Theo dõi tiến độ sản xuất
- **Tính và lưu giá vốn theo lô**

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Quyết định sản xuất gì, bao nhiêu | `supply-chain` |
| Hồ sơ nhà cung cấp, mua nguyên liệu | `procurement` |
| Kiểm định chất lượng | `quality` |
| Nhập kho | `warehouse` |

---

## 3. Phân biệt Purchase Order và Production Order

Đây là phân biệt quan trọng, không được gộp:

```text
Purchase Order (module procurement):
    Mua hàng CÓ SẴN từ nhà cung cấp
    Đặt hàng → Giao → Nhập kho → Kiểm → Bán

Production Order (module này):
    Đặt SẢN XUẤT theo thiết kế của nền tảng
    Tech pack → Làm mẫu → Duyệt mẫu → Đặt nguyên liệu
    → Sản xuất → QC tại xưởng → Giao → Nhập kho → QC → Bán
```

Luồng thứ hai dài hơn nhiều và có nhiều điểm có thể thất bại. Gộp chung sẽ mất khả năng quản lý các bước riêng của sản xuất.

---

## 4. ProductionBatch — khái niệm then chốt

```text
ProductionBatch {
    id
    production_order_id
    sku_id
    quantity_produced
    quantity_passed_qc
    quantity_rejected
    unit_cost               ← GIÁ VỐN CỦA LÔ NÀY
    production_date
    supplier_id
    material_batch_refs[]   ← truy vết nguyên liệu
    certificates[]
    status
}
```

### Vì sao bắt buộc phải có

| Câu hỏi | Không có batch | Có batch |
|---|---|---|
| Giá vốn thật của đơn hàng này? | Ước lượng | Chính xác |
| Lô nào tỷ lệ lỗi cao? | Không biết | Truy vết được |
| Cần thu hồi hàng lỗi — thu hồi cái nào? | **Toàn bộ SKU** | **Chỉ lô liên quan** |
| Nhà cung cấp nào chất lượng kém? | Không đo được | Đo theo lô |

**Kịch bản thu hồi:** phát hiện một lô vải có vấn đề (phai màu, hóa chất vượt ngưỡng). Không có `ProductionBatch`, phải thu hồi toàn bộ SKU — chi phí lớn hơn nhiều lần.

### Vì sao giá vốn phải theo lô, không theo SKU

```text
Hai lô cùng SKU sản xuất cách nhau ba tháng:
    - Giá vải đổi
    - Tỷ giá đổi
    - Số lượng đặt khác nhau → đơn giá khác

Nếu lưu một giá vốn duy nhất cho SKU:
    → mọi tính toán biên lợi nhuận đều sai
```

Xem [../01-business/supplier.md](../01-business/supplier.md) mục 4.

---

## 5. Cấu thành giá vốn

```text
Chi phí nguyên liệu              45.000đ
Chi phí gia công                 35.000đ
Chi phí phụ liệu (nhãn, bao bì)   8.000đ
Chi phí vận chuyển nhập           7.000đ
Chi phí QC                        3.000đ
Hao hụt sản xuất (2%)             2.000đ
─────────────────────────────────────────
COGS đơn vị                     100.000đ
```

Mỗi thành phần được lưu riêng để phân tích nguyên nhân biến động giá vốn.

---

## 6. Vòng đời đơn sản xuất

```text
    DRAFT
      ↓
    SENT (gửi nhà cung cấp)
      ↓
    CONFIRMED (nhà cung cấp xác nhận)
      ↓
    MATERIAL_SOURCING (đang mua nguyên liệu)
      ↓
    IN_PRODUCTION
      ↓
    QC_PENDING (chờ kiểm định tại xưởng)
      │
      ├──→ QC_FAILED → thương lượng: làm lại / giảm giá / hủy
      ↓
    QC_PASSED
      ↓
    SHIPPED
      ↓
    RECEIVED (đã về kho)
```

**Yêu cầu theo dõi tiến độ:** hệ thống phải cảnh báo khi tiến độ đe dọa ngày ra mắt bộ sưu tập. Hàng thời trang về trễ mùa mất phần lớn giá trị.

---

## 7. Dữ liệu sở hữu

```sql
production_order
production_order_line       -- phân bổ theo size
production_batch
bill_of_materials           -- định mức nguyên phụ liệu
production_milestone        -- mốc tiến độ
cost_breakdown              -- chi tiết cấu thành giá vốn
```

---

## 8. Interface công khai

```go
type PublicAPI interface {
    CreateProductionOrder(ctx, req CreateOrderRequest) (*ProductionOrder, error)
    GetProductionOrder(ctx, orderID string) (*ProductionOrderView, error)
    UpdateProductionStatus(ctx, orderID string, status string) error

    GetProductionBatch(ctx, batchID string) (*BatchView, error)
    GetBatchesBySKU(ctx, skuID string) ([]BatchView, error)

    // Cho việc tính biên lợi nhuận
    GetUnitCost(ctx, batchID string) (Money, error)
    GetUnitCostForSKU(ctx, skuID string, method CostMethod) (Money, error)

    // Truy vết thu hồi
    GetBatchTraceability(ctx, batchID string) (*TraceabilityReport, error)
}
```

---

## 9. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `production_order.created` | procurement, payment (đặt cọc) |
| `production_order.confirmed` | supply-chain |
| `production_order.delayed` | notification (cảnh báo) |
| `production_batch.completed` | **quality** |
| `production_batch.cost_finalized` | payment (ghi COGS) |

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `production_plan.created` | supply-chain | Tạo đơn sản xuất |
| `quality.approved` | quality | Cập nhật số lượng đạt |
| `quality.rejected` | quality | Xử lý lô không đạt |
| `warehouse.goods_received` | warehouse | Chốt lô đã nhập |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Không tạo đơn sản xuất khi chưa duyệt mẫu |
| 2 | Giá vốn gắn với lô, không gắn với SKU |
| 3 | Mọi lô phải truy vết được nguyên liệu |
| 4 | Đơn sản xuất phân bổ theo size |
| 5 | Cảnh báo khi tiến độ đe dọa ngày ra mắt |
| 6 | Lô không đạt QC không được nhập kho bán |

---

## 11. Tài liệu liên quan

- [supply-chain.md](supply-chain.md) — lập kế hoạch
- [procurement.md](procurement.md) — mua hàng và nhà cung cấp
- [quality.md](quality.md) — kiểm định
- [../07-workflows/supplier-production.md](../07-workflows/supplier-production.md)
