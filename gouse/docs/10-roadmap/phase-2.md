# Phase 2 — Creator Commerce và hoàn thiện vận hành

## 1. Mục tiêu

> Kích hoạt **động cơ tạo nhu cầu** (creator/nội dung) và hoàn thiện các quy trình vận hành mà MVP xử lý thủ công.

Đây là giai đoạn nền tảng bắt đầu khác biệt so với một website thương mại điện tử thông thường.

---

## 2. Module thêm mới (7 module)

| Module | Lý do ở Phase 2 |
|---|---|
| `creator` | Danh tính creator |
| `content` | Nội dung, outfit, product tag |
| `affiliate` | Link, click, quy kết, hoa hồng |
| `campaign` | Chiến dịch với ba cấu trúc chi phí |
| `recommendation` | Gợi ý bằng quy tắc đơn giản |
| `return` | Quy trình hoàn hàng đầy đủ |
| `warehouse` | Vận hành kho, nhập hàng, kiểm kê |

---

## 3. Vì sao thứ tự này

### Creator commerce trước chuỗi cung ứng

```text
Creator commerce (Phase 2):
    - Tạo nhu cầu → tăng doanh số ngay
    - Chi phí thấp hơn quảng cáo
    - Sinh dữ liệu hành vi cho chuỗi cung ứng sau này

Chuỗi cung ứng (Phase 3):
    - Cần dữ liệu nhu cầu tích lũy (đã ghi từ MVP)
    - Chu kỳ dài, không tạo doanh thu ngay
    - Chỉ có giá trị khi đã có quy mô
```

### Return ở Phase 2, không sớm hơn

```text
MVP xử lý thủ công được vì khối lượng nhỏ.

Phase 2 bắt buộc phải có vì:
    - Khối lượng đơn tăng, xử lý tay không kịp
    - Creator commerce làm tăng đơn từ khách mới → tỷ lệ hoàn cao hơn
    - Cần dữ liệu lý do hoàn chuẩn hóa để cải thiện sản phẩm
```

---

## 4. Phạm vi chi tiết

### Creator và nội dung

```text
✓ Đăng ký creator, duyệt hồ sơ, xác minh kênh mạng xã hội
✓ Tạo nội dung: video, ảnh, lookbook, bài viết, OUTFIT
✓ Gắn sản phẩm vào nội dung (có vị trí trên ảnh/video)
✓ Kiểm duyệt nội dung (tự động + thủ công)
✓ Nhãn "Được tài trợ" TỰ ĐỘNG
✓ Feed khám phá
✓ Sản phẩm hết hàng trong nội dung → hiển thị thay thế
```

### Affiliate

```text
✓ Tạo affiliate link
✓ Ghi click bất đồng bộ (không làm chậm chuyển hướng)
✓ LƯU TOÀN BỘ CHUỖI CLICK (không chỉ click cuối)
✓ Quy kết last-click, cửa sổ 7 ngày
✓ Hoa hồng creator, đóng băng tỷ lệ vào Attribution
✓ Đảo ngược khi hoàn hàng
✓ Đối soát và chi trả creator
```

**Điểm quan trọng:** dù dùng last-click, phải lưu đủ chuỗi click để sau này đổi mô hình quy kết mà vẫn tính lại được dữ liệu quá khứ.

### Chiến dịch

```text
✓ Ba cấu trúc chi phí: COMMISSION_ONLY, FIXED_FEE, HYBRID
✓ Xác định bên chịu chi phí (PLATFORM / SELLER / SHARED)
✓ Quản lý ngân sách, tự dừng khi hết
✓ Mời creator tham gia
```

### Trả hàng

```text
✓ Yêu cầu trả hàng với LÝ DO CHUẨN HÓA
✓ Duyệt (tự động một số trường hợp)
✓ Nhận hàng, kiểm định
✓ Nhập lại kho theo kết quả kiểm định
✓ Hoàn tiền theo GIÁ THỰC TRẢ (sau phân bổ giảm giá)
✓ Đảo ngược ĐỦ chuỗi: hoa hồng NT, số dư seller, hoa hồng creator
✓ Ghi lịch sử size vào hồ sơ khách
```

### Kho

```text
✓ Nhiều địa điểm lưu kho
✓ Quy trình nhập hàng
✓ Lấy hàng, đóng gói (quét mã xác nhận)
✓ Kiểm kê
✓ Khu vực riêng cho hàng hoàn
✓ Hàng ký gửi của seller (PLATFORM_SERVICE)
```

### Gợi ý

```text
✓ Sản phẩm tương tự (cùng danh mục, khoảng giá, còn hàng)
✓ "Complete the look" — LẤY TỪ DỮ LIỆU OUTFIT
✓ Xu hướng (doanh số gần đây có trọng số thời gian)
✓ Cá nhân hóa cơ bản (danh mục, thương hiệu đã mua/xem)
✓ LỌC THEO SIZE khách mặc
```

**Lưu ý:** "Complete the look" dùng dữ liệu `Outfit` do stylist tạo — chất lượng cao mà không cần thuật toán phức tạp. Đây là ví dụ tốt của nguyên tắc P14.

---

## 5. Nâng cấp module có sẵn

| Module | Bổ sung ở Phase 2 |
|---|---|
| `customer` | Dữ liệu size, gợi ý size, gộp danh tính guest |
| `inventory` | Nhiều địa điểm, chuyển kho, xử lý hàng hoàn |
| `fulfillment` | Nhiều kho, phân bổ nguồn hàng, nhiều đối tác vận chuyển, xử lý giao thất bại |
| `payment` | Đối soát tự động, payout tự động, hoàn tiền |
| `seller` | Chấm điểm hiệu suất, chính sách riêng |
| `marketplace` | Buy box đầy đủ, kiểm soát thương hiệu bảo vệ |
| `promotion` | Khuyến mãi tự động, phân bổ chi phí, khuyến mãi của seller |
| `product` | Chống trùng lặp, quy trình gộp sản phẩm |
| `catalog` | Bộ sưu tập, ủy quyền thương hiệu |
| `analytics` | Phễu chuyển đổi, chỉ số creator, dashboard seller |
| `notification` | Nhiều kênh (SMS, push), nhắc giỏ bỏ quên |

---

## 6. Hạ tầng bổ sung

```text
Thêm KHI ĐO ĐƯỢC nhu cầu:

Cache
    → khi có điểm nóng đo được, database tải cao

Chỉ mục tìm kiếm riêng
    → khi tìm kiếm SQL chậm hoặc thiếu tính năng lọc phức tạp

Bản sao chỉ đọc
    → khi truy vấn báo cáo ảnh hưởng giao dịch
```

**Nguyên tắc:** không thêm hạ tầng theo lịch. Thêm khi có số liệu chứng minh cần.

---

## 7. Vòng lặp bánh đà bắt đầu quay

Đây là điểm quan trọng nhất của Phase 2:

```text
Creator tạo nội dung
    ↓
Khách xem, click, thêm giỏ, mua
    ↓
Dữ liệu hành vi được ghi:
    - content.viewed
    - affiliate.click_recorded
    - cart.item_added (có source_content_id)
    - order.placed
    - inventory.depleted
    - return.inspected (kèm lý do)
    ↓
demand_signal tích lũy
    ↓
(Phase 3 sẽ dùng để lập kế hoạch sản xuất)
```

Cuối Phase 2, nền tảng có **dữ liệu nhu cầu thật** để bước vào Phase 3.

---

## 8. Tiêu chí hoàn thành

### Chức năng

```text
✓ Creator đăng nội dung, gắn sản phẩm, tạo affiliate link
✓ Khách mua qua nội dung → quy kết đúng creator
✓ Hoa hồng creator tính đúng, đảo ngược đúng khi hoàn hàng
✓ Khách yêu cầu trả hàng qua hệ thống, không cần liên hệ thủ công
✓ Hàng hoàn qua kiểm định trước khi nhập lại kho
✓ Đối soát và chi trả seller/creator tự động theo chu kỳ
```

### Chất lượng

```text
✓ Độ trễ chuyển hướng affiliate link < 50ms
✓ Tỷ lệ quy kết bị đảo ngược trong ngưỡng
✓ Chuỗi đảo ngược tài chính khi hoàn hàng ĐỦ 100%
✓ Không có nội dung dẫn tới trang lỗi khi sản phẩm hết
```

### Dữ liệu

```text
✓ demand_signal đã tích lũy đủ để phân tích (tối thiểu 6 tháng)
✓ Lý do hoàn hàng chuẩn hóa, phân tích được
✓ Toàn bộ chuỗi click được lưu (không chỉ click quy kết)
```

---

## 9. Rủi ro chính

| Rủi ro | Giảm thiểu |
|---|---|
| Creator không tin số liệu quy kết | Minh bạch từng dòng, giải thích rõ mô hình last-click |
| Gian lận click | Ghi đủ ngữ cảnh, phát hiện bất đồng bộ |
| Chuỗi đảo ngược hoàn hàng thiếu bước | Kiểm tra tự động: mọi return.refunded phải sinh đủ N bút toán |
| Nội dung mô tả sai lệch để bán hàng | Theo dõi tỷ lệ hoàn theo nội dung |
| Bảng `click` phình quá nhanh | Phân vùng theo ngày, chính sách lưu trữ 90 ngày |

---

## 10. Tài liệu liên quan

- [mvp.md](mvp.md), [phase-3.md](phase-3.md)
- [../01-business/content-commerce.md](../01-business/content-commerce.md)
- [../07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md)
