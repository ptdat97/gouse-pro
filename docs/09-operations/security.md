# Bảo mật

## 0. Trạng thái triển khai (20/08/2026)

Tài liệu này mô tả THIẾT KẾ. Bảng dưới ghi mức đã thực sự cài.

| Phần | Trạng thái |
|---|---|
| Xác thực: access token 15 phút + refresh cookie httpOnly 30 ngày | ✅ |
| Xoay refresh token, phát hiện dùng lại | ✅ có test song song |
| `RequireRole` ở tầng middleware | ✅ |
| **Ranh giới dữ liệu kiểm ở domain/application, không ở HTTP** | ✅ xem dưới |
| Kiểm tra đầu vào ở biên | ✅ từng handler, lỗi theo `apierror` |
| CORS theo danh sách trắng | ✅ 3 origin phát triển; production KHÔNG có mặc định |
| Rate limit | 🟡 CHỈ đăng ký; đếm trong BỘ NHỚ (P3-16) |
| 2FA cho `ADMIN` / `OPS_FINANCE` | 🔴 hoãn sau phát hành (P3-5) |
| Nhật ký kiểm toán cho thao tác nhạy cảm | ✅ cùng giao dịch với thao tác |
| Rà soát lộ dữ liệu theo danh sách TRẮNG | 🟡 mới làm cho `/api/v1/sellers` |

### Ranh giới dữ liệu — đã cài ở đâu

Kiểm ở **domain hoặc application**, không phải ở tầng HTTP: tầng HTTP là
nơi dễ quên nhất khi thêm endpoint mới.

```text
Order.ViewableBy              đơn ĐÃ CÓ CHỦ thì CHỈ chủ mở được
Address.BelongsTo             địa chỉ khách khác không đọc được
Wishlist.BelongsTo
FulfillmentOrder.BelongsTo    + WHERE seller_id ở SQL — phòng vệ HAI lớp
marketplace.OwnedOffer        không sở hữu → ErrNotFound, không phải 403
inventory.FindOwnedItem       lọc theo CHỦ SỞ HỮU tồn kho
```

**Hai bài học đã trả giá:**

1. `Order.ViewableBy` từng cho mở đơn của khách ĐÃ ĐĂNG KÝ chỉ bằng số
   điện thoại. Quy tắc đúng: **không suy quyền từ id hay số điện thoại.**
2. `OwnedOffer` trả `ErrNotFound` chứ không phải `Forbidden`. Phân biệt
   hai lỗi đó cho kẻ dò biết offer nào TỒN TẠI.

Việc còn lại — bảng liệt kê MỌI resource × MỌI vai trò, và test khẳng định
response công khai không chứa dữ liệu nội bộ — ở
[backlog.md mục 2.9](../10-roadmap/backlog.md).

---

## 1. Ba tài sản cần bảo vệ nhất

```text
1. TIỀN          — sổ cái, số dư, chi trả
2. DỮ LIỆU KHÁCH — thông tin cá nhân, số đo cơ thể, địa chỉ
3. RANH GIỚI SELLER — seller không thấy dữ liệu seller khác
```

Thứ tự này phản ánh mức thiệt hại khi bị vi phạm.

---

## 2. Xác thực

### Token

```text
Access token   — thời hạn ngắn (15 phút)
Refresh token  — thời hạn dài (30 ngày), thu hồi được

Lưu ở frontend:
    Access token  → bộ nhớ (KHÔNG localStorage — chống XSS)
    Refresh token → httpOnly cookie
```

### Yêu cầu

| Yêu cầu | Lý do |
|---|---|
| Mật khẩu băm bằng thuật toán chậm, có muối | Chống dò mật khẩu offline |
| Giới hạn số lần đăng nhập sai | Chống thử vét cạn |
| Refresh token thu hồi được | Xử lý khi tài khoản bị lộ |
| Ghi log mọi lần đăng nhập | Phát hiện bất thường |
| Người dùng xem và thu hồi phiên | Tự kiểm soát |

### Xác thực hai lớp — bắt buộc

```text
ADMIN            — toàn quyền hệ thống
OPS_FINANCE      — thao tác với tiền
SELLER_OWNER     — nhận tiền, đổi tài khoản ngân hàng
```

Ba vai trò này liên quan trực tiếp tới tiền.

---

## 3. Phân quyền — ba tầng

```text
Tầng 1 — Xác thực (platform/auth)
    "Token hợp lệ, user_id=123, role=SELLER_OWNER, seller_id=sel_01J9X"
    → hạ tầng, trung lập domain

Tầng 2 — Kiểm tra vai trò (middleware)
    "Endpoint /api/v1/seller/* yêu cầu vai trò SELLER_*"

Tầng 3 — Phạm vi dữ liệu (module sở hữu dữ liệu)  ← QUAN TRỌNG NHẤT
    "Truy vấn thêm WHERE seller_id = $ctx.seller_id"
```

### Tầng 3 là nơi bảo mật thật sự

```go
// ❌ SAI — lọc ở tầng hiển thị
orders := repo.FindAll(ctx)
return filterBySeller(orders, sellerID)   // dữ liệu ĐÃ rời database

// ✅ ĐÚNG — lọc trong truy vấn
orders := repo.FindBySeller(ctx, sellerID)
```

Cách sai vừa chậm vừa nguy hiểm — quên gọi hàm lọc một lần là rò rỉ dữ liệu đối thủ.

**Quy tắc:** với mọi endpoint trả dữ liệu, câu hỏi đầu tiên phải là *"người gọi này được xem những bản ghi nào?"* — và điều kiện lọc phải nằm trong truy vấn.

---

## 4. Ranh giới dữ liệu bắt buộc

### Seller

```text
✓ Thấy: FulfillmentOrder của mình, offer của mình, số dư của mình,
        thông tin giao hàng trên đơn của mình

✗ KHÔNG thấy: dữ liệu seller khác (kể cả gián tiếp qua báo cáo
              tổng hợp có thể suy ngược), Order đầy đủ,
              lịch sử mua hàng của khách
```

**Về suy ngược:** không hiển thị "thị phần của bạn trong danh mục: 40%" nếu danh mục chỉ có hai seller — seller kia suy ra doanh số của đối thủ.

### Creator

```text
✓ Thấy: số liệu TỔNG HỢP (click, đơn, GMV quy kết), hoa hồng của mình

✗ KHÔNG thấy: tên, email, số điện thoại, địa chỉ khách hàng,
              mã đơn hàng thật, lịch sử mua của cá nhân nào
```

Creator không phải bên xử lý dữ liệu cá nhân.

### Nhân viên

```text
✓ Thấy: theo vai trò

Bắt buộc: MỌI truy cập dữ liệu cá nhân khách hàng đều ghi audit log
          kèm LÝ DO truy cập
```

---

## 5. Bảo vệ dữ liệu cá nhân

### Phân loại

```text
Nhạy cảm cao (mã hóa khi lưu):
    - Số đo cơ thể       ← đặc thù thời trang
    - Thông tin thanh toán (chỉ token, không lưu số thẻ)
    - Giấy tờ tùy thân của seller

Cá nhân (bảo vệ, hạn chế truy cập):
    - Tên, email, số điện thoại
    - Địa chỉ
    - Lịch sử mua hàng

Ẩn danh hóa khi lưu:
    - IP (băm)
    - Dấu vân tay thiết bị
```

### Về số đo cơ thể

Đây là dữ liệu đặc thù của nền tảng thời trang và **nhạy cảm hơn nhiều người nghĩ**:

```text
Yêu cầu:
    ✓ Mã hóa khi lưu
    ✓ Chỉ khách và hệ thống gợi ý size truy cập được
    ✓ KHÔNG hiển thị cho nhân viên trừ trường hợp bắt buộc (có audit)
    ✓ KHÔNG đưa vào analytics
    ✓ Xóa khi khách yêu cầu
```

---

## 6. Quyền của khách hàng

```text
Quyền xem      → xuất dữ liệu của mình
Quyền sửa      → cập nhật hồ sơ
Quyền xóa      → xóa tài khoản
Quyền rút đồng ý → hủy nhận marketing
```

### Xóa tài khoản — quy trình

```text
XÓA / ẨN DANH HÓA:
    customer.name       → "Đã xóa"
    customer.email      → "deleted-<hash>@invalid"
    customer.phone      → NULL
    customer_address    → xóa
    customer_preference → xóa (bao gồm số đo)
    lịch sử duyệt web   → xóa
    order_address       → ẩn danh hóa (giữ tỉnh/thành cho thống kê)

GIỮ NGUYÊN:
    order, order_line   → nghĩa vụ lưu trữ chứng từ
    ledger_entry        → đã dùng tính hoa hồng trả seller
```

**Vì sao giữ đơn hàng:** không thể xóa một đơn đã dùng để tính tiền trả cho seller — sổ sách sẽ không cân.

---

## 7. Bảo mật thanh toán

```text
KHÔNG BAO GIỜ lưu:
    ✗ Số thẻ đầy đủ
    ✗ CVV
    ✗ Mã PIN

CHỈ lưu:
    ✓ Token do cổng thanh toán cấp
    ✓ Bốn số cuối (để hiển thị)
    ✓ Loại thẻ
```

### Webhook thanh toán

```text
1. XÁC MINH CHỮ KÝ — bắt buộc
   → không có bước này, ai cũng gửi được "thanh toán thành công" giả

2. ĐỐI CHIẾU SỐ TIỀN
   → số tiền webhook báo phải khớp payment_intent trong hệ thống
   → không khớp = không xử lý + cảnh báo ngay

3. IDEMPOTENT
   → nhà cung cấp sẽ gửi trùng
```

### Payout — thao tác nguy hiểm nhất

```text
Chuyển tiền thật ra ngoài hệ thống.

Yêu cầu:
    ✓ Xác thực hai lớp
    ✓ Idempotency-Key bắt buộc
    ✓ Kiểm tra trạng thái TRƯỚC khi gọi API ngân hàng
    ✓ Ghi audit log đầy đủ
    ✓ Đối chiếu định kỳ với sao kê ngân hàng
```

---

## 8. Bảo mật API

| Biện pháp | Chi tiết |
|---|---|
| HTTPS bắt buộc | Chuyển hướng HTTP |
| Giới hạn tốc độ | Theo IP, theo user, chặt hơn cho đăng nhập/đặt hàng |
| Kiểm tra kích thước request | Chống payload lớn |
| Kiểm tra kiểu và định dạng | Chống chèn mã |
| Truy vấn tham số hóa | Chống SQL injection |
| Không lộ thông tin trong lỗi | Không stack trace, không cấu trúc nội bộ |
| CORS chặt | Chỉ domain của mình |

### Thông báo lỗi — cân bằng

```text
❌ Quá chi tiết (lộ thông tin):
   "User admin@company.vn không tồn tại"
   → kẻ tấn công biết email nào có trong hệ thống

❌ Quá mơ hồ (vô dụng):
   "Đã có lỗi xảy ra"

✅ Cân bằng:
   "Email hoặc mật khẩu không đúng"     (đăng nhập)
   "Chỉ còn 2 sản phẩm"                 (nghiệp vụ — chi tiết được)
```

Nguyên tắc: lỗi **xác thực** phải mơ hồ; lỗi **nghiệp vụ** nên chi tiết để khách xử lý được.

---

## 9. Rủi ro đặc thù marketplace

### Hàng giả

```text
Kiểm soát:
    ✓ brand.protection_level (OPEN / VERIFIED_ONLY / RESTRICTED)
    ✓ brand_authorization có valid_until
    ✓ Kiểm tra ở TẦNG DOMAIN khi tạo offer, không phải quy trình thủ công
    ✓ Quy trình gỡ bỏ nhanh khi chủ thương hiệu báo cáo
```

### Gian lận creator

```text
Click ảo · Tự mua rồi hoàn · Cookie stuffing

Phát hiện: BẤT ĐỒNG BỘ, không nằm trong đường đi chính
           → không làm chậm trải nghiệm khách
```

### Seller gian lận

```text
Rủi ro: đăng ký bằng giấy tờ giả, bán hàng, rút tiền rồi biến mất

Kiểm soát:
    ✓ Xác minh tài khoản ngân hàng khớp tên đăng ký
    ✓ Giữ bảo đảm (reserve) với seller mới
    ✓ Chỉ payout sau khi hết hạn đổi trả
    ✓ Theo dõi tỷ lệ hoàn hàng bất thường
```

---

## 10. Quản lý bí mật

```text
✗ KHÔNG trong code
✗ KHÔNG trong repository
✗ KHÔNG trong biến môi trường thường ở production

✓ Dịch vụ quản lý bí mật
✓ Xoay vòng định kỳ
✓ Bí mật production chỉ người vận hành truy cập
✓ Lập trình viên dùng bí mật môi trường phát triển
```

---

## 11. Ứng phó sự cố bảo mật

```text
1. PHÁT HIỆN     — cảnh báo tự động hoặc báo cáo
2. NGĂN CHẶN     — thu hồi token, khóa tài khoản, chặn IP
3. ĐÁNH GIÁ      — phạm vi ảnh hưởng, dữ liệu nào bị lộ
4. KHẮC PHỤC     — vá lỗ hổng
5. THÔNG BÁO     — theo quy định pháp luật nếu rò rỉ dữ liệu cá nhân
6. RÚT KINH NGHIỆM — phân tích nguyên nhân gốc
```

**Chuẩn bị trước:** danh sách liên hệ khẩn cấp, quy trình thu hồi hàng loạt token, kịch bản khóa tính năng nhạy cảm.

---

## 12. Kiểm tra định kỳ

```text
Mỗi lần triển khai:  quét phụ thuộc có lỗ hổng đã biết
Hàng tháng:          rà soát quyền truy cập, audit log bất thường
Hàng quý:            rà soát vai trò và quyền, kiểm tra bí mật đã xoay vòng
Hàng năm:            kiểm thử xâm nhập
```

---

## 13. Tài liệu liên quan

- [../05-data/audit.md](../05-data/audit.md)
- [../06-api/api-domains.md](../06-api/api-domains.md) mục 4
- [../04-modules/identity.md](../04-modules/identity.md)
- [disaster-recovery.md](disaster-recovery.md)
