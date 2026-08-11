# Seller Center

Ứng dụng cho nhà bán quản lý gian hàng.

---

## 1. Sơ đồ trang

```text
Dashboard          — tổng quan
Products           — sản phẩm và offer
Inventory          — tồn kho
Orders             — đơn cần xử lý (FulfillmentOrder)
Returns            — yêu cầu đổi trả
Marketing          — khuyến mãi của gian hàng
Affiliate          — chiến dịch creator
Finance            — số dư, đối soát, chi trả
Analytics          — báo cáo
Settings           — hồ sơ, chính sách, tài khoản ngân hàng
```

---

## 2. Ràng buộc bảo mật quan trọng nhất

> **Seller không bao giờ thấy dữ liệu của seller khác.**

Bao gồm cả việc **suy ngược từ báo cáo tổng hợp**.

```text
❌ KHÔNG hiển thị:
    "Thị phần của bạn trong danh mục: 40%"
    → nếu danh mục chỉ có 2 seller, seller kia suy ra doanh số của bạn

✅ Có thể hiển thị:
    "Thứ hạng của bạn: top 10% trong danh mục"
    → không suy ngược ra con số cụ thể của ai
```

Vi phạm ràng buộc này làm mất niềm tin toàn bộ nhà bán — thiệt hại lớn hơn nhiều so với lợi ích của tính năng.

---

## 3. Dashboard

```text
┌──────────────────────────────────────────────┐
│ CẦN XỬ LÝ NGAY                               │
│ ▸ 8 đơn chờ xác nhận (2 sắp quá hạn SLA)     │
│ ▸ 3 yêu cầu đổi trả                          │
│ ▸ 5 sản phẩm sắp hết hàng                    │
├──────────────────────────────────────────────┤
│ HÔM NAY                                      │
│ Đơn: 24 · Doanh số: 8.450.000đ               │
├──────────────────────────────────────────────┤
│ SỐ DƯ                                        │
│ Khả dụng: 8.320.000đ                         │
│ Chờ (trong hạn đổi trả): 2.450.000đ          │
│ Kỳ đối soát tiếp theo: 13/08                 │
├──────────────────────────────────────────────┤
│ HIỆU SUẤT (30 ngày)                          │
│ Tỷ lệ hủy đơn      2,1%  ✓ (ngưỡng 3%)      │
│ Giao đúng hạn     94,0%  ⚠ (ngưỡng 95%)     │
│ Đánh giá TB         4,6  ✓                   │
│ Tỷ lệ thắng buy box  62%                     │
└──────────────────────────────────────────────┘
```

**Nguyên tắc:** ưu tiên "cần xử lý ngay" ở trên cùng. Seller vào đây để **làm việc**, không phải để ngắm số liệu.

---

## 4. Trang đơn hàng — điểm khác biệt quan trọng

Seller thấy **FulfillmentOrder**, không phải **Order**.

```text
┌──────────────────────────────────────────────┐
│ Đơn FC-2026-08-001234-B                      │
│ Nhận lúc 14:25 ngày 10/08                    │
│ ⏱ Cần xác nhận trước 14:25 ngày 11/08        │
│                                              │
│ SẢN PHẨM                                     │
│ Giày loafer da · Nâu/39 · x1                 │
│ SKU: LF-DA-BRN-39         650.000đ           │
│                                              │
│ GIAO TỚI                                     │
│ Nguyễn Văn A · 0901234567                    │
│ 123 Đường ABC, P.X, Q.Y, TP.HCM              │
│                                              │
│ ƯỚC TÍNH BẠN NHẬN                            │
│ Giá bán            650.000đ                  │
│ Hoa hồng (8%)      −52.000đ                  │
│ Phí thanh toán      −9.750đ                  │
│ ─────────────────────────────                │
│ Dự kiến nhận       588.250đ                  │
│                                              │
│ [ Xác nhận ]  [ Báo hết hàng ]               │
└──────────────────────────────────────────────┘
```

### Điều seller KHÔNG thấy

```text
✗ Mã đơn tổng (FC-2026-08-001234)
✗ Tổng tiền cả đơn (1.280.000đ)
✗ Hai lô hàng còn lại
✗ Tên các seller khác
✗ Email khách, lịch sử mua hàng
```

**Về "ước tính bạn nhận":** hiển thị ngay từ lúc nhận đơn tạo minh bạch, giảm tranh chấp về sau. Seller biết chính xác mình được bao nhiêu trước khi xử lý.

---

## 5. Trang tài chính

```text
┌──────────────────────────────────────────────┐
│ SỐ DƯ                                        │
│ Chờ (trong hạn đổi trả)     2.450.000đ       │
│ Khả dụng                    8.320.000đ       │
│ Đang chuyển                         0đ       │
│ Bị giữ                              0đ       │
│ Giữ bảo đảm (10%, 30 ngày)   500.000đ        │
├──────────────────────────────────────────────┤
│ ĐỐI SOÁT 01/08 – 07/08        Đã xác nhận    │
│ Doanh số                   12.500.000đ       │
│ Hoa hồng nền tảng          −1.250.000đ       │
│ Phí thanh toán               −187.500đ       │
│ Phí fulfillment              −300.000đ       │
│ Hoa hồng creator             −180.000đ       │
│ Hoàn tiền                    −890.000đ       │
│ ─────────────────────────────────────────    │
│ Thực nhận                   9.692.500đ       │
│                                              │
│ [ Xem chi tiết từng dòng ]                   │
└──────────────────────────────────────────────┘
```

**Yêu cầu minh bạch tuyệt đối:** seller phải xem được **từng dòng** cấu thành số tiền. Đối soát không minh bạch là nguyên nhân tranh chấp lớn nhất giữa nền tảng và nhà bán.

### Giải thích năm trạng thái số dư

Hiển thị đủ năm trạng thái giúp seller hiểu **vì sao chưa nhận được tiền** — giảm khiếu nại đáng kể.

---

## 6. Trang hiệu suất

```text
┌──────────────────────────────────────────────┐
│ HIỆU SUẤT 30 NGÀY                            │
│                                              │
│ Tỷ lệ hủy đơn        2,1%   ✓  (< 3%)        │
│ Xác nhận đơn TB      6 giờ  ✓  (< 24 giờ)    │
│ Giao đúng hạn       94,0%   ⚠  (≥ 95%)       │
│ Hoàn do mô tả sai    1,8%   ✓  (< 5%)        │
│ Đánh giá TB           4,6   ✓  (≥ 4,0)       │
│ Độ chính xác tồn kho 97,0%  ✓  (≥ 95%)       │
│                                              │
│ ẢNH HƯỞNG                                    │
│ Tỷ lệ thắng buy box: 62%                     │
│ 💡 Cải thiện "giao đúng hạn" lên 95%         │
│    sẽ tăng cơ hội thắng buy box              │
└──────────────────────────────────────────────┘
```

**Nguyên tắc P14 áp dụng:** chỉ số, ngưỡng, và **tác động** đều công khai, tường minh. Seller hiểu được mình đang ở đâu và cần làm gì.

Mô hình chấm điểm hộp đen tạo tranh chấp không giải quyết được và cảm giác bất công.

---

## 7. Quản lý sản phẩm và offer

```text
Hai luồng:
    A. Tạo offer trên sản phẩm CÓ SẴN  ← ưu tiên
    B. Tạo sản phẩm mới                ← chỉ khi không tìm thấy

Khi tạo sản phẩm mới:
    → hệ thống ĐỐI SÁNH trùng lặp
    → gợi ý: "Có phải bạn muốn bán sản phẩm này?"
```

### Thông báo lỗi hữu ích

```text
❌ "Không thể tạo offer"

✅ "Thương hiệu này yêu cầu giấy ủy quyền.
    [ Tải lên giấy ủy quyền ]"

✅ "Giá 50.000đ thấp hơn mức tối thiểu 150.000đ.
    Bạn có nhập thiếu số 0 không?"
```

Thông báo lỗi thứ ba bắt được lỗi nhập liệu phổ biến — bảo vệ chính seller.

---

## 8. Tài liệu liên quan

- [frontend-architecture.md](frontend-architecture.md)
- [../06-api/seller-api.md](../06-api/seller-api.md)
- [../04-modules/seller.md](../04-modules/seller.md)
