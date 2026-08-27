# Nghiệp vụ: Chuỗi cung ứng (Supply Chain)

## 1. Vì sao chuỗi cung ứng là miền chiến lược

Đây là luận điểm trung tâm của toàn bộ kiến trúc này.

| Năng lực | Đối thủ sao chép được không? | Thời gian sao chép |
|---|---|---|
| Giao diện đẹp | Có | Vài tuần |
| Chính sách hoa hồng | Có | Vài ngày |
| Chương trình khuyến mãi | Có | Vài ngày |
| Mạng lưới creator | Khó | Vài tháng |
| **Năng lực chuyển nhu cầu thành hàng hóa** | **Rất khó** | **Nhiều năm** |

Chuỗi cung ứng không phải "module quản lý kho". Nó là năng lực trả lời câu hỏi: *thị trường đang muốn gì, và làm sao có được thứ đó trong tay khách hàng nhanh nhất, với đúng số lượng?*

**Hệ quả kiến trúc:** chuỗi cung ứng được đối xử như một **core domain**, không phải supporting domain. Nó xứng đáng có mô hình domain đầy đủ, không phải vài bảng CRUD. Xem [../02-domain/domain-map.md](../02-domain/domain-map.md).

---

## 2. Chuỗi giá trị đầy đủ

```text
   Demand Signal (Tín hiệu nhu cầu)
        │  ← từ hành vi khách, nội dung, marketplace, xu hướng
        ▼
   Forecast (Dự báo)
        │  ← dự đoán số lượng bán theo SKU, theo tuần
        ▼
   Product Planning (Kế hoạch sản phẩm)
        │  ← quyết định làm gì, bao nhiêu, khi nào, phân bổ size
        ▼
   Procurement (Thu mua)
        │  ← chọn nhà cung cấp, đàm phán giá, MOQ
        ▼
   Purchase Order / Production Order
        │
        ▼
   Supplier / Manufacturer
        │
        ▼
   Production Order (Đơn sản xuất)
        │
        ▼
   Production Batch (Lô sản xuất)
        │  ← đơn vị truy vết giá vốn và chất lượng
        ▼
   Quality Control (Kiểm định)
        │
        ├──→ Không đạt → làm lại / giảm giá / trả nhà cung cấp
        │
        ▼
   Warehouse (Nhập kho)
        │
        ▼
   Inventory (Tồn kho khả dụng)
        │
        ▼
   Fulfillment (Giao hàng)
        │
        ▼
   Customer (Khách hàng)
        │
        └──→ Dữ liệu bán hàng, dữ liệu hoàn hàng → quay lại Demand Signal
```

Vòng lặp ở cuối là bánh đà. Nếu nó đứt, chuỗi cung ứng chỉ còn là hệ thống ghi chép.

---

## 3. Hai mô hình cung ứng song song

Hệ thống phải hỗ trợ đồng thời hai luồng khác nhau về bản chất.

### 3.1 Nguồn cung marketplace

```text
Seller (tự lo hàng)
   ↓
Nền tảng (trung gian)
   ↓
Khách hàng
```

Đặc điểm:
- Nền tảng **không sở hữu** hàng, không kiểm soát tồn kho.
- Chỉ **quan sát** tồn kho do seller khai báo.
- Rủi ro: seller khai tồn kho sai → oversell → hủy đơn → mất uy tín.
- Vai trò của nền tảng: giám sát độ tin cậy tồn kho của seller.

**Chỉ số cần theo dõi:** tỷ lệ hủy đơn do hết hàng theo seller. Seller có tỷ lệ cao phải bị giảm thứ hạng buy box.

### 3.2 Nguồn cung own brand

```text
Nền tảng (quyết định sản xuất)
   ↓
Nhà cung cấp / Nhà máy
   ↓
Sản xuất
   ↓
QC
   ↓
Kho nền tảng
   ↓
Khách hàng
```

Đặc điểm:
- Nền tảng **sở hữu** hàng, chịu rủi ro tồn kho.
- Kiểm soát toàn bộ: chất lượng, giá vốn, thời điểm.
- Chu kỳ dài: 3–6 tháng từ quyết định đến bán.
- Rủi ro: sản xuất sai số lượng, sai size, trễ mùa.

**Hệ quả kiến trúc:** hai luồng này dùng chung khái niệm `Inventory` nhưng khác nhau ở `inventory_owner` và ở việc có hay không có liên kết tới `ProductionBatch`. Xem [../04-modules/inventory.md](../04-modules/inventory.md).

---

## 4. Tín hiệu nhu cầu (Demand Signal)

### 4.1 Nguồn tín hiệu

```text
Views              — lượt xem sản phẩm
Searches           — từ khóa tìm kiếm, đặc biệt là tìm không có kết quả
Clicks             — click từ nội dung, từ danh mục
Add to Cart        — thêm giỏ hàng (tín hiệu mạnh hơn view nhiều)
Wishlist           — lưu để mua sau (tín hiệu ý định rõ ràng)
Orders             — đơn hàng thực tế
Conversion         — tỷ lệ chuyển đổi
Returns            — hoàn hàng và lý do hoàn
Reviews            — đánh giá và nội dung đánh giá
Trend              — xu hướng từ bên ngoài, mùa vụ
Stockout events    — sự kiện hết hàng (nhu cầu bị bỏ lỡ)
```

### 4.2 Tín hiệu quan trọng nhất thường bị bỏ qua

**Tìm kiếm không có kết quả** và **sự kiện hết hàng** là hai tín hiệu giá trị nhất, vì chúng đo **nhu cầu không được đáp ứng** — thứ không xuất hiện trong dữ liệu bán hàng.

```text
Nếu chỉ nhìn dữ liệu bán hàng:
  "Áo khoác dạ bán được 200 chiếc"  →  kết luận: nhu cầu là 200

Thực tế:
  - Bán 200 chiếc, hết hàng từ tuần thứ 3
  - 1.500 lượt tìm kiếm sau khi hết hàng
  - 400 lượt đăng ký thông báo có hàng
  →  nhu cầu thật gần 800, không phải 200
```

Nếu lập kế hoạch sản xuất chỉ dựa vào doanh số lịch sử, hệ thống sẽ **liên tục sản xuất thiếu** những mặt hàng bán chạy. Đây là sai lầm kinh điển.

**Hệ quả kiến trúc:** phải ghi nhận và lưu giữ các sự kiện nhu cầu bị bỏ lỡ. Đây là dữ liệu mà module `inventory` và `catalog` phải phát ra dưới dạng event.

### 4.3 Từ tín hiệu thô đến chỉ báo

```text
Tín hiệu thô (event stream)
     │
     ▼
Tổng hợp theo SKU / theo tuần / theo vùng
     │
     ▼
Chuẩn hóa (loại bỏ ảnh hưởng khuyến mãi, mùa vụ)
     │
     ▼
Demand Indicator (chỉ báo nhu cầu)
     │
     ▼
Đầu vào cho Forecast và Planning
```

**Nguyên tắc P14:** cài đặt đầu tiên dùng **quy tắc và trung bình có trọng số**, không dùng học máy. Interface `DemandSignalProvider` được thiết kế để sau này thay bằng mô hình mà không sửa module planning.

---

## 5. Lập kế hoạch sản phẩm (Product Planning)

Đây là nơi ra quyết định tốn tiền nhất.

### Đầu vào

```text
Dự báo nhu cầu theo SKU
Tồn kho hiện tại
Hàng đang trên đường về
Lead time của nhà cung cấp
MOQ của nhà cung cấp
Ngân sách sản xuất
Lịch mùa vụ / lịch ra mắt bộ sưu tập
Biên lợi nhuận mục tiêu
```

### Đầu ra

```text
Kế hoạch sản xuất:
  - SKU nào
  - số lượng theo từng size
  - nhà cung cấp nào
  - thời điểm đặt
  - thời điểm cần hàng về
  - ngân sách phân bổ
```

### Mâu thuẫn cốt lõi cần được hệ thống hiển thị

```text
Dự báo bán:        300 chiếc
MOQ nhà cung cấp:  500 chiếc
Lead time:         10 tuần
Mùa còn lại:       14 tuần

Quyết định:
  A. Đặt 500  → dư 200, rủi ro tồn kho khoảng 40% giá trị lô
  B. Đặt 0    → mất doanh số 300 chiếc
  C. Tìm nhà cung cấp khác có MOQ thấp hơn, giá cao hơn
  D. Đặt 500 và chủ động lên kế hoạch xả 200 cuối mùa
```

**Hệ quả kiến trúc:** hệ thống phải **hiển thị mâu thuẫn này ở bước lập kế hoạch**, kèm ước tính tài chính cho từng phương án. Không phải để người dùng phát hiện sau khi hàng đã về kho.

Đây là ví dụ của việc phần mềm hỗ trợ ra quyết định, không chỉ ghi chép quyết định.

---

## 6. Bổ sung hàng (Replenishment)

Khác với kế hoạch sản phẩm mới, bổ sung là quyết định lặp lại cho sản phẩm đang bán tốt.

```text
Theo dõi liên tục:
  - Tốc độ bán (units/tuần)
  - Tồn kho hiện tại
  - Hàng đang về
  - Lead time
        │
        ▼
Tính điểm đặt hàng lại (reorder point):

  Reorder point = (Tốc độ bán × Lead time) + Safety stock

  Ví dụ: bán 50 chiếc/tuần, lead time 6 tuần, safety stock 100
         → Reorder point = 50 × 6 + 100 = 400
        │
        ▼
Khi tồn kho ≤ Reorder point → tạo đề xuất bổ sung
        │
        ▼
Người phụ trách xem xét và phê duyệt
        │
        ▼
Tạo Purchase Order / Production Order
```

**Nguyên tắc thiết kế:** hệ thống **đề xuất**, con người **quyết định**. Tự động đặt hàng hoàn toàn là rủi ro lớn ở giai đoạn đầu — một lỗi tính toán có thể dẫn tới đơn sản xuất hàng trăm triệu đồng sai.

Xem [../07-workflows/replenishment.md](../07-workflows/replenishment.md).

---

## 7. Kiểm định chất lượng (Quality Control)

QC không phải một bước, mà là nhiều điểm kiểm tra.

```text
1. Duyệt mẫu (Sample Approval)
   → trước khi sản xuất hàng loạt

2. QC trong quá trình sản xuất (Inline QC)
   → phát hiện lỗi sớm, tránh làm hỏng cả lô

3. QC cuối chuyền tại nhà máy (Final QC)
   → kiểm mẫu theo tiêu chuẩn AQL trước khi xuất

4. QC khi nhập kho (Receiving QC)
   → kiểm tra lại khi hàng về kho nền tảng

5. QC hàng hoàn (Return QC)
   → quyết định hàng hoàn có bán lại được không
```

### Tiêu chuẩn AQL

Kiểm 100% sản phẩm là không khả thi về chi phí. Thực tế dùng kiểm mẫu:

```text
Lô 1.000 chiếc
Cỡ mẫu kiểm: 80 chiếc
Ngưỡng chấp nhận: tối đa 5 lỗi nhẹ, 0 lỗi nặng

Kết quả:
  ≤ ngưỡng  → Chấp nhận lô
  > ngưỡng  → Từ chối lô → thương lượng: làm lại / giảm giá / trả hàng
```

**Hệ quả kiến trúc:** `QualityInspection` phải ghi nhận: cỡ mẫu, số lỗi theo loại, kết luận, người kiểm, ảnh chứng minh. Đây là dữ liệu dùng để đánh giá nhà cung cấp và giải quyết tranh chấp.

Phân loại lỗi cần chuẩn hóa:

```text
Lỗi nghiêm trọng (Critical) — không an toàn, không dùng được
Lỗi nặng (Major)            — ảnh hưởng chức năng hoặc thẩm mỹ rõ rệt
Lỗi nhẹ (Minor)             — sai lệch nhỏ, khách khó nhận ra
```

Xem [../04-modules/quality.md](../04-modules/quality.md).

---

## 8. Lô sản xuất — đơn vị truy vết

`ProductionBatch` là khái niệm then chốt, thường bị bỏ sót.

```text
ProductionBatch {
  id
  production_order_id
  sku_id
  quantity
  supplier_id
  production_date
  unit_cost           ← giá vốn của LÔ NÀY
  quality_result
  certificates        ← chứng nhận chất liệu, xuất xứ
}
```

### Vì sao cần

| Câu hỏi | Không có batch | Có batch |
|---|---|---|
| Giá vốn thật của đơn hàng này? | Ước lượng | Chính xác |
| Lô nào có tỷ lệ lỗi cao? | Không biết | Truy vết được |
| Cần thu hồi hàng lỗi, thu hồi cái nào? | Toàn bộ SKU | Chỉ lô liên quan |
| Nhà cung cấp nào chất lượng kém? | Không đo được | Đo theo lô |

**Kịch bản thu hồi:** nếu phát hiện một lô vải có vấn đề (phai màu, hóa chất vượt ngưỡng), phải xác định được chính xác những đơn hàng nào chứa sản phẩm từ lô đó. Không có `ProductionBatch`, phải thu hồi toàn bộ SKU — chi phí lớn hơn nhiều lần.

---

## 9. Lộ trình triển khai

Chuỗi cung ứng đầy đủ là Phase 3. Nhưng cần chuẩn bị từ sớm:

| Giai đoạn | Phạm vi |
|---|---|
| MVP | `Inventory` cơ bản, nhập kho thủ công. **Bắt đầu ghi nhận tín hiệu nhu cầu.** |
| Phase 2 | `Warehouse`, `Fulfillment`, `Return` với QC hàng hoàn |
| Phase 3 | `Supplier`, `Procurement`, `Manufacturing`, `Quality`, `Replenishment` |
| Phase 4 | `Demand Intelligence`, dự báo nâng cao, tối ưu phân bổ |

**Điểm quan trọng:** việc **ghi nhận tín hiệu nhu cầu phải bắt đầu từ MVP**, dù chưa dùng tới. Lý do: dữ liệu lịch sử không thể tạo ngược. Đến Phase 3 mới bắt đầu ghi thì phải chờ thêm nhiều tháng mới đủ dữ liệu lập kế hoạch.

Đây là một trong số ít trường hợp mà "làm sớm dù chưa cần" là quyết định đúng — vì dữ liệu có tính tích lũy theo thời gian.

---

## 10. Chỉ số chuỗi cung ứng

| Chỉ số | Ý nghĩa | Mục tiêu tham khảo |
|---|---|---|
| Forecast accuracy | Độ chính xác dự báo | > 70% |
| On-time delivery | Nhà cung cấp giao đúng hạn | > 90% |
| Quality pass rate | Lô đạt QC lần đầu | > 95% |
| Inventory turnover | Vòng quay tồn kho | > 4 lần/năm |
| Stockout rate | Tỷ lệ hết hàng ở SKU bán chạy | < 5% |
| Excess inventory | Tỷ lệ hàng tồn quá hạn mùa | < 15% |
| Lead time variance | Độ ổn định thời gian giao | < 20% |
| Cash-to-cash cycle | Chu kỳ tiền mặt | Càng ngắn càng tốt |

---

## 11. Tài liệu liên quan

- [supplier.md](supplier.md) — tác nhân nhà cung cấp
- [own-brand.md](own-brand.md) — vòng đời sản phẩm own brand
- [../04-modules/supply-chain.md](../04-modules/supply-chain.md) — đặc tả module
- [../07-workflows/replenishment.md](../07-workflows/replenishment.md) — luồng bổ sung hàng
