# Module: Procurement

| | |
|---|---|
| **Bounded Context** | Supply Chain |
| **Phân loại** | Supporting |
| **Giai đoạn** | Phase 3 |

---

## 1. Trách nhiệm

- Quản lý hồ sơ nhà cung cấp
- Đánh giá năng lực và chứng nhận nhà cung cấp
- Quản lý báo giá
- Tạo và theo dõi đơn mua hàng (`PurchaseOrder`)
- Chấm điểm hiệu suất nhà cung cấp

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Đơn sản xuất, lô sản xuất | `manufacturing` |
| Quyết định mua bao nhiêu | `supply-chain` |
| Kiểm định chất lượng | `quality` |
| Nhập kho | `warehouse` |
| Thanh toán nhà cung cấp | `payment` |

---

## 3. Bốn loại nhà cung cấp

```text
Supplier            — cung cấp hàng thành phẩm có sẵn
Manufacturer        — gia công theo tech pack của nền tảng
Material Supplier   — nguyên phụ liệu (vải, cúc, khóa, nhãn)
Production Partner  — thành phẩm + tham gia phát triển mẫu
```

Khác biệt chính: nền tảng có cung cấp thiết kế hay không, và ai sở hữu quyền sở hữu trí tuệ.

Chi tiết: [../01-business/supplier.md](../01-business/supplier.md) mục 1.

---

## 4. Đánh giá năng lực — trước khi phê duyệt

```text
SupplierCapability {
    production_capacity_per_month
    product_types[]              -- làm được loại nào
    moq                          -- số lượng đặt tối thiểu
    typical_lead_time_days
    quality_standards[]
}

SupplierCertification {
    certification_type           -- ISO, tiêu chuẩn lao động, môi trường
    issuer
    valid_from, valid_until      ← THEO DÕI HẠN
    document_url
}
```

**Về chứng nhận:** không chỉ là vấn đề đạo đức — nhiều thị trường xuất khẩu yêu cầu truy xuất nguồn gốc chuỗi cung ứng. Hệ thống phải cảnh báo khi chứng nhận sắp hết hạn.

---

## 5. Chỉ số hiệu suất nhà cung cấp

| Chỉ số | Ý nghĩa |
|---|---|
| On-time delivery rate | Tỷ lệ giao đúng hạn |
| Quality pass rate | Tỷ lệ lô đạt QC lần đầu |
| Defect rate | Tỷ lệ sản phẩm lỗi |
| Cost variance | Chênh lệch giá thực tế so với báo giá |
| **Sample approval cycles** | **Số vòng làm mẫu trung bình** |

Chỉ số cuối đặc biệt quan trọng với thời trang: nhà cung cấp cần 5 vòng làm mẫu sẽ làm chậm toàn bộ lịch ra mắt bộ sưu tập.

---

## 6. Rủi ro cần theo dõi

| Rủi ro | Hệ quả | Hệ thống hỗ trợ |
|---|---|---|
| Giao trễ | Lỡ mùa bán, hàng mất giá | Theo dõi mốc, cảnh báo sớm |
| Chất lượng không đạt | Làm lại, trễ thêm | Lịch sử lỗi theo nhà cung cấp |
| Phụ thuộc một nhà cung cấp | Rủi ro tập trung | Báo cáo tỷ trọng |
| Giá nguyên liệu biến động | Biên lợi nhuận giảm | Theo dõi chênh lệch giá |
| MOQ cao hơn nhu cầu | Tồn kho dư | Đưa MOQ vào bước lập kế hoạch |

---

## 7. Dữ liệu sở hữu

```sql
supplier
supplier_capability
supplier_certification
supplier_performance
supplier_contact
supplier_quotation
purchase_order
purchase_order_line
```

---

## 8. Interface công khai

```go
type PublicAPI interface {
    GetSupplier(ctx, supplierID string) (*SupplierView, error)
    GetActiveSuppliers(ctx, filter SupplierFilter) ([]SupplierView, error)
    GetSupplierCapability(ctx, supplierID string) (*CapabilityView, error)
    GetSupplierPerformance(ctx, supplierID string) (*PerformanceView, error)

    // Cho lập kế hoạch — cần biết MOQ và lead time
    GetSourcingOptions(ctx, skuID string) ([]SourcingOption, error)

    CreatePurchaseOrder(ctx, req CreatePORequest) (*PurchaseOrder, error)
    GetPurchaseOrder(ctx, poID string) (*PurchaseOrderView, error)
    UpdatePOStatus(ctx, poID string, status string) error
}
```

`GetSourcingOptions` là interface quan trọng — cung cấp MOQ, lead time, giá cho `supply-chain` khi lập kế hoạch. Đây là cách hiển thị mâu thuẫn MOQ/dự báo.

---

## 9. Event

**Phát ra:** `supplier.approved`, `supplier.suspended`, `purchase_order.created`, `purchase_order.confirmed`, `purchase_order.delayed`, `supplier_certification.expiring`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `production_plan.created` | supply-chain | Tạo đơn mua nguyên liệu |
| `quality.rejected` | quality | Cập nhật chỉ số chất lượng nhà cung cấp |
| `warehouse.goods_received` | warehouse | Chốt đơn mua |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Chỉ đặt hàng nhà cung cấp đã phê duyệt |
| 2 | Số lượng đặt ≥ MOQ |
| 3 | Cảnh báo khi chứng nhận sắp hết hạn |
| 4 | Theo dõi tỷ trọng để phát hiện rủi ro tập trung |
| 5 | Ghi nhận mọi chênh lệch giá so với báo giá |

---

## 11. Tài liệu liên quan

- [../01-business/supplier.md](../01-business/supplier.md)
- [manufacturing.md](manufacturing.md), [supply-chain.md](supply-chain.md)
