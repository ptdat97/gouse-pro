# Tác nhân: Khách hàng (Customer)

## 1. Phân loại

```text
Guest                  — Khách vãng lai, chưa có tài khoản
Registered Customer    — Đã đăng ký tài khoản
Member                 — Đã tham gia chương trình khách hàng thân thiết
VIP                    — Hạng cao nhất, theo chi tiêu hoặc lời mời
```

**Quan trọng:** đây là **bốn trạng thái của một khái niệm duy nhất**, không phải bốn entity. Một người có thể đi từ Guest → Registered → Member → VIP mà vẫn giữ nguyên lịch sử mua hàng.

### Bảng phân biệt

| | Guest | Registered | Member | VIP |
|---|---|---|---|---|
| Duyệt và tìm kiếm | Có | Có | Có | Có |
| Thêm giỏ hàng | Có | Có | Có | Có |
| Đặt hàng | Có | Có | Có | Có |
| Lưu địa chỉ | Không | Có | Có | Có |
| Lịch sử đơn hàng | Tra bằng mã đơn + số điện thoại | Có | Có | Có |
| Wishlist | Không | Có | Có | Có |
| Tích điểm | Không | Không | Có | Có |
| Ưu đãi riêng | Không | Không | Có | Có |
| Ưu tiên hỗ trợ | Không | Không | Không | Có |
| Sớm tiếp cận bộ sưu tập | Không | Không | Không | Có |

**Quyết định thiết kế:** Guest **được phép đặt hàng**. Bắt buộc đăng ký trước khi mua là rào cản chuyển đổi lớn, đặc biệt với khách đến từ nội dung của creator — họ đang ở trạng thái mua ngẫu hứng.

**Hệ quả kỹ thuật:** `Order` không được bắt buộc có `customer_id`. Đơn của khách vãng lai định danh bằng email/số điện thoại. Khi khách đăng ký sau bằng cùng email, hệ thống phải gộp được lịch sử — xem mục 5.4.

---

## 2. Trách nhiệm

Khách hàng trong hệ thống là bên:

- Khám phá và tìm kiếm sản phẩm
- Tiêu thụ nội dung của creator
- Tạo giỏ hàng và đặt đơn
- Thanh toán
- Nhận hàng
- Đánh giá sản phẩm
- Yêu cầu đổi/trả
- Tạo ra **dữ liệu hành vi** — đầu vào của bánh đà nhu cầu

Vai trò cuối cùng thường bị bỏ quên nhưng là vai trò chiến lược nhất. Mỗi lượt xem, tìm kiếm, thêm giỏ, bỏ giỏ đều là tín hiệu nhu cầu. Xem [supply-chain.md](supply-chain.md).

---

## 3. Quyền hạn

| Hành động | Điều kiện |
|---|---|
| Xem sản phẩm công khai | Luôn được |
| Đặt hàng | Guest trở lên |
| Xem đơn hàng của mình | Chủ sở hữu đơn hoặc có mã tra cứu |
| Hủy đơn | Chỉ khi đơn chưa được đóng gói |
| Yêu cầu trả hàng | Trong thời hạn đổi trả, đơn đã giao |
| Đánh giá sản phẩm | Chỉ khi đã mua và nhận hàng sản phẩm đó |
| Dùng điểm thưởng | Member trở lên |

**Nguyên tắc bảo mật:** khách chỉ truy cập được dữ liệu của chính mình. Mọi API khách hàng phải lọc theo chủ thể đăng nhập ở tầng backend — không dựa vào việc frontend không hiển thị. Xem [../09-operations/security.md](../09-operations/security.md).

---

## 4. Quan hệ doanh thu

Khách hàng là **nguồn tiền vào duy nhất** của toàn hệ thống. Mọi dòng doanh thu khác (hoa hồng seller, hoa hồng creator, retail media) đều là cách phân chia tiền do khách hàng chi ra.

Chỉ số quan trọng:

| Chỉ số | Ý nghĩa |
|---|---|
| AOV (Average Order Value) | Giá trị đơn trung bình |
| Purchase Frequency | Tần suất mua lại |
| CLV (Customer Lifetime Value) | Giá trị vòng đời khách hàng |
| CAC (Customer Acquisition Cost) | Chi phí thu hút khách |
| Return Rate | Tỷ lệ hoàn hàng — rất cao trong thời trang do vấn đề size |

**Đặc thù thời trang:** tỷ lệ hoàn hàng ngành thời trang cao hơn hẳn các ngành khác, chủ yếu do sai size. Điều này có hệ quả kiến trúc trực tiếp: quy trình hoàn hàng không phải trường hợp ngoại lệ hiếm gặp, mà là **luồng chính** cần được thiết kế kỹ ngay từ đầu. Xem [../07-workflows/return.md](../07-workflows/return.md).

---

## 5. Vòng đời

### 5.1 Sơ đồ trạng thái

```text
   Ẩn danh (Anonymous)
        │ (duyệt web, có session id)
        ▼
      Guest ─────────────┐
        │ (đăng ký)      │ (đặt hàng không đăng ký)
        ▼                ▼
   Registered ◄──── gộp lịch sử khi đăng ký cùng email
        │ (tham gia loyalty)
        ▼
      Member
        │ (đạt ngưỡng chi tiêu / được mời)
        ▼
       VIP
        │
        ├─→ Inactive (không hoạt động > 12 tháng)
        └─→ Closed (yêu cầu xóa tài khoản)
```

### 5.2 Trạng thái Inactive

Khách không hoạt động quá 12 tháng chuyển sang `Inactive`. Đây không phải xóa dữ liệu — là tín hiệu cho marketing và cho việc loại khỏi mẫu tính chỉ số hoạt động.

### 5.3 Đóng tài khoản và quyền được xóa dữ liệu

Khi khách yêu cầu xóa tài khoản:

- **Xóa/ẩn danh hóa:** thông tin định danh cá nhân, địa chỉ, lịch sử duyệt web.
- **Giữ lại:** bản ghi đơn hàng và bút toán tài chính, ở dạng đã ẩn danh hóa.

**Lý do giữ lại:** nghĩa vụ lưu trữ chứng từ kế toán và đối soát với seller. Không thể xóa một đơn hàng đã dùng để tính hoa hồng trả cho seller. Xem [../05-data/audit.md](../05-data/audit.md).

### 5.4 Gộp danh tính Guest → Registered

Khi khách vãng lai từng đặt hàng với email `a@b.com` sau đó đăng ký tài khoản với cùng email:

```text
1. Xác minh quyền sở hữu email (bắt buộc — nếu không sẽ là lỗ hổng chiếm đoạt đơn hàng)
2. Liên kết các đơn cũ vào customer_id mới
3. Phát event CustomerIdentitiesMerged
4. Các module quan tâm (loyalty, analytics) tự cập nhật
```

Bước xác minh email là bắt buộc về mặt bảo mật: nếu không, bất kỳ ai đăng ký bằng email người khác đều đọc được lịch sử mua hàng của họ.

---

## 6. Luồng nghiệp vụ chính

| Luồng | Tài liệu |
|---|---|
| Mua hàng | [../07-workflows/customer-purchase.md](../07-workflows/customer-purchase.md) |
| Thanh toán đơn nhiều nhà bán | [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) |
| Trả hàng | [../07-workflows/return.md](../07-workflows/return.md) |
| Mua từ nội dung creator | [../07-workflows/content-commerce.md](../07-workflows/content-commerce.md) |

---

## 7. Dữ liệu sở hữu

Module [customer](../04-modules/customer.md) sở hữu:

```text
customer                  — hồ sơ khách hàng
customer_address          — sổ địa chỉ
customer_preference       — tùy chọn (size ưa thích, thương hiệu quan tâm)
customer_consent          — đồng ý nhận marketing, đồng ý xử lý dữ liệu
```

Module khác sở hữu nhưng tham chiếu tới khách hàng:

```text
order                     — thuộc module order
cart                      — thuộc module cart
loyalty_account           — thuộc module loyalty
wishlist                  — thuộc module customer
review                    — thuộc module content
```

**Quy tắc:** các module trên chỉ giữ `customer_id`, không sao chép tên/email/số điện thoại của khách — trừ trường hợp đóng băng dữ liệu giao dịch trong đơn hàng (nguyên tắc P9), khi đó địa chỉ giao hàng được sao chép vào đơn vì địa chỉ có thể bị sửa sau này.

### Dữ liệu đặc thù thời trang cần lưu

Đây là điểm khác biệt so với ecommerce tổng quát:

```text
Số đo cơ thể      — chiều cao, cân nặng, số đo (tùy chọn, khách tự khai)
Size theo brand   — size khách thường mặc ở từng thương hiệu
Phong cách        — sở thích phong cách, dùng cho gợi ý
Lịch sử size trả  — size nào khách đã trả vì không vừa
```

Dữ liệu này giảm tỷ lệ hoàn hàng — vấn đề kinh tế lớn nhất của thương mại thời trang. Đây là dữ liệu nhạy cảm, phải được xử lý theo [../09-operations/security.md](../09-operations/security.md).
