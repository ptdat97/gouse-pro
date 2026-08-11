# Khôi phục sau thảm họa

## 1. Phân loại sự cố

```text
Cấp 1 — Suy giảm
    Một tính năng chậm hoặc lỗi, phần còn lại hoạt động
    Ví dụ: tìm kiếm chậm, gợi ý không hiển thị

Cấp 2 — Mất một phần
    Một luồng nghiệp vụ không dùng được
    Ví dụ: không thanh toán được, không đăng sản phẩm được

Cấp 3 — Mất toàn bộ
    Hệ thống không truy cập được
    Ví dụ: database hỏng, mất cả vùng hạ tầng

Cấp 4 — Mất dữ liệu
    Dữ liệu bị hỏng hoặc mất
    Ví dụ: xóa nhầm bảng, hỏng dữ liệu tài chính
```

Cấp 4 nghiêm trọng nhất — có thể không khôi phục hoàn toàn được.

---

## 2. Nguyên tắc suy giảm có kiểm soát

> Khi một phần hỏng, phần còn lại phải tiếp tục hoạt động.

### Bảng ưu tiên

```text
PHẢI hoạt động (không được hy sinh):
    ✓ Xem sản phẩm
    ✓ Thêm giỏ hàng
    ✓ Đặt hàng và thanh toán
    ✓ Ghi sổ tài chính

CÓ THỂ suy giảm:
    ~ Gợi ý sản phẩm → hiển thị "bán chạy" thay vì cá nhân hóa
    ~ Tìm kiếm nâng cao → tìm kiếm cơ bản bằng SQL
    ~ Feed nội dung → hiển thị nội dung mới nhất
    ~ Thông báo → xếp hàng, gửi sau

CÓ THỂ tạm ngừng:
    ✗ Analytics
    ✗ Báo cáo
    ✗ Tổng hợp tín hiệu nhu cầu
    ✗ Job nền không khẩn cấp
```

### Cài đặt

```text
Mọi lời gọi tới năng lực có thể suy giảm:
    ✓ Timeout ngắn (ví dụ 200ms cho gợi ý)
    ✓ Luôn có phương án dự phòng
    ✓ Không để lỗi lan ra luồng chính

Ví dụ:
    recommendation lỗi → trả về danh sách bán chạy
    → trang sản phẩm VẪN hiển thị, VẪN mua được
```

---

## 3. Kịch bản: Database chính hỏng

```text
Phát hiện:
    - Health check thất bại
    - Cảnh báo P1

Xử lý:
    1. Xác nhận bản chính thật sự hỏng (không phải mạng)
    2. Chuyển sang replica (promote)
    3. Cập nhật thông tin kết nối
    4. Khởi động lại các tiến trình ứng dụng
    5. Kiểm tra tính toàn vẹn dữ liệu
    6. Tạo replica mới từ bản chính mới

RTO mục tiêu: < 1 giờ
RPO thực tế:  độ trễ sao chép (thường vài giây)
```

### Kiểm tra bắt buộc sau khi chuyển đổi

```text
✓ Mọi ledger_entry cân bằng?
✓ balance_snapshot khớp tổng bút toán?
✓ Có inventory_item âm?
✓ Có order PAID nhưng thiếu FulfillmentOrder?
✓ Outbox có event chưa phát bị kẹt?
```

**Rủi ro cụ thể:** giao dịch đang thực hiện lúc sự cố có thể ở trạng thái nửa vời. Ví dụ: đã trừ tồn kho nhưng chưa tạo đơn. Job điều hòa phải phát hiện và báo cáo.

---

## 4. Kịch bản: Xóa nhầm dữ liệu

Đây là kịch bản mà replica **không** cứu được.

```text
Ví dụ: chạy nhầm DELETE FROM "order" WHERE ... lúc 14:23

Xử lý:
    1. NGỪNG NGAY mọi ghi vào bảng bị ảnh hưởng
    2. Xác định thời điểm chính xác của thao tác sai (qua audit log)
    3. Khôi phục database vào môi trường RIÊNG, tới thời điểm 14:22
    4. Trích xuất dữ liệu bị mất
    5. Chèn lại vào production
    6. Kiểm tra tính toàn vẹn và tính nhất quán chéo
    7. Điều tra nguyên nhân, siết quyền truy cập
```

**Điểm quan trọng:** KHÔNG khôi phục toàn bộ production về 14:22 — sẽ mất mọi giao dịch từ 14:22 tới hiện tại. Chỉ trích xuất phần bị mất.

### Phòng ngừa

```text
✓ Sổ cái có RULE chặn UPDATE/DELETE ở tầng database
✓ Audit log bất biến
✓ Không xóa cứng dữ liệu giao dịch (chỉ đánh dấu trạng thái)
✓ Quyền xóa trực tiếp trên production hạn chế tối đa
✓ Thao tác nhạy cảm yêu cầu lý do và xác thực hai lớp
```

---

## 5. Kịch bản: Hỏng dữ liệu tài chính

Nghiêm trọng nhất.

```text
Phát hiện (qua job kiểm tra hàng ngày):
    - Bút toán không cân bằng
    - balance_snapshot lệch với tổng bút toán
    - Độ lệch khi đối chiếu với PSP/ngân hàng

Xử lý:
    1. TẠM DỪNG mọi payout ngay lập tức
    2. Xác định phạm vi: bút toán nào sai, từ khi nào
    3. Tính lại số dư từ bút toán gốc (KHÔNG tin snapshot)
    4. Đối chiếu với sao kê PSP và ngân hàng
    5. Tạo bút toán ĐIỀU CHỈNH (không sửa bút toán cũ)
    6. Ghi audit đầy đủ
    7. Thông báo bên bị ảnh hưởng nếu số tiền thay đổi
```

**Nguyên tắc:** dừng chi tiền trước, điều tra sau. Chi nhầm dễ hơn nhiều so với đòi lại.

**Vì sao ledger bất biến quan trọng ở đây:** vì bút toán không sửa được, luôn tái dựng được số dư đúng từ dữ liệu gốc. Nếu ledger cho phép UPDATE, không biết đâu là sự thật.

---

## 6. Kịch bản: Cổng thanh toán ngừng hoạt động

```text
Ảnh hưởng: khách không thanh toán được → mất doanh thu trực tiếp

Xử lý ngắn hạn:
    1. Hiển thị thông báo rõ ràng cho khách
    2. Chuyển sang phương thức thanh toán khác (nếu có)
    3. Cho phép giữ giỏ hàng, gửi link thanh toán sau

Chuẩn bị dài hạn:
    ✓ Tích hợp ít nhất hai cổng thanh toán (Phase 3)
    ✓ Cơ chế chuyển đổi bằng cấu hình
    → đây là lý do PaymentGateway nằm sau interface (P13)
```

---

## 7. Kịch bản: Reservation bị kẹt

Sự cố ít nghiêm trọng nhưng dễ xảy ra và gây thiệt hại âm thầm.

```text
Nguyên nhân: tiến trình dọn reservation hết hạn ngừng chạy

Hậu quả:
    - Hàng bị khóa vĩnh viễn ở trạng thái Reserved
    - Tồn kho khả dụng giảm dần
    - Cuối cùng không bán được gì, dù kho đầy hàng

Phát hiện:
    Cảnh báo khi số reservation quá hạn > 100

Xử lý:
    1. Khởi động lại tiến trình dọn dẹp
    2. Chạy dọn thủ công cho phần tồn đọng
    3. Kiểm tra tổng tồn kho có đúng không
```

**Đây là ví dụ vì sao cần giám sát chỉ số nghiệp vụ:** mọi chỉ số kỹ thuật đều bình thường (API nhanh, không lỗi), nhưng doanh thu giảm dần.

---

## 8. Ma trận trách nhiệm khi có sự cố

```text
Người trực (on-call)
    → phát hiện, đánh giá cấp độ, khắc phục ban đầu, leo thang nếu cần

Trưởng nhóm kỹ thuật
    → quyết định kỹ thuật lớn (chuyển đổi, quay lui)

Người phụ trách tài chính
    → mọi sự cố liên quan tiền, quyết định tạm dừng payout

Người phụ trách vận hành
    → thông báo seller, creator, khách hàng

Lãnh đạo
    → sự cố cấp 3–4, sự cố kéo dài, sự cố có ảnh hưởng pháp lý
```

---

## 9. Truyền thông khi có sự cố

```text
Nội bộ:
    - Kênh liên lạc riêng cho sự cố
    - Cập nhật mỗi 30 phút với sự cố cấp 3–4

Bên ngoài:
    - Trang trạng thái công khai
    - Thông báo trong ứng dụng nếu ảnh hưởng khách
    - Email cho seller nếu ảnh hưởng đơn hàng của họ

Nguyên tắc:
    ✓ Thông báo sớm, kể cả khi chưa biết nguyên nhân
    ✓ Nói rõ ảnh hưởng gì, đang làm gì, khi nào cập nhật tiếp
    ✗ KHÔNG hứa thời gian khắc phục khi chưa chắc chắn
```

---

## 10. Sau sự cố

```text
Trong 48 giờ:
    ✓ Viết báo cáo sự cố
      - Dòng thời gian
      - Nguyên nhân gốc
      - Ảnh hưởng (số đơn, số tiền, số khách)
      - Cách khắc phục
      - Hành động phòng ngừa

Nguyên tắc: KHÔNG ĐỔ LỖI CÁ NHÂN
    → tập trung vào lỗi hệ thống và quy trình
    → người ta chỉ báo cáo trung thực khi không sợ bị phạt
```

---

## 11. Diễn tập định kỳ

```text
Hàng quý:
    ✓ Diễn tập chuyển đổi database
    ✓ Diễn tập khôi phục tới thời điểm cụ thể

Hàng năm:
    ✓ Diễn tập mất toàn bộ vùng hạ tầng
    ✓ Diễn tập quy trình ứng phó sự cố bảo mật
```

**Nguyên tắc:** quy trình chưa diễn tập là quy trình chưa tồn tại. Khi sự cố thật xảy ra, không ai có thời gian đọc tài liệu.

---

## 12. Tài liệu liên quan

- [backup.md](backup.md) — sao lưu là nền tảng của khôi phục
- [observability.md](observability.md) — phát hiện sự cố
- [security.md](security.md) — sự cố bảo mật
- [../05-data/consistency.md](../05-data/consistency.md) mục 10 — điều hòa dữ liệu
