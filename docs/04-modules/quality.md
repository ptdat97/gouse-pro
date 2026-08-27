# Module: Quality

| | |
|---|---|
| **Bounded Context** | Supply Chain |
| **Phân loại** | Supporting |
| **Giai đoạn** | Phase 3 |

---

## 1. Trách nhiệm

- Quản lý tiêu chuẩn chất lượng
- Thực hiện và ghi nhận kiểm định
- Phân loại lỗi
- Quyết định chấp nhận/từ chối lô hàng
- Kiểm định hàng hoàn về

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Sản xuất | `manufacturing` |
| Nhập kho | `warehouse` |
| Cập nhật tồn kho | `inventory` |
| Xử lý yêu cầu trả hàng | `return` |

---

## 3. Năm điểm kiểm định

```text
1. Sample Approval    — duyệt mẫu trước khi sản xuất hàng loạt
2. Inline QC          — kiểm trong quá trình sản xuất (phát hiện lỗi sớm)
3. Final QC           — kiểm cuối chuyền tại nhà máy trước khi xuất
4. Receiving QC       — kiểm khi hàng về kho nền tảng
5. Return QC          — kiểm hàng hoàn, quyết định bán lại được không
```

Điểm 5 là điểm được dùng nhiều nhất trong vận hành thời trang, vì tỷ lệ hoàn hàng cao.

---

## 4. Kiểm mẫu theo AQL

Kiểm 100% sản phẩm không khả thi về chi phí. Thực tế dùng kiểm mẫu:

```text
Lô 1.000 chiếc
Cỡ mẫu kiểm:      80 chiếc
Ngưỡng chấp nhận: tối đa 5 lỗi nhẹ, 0 lỗi nghiêm trọng

Kết quả:
    ≤ ngưỡng  → Chấp nhận lô
    > ngưỡng  → Từ chối → thương lượng: làm lại / giảm giá / trả hàng
```

### Phân loại lỗi — chuẩn hóa bắt buộc

```text
CRITICAL  — không an toàn, không dùng được
MAJOR     — ảnh hưởng chức năng hoặc thẩm mỹ rõ rệt
MINOR     — sai lệch nhỏ, khách khó nhận ra
```

Với thời trang, các lỗi thường gặp:

```text
Đường may lỗi · Sai màu so với mẫu · Sai số đo
Vải lỗi · Phụ kiện lỗi (khóa, cúc) · Nhãn mác sai
Vết bẩn · Phai màu
```

---

## 5. Dữ liệu ghi nhận

```text
QualityInspection {
    id
    inspection_type      SAMPLE | INLINE | FINAL | RECEIVING | RETURN
    reference_type       PRODUCTION_BATCH | RETURN_LINE | PURCHASE_ORDER
    reference_id
    sample_size
    total_quantity
    defects[]            -- theo loại
    result               PASSED | FAILED | CONDITIONAL_PASS
    inspector_id
    photos[]             ← BẮT BUỘC với lỗi
    notes
    inspected_at
}
```

**Ảnh chứng minh là bắt buộc** với mọi lỗi — đây là bằng chứng khi thương lượng với nhà cung cấp hoặc giải quyết tranh chấp.

---

## 6. Quyết định với hàng hoàn

```text
Hàng hoàn về → kiểm định

    Còn nguyên tem mác, không lỗi    → RESTOCK_AVAILABLE
    Vết bẩn nhẹ, xử lý được          → RESTOCK_AFTER_TREATMENT
    Đã sử dụng, không bán giá gốc    → RESTOCK_OUTLET
    Hỏng, bẩn, thiếu phụ kiện        → DAMAGED
    Không phải hàng của mình         → REJECT_RETURN
```

Trường hợp cuối cần xử lý riêng: khách gửi trả sai hàng (cố ý hoặc nhầm lẫn). Phải có quy trình và bằng chứng ảnh.

---

## 7. Dữ liệu sở hữu

```sql
quality_standard
quality_inspection
defect_record
defect_type             -- danh mục loại lỗi
inspection_photo
```

---

## 8. Interface công khai

```go
type PublicAPI interface {
    CreateInspection(ctx, req InspectionRequest) (*Inspection, error)
    RecordDefects(ctx, inspectionID string, defects []Defect) error
    CompleteInspection(ctx, req CompleteRequest) (*InspectionResult, error)

    GetInspection(ctx, inspectionID string) (*InspectionView, error)
    GetInspectionsByBatch(ctx, batchID string) ([]InspectionView, error)

    GetQualityStandard(ctx, productType string) (*StandardView, error)
    GetDefectStatsBySupplier(ctx, supplierID string, period DateRange) (*DefectStats, error)
}
```

---

## 9. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `quality.approved` | warehouse, manufacturing, inventory |
| `quality.rejected` | manufacturing, procurement, payment |
| `quality.return_inspected` | **return**, **inventory** |
| `quality.defect_pattern_detected` | notification (cảnh báo) |

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `production_batch.completed` | manufacturing | Tạo yêu cầu kiểm định |
| `return.received` | return | Tạo yêu cầu kiểm định hàng hoàn |
| `warehouse.goods_received` | warehouse | Tạo yêu cầu kiểm nhập |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Lô không đạt QC không được nhập kho bán |
| 2 | Mọi lỗi phải có ảnh chứng minh |
| 3 | Loại lỗi phải chuẩn hóa |
| 4 | Hàng hoàn bắt buộc kiểm trước khi nhập lại |
| 5 | Cỡ mẫu kiểm ≤ số lượng lô |
| 6 | Kết quả kiểm định gắn với lô sản xuất cụ thể |
| 7 | Thống kê lỗi theo nhà cung cấp phải truy vết được |

---

## 11. Tài liệu liên quan

- [manufacturing.md](manufacturing.md) — lô sản xuất
- [return.md](return.md) — hàng hoàn
- [../01-business/supply-chain.md](../01-business/supply-chain.md) mục 7
