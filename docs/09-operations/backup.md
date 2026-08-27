# Sao lưu

## 1. Nguyên tắc

> **Sao lưu chưa từng thử khôi phục = không có sao lưu.**

Đây là nguyên tắc quan trọng nhất. Nhiều tổ chức phát hiện sao lưu hỏng đúng lúc cần dùng.

---

## 2. Hai chỉ số mục tiêu

```text
RPO (Recovery Point Objective)
    — Mất tối đa bao nhiêu dữ liệu?
    — Quyết định tần suất sao lưu

RTO (Recovery Time Objective)
    — Khôi phục trong bao lâu?
    — Quyết định cách sao lưu và chuẩn bị
```

### Mục tiêu theo loại dữ liệu

| Dữ liệu | RPO | RTO | Lý do |
|---|---|---|---|
| **Sổ cái tài chính** | **0** | < 1 giờ | Mất bút toán = không đối soát được |
| Đơn hàng | < 1 phút | < 1 giờ | Khách đã trả tiền |
| Tồn kho | < 1 phút | < 1 giờ | Bán quá số lượng |
| Danh mục sản phẩm | < 1 giờ | < 4 giờ | Tạo lại được nhưng tốn công |
| Nội dung/media | < 1 giờ | < 4 giờ | Object storage có sao chép riêng |
| Analytics | < 24 giờ | < 24 giờ | Không ảnh hưởng giao dịch |

**RPO = 0 cho sổ cái** là yêu cầu nghiêm ngặt nhất, đạt được bằng lưu WAL liên tục.

---

## 3. Chiến lược sao lưu database

```text
1. Sao lưu toàn bộ (full backup)
   → hàng ngày, ngoài giờ cao điểm

2. Lưu WAL liên tục (Write-Ahead Log)
   → cho phép khôi phục tới THỜI ĐIỂM BẤT KỲ (PITR)
   → đây là cách đạt RPO gần 0

3. Bản sao dự phòng (replica)
   → sao chép gần thời gian thực
   → chuyển đổi nhanh khi bản chính hỏng
```

### Vì sao cần cả ba

```text
Replica       — nhanh nhưng KHÔNG bảo vệ khỏi lỗi logic
                (xóa nhầm bảng → replica cũng xóa theo)

Full backup   — bảo vệ khỏi lỗi logic nhưng mất dữ liệu tới 24 giờ

WAL           — kết hợp cả hai: khôi phục tới thời điểm
                ngay trước khi xảy ra lỗi
```

**Kịch bản thực tế:** ai đó chạy nhầm `DELETE FROM order WHERE ...` lúc 14:23. Replica không cứu được. WAL cho phép khôi phục tới 14:22.

---

## 4. Vị trí lưu trữ

```text
Bản sao 1: cùng vùng với database chính  → khôi phục nhanh
Bản sao 2: vùng địa lý KHÁC              → chống sự cố cả vùng
Bản sao 3: lưu trữ lạnh, dài hạn          → nghĩa vụ pháp lý
```

**Nguyên tắc 3-2-1:** ít nhất 3 bản sao, 2 loại phương tiện khác nhau, 1 bản ở vị trí khác.

---

## 5. Thời gian lưu giữ

| Loại | Thời gian giữ | Lý do |
|---|---|---|
| Sao lưu hàng ngày | 30 ngày | Khôi phục sự cố gần |
| Sao lưu hàng tuần | 3 tháng | Phát hiện lỗi muộn |
| Sao lưu hàng tháng | 1 năm | Kiểm toán |
| Dữ liệu tài chính | Theo quy định kế toán | Nghĩa vụ pháp lý |
| WAL | 7–30 ngày | Đủ cho PITR |

---

## 6. Sao lưu object storage

```text
Ảnh, video sản phẩm và nội dung:

    ✓ Bật versioning (giữ phiên bản cũ khi ghi đè)
    ✓ Sao chép sang vùng khác
    ✓ Bật khóa xóa (chống xóa nhầm hàng loạt)
```

**Lưu ý:** media không nằm trong database backup. Khôi phục database mà không khôi phục media sẽ dẫn tới sản phẩm không có ảnh — vẫn là sự cố nghiêm trọng với thương mại thời trang.

---

## 7. Kiểm tra khôi phục — bắt buộc

```text
Hàng tháng:
    ✓ Khôi phục sao lưu vào môi trường riêng
    ✓ Kiểm tra dữ liệu toàn vẹn
    ✓ Đo thời gian khôi phục thực tế
    ✓ So với RTO mục tiêu

Hàng quý:
    ✓ Diễn tập khôi phục tới thời điểm cụ thể (PITR)
    ✓ Diễn tập chuyển đổi sang replica
```

### Kiểm tra tính toàn vẹn sau khôi phục

```text
✓ Mọi ledger_entry có Σ DEBIT = Σ CREDIT?
✓ balance_snapshot khớp tổng bút toán?
✓ Có inventory_item âm?
✓ Số lượng bản ghi khớp với thời điểm sao lưu?
✓ Tham chiếu chéo còn nguyên vẹn?
```

Khôi phục thành công về mặt kỹ thuật nhưng dữ liệu tài chính sai vẫn là thất bại.

---

## 8. Bảo mật sao lưu

```text
✓ Mã hóa khi lưu và khi truyền
✓ Kiểm soát truy cập chặt (chỉ người vận hành)
✓ Ghi log mọi lần truy cập sao lưu
✓ Sao lưu chứa dữ liệu cá nhân → cùng mức bảo vệ như production
```

**Rủi ro thường bị bỏ qua:** sao lưu là bản sao đầy đủ của toàn bộ dữ liệu khách hàng. Sao lưu không được bảo vệ là lỗ hổng lớn ngang với database production bị lộ.

---

## 9. Sao lưu cấu hình và mã nguồn

```text
Mã nguồn:     hệ thống quản lý phiên bản (đã có sẵn)
Cấu hình:     dưới dạng mã, trong repository
Bí mật:       dịch vụ quản lý bí mật có sao lưu riêng
Migration:    trong repository
Đặc tả API:   trong repository
```

**Nguyên tắc:** phải tái dựng được toàn bộ hệ thống từ repository + sao lưu database + sao lưu media.

---

## 10. Tài liệu liên quan

- [disaster-recovery.md](disaster-recovery.md) — kịch bản khôi phục
- [../05-data/database.md](../05-data/database.md) mục 6
- [../05-data/audit.md](../05-data/audit.md) mục 6
