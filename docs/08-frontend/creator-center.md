# Creator Center

Ứng dụng cho creator quản lý nội dung và thu nhập.

---

## 1. Sơ đồ trang

```text
Profile        — hồ sơ, kênh mạng xã hội
Content        — quản lý nội dung
Products       — tìm sản phẩm để gắn vào nội dung
Campaigns      — chiến dịch đang mở và đã tham gia
Affiliate      — link affiliate
Earnings       — thu nhập và đối soát
Analytics      — hiệu suất nội dung
```

---

## 2. Ranh giới quyền riêng tư — ràng buộc tuyệt đối

> **Creator KHÔNG BAO GIỜ thấy danh tính khách hàng.**

```text
Creator ĐƯỢC thấy:
    ✓ Số click, số đơn, doanh thu quy kết (TỔNG HỢP)
    ✓ Hoa hồng của mình
    ✓ Hiệu suất theo nội dung, theo sản phẩm
    ✓ Tỷ lệ hoàn hàng theo nội dung

Creator KHÔNG thấy:
    ✗ Tên, email, số điện thoại khách
    ✗ Địa chỉ giao hàng
    ✗ Mã đơn hàng thật
    ✗ Lịch sử mua của cá nhân nào
```

**Lý do:** creator không phải bên xử lý dữ liệu cá nhân. Cung cấp dữ liệu khách cho creator vi phạm quy định bảo vệ dữ liệu ở nhiều thị trường.

**Hệ quả thiết kế:** nếu một số liệu có thể suy ngược ra cá nhân (ví dụ "1 đơn hàng lúc 14:23 hôm nay"), cần ngưỡng tối thiểu trước khi hiển thị.

---

## 3. Dashboard

```text
┌──────────────────────────────────────────────┐
│ THU NHẬP                                     │
│ Khả dụng                    3.480.000đ       │
│ Chờ (trong hạn đổi trả)     1.250.000đ       │
│ Kỳ chi trả tiếp theo: 05/09                  │
├──────────────────────────────────────────────┤
│ 30 NGÀY QUA                                  │
│ Lượt xem nội dung             125.000        │
│ Click sản phẩm                  8.400        │
│ Đơn hàng quy kết                  152        │
│ GMV quy kết                45.600.000đ       │
│ Hoa hồng                    2.280.000đ       │
├──────────────────────────────────────────────┤
│ CHIẾN DỊCH ĐANG MỞ                           │
│ ▸ Bộ sưu tập Thu Đông 2026                   │
│   Hoa hồng 8% + phí 2.000.000đ               │
│   Hạn đăng ký: 20/08                         │
└──────────────────────────────────────────────┘
```

---

## 4. Tạo nội dung — Outfit

```text
┌──────────────────────────────────────────────┐
│ TẠO OUTFIT                                   │
│                                              │
│ Tiêu đề: [Đi làm mùa thu            ]        │
│ Ảnh:     [+ Tải lên]                         │
│                                              │
│ SẢN PHẨM TRONG BỘ                            │
│ ┌──────────────────────────────────────┐     │
│ │ Áo sơ mi linen · Trắng/M   299.000đ  │     │
│ │ Vai trò: Chính        [Xóa]          │     │
│ └──────────────────────────────────────┘     │
│ ┌──────────────────────────────────────┐     │
│ │ Quần âu · Đen/28          450.000đ   │     │
│ │ Vai trò: Chính        [Xóa]          │     │
│ │ ⚠ Gợi ý thêm sản phẩm thay thế       │     │
│ └──────────────────────────────────────┘     │
│ [+ Thêm sản phẩm]                            │
│                                              │
│ Tổng: 749.000đ                               │
│                                              │
│ Chiến dịch: [Thu Đông 2026 ▾]                │
│ ℹ Nội dung sẽ được gắn nhãn "Được tài trợ"   │
│                                              │
│ [ Lưu nháp ]  [ Gửi duyệt ]                  │
└──────────────────────────────────────────────┘
```

### Hai điểm thiết kế quan trọng

**a. Nhãn tài trợ tự động**

```text
Creator chọn chiến dịch có trả phí
    → hệ thống TỰ ĐỘNG gắn nhãn "Được tài trợ"
    → creator KHÔNG tắt được
    → thông báo rõ ràng ngay khi tạo
```

Đây là nghĩa vụ pháp lý của nền tảng, không thể phụ thuộc vào việc creator có nhớ ghi hay không.

**b. Nhắc thêm sản phẩm thay thế**

```text
Nội dung sống lâu hơn sản phẩm.
Khuyến khích creator thêm substitute ngay từ đầu
→ khi sản phẩm hết hàng, nội dung vẫn dẫn tới đâu đó có ích
```

---

## 5. Trang phân tích

```text
┌──────────────────────────────────────────────┐
│ PHỄU CHUYỂN ĐỔI (30 ngày)                    │
│                                              │
│ Lượt xem nội dung   125.000                  │
│         ↓ 6,7%                               │
│ Click sản phẩm        8.400                  │
│         ↓ 14,9%                              │
│ Thêm giỏ hàng         1.250                  │
│         ↓ 12,2%                              │
│ Đơn hàng                152                  │
├──────────────────────────────────────────────┤
│ NỘI DUNG HIỆU QUẢ NHẤT                       │
│                                              │
│ "Đi làm mùa thu"                             │
│ 45.000 xem · 3.200 click · 68 đơn            │
│ GMV 18.900.000đ · Hoa hồng 945.000đ          │
│ Tỷ lệ hoàn: 9% ✓                             │
│                                              │
│ "Phối đồ dạo phố"                            │
│ 32.000 xem · 1.800 click · 41 đơn            │
│ GMV 9.200.000đ · Hoa hồng 460.000đ           │
│ Tỷ lệ hoàn: 28% ⚠                            │
│ 💡 Tỷ lệ hoàn cao — kiểm tra lại mô tả       │
│    size và chất liệu trong nội dung          │
└──────────────────────────────────────────────┘
```

### Vì sao hiển thị tỷ lệ hoàn theo nội dung

Đây là chỉ số **hai chiều**:

```text
Với creator:
    Biết nội dung nào gây hiểu nhầm → điều chỉnh cách mô tả
    Tỷ lệ hoàn thấp = nội dung trung thực = uy tín lâu dài

Với nền tảng:
    Phát hiện nội dung mô tả sai lệch để bán được hàng
```

Creator có động lực giảm tỷ lệ hoàn vì hoa hồng bị đảo ngược khi khách trả hàng.

---

## 6. Trang thu nhập

```text
┌──────────────────────────────────────────────┐
│ ĐỐI SOÁT THÁNG 08/2026                       │
│                                              │
│ Hoa hồng từ đơn hàng        2.280.000đ       │
│ Phí cố định chiến dịch      2.000.000đ       │
│ Quy kết bị đảo (hoàn hàng)   −185.000đ       │
│ ─────────────────────────────────────────    │
│ Thực nhận                   4.095.000đ       │
│                                              │
│ [ Xem chi tiết từng đơn ]                    │
└──────────────────────────────────────────────┘
```

Khi xem chi tiết:

```text
┌──────────────────────────────────────────────┐
│ Mã tham chiếu   Nội dung        Hoa hồng     │
│ #A7X2K          Đi làm mùa thu   45.000đ     │
│ #B3M9P          Đi làm mùa thu   62.000đ     │
│ #C1Q4R          Phối đồ dạo phố  38.000đ     │
│ #D8N2S          (hoàn hàng)     −29.000đ     │
└──────────────────────────────────────────────┘
```

**Mã tham chiếu ẩn danh** — creator đối chiếu được từng giao dịch mà không lộ danh tính khách.

---

## 7. Ba cấu trúc chi phí chiến dịch

```text
┌──────────────────────────────────────────────┐
│ CHIẾN DỊCH: Bộ sưu tập Thu Đông 2026         │
│                                              │
│ Loại: Hỗn hợp (phí + hoa hồng)               │
│ Phí cố định:  2.000.000đ (sau khi đăng đủ)   │
│ Hoa hồng:     8% mỗi đơn quy kết             │
│                                              │
│ YÊU CẦU                                      │
│ • Tối thiểu 3 nội dung                       │
│ • Loại: Video hoặc Outfit                    │
│ • Hạn: 20/08/2026                            │
│                                              │
│ [ Đăng ký tham gia ]                         │
└──────────────────────────────────────────────┘
```

Hiển thị rõ ba cấu trúc — `COMMISSION_ONLY`, `FIXED_FEE`, `HYBRID` — vì mỗi loại creator phù hợp với một cấu trúc khác nhau.

---

## 8. Tài liệu liên quan

- [frontend-architecture.md](frontend-architecture.md)
- [../06-api/creator-api.md](../06-api/creator-api.md)
- [../04-modules/creator.md](../04-modules/creator.md), [../04-modules/affiliate.md](../04-modules/affiliate.md)
