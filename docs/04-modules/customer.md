# Module: Customer

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | Supporting |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý hồ sơ khách hàng
- Sổ địa chỉ
- Sở thích và **dữ liệu size** — đặc thù thời trang
- Danh sách yêu thích (wishlist)
- Quản lý đồng ý xử lý dữ liệu và nhận marketing
- Gộp danh tính khách vãng lai khi đăng ký

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Tài khoản đăng nhập, mật khẩu | `identity` |
| Đơn hàng | `order` |
| Giỏ hàng | `cart` |
| Điểm thưởng, hạng thành viên | `loyalty` |

---

## 3. Dữ liệu size — điểm khác biệt của thời trang

Đây là phần quan trọng nhất và thường bị bỏ qua.

```text
CustomerPreference {
    customer_id
    body_measurements       — chiều cao, cân nặng, số đo (khách tự khai, tùy chọn)
    preferred_sizes         — size theo từng thương hiệu
    style_preferences       — phong cách ưa thích
    return_size_history     — size nào đã trả vì không vừa
    color_preferences
    material_avoidance      — dị ứng, không thích chất liệu nào
}
```

### Vì sao dữ liệu này có giá trị kinh tế trực tiếp

```text
Vấn đề: sai size là nguyên nhân hoàn hàng số một trong thời trang.
        Mỗi lần hoàn hàng tốn chi phí vận chuyển hai chiều,
        kiểm định, đóng gói lại, và rủi ro không bán lại được.

Giải pháp: nếu biết khách thường mặc M ở brand A và L ở brand B,
          có thể gợi ý đúng size ngay từ trang sản phẩm.
```

`return_size_history` đặc biệt hữu ích: nếu khách đã trả một chiếc size M vì "chật", lần sau nên gợi ý L cho sản phẩm tương tự cùng thương hiệu.

**Lưu ý bảo mật:** số đo cơ thể là **dữ liệu cá nhân nhạy cảm**. Phải mã hóa khi lưu, hạn chế truy cập, và cho phép khách xóa. Xem [../09-operations/security.md](../09-operations/security.md).

---

## 4. Bốn trạng thái khách hàng

```text
Guest → Registered → Member → VIP
```

Đây là **bốn trạng thái của một khái niệm**, không phải bốn entity. Một người đi qua các trạng thái mà vẫn giữ nguyên lịch sử mua hàng.

Chi tiết quyền hạn từng trạng thái: [../01-business/customer.md](../01-business/customer.md) mục 1.

---

## 5. Gộp danh tính Guest → Registered

Tình huống: khách vãng lai từng đặt hàng với email `a@b.com`, sau đó đăng ký tài khoản cùng email.

```text
1. XÁC MINH quyền sở hữu email  ← BẮT BUỘC
2. Liên kết các đơn cũ vào customer_id mới
3. Phát event customer.identities_merged
4. loyalty, analytics tự cập nhật
```

**Bước 1 là yêu cầu bảo mật, không phải tùy chọn.** Nếu không xác minh, bất kỳ ai đăng ký bằng email người khác đều đọc được lịch sử mua hàng của họ — kể cả địa chỉ nhà.

---

## 6. Quyền riêng tư và xóa dữ liệu

Khi khách yêu cầu xóa tài khoản:

```text
XÓA / ẨN DANH HÓA:
    - Tên, email, số điện thoại
    - Địa chỉ
    - Số đo cơ thể
    - Lịch sử duyệt web

GIỮ LẠI (ở dạng ẩn danh):
    - Bản ghi đơn hàng
    - Bút toán tài chính
```

**Vì sao giữ lại đơn hàng:** nghĩa vụ lưu trữ chứng từ kế toán và đối soát với seller. Không thể xóa một đơn hàng đã dùng để tính hoa hồng đã trả cho seller.

Cách xử lý: thay thông tin định danh bằng giá trị ẩn danh, giữ nguyên `customer_id` và dữ liệu giao dịch.

---

## 7. Dữ liệu sở hữu

```sql
customer
customer_address
customer_preference
customer_consent        -- đồng ý nhận marketing, xử lý dữ liệu
wishlist
wishlist_item
customer_merge_log      -- lịch sử gộp danh tính
```

```sql
CREATE TABLE customer_consent (
    id             UUID PRIMARY KEY,
    customer_id    UUID NOT NULL,
    consent_type   TEXT NOT NULL,   -- MARKETING_EMAIL | MARKETING_SMS | DATA_PROCESSING | PERSONALIZATION
    granted        BOOLEAN NOT NULL,
    granted_at     TIMESTAMPTZ,
    revoked_at     TIMESTAMPTZ,
    source         TEXT NOT NULL    -- nơi khách đồng ý
);
```

Bảng `customer_consent` là **bắt buộc về mặt pháp lý** ở nhiều thị trường: phải chứng minh được khách đã đồng ý, khi nào, ở đâu.

---

## 8. Interface công khai

```go
type PublicAPI interface {
    GetCustomer(ctx, customerID string) (*CustomerView, error)
    GetCustomersByIDs(ctx, ids []string) (map[string]CustomerView, error)

    GetAddresses(ctx, customerID string) ([]AddressView, error)
    GetDefaultAddress(ctx, customerID string) (*AddressView, error)
    AddAddress(ctx, req AddAddressRequest) (*AddressView, error)

    GetPreferences(ctx, customerID string) (*PreferenceView, error)
    GetSizeRecommendation(ctx, customerID, brandID, productType string) (*SizeHint, error)

    HasConsent(ctx, customerID string, consentType string) (bool, error)

    MergeGuestIdentity(ctx, req MergeRequest) error
}
```

`GetSizeRecommendation` là interface phục vụ giảm tỷ lệ hoàn hàng — cài đặt ban đầu dùng quy tắc đơn giản (size khách hay mua ở brand đó), sau nâng cấp.

---

## 9. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `customer.registered` | Đăng ký |
| `customer.identities_merged` | Gộp danh tính |
| `customer.consent_changed` | Thay đổi đồng ý |
| `customer.deletion_requested` | Yêu cầu xóa tài khoản |
| `wishlist.item_added` | Thêm yêu thích — **tín hiệu nhu cầu mạnh** |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `order.completed` | order | Cập nhật thống kê khách |
| `return.inspected` | return | Ghi nhận lịch sử trả hàng theo size |

Event thứ hai là mắt xích quan trọng: lý do trả hàng "không vừa size" phải quay về hồ sơ khách để lần sau gợi ý đúng.

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Khách vãng lai được đặt hàng |
| 2 | Gộp danh tính bắt buộc xác minh email |
| 3 | Số đo cơ thể là dữ liệu nhạy cảm, phải mã hóa |
| 4 | Xóa tài khoản giữ lại dữ liệu giao dịch ở dạng ẩn danh |
| 5 | Mọi đồng ý marketing phải ghi nhận nguồn và thời điểm |
| 6 | Lý do trả hàng theo size phải quay về hồ sơ khách |

---

## 11. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Hồ sơ, địa chỉ, wishlist, đồng ý cơ bản |
| **Phase 2** | Dữ liệu size, gợi ý size, gộp danh tính |
| **Phase 3** | Phân khúc khách hàng, cá nhân hóa sâu |

---

## 12. Trạng thái triển khai (MVP — 14/08/2026)

Mã nguồn: `internal/modules/customer/`. Migration: `000017_customer`.
Kiểm chứng: 28 test tích hợp trên PostgreSQL thật, đã kiểm chứng ngược.

**Đã có (đúng phạm vi MVP ở mục 11):** hồ sơ · sổ địa chỉ · wishlist ·
đồng ý cơ bản · ẩn danh hóa · gắn tài khoản cho khách vãng lai.

**Chưa có (Phase 2 trở đi):** dữ liệu size và gợi ý size · phát event ·
phân khúc khách hàng.

### Bốn chỗ code KHÁC tài liệu — và vì sao code đúng

Theo nguyên tắc P17, phần này ghi lại thực tế thay vì ép code khớp bản
thiết kế.

**1. Không có `customer_preference` ở MVP**

Mục 3 mô tả dữ liệu size rất kỹ, nhưng mục 11 xếp nó vào **Phase 2**. Code
theo mục 11. Bảng chưa tồn tại vì chưa có gì ghi vào nó: `return.inspected`
là nguồn dữ liệu chính, mà module `return` chưa được xây.

Tạo bảng rỗng từ giờ không giúp gì — nó chỉ tạo cảm giác tính năng đã có.

**2. `MergeGuestIdentity` KHÔNG chuyển đơn hàng**

Mục 5 mô tả bốn bước, trong đó bước 2 là "liên kết các đơn cũ vào
customer_id mới". Code chỉ làm bước 1 và 3 (xác minh, ghi nhật ký) rồi ẩn
danh hồ sơ nguồn.

Lý do: bảng `order` thuộc module khác. `customer` sửa bảng đó là vi phạm
quy tắc R2 — hai module dùng chung bảng thì thực chất là một module. Việc
chuyển đơn thuộc `order`, kích hoạt bằng event.

Ở MVP chưa phát event, nên bước đó **chưa chạy**. Đây là giới hạn thật,
không phải thiết kế xong.

**3. Interface công khai khác mục 8**

Thiếu `GetSizeRecommendation` (thuộc Phase 2, xem điểm 1). Thêm:
`EnsureByEmail`, `LinkUser`, `Anonymize`, `CountWishlistForProduct`.

`EnsureByEmail` là hàm quan trọng nhất không có trong bản thiết kế: khách
vãng lai quay lại phải vào ĐÚNG hồ sơ cũ, và nó phải an toàn khi mười
request chạy song song.

**4. Chưa phát event nào**

Mục 9 liệt kê năm event. Chưa event nào được phát, vì chưa có bên nghe —
`analytics` chưa tồn tại. `wishlist.item_added` sẽ có giá trị nhất (tín
hiệu nhu cầu cho `supplychain`), nhưng phát event không ai nghe là thêm
chỗ hỏng mà không đổi lấy gì.

### Hai phát hiện từ kiểm chứng ngược

**Địa chỉ có hai lớp cách ly, không phải một.** `BelongsTo` ở tầng
application và điều kiện `customer_id` trong SQL. Ban đầu chỉ có lớp thứ
nhất; kiểm chứng ngược cho thấy phá nó là seller đọc/sửa được địa chỉ nhà
của người khác mà không gì chặn lại.

**Điều kiện `status <> 'ANONYMIZED'` trong `FindByEmail` là lớp phòng xa.**
Bỏ nó KHÔNG làm test nào đỏ, vì `Anonymize` đã thay email bằng chuỗi giả.
Giữ lại và ghi rõ vai trò thật của nó, kèm một test cô lập riêng — thay vì
để lại một dòng trông như đang gánh việc.

### Giới hạn đã biết, có chủ ý

| Giới hạn | Vì sao chấp nhận ở MVP |
|---|---|
| Ẩn danh hóa không chạy trong MỘT giao dịch | Cập nhật hồ sơ rồi xóa từng địa chỉ. Thất bại giữa chừng để lại hồ sơ đã ẩn danh mà địa chỉ còn — gọi lại `Anonymize` là dọn xong, vì nó idempotent. |
| Số đo cơ thể chưa mã hóa | Chưa có bảng nào lưu chúng (xem điểm 1). Khi thêm, phải mã hóa theo mục 3. |
| Chưa có giới hạn số địa chỉ mỗi khách | Một tài khoản bị chiếm có thể thêm hàng nghìn địa chỉ. Cần giới hạn ở tầng interfaces. |

---

## 13. Tài liệu liên quan

- [../01-business/customer.md](../01-business/customer.md) — tác nhân khách hàng
- [identity.md](identity.md) — tài khoản đăng nhập
- [../09-operations/security.md](../09-operations/security.md) — bảo vệ dữ liệu cá nhân
