# ADR-0014: Mã hóa trường nhạy cảm bằng AES-256-GCM, khóa từ cấu hình

**Trạng thái:** Accepted
**Ngày:** 27/08/2026

---

## Context

`applyAsSeller` có trong đặc tả từ đầu nhưng chưa bao giờ có route, nên
nhà bán không tự đăng ký được — cách duy nhất là admin chạy công cụ dòng
lệnh. Với một cái chợ, đó là chặn ngay ở cửa.

Khi bắt tay làm thì lộ ra chỗ vướng thật. Đặc tả bắt buộc `bank_account`
trong thân request, còn domain chỉ có **một cờ boolean**
`bankAccountVerified`. Không bảng nào lưu số tài khoản — đã kiểm cả schema
`seller` lẫn toàn bộ danh sách bảng.

Hệ quả không chỉ là thiếu một endpoint:

```text
seller.bank_account_verified = true
   ↑ điều kiện BẮT BUỘC để kích hoạt gian hàng
   ↑ nhưng không ai biết nó nói về TÀI KHOẢN NÀO
```

Dữ liệu thật có đúng tình trạng đó: một nhà bán ACTIVE, cờ đã bật, không
tài khoản nào. Họ bán được hàng nhưng không nhận được tiền.

`docs/09-operations/security.md` mục 5 xếp thông tin thanh toán vào nhóm
**"mã hóa khi lưu"**, nhưng không nói mã hóa bằng gì. `internal/platform/privacy`
lúc đó chỉ có băm địa chỉ IP.

## Decision

### 1. AES-256-GCM, khóa 32 byte từ `ENCRYPTION_KEY`

GCM cho cả bí mật **lẫn toàn vẹn**. Với số tài khoản ngân hàng, khác biệt
giữa "báo lỗi" và "trả về rác trông như số thật" là khác biệt giữa dừng
lại và chuyển tiền cho người lạ.

Nonce ngẫu nhiên mỗi lần mã hóa. Bản mã giống nhau sẽ tiết lộ rằng hai nhà
bán dùng chung một số tài khoản — một rò rỉ thật, không cần giải mã gì cả.

Mỗi bản mã mang nhãn phiên bản `v1:`. Xoay khóa là việc chắc chắn sẽ tới;
không có nhãn thì lúc đó không phân biệt được bản mã cũ với mới, và cách
duy nhất còn lại là giải mã thử.

Thiếu khóa ở production là **lỗi khởi động**, cùng lý lẽ với
`AUTH_JWT_SECRET`: mặc định sai nguy hiểm hơn thiếu, vì nó chạy được và
không ai để ý.

### 2. Số tài khoản đầy đủ KHÔNG nằm trong entity

`Seller` mang `TaiKhoanNganHang{BankCode, Holder, Last4}` — đủ cho mọi màn
hình hiển thị, mọi khâu duyệt hồ sơ, mọi lần đối chiếu tên.

Số đầy đủ chỉ cần cho đúng **một** việc: chuyển tiền. Để nó trong entity
nghĩa là nó đi theo mọi lời gọi đọc nhà bán, vào mọi log ai đó lỡ in cả
struct, và vào mọi response ai đó lỡ trả nguyên vẹn.

Cắt nó ra khỏi entity là cắt luôn các đường ấy. Muốn số đầy đủ thì phải
gọi `LaySoTaiKhoan` — một hàm riêng, đếm được, và là chỗ **duy nhất** giải
mã. `cols` của kho lưu trữ cố ý không chứa cột bản mã: thứ không được đọc
thì không lộ được.

### 3. Bốn số cuối lưu RIÊNG ở dạng rõ

Theo đúng quy tắc đã áp cho thẻ ở `security.md` mục 7. Không có nó thì mọi
màn hình muốn hiện `…4567` đều phải giải mã, và đường giải mã càng nhiều
nơi gọi thì càng khó canh.

Tên chủ tài khoản cũng để rõ: nó phải đối chiếu được với tên đăng ký ngay
lúc duyệt hồ sơ, và mã hóa nó chỉ mở thêm một đường đọc mà không giấu được
gì — tên doanh nghiệp vốn đã nằm ngay cột bên cạnh.

### 4. Ràng buộc ở tầng dữ liệu, thêm dạng `NOT VALID`

```sql
CHECK (NOT bank_account_verified
       OR seller_type = 'INTERNAL'
       OR length(bank_account_number_enc) > 0)
```

Hợp với ràng buộc `seller_active_needs_bank` có sẵn, chuỗi thành:
**ACTIVE ⇒ đã xác minh ⇒ có tài khoản.**

`NOT VALID` vì dữ liệu thật đã có nhà bán ACTIVE thiếu tài khoản, và cả
hai cách xử lý trong migration đều tệ: kiểm ngay thì chặn cả đợt triển
khai; tự hạ trạng thái thì âm thầm ngắt một gian hàng đang bán. Cách thứ
ba là chặn từ đây trở đi và để việc dọn thành thao tác vận hành có người
nhìn — migration ghi sẵn câu truy vấn tìm và câu `VALIDATE CONSTRAINT`.

## Alternatives

**Chỉ lưu token do cổng thanh toán cấp** — đúng với thẻ, sai với chi trả.
Token thẻ dùng để TRỪ tiền; ở đây ta cần CHUYỂN tiền tới, và không có nhà
cung cấp payout nào đang tích hợp để tokenize.

**Mã hóa ở tầng database (pgcrypto)** — loại. Khóa nằm trong câu SQL hoặc
trong cấu hình database, nên ai đọc được log truy vấn hoặc `pg_stat_statements`
là đọc được khóa. Mã hóa trong ứng dụng giữ khóa ở đúng một nơi.

**Mã hóa cả tên chủ tài khoản** — loại, xem quyết định 3.

**Hoãn `bank_account` sang một endpoint riêng sau khi duyệt** — cân nhắc
nghiêm túc, và nó mở được luồng đăng ký sớm hơn. Loại vì phải sửa hợp đồng
API để bỏ một trường đang là bắt buộc, và vì thứ tự ấy sai: duyệt một hồ
sơ mà chưa biết trả tiền đi đâu là duyệt thiếu thông tin quan trọng nhất.

## Consequences

**Tốt**

- Nhà bán tự đăng ký được. Luồng onboarding không còn cần admin chạy lệnh.
- `bank_account_verified` từ nay có nghĩa: nó nói về một tài khoản cụ thể.
- Số tài khoản không nằm ở dạng rõ trong database, và có test quét TOÀN BỘ
  cột văn bản của bảng `seller` để giữ điều đó.
- Một chỗ duy nhất giải mã, nên thêm audit sau này chỉ phải sửa một nơi.

**Xấu**

- Thêm một biến môi trường bắt buộc ở production. Quên nó thì hệ thống
  khởi động được nhưng hồ sơ đăng ký bị từ chối — đã chuyển thành lỗi khởi
  động để không ai gặp tình trạng đó.
- Mất khóa là mất dữ liệu. Không có cách khôi phục, và cũng không nên có.
- `save` phải mang theo bản mã đang có qua một truy vấn con — xem *Ghi chú
  kỹ thuật* bên dưới.

## CHƯA làm

Ghi rõ để không ai tưởng phần này đã xong:

| Việc | Vì sao chưa |
|---|---|
| Xoay khóa | Nhãn `v1:` đã có sẵn để đỡ, nhưng chưa có quy trình lẫn công cụ di trú |
| KMS / HSM | Khóa hiện đọc từ biến môi trường. Production thật nên lấy từ KMS |
| Audit khi đọc số tài khoản | `LaySoTaiKhoan` chưa ghi vết. Phải có trước khi đường chi trả chạy |
| Mã hóa số đo cơ thể | `security.md` cũng yêu cầu; module customer chưa có chỗ lưu |

## Ghi chú kỹ thuật: `INSERT … ON CONFLICT` và ràng buộc CHECK

Mất một lúc mới tìm ra, và nó phản trực giác nên ghi lại.

`save` là một upsert. Entity không mang số tài khoản, nên ban đầu câu
INSERT không liệt kê cột bản mã — nhánh `DO UPDATE` cũng không đụng tới nó,
đúng như mong muốn: bản mã phải sống sót qua mọi lần ghi thường.

Nhưng mọi lần đặt `bank_account_verified = true` đều vi phạm ràng buộc,
kể cả khi bản mã đang có 59 ký tự trong database.

Lý do: PostgreSQL kiểm CHECK trên **dòng đề xuất** trước khi phát hiện
xung đột và chuyển sang nhánh `DO UPDATE`. Dòng đề xuất có
`bank_account_verified = true` và `bank_account_number_enc = ''` (giá trị
mặc định của cột không được liệt kê) — bị bác ngay tại đó. Nhánh
`DO UPDATE` không bao giờ chạy tới.

Cách sửa: cho câu INSERT mang theo bản mã đang có.

```sql
COALESCE((SELECT bank_account_number_enc FROM seller WHERE id = $1), '')
```

Bài học chung: với upsert, ràng buộc CHECK phải đúng với **dòng đề xuất**,
không chỉ với dòng cuối cùng.
