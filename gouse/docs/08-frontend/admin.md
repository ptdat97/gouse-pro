# Admin

Ứng dụng cho nhân viên vận hành nền tảng.

---

## 1. Sơ đồ trang

```text
Users          — người dùng
Sellers        — nhà bán, duyệt hồ sơ
Creators       — creator, duyệt hồ sơ
Products       — danh mục, duyệt sản phẩm, gộp trùng lặp
Orders         — đơn hàng, hỗ trợ khách
Brands         — thương hiệu, ủy quyền
Content        — kiểm duyệt nội dung
Campaigns      — chiến dịch
Supply Chain   — nhà cung cấp, đơn sản xuất, kế hoạch
Warehouse      — kho, nhập hàng, kiểm kê
Finance        — sổ cái, đối soát, chi trả
Analytics      — báo cáo
Audit Log      — nhật ký thao tác
```

---

## 2. Phân quyền theo vai trò

```text
ADMIN              — toàn quyền
OPS_MERCHANDISING  — sản phẩm, danh mục, seller
OPS_WAREHOUSE      — kho, tồn kho, fulfillment
OPS_FINANCE        — sổ cái, đối soát, chi trả
OPS_SUPPORT        — đơn hàng, khách hàng (đọc + hỗ trợ)
OPS_CONTENT        — kiểm duyệt nội dung
```

**Giao diện chỉ hiển thị phần được phép** — nhưng đây chỉ là trải nghiệm. Backend luôn kiểm tra lại.

### Hai yêu cầu bắt buộc

```text
1. Xác thực hai lớp cho ADMIN và OPS_FINANCE
2. Mọi truy cập dữ liệu cá nhân khách hàng được ghi audit
```

---

## 3. Thao tác nhạy cảm — bắt buộc nhập lý do

```text
┌──────────────────────────────────────────────┐
│ ĐIỀU CHỈNH SỔ CÁI                            │
│                                              │
│ ⚠ Thao tác này KHÔNG THỂ hoàn tác.           │
│   Sổ cái bất biến — điều chỉnh tạo bút toán  │
│   MỚI, không sửa bút toán cũ.                │
│                                              │
│ Tham chiếu: [Đơn FC-2026-08-001234    ]      │
│                                              │
│ Lý do (bắt buộc, tối thiểu 20 ký tự):        │
│ ┌──────────────────────────────────────┐     │
│ │ Ghi nhầm tỷ lệ hoa hồng 12% thay vì  │     │
│ │ 10% do lỗi cấu hình ngày 09/08       │     │
│ └──────────────────────────────────────┘     │
│                                              │
│ BÚT TOÁN                                     │
│ DEBIT   PLATFORM_REVENUE      5.980đ         │
│ CREDIT  SELLER_PAYABLE        5.980đ         │
│ ─────────────────────────────────────        │
│ Cân bằng ✓                                   │
│                                              │
│ [ Xác nhận (yêu cầu 2FA) ]                   │
└──────────────────────────────────────────────┘
```

**Ba lớp bảo vệ:**

```text
1. Cảnh báo rõ ràng về tính không thể hoàn tác
2. Lý do bắt buộc, có độ dài tối thiểu
3. Kiểm tra cân bằng bút toán trước khi cho xác nhận
4. Xác thực hai lớp
```

Các thao tác cần lý do: điều chỉnh sổ cái, điều chỉnh tồn kho, đình chỉ seller/creator, gỡ nội dung, hủy đơn, hoàn tiền ngoài quy trình.

---

## 4. Trang duyệt seller

```text
┌──────────────────────────────────────────────┐
│ HỒ SƠ CHỜ DUYỆT — Công ty ABC                │
│                                              │
│ ✓ Giấy phép kinh doanh    [Xem]              │
│ ✓ Mã số thuế 0123456789   Đã xác minh        │
│ ⚠ Tài khoản ngân hàng     CHƯA xác minh      │
│   Tên chủ TK: "CONG TY ABC"                  │
│   Tên đăng ký: "Công ty ABC"                 │
│   [ Xác minh khớp ]                          │
│                                              │
│ CHÍNH SÁCH ÁP DỤNG                           │
│ Loại seller:      [Business ▾]               │
│ Chính sách HH:    [Chuẩn theo ngành ▾]       │
│ Giữ bảo đảm:      [10%] trong [30] ngày      │
│ Chu kỳ đối soát:  [Hàng tuần ▾]              │
│                                              │
│ [ Duyệt ]  [ Từ chối (nêu lý do) ]           │
└──────────────────────────────────────────────┘
```

**Xác minh tài khoản ngân hàng là bước bắt buộc** — sai tài khoản nghĩa là chuyển tiền nhầm người, rất khó thu hồi.

---

## 5. Trang chuỗi cung ứng — hỗ trợ ra quyết định

```text
┌──────────────────────────────────────────────┐
│ ĐỀ XUẤT BỔ SUNG                              │
│                                              │
│ SM-LIN-OXF-WHT-M                             │
│ Áo sơ mi linen Oxford · Trắng/M              │
│                                              │
│ TÌNH TRẠNG                                   │
│ Tồn kho hiện tại:              42            │
│ Điểm đặt hàng lại:            400            │
│ Tốc độ bán:          50 chiếc/tuần           │
│ Lead time:                6 tuần             │
│                                              │
│ TÍN HIỆU NHU CẦU (30 ngày)                   │
│ Lần hết hàng:                   3            │
│ Tìm không ra kết quả:         240            │
│ Đăng ký nhận thông báo:        85            │
│ Thêm wishlist:                190            │
│                                              │
│ ⚠ MÂU THUẪN                                  │
│ MOQ nhà cung cấp:             500            │
│ Dự báo nhu cầu:               300            │
│ → Rủi ro tồn 200 đơn vị (~20 triệu đồng)     │
│                                              │
│ PHƯƠNG ÁN                                    │
│ ○ Đặt theo MOQ (500)                         │
│   Rủi ro tồn dư: ~20.000.000đ                │
│ ○ Bỏ qua                                     │
│   Doanh số mất: ~89.700.000đ                 │
│ ○ Nhà cung cấp B (MOQ 200, +15.000đ/đơn vị)  │
│                                              │
│ [ Phê duyệt ]  [ Bỏ qua (nêu lý do) ]        │
└──────────────────────────────────────────────┘
```

**Đây là ví dụ rõ nhất về nguyên tắc thiết kế của toàn hệ thống:**

```text
Hệ thống KHÔNG tự đặt hàng.
Hệ thống hiển thị:
    ✓ Tín hiệu nhu cầu ĐẦY ĐỦ (kể cả nhu cầu BỊ BỎ LỠ)
    ✓ Mâu thuẫn giữa ràng buộc và dự báo
    ✓ Ước tính tài chính của TỪNG phương án

Con người quyết định.
```

Tự động đặt hàng hoàn toàn là rủi ro lớn — một lỗi tính toán có thể dẫn tới đơn sản xuất sai hàng trăm triệu đồng.

---

## 6. Trang hỗ trợ khách hàng

```text
┌──────────────────────────────────────────────┐
│ TRA CỨU ĐƠN — FC-2026-08-001234              │
│                                              │
│ ⓘ Truy cập dữ liệu khách hàng sẽ được ghi log│
│ Lý do truy cập (bắt buộc):                   │
│ [Xử lý khiếu nại giao hàng chậm        ]     │
│                                              │
│ [ Xem chi tiết ]                             │
└──────────────────────────────────────────────┘
```

Sau khi xem:

```text
┌──────────────────────────────────────────────┐
│ Đơn FC-2026-08-001234                        │
│ Khách: Nguyễn Văn A · 0901234567             │
│ Tổng: 1.280.000đ · Đặt 10/08 14:25           │
│                                              │
│ LÔ HÀNG                                      │
│ ▸ -A Own Brand      Đã giao 12/08 10:15      │
│ ▸ -B Cửa hàng ABC   Đang giao · VN123456789  │
│ ▸ -C Shop XYZ       Chờ xử lý ⚠ quá hạn SLA  │
│                                              │
│ TÀI CHÍNH                                    │
│ [ Xem bút toán liên quan ]                   │
│                                              │
│ LỊCH SỬ THAO TÁC                             │
│ 10/08 14:25 · Hệ thống · Tạo đơn             │
│ 10/08 14:26 · Hệ thống · Thanh toán thành công│
│ 11/08 09:30 · Seller ABC · Bàn giao vận chuyển│
│                                              │
│ [ Liên hệ seller ]  [ Hủy lô C ]             │
└──────────────────────────────────────────────┘
```

**Trang này liên kết mọi thứ:** đơn hàng → lô giao → bút toán → lịch sử thao tác. Nhân viên hỗ trợ cần thấy toàn cảnh để trả lời khách.

---

## 7. Trang audit log

```text
┌──────────────────────────────────────────────┐
│ NHẬT KÝ THAO TÁC                             │
│ Lọc: [Sổ cái ▾] [01/08 – 31/08]              │
│                                              │
│ 15/08 10:23 · nv.hoa · ledger.adjust         │
│   Đơn FC-...-001234 · 5.980đ                 │
│   "Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10%"  │
│                                              │
│ 14/08 16:45 · nv.tuan · seller.suspend       │
│   Seller sel_01J9X                           │
│   "Tỷ lệ hủy đơn 8% vượt ngưỡng 3%"          │
└──────────────────────────────────────────────┘
```

Audit log **chỉ đọc**. Không có giao diện nào cho phép sửa hoặc xóa — nếu sửa được, nó mất hết giá trị.

---

## 8. Nguyên tắc thiết kế giao diện admin

| Nguyên tắc | Lý do |
|---|---|
| Ưu tiên "cần xử lý ngay" | Nhân viên vào để làm việc |
| Cảnh báo trước thao tác không hoàn tác | Chống lỗi thao tác |
| Bắt buộc lý do cho thao tác nhạy cảm | Kiểm toán |
| Hiển thị tác động dự kiến trước khi xác nhận | Ví dụ: "sẽ ẩn 142 offer" |
| Không giấu số liệu bất thường | Số dư âm, chênh lệch phải nổi bật |
| Liên kết chéo giữa các thực thể | Đơn → bút toán → seller → hiệu suất |

---

## 9. Tài liệu liên quan

- [frontend-architecture.md](frontend-architecture.md)
- [../06-api/admin-api.md](../06-api/admin-api.md)
- [../05-data/audit.md](../05-data/audit.md)
- [../09-operations/security.md](../09-operations/security.md)
