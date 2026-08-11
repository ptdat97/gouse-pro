# Phase 3 — Chuỗi cung ứng

## 1. Mục tiêu

> Kích hoạt **lợi thế cạnh tranh dài hạn**: biến dữ liệu nhu cầu tích lũy từ MVP và Phase 2 thành năng lực sản xuất đúng thứ, đúng lượng, đúng lúc.

Đây là giai đoạn nền tảng trở thành thứ không sao chép được.

---

## 2. Module thêm mới (5 module)

| Module | Vai trò |
|---|---|
| `supply-chain` | Tín hiệu nhu cầu, dự báo, lập kế hoạch, phát triển sản phẩm |
| `procurement` | Nhà cung cấp, đơn mua hàng |
| `manufacturing` | Đơn sản xuất, lô sản xuất, giá vốn |
| `quality` | Kiểm định chất lượng |
| `loyalty` | Điểm thưởng, hạng thành viên |

---

## 3. Điều kiện tiên quyết

Phase 3 chỉ khả thi nếu Phase 1–2 đã làm đúng:

```text
✓ demand_signal đã tích lũy tối thiểu 12 tháng
  → cần dữ liệu cùng kỳ năm trước để so sánh mùa vụ

✓ Dữ liệu bán hàng theo SIZE đầy đủ
  → phân bổ size là quyết định tốn tiền nhất

✓ Lý do hoàn hàng chuẩn hóa
  → biết size nào bị trả vì chật/rộng để sửa thiết kế

✓ Inventory có trạng thái đầy đủ và chính xác
  → nếu tồn kho sai, mọi tính toán bổ sung đều sai
```

**Nếu chưa đủ dữ liệu:** làm phần thực thi (procurement, manufacturing, quality) trước, phần thông minh (dự báo, lập kế hoạch) sau.

---

## 4. Phạm vi chi tiết

### Tín hiệu nhu cầu và dự báo

```text
✓ Tổng hợp demand_signal theo SKU / theo tuần
✓ Chuẩn hóa (loại ảnh hưởng khuyến mãi, mùa vụ)
✓ Dự báo bằng QUY TẮC và trung bình có trọng số
✓ Đo độ chính xác dự báo

KHÔNG dùng học máy ở giai đoạn này (nguyên tắc P14)
→ interface DemandSignalProvider thiết kế sẵn để thay sau
```

### Phát triển sản phẩm own brand

```text
✓ ProductDevelopment: concept → design → tech pack → costing
                      → sampling → duyệt mẫu
✓ Tech pack với thông số, số đo theo size, định mức nguyên liệu
✓ Quản lý vòng làm mẫu (theo dõi sample_approval_cycles)
✓ Kiểm soát giá vốn mục tiêu ở bước costing
✓ Anti-Corruption Layer → tạo Product trong Catalog khi duyệt mẫu
```

### Lập kế hoạch sản xuất

```text
✓ Kế hoạch ở mức SKU (bao gồm PHÂN BỔ SIZE)
✓ Đưa MOQ và lead time vào quyết định
✓ HIỂN THỊ MÂU THUẪN MOQ vs dự báo, kèm ước tính tài chính
✓ Ràng buộc mùa vụ (không sản xuất khi hàng về sẽ trễ mùa)
```

### Thu mua và sản xuất

```text
✓ Hồ sơ nhà cung cấp, năng lực, chứng nhận (theo dõi hạn)
✓ Purchase Order (mua hàng có sẵn)
✓ Production Order (đặt sản xuất theo tech pack)
✓ ProductionBatch với GIÁ VỐN THEO LÔ
✓ Truy vết nguyên liệu (cho kịch bản thu hồi)
✓ Theo dõi tiến độ theo mốc, cảnh báo khi đe dọa ngày ra mắt
✓ Chấm điểm hiệu suất nhà cung cấp
```

### Kiểm định chất lượng

```text
✓ Năm điểm kiểm: duyệt mẫu, inline, final, nhập kho, hàng hoàn
✓ Kiểm mẫu theo AQL
✓ Phân loại lỗi chuẩn hóa (CRITICAL/MAJOR/MINOR)
✓ Ảnh chứng minh BẮT BUỘC với mọi lỗi
✓ Xử lý lô không đạt: làm lại / giảm giá / trả hàng
```

### Bổ sung hàng

```text
✓ Tính điểm đặt hàng lại (reorder point)
✓ Đề xuất bổ sung theo SIZE
✓ Hiển thị tín hiệu nhu cầu BỊ BỎ LỠ (stockout, tìm không ra kết quả)
✓ Ràng buộc mùa vụ
✓ HỆ THỐNG ĐỀ XUẤT, CON NGƯỜI QUYẾT ĐỊNH
```

---

## 5. Nguyên tắc thiết kế quan trọng nhất

> **Hệ thống đề xuất, con người quyết định.**

```text
KHÔNG tự động đặt hàng.

Lý do: một lỗi tính toán có thể dẫn tới đơn sản xuất sai
       hàng trăm triệu đồng.

Rủi ro quá lớn so với lợi ích tiết kiệm vài phút thao tác.
```

Hệ thống hiển thị:

```text
✓ Tín hiệu nhu cầu đầy đủ (kể cả nhu cầu bị bỏ lỡ)
✓ Mâu thuẫn giữa ràng buộc (MOQ, lead time, mùa vụ) và dự báo
✓ Ước tính tài chính của TỪNG phương án
```

Đây là **phần mềm hỗ trợ ra quyết định**, không phải phần mềm ghi chép quyết định.

---

## 6. Bánh đà hoàn chỉnh

Cuối Phase 3, vòng lặp khép kín:

```text
Customer → Discovery → Content/Creator → Purchase
    → Behavior Data → Demand Signal → Product Planning
    → Own Brand/Supplier → Production → Inventory → Sales
    → More Data ↺
```

### Ví dụ vòng lặp thực tế

```text
1. Video creator về áo khoác dạ oversize: 50.000 lượt xem
2. 3.000 click, 850 thêm wishlist
3. Sản phẩm chỉ còn size S → inventory.depleted
4. 240 lượt tìm kiếm không ra kết quả
5. demand_signal tổng hợp: nhu cầu ~800, cung 200
6. Đề xuất: sản xuất 800, phân bổ nhiều M và L
7. Sản xuất, QC, nhập kho
8. Bán → dữ liệu mới → vòng lặp tiếp
```

Trước Phase 3, bước 5–7 không tồn tại — bánh đà bị đứt.

---

## 7. Nâng cấp module có sẵn

| Module | Bổ sung |
|---|---|
| `inventory` | Truy vết theo lô sản xuất, tích hợp sâu warehouse |
| `payment` | Thanh toán nhà cung cấp, ghi COGS theo lô, đa tiền tệ |
| `product` | Nhận sản phẩm từ Supply Chain qua ACL |
| `catalog` | Quản lý mùa vụ, cảnh báo tiến độ bộ sưu tập |
| `pricing` | Quy tắc giảm giá theo mùa, cảnh báo sell-through |
| `warehouse` | Nhiều kho, chuyển kho, tối ưu đường lấy hàng |
| `fulfillment` | Dịch vụ fulfillment cho seller |
| `return` | Đổi hàng (không chỉ trả), phân tích nguyên nhân |
| `seller` | Duyệt tự động một phần, phân hạng |

---

## 8. Hạ tầng bổ sung

```text
Lưu trữ chuỗi thời gian cho analytics
    → khi ghi analytics ảnh hưởng database chính

Cân nhắc tách service (nhóm 1):
    → media processing, search, notification, analytics
    → CHỈ KHI có lý do đo được, theo ADR-0009
```

---

## 9. Tiêu chí hoàn thành

### Chức năng

```text
✓ Tạo được sản phẩm own brand từ concept tới lên sàn qua hệ thống
✓ Đặt sản xuất, theo dõi tiến độ, kiểm định, nhập kho
✓ Giá vốn tính theo LÔ, không phải theo SKU
✓ Đề xuất bổ sung hiển thị mâu thuẫn và ước tính tài chính
✓ Truy vết được: đơn hàng → lô sản xuất → nguyên liệu
```

### Chất lượng

```text
✓ Forecast accuracy > 70%
✓ Quality pass rate > 95%
✓ On-time delivery (nhà cung cấp) > 90%
✓ Stockout rate ở SKU bán chạy < 5%
✓ Concept-to-shelf time < 120 ngày
```

### Kiến trúc

```text
✓ ACL giữa Supply Chain và Catalog hoạt động đúng
  → Catalog KHÔNG biết về tech pack, giá vốn, nhà cung cấp
✓ Mọi lô sản xuất có ProductionBatch với unit_cost
✓ Lô chưa qua QC không vào tồn kho bán được
```

---

## 10. Rủi ro chính

| Rủi ro | Giảm thiểu |
|---|---|
| Dữ liệu nhu cầu chưa đủ để dự báo | Làm phần thực thi trước, phần thông minh sau |
| Dự báo sai → sản xuất thừa/thiếu | Con người quyết định, hiển thị rõ độ không chắc chắn |
| ACL bị rò rỉ khái niệm sản xuất sang Catalog | Kiểm tra ranh giới trong CI |
| Giá vốn không gắn lô → tính biên sai | Bắt buộc `production_batch_id` khi nhập kho own brand |
| Quy trình phức tạp, người dùng không theo | Đào tạo, giao diện hỗ trợ ra quyết định rõ ràng |

---

## 11. Phase 4 — tóm tắt

Phase 4 **không thêm module mới**. Nâng cấp chiều sâu:

```text
recommendation  → cá nhân hóa nâng cao, có thể dùng ML
                  (tách service — lý do: chuyên biệt công nghệ)

supply-chain    → dự báo nâng cao, tối ưu phân bổ size

marketplace     → retail media (vị trí được tài trợ)

content         → live commerce

campaign        → creator marketplace (kết nối brand ↔ creator)

analytics       → kho dữ liệu, phân tích chuyên sâu
```

Xem [scale.md](scale.md).

---

## 12. Tài liệu liên quan

- [phase-2.md](phase-2.md), [scale.md](scale.md)
- [../01-business/supply-chain.md](../01-business/supply-chain.md)
- [../07-workflows/replenishment.md](../07-workflows/replenishment.md)
- [../07-workflows/own-brand-product.md](../07-workflows/own-brand-product.md)
