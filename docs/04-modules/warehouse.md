# Module: Warehouse

| | |
|---|---|
| **Bounded Context** | Supply Chain |
| **Phân loại** | Supporting |
| **Giai đoạn** | Phase 2 |

---

## 1. Trách nhiệm

- Quản lý kho vật lý và vị trí lưu trữ
- Quy trình nhập hàng (goods receipt)
- Quy trình lấy hàng (picking) và đóng gói (packing)
- Kiểm kê
- Chuyển hàng giữa các kho

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| **Số lượng tồn kho (nguồn sự thật)** | `inventory` |
| Quyết định kho nào xuất hàng | `fulfillment` |
| Kiểm định chất lượng | `quality` |
| Vận chuyển tới khách | `fulfillment` |

---

## 3. Ranh giới với Inventory — dễ nhầm

```text
inventory  — SỐ LƯỢNG: "SKU X còn 12 cái khả dụng ở kho HN"
             → nguồn sự thật về số lượng và trạng thái

warehouse  — VỊ TRÍ VÀ THAO TÁC: "12 cái đó nằm ở kệ A-03-12"
             → quy trình vật lý: nhập, lấy, đóng gói, kiểm kê
```

**Quan hệ:** `warehouse` thực hiện thao tác vật lý rồi **báo cho** `inventory` cập nhật số lượng qua event. Không tự sửa số lượng.

---

## 4. Quy trình nhập hàng

```text
Hàng về kho
    ↓
Kiểm tra chứng từ, đối chiếu đơn mua/đơn sản xuất
    ↓
Đếm số lượng thực nhận
    ↓
    ├── Khớp        → tiếp tục
    └── Không khớp  → ghi nhận chênh lệch, báo procurement
    ↓
Yêu cầu kiểm định chất lượng (quality)
    ↓
    ├── Đạt      → xếp vào vị trí, phát warehouse.goods_received
    └── Không đạt → khu vực chờ xử lý, KHÔNG nhập kho bán
    ↓
inventory tăng số lượng Available
```

**Quy tắc:** hàng chưa qua kiểm định **không được** cộng vào tồn kho khả dụng.

---

## 5. Đặc thù kho hàng thời trang

| Yêu cầu | Lý do |
|---|---|
| Treo thay vì gấp với hàng cao cấp | Tránh nếp gấp làm giảm giá trị cảm nhận |
| Kiểm soát độ ẩm | Vải bị ẩm mốc |
| Sắp xếp theo bộ sưu tập/mùa | Dễ xả hàng cuối mùa |
| Khu vực riêng cho hàng hoàn | Chờ kiểm định, không lẫn hàng mới |
| Khu vực hàng ký gửi của seller | Hàng nền tảng giữ hộ nhưng không sở hữu |

Yêu cầu cuối liên quan trực tiếp tới mô hình `PLATFORM_SERVICE` — hàng ở kho nền tảng nhưng thuộc sở hữu seller. Xem [fulfillment.md](fulfillment.md) mục 4.

---

## 6. Lấy hàng và đóng gói

```text
FulfillmentOrder được phân bổ
    ↓
Tạo PickList (gộp nhiều đơn để tối ưu đường đi)
    ↓
Nhân viên lấy hàng theo vị trí
    ↓
Quét mã xác nhận đúng SKU  ← chống lấy nhầm
    ↓
Đóng gói theo quy cách
    ↓
In nhãn vận chuyển
    ↓
Bàn giao đơn vị vận chuyển
```

**Bước quét mã bắt buộc:** giao sai hàng là một trong những lý do hoàn hàng tốn kém nhất — vừa mất chi phí hai chiều, vừa mất niềm tin khách.

### Đóng gói — điểm chạm thương hiệu

```text
Hàng own brand cao cấp:
    - Túi/hộp có thương hiệu
    - Giấy gói
    - Thiệp cảm ơn
    - Phiếu hướng dẫn đổi trả  ← giảm ma sát khi cần đổi size
```

Trải nghiệm mở hộp là khác biệt thật sự của own brand so với marketplace.

---

## 7. Kiểm kê

```text
Kiểm kê định kỳ hoặc theo chu kỳ (cycle counting)
    ↓
Đếm thực tế, so với hệ thống
    ↓
Chênh lệch → điều tra nguyên nhân
    ↓
Điều chỉnh qua inventory.Adjust() — CÓ LÝ DO, CÓ NGƯỜI THỰC HIỆN
```

**Ngưỡng cảnh báo:** độ lệch kiểm kê > 1% cần điều tra. Nguyên nhân thường gặp: mất cắp, lấy nhầm không ghi nhận, lỗi nhập liệu.

---

## 8. Dữ liệu sở hữu

```sql
warehouse
warehouse_zone
storage_location        -- vị trí kệ cụ thể
goods_receipt
goods_receipt_line
pick_list
pick_list_item
packing_record
stock_count             -- kiểm kê
stock_count_line
```

---

## 9. Interface công khai

```go
type PublicAPI interface {
    GetWarehouse(ctx, warehouseID string) (*WarehouseView, error)
    GetStorageLocation(ctx, skuID, warehouseID string) ([]LocationView, error)

    CreateGoodsReceipt(ctx, req ReceiptRequest) (*GoodsReceipt, error)
    ConfirmGoodsReceipt(ctx, receiptID string) error

    CreatePickList(ctx, req PickListRequest) (*PickList, error)
    ConfirmPick(ctx, req ConfirmPickRequest) error
    RecordPacking(ctx, req PackingRequest) error

    CreateStockCount(ctx, req StockCountRequest) (*StockCount, error)
    SubmitStockCount(ctx, req SubmitCountRequest) error
}
```

---

## 10. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `warehouse.goods_received` | **inventory**, manufacturing, procurement, payment |
| `warehouse.pick_completed` | fulfillment |
| `warehouse.packing_completed` | fulfillment |
| `warehouse.stock_count_completed` | inventory, analytics |
| `warehouse.discrepancy_detected` | notification (cảnh báo) |

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `fulfillment_order.allocated` | fulfillment | Tạo pick list |
| `quality.approved` | quality | Cho phép xếp vào khu bán được |
| `return.approved` | return | Chuẩn bị nhận hàng hoàn |

---

## 11. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | `warehouse` không tự sửa số lượng — báo cho `inventory` |
| 2 | Hàng chưa qua QC không vào khu bán được |
| 3 | Bắt buộc quét mã khi lấy hàng |
| 4 | Hàng ký gửi seller tách khu vực và tách quyền sở hữu |
| 5 | Mọi điều chỉnh kiểm kê phải có lý do và người thực hiện |
| 6 | Hàng hoàn có khu vực riêng, chờ kiểm định |

---

## 12. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **Phase 2** | Một kho, nhập/lấy/đóng gói cơ bản, kiểm kê |
| **Phase 3** | Nhiều kho, chuyển kho, hàng ký gửi seller, tối ưu đường lấy hàng |
| **Phase 4** | Tự động hóa, tích hợp thiết bị kho |

---

## 13. Tài liệu liên quan

- [inventory.md](inventory.md) — ranh giới số lượng vs vị trí
- [fulfillment.md](fulfillment.md), [quality.md](quality.md)
