# Module: Return

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | Supporting (**nhưng quan trọng đặc biệt với thời trang**) |
| **Giai đoạn** | Phase 2 |

---

## 1. Vì sao module này quan trọng hơn vẻ ngoài

Tỷ lệ hoàn hàng ngành thời trang cao hơn hẳn các ngành khác — khách không thử được trước khi mua.

**Hệ quả:** hoàn hàng **không phải trường hợp ngoại lệ hiếm gặp**, mà là **luồng chính** cần thiết kế kỹ ngay từ đầu.

Ngoài ra, dữ liệu lý do hoàn hàng là đầu vào quan trọng cho:
- Sửa bảng size
- Sửa mô tả sản phẩm
- Đánh giá chất lượng nhà cung cấp
- Điều chỉnh thiết kế lô sau

---

## 2. Trách nhiệm

- Tiếp nhận và xử lý yêu cầu trả hàng
- Phân loại lý do hoàn — **chuẩn hóa, không phải văn bản tự do**
- Điều phối kiểm định hàng hoàn
- Quyết định hoàn tiền
- Xác định bên chịu chi phí hoàn hàng

## 3. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Thực hiện hoàn tiền | `payment` |
| Nhập lại hàng vào kho | `inventory`, `warehouse` |
| Kiểm định chất lượng chi tiết | `quality` |
| Vận chuyển hàng về | `fulfillment` |

---

## 4. Lý do hoàn hàng — chuẩn hóa bắt buộc

```text
SIZE_TOO_SMALL          — chật
SIZE_TOO_LARGE          — rộng
NOT_AS_DESCRIBED        — khác mô tả
COLOR_DIFFERENT         — màu khác ảnh
QUALITY_ISSUE           — chất lượng kém
DEFECTIVE               — lỗi sản xuất
WRONG_ITEM_SENT         — giao sai hàng
DAMAGED_IN_TRANSIT      — hỏng khi vận chuyển
CHANGED_MIND            — đổi ý
LATE_DELIVERY           — giao quá chậm
```

### Vì sao phải chuẩn hóa

Lý do hoàn **quyết định dòng tiền**, không chỉ là thống kê:

| Lý do | Bên chịu chi phí | Hành động khắc phục |
|---|---|---|
| `DEFECTIVE`, `WRONG_ITEM_SENT` | Seller / nền tảng | Truy vết lô, làm việc với nhà cung cấp |
| `NOT_AS_DESCRIBED`, `COLOR_DIFFERENT` | Seller | Sửa nội dung, cảnh cáo seller |
| `SIZE_TOO_SMALL/LARGE` | Theo chính sách | **Sửa bảng size** |
| `DAMAGED_IN_TRANSIT` | Đơn vị vận chuyển / nền tảng | Khiếu nại đối tác |
| `CHANGED_MIND` | Khách (thường) | Không hành động |

Nếu lý do là văn bản tự do, không phân tích được và không ra được quyết định nào.

---

## 5. Vòng đời yêu cầu trả hàng

```text
    REQUESTED (khách gửi yêu cầu)
        │
        ├── Kiểm tra: còn trong thời hạn đổi trả?
        ├── Kiểm tra: sản phẩm có thuộc diện được trả?
        │
        ├──→ REJECTED (có lý do)
        │
        ▼
    APPROVED
        │  → gửi hướng dẫn gửi hàng / đặt lịch lấy hàng
        ▼
    IN_TRANSIT (khách đã gửi)
        │
        ▼
    RECEIVED (kho đã nhận)
        │
        ▼
    INSPECTING (kiểm định)
        │
        ├──→ INSPECTION_FAILED (hàng không đúng, đã sử dụng nhiều)
        │        → thương lượng hoặc trả lại khách
        │
        ▼
    INSPECTED
        │  → quyết định: Available / Damaged
        ▼
    REFUNDED
        │
        ▼
    COMPLETED
```

---

## 6. Quy tắc nhập lại kho — bắt buộc

```text
Hàng hoàn về KHÔNG BAO GIỜ tự động cộng vào Available.

Bắt buộc qua kiểm định:
    Còn nguyên tem mác, không lỗi     → Available
    Vết bẩn nhẹ, xử lý được           → Available (sau xử lý)
    Đã sử dụng, không bán giá gốc     → Available (kênh giảm giá)
    Hỏng, bẩn, thiếu phụ kiện         → Damaged
```

Vi phạm quy tắc này dẫn tới bán lại hàng hỏng cho khách khác — thiệt hại uy tín lớn hơn nhiều giá trị món hàng.

---

## 7. Chuỗi đảo ngược tài chính

Hoàn hàng phải đảo ngược **toàn bộ** chuỗi, không chỉ hoàn tiền khách:

```text
Hoàn hàng một dòng hàng
    │
    ├──→ payment: hoàn tiền khách (theo giá thực trả, đã trừ giảm giá phân bổ)
    ├──→ payment: đảo ngược hoa hồng nền tảng
    ├──→ payment: đảo ngược số dư seller
    ├──→ affiliate: đảo ngược hoa hồng creator
    ├──→ promotion: giải phóng lượt dùng mã (nếu hủy toàn bộ)
    ├──→ inventory: nhập lại sau kiểm định
    ├──→ loyalty: thu hồi điểm đã tích
    └──→ customer: ghi nhận lịch sử size (nếu lý do liên quan size)
```

**Điểm dễ sai:** hoàn tiền theo **giá thực trả sau khi phân bổ giảm giá**, không phải giá niêm yết. Xem [promotion.md](promotion.md) mục 8.

---

## 8. Dữ liệu sở hữu

```sql
return_request
return_line
return_inspection
return_policy           -- chính sách theo seller/danh mục
```

```sql
CREATE TABLE return_line (
    id                 UUID PRIMARY KEY,
    return_request_id  UUID NOT NULL,
    order_line_id      UUID NOT NULL,
    sku_id             UUID NOT NULL,
    quantity           INT NOT NULL CHECK (quantity > 0),
    reason_code        TEXT NOT NULL,      -- CHUẨN HÓA
    reason_detail      TEXT,               -- mô tả thêm, tùy chọn
    refund_amount      BIGINT NOT NULL,    -- theo giá THỰC TRẢ
    inspection_result  TEXT,
    restock_decision   TEXT                -- AVAILABLE | DAMAGED | DISPOSE
);
```

---

## 9. Interface công khai

```go
type PublicAPI interface {
    RequestReturn(ctx, req ReturnRequest) (*ReturnView, error)
    GetReturn(ctx, returnID string) (*ReturnView, error)
    GetReturnsByOrder(ctx, orderID string) ([]ReturnView, error)
    GetReturnsBySeller(ctx, sellerID string, page Pagination) (*ReturnList, error)

    ApproveReturn(ctx, returnID string) error
    RejectReturn(ctx, returnID string, reason string) error
    RecordReceived(ctx, returnID string) error
    RecordInspection(ctx, req InspectionRequest) error

    IsReturnable(ctx, orderLineID string) (bool, string, error)
}
```

---

## 10. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `return.requested` | seller, notification, fulfillment |
| `return.approved` | notification, warehouse |
| `return.received` | quality |
| `return.inspected` | **inventory**, customer, seller |
| `return.refunded` | **payment**, **affiliate**, order, loyalty |
| `return.rejected` | notification |

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `fulfillment_order.delivered` | fulfillment | Bắt đầu đếm thời hạn đổi trả |

---

## 11. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Lý do hoàn phải chuẩn hóa |
| 2 | Chỉ trả trong thời hạn quy định |
| 3 | Hàng hoàn phải qua kiểm định trước khi nhập lại |
| 4 | Hoàn tiền theo giá thực trả (sau phân bổ giảm giá) |
| 5 | Phải đảo ngược đủ chuỗi tài chính |
| 6 | Lý do liên quan size phải quay về hồ sơ khách |
| 7 | Số lượng trả ≤ số lượng đã mua |
| 8 | Bên chịu chi phí xác định theo lý do |

---

## 12. Giai đoạn triển khai

### Trạng thái ở MVP — đọc kỹ để tránh hiểu nhầm

```text
Domain model:   CÓ    — mô hình đã thiết kế xong, giữ nguyên trong tài liệu
Database:       CÓ    — bảng có thể tạo từ MVP nếu cần lưu vết
API:            KHÔNG — Phase 2
UI:             KHÔNG — Phase 2
Workflow:       THỦ CÔNG ở MVP — xử lý ngoài hệ thống, ghi lại bằng tay
Automation:     KHÔNG — Phase 3
```

**Vì sao giữ domain model dù chưa cài:** hoàn hàng là **luồng chính** của
thương mại thời trang, không phải ngoại lệ hiếm gặp. Mô hình dữ liệu của
`order` và `payment` đã được thiết kế để chịu được hoàn từng phần — nếu bỏ
domain model của return, hai module kia sẽ được thiết kế thiếu và phải sửa
lại khi Phase 2 tới.

**Cụ thể những gì MVP đã chuẩn bị sẵn cho return:**

| Đã có ở MVP | Ở module | Vì sao cần cho return |
|---|---|---|
| `Adjustment` gắn từng dòng hàng | `order` | Hoàn từng phần đọc thẳng số tiền, không tính lại tỷ lệ |
| `order_line.status = RETURNED` | `order` | Đánh dấu dòng đã trả mà không xóa dữ liệu |
| Bút toán đảo ngược | `payment` | Ghi sổ hoàn tiền không sửa bút toán cũ |
| Trạng thái `Returned` trong kho | `inventory` | Hàng hoàn không tự vào `Available` |
| Hoa hồng chỉ khả dụng sau hạn đổi trả | `payment` | Phần lớn hoàn xảy ra trước khi tiền được chi |

Nói cách khác: **return chưa có API, nhưng hệ thống đã chịu được nó.**

### Lộ trình

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Không có API/UI. Xử lý thủ công, dùng bút toán điều chỉnh của `payment` |
| **Phase 2** | Yêu cầu trả, duyệt, kiểm định, hoàn tiền, lý do chuẩn hóa |
| **Phase 3** | Đổi hàng (không chỉ trả), phân tích nguyên nhân, tự động duyệt |
| **Phase 4** | Dự đoán rủi ro hoàn hàng, gợi ý size chủ động |

---

## 13. Tài liệu liên quan

- [../01-business/kpi.md](../01-business/kpi.md) mục 3 — vì sao tỷ lệ hoàn là chỉ số then chốt
- [inventory.md](inventory.md) — nhập lại kho
- [quality.md](quality.md) — kiểm định
- [../07-workflows/return.md](../07-workflows/return.md)
