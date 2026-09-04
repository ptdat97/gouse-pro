# ADR-0015 — Cấu hình vận hành sửa được lúc chạy

**Trạng thái:** Đã chấp nhận · 04/09/2026

## Bối cảnh

Một số con số quyết định cách hệ thống hành xử với người dùng nằm dưới
dạng hằng số trong mã: hạn giao hàng của nhà bán, ngưỡng chấm hiệu suất,
cỡ mẫu tối thiểu.

Đổi chúng đòi hỏi build lại và triển khai lại. Với một con số mà người
kinh doanh quyết chứ không phải kỹ sư, đó là một vòng lặp sai người và sai
tốc độ.

Nhưng "đưa cấu hình lên giao diện" là một mệnh đề nguy hiểm nếu nhận nguyên
văn, vì không phải hằng số nào cũng cùng loại.

## Quyết định

### 1. Hai loại hằng số, và chỉ một loại được đưa lên

Hệ thống có hai loại hằng số **trông giống hệt nhau**:

| | Ví dụ | Ai quyết |
|---|---|---|
| chính sách kinh doanh | hạn giao 48 giờ | người kinh doanh |
| kiểm soát đúng đắn | lý do tối thiểu 20 ký tự | kỹ sư, có review |

**Chỉ loại thứ nhất được đưa lên giao diện.**

Loại thứ hai không bao giờ, và lý do rất cụ thể: một kiểm soát tự nới lỏng
được từ giao diện quản trị thì **không còn là kiểm soát**. Người muốn lách
nó chỉ cần đổi một con số — và thao tác đổi đó cũng do chính họ ký.

Những thứ CỐ Ý không đưa vào, kèm hậu quả nếu đưa:

```text
audit.minReasonLen          hạ xuống 1 → vô hiệu hóa toàn bộ nhật ký truy cập
token.minSecretLen          hạ xuống   → mở cửa cho khóa yếu
eventbus.maxAttempts        hạ xuống   → đẩy cả hàng đợi vào dead letter
identity.MaxFailedAttempts  nâng lên   → mở cửa cho dò mật khẩu
database.MaxConns           thuộc cấu hình TRIỂN KHAI (biến môi trường)
checkout.DefaultTTL         gắn chặt với TTL giữ hàng của inventory; đổi
                            một bên mà không đổi bên kia là bẫy
```

### 2. Sổ đăng ký ĐÓNG, khai trong mã

Bảng `ops_config` chỉ giữ **giá trị**. Danh sách khóa hợp lệ, kiểu, biên và
mặc định nằm ở `internal/platform/opsconfig/registry.go`.

Hệ quả có chủ ý: **không có đường nào thêm tham số mới từ giao diện.** Thêm
một tham số là việc của người viết mã, có review — vì mỗi tham số mới là
một câu hỏi "sửa được lúc chạy có an toàn không", và câu hỏi đó không trả
lời được bằng một cái form.

Khóa lạ trả 404. Dòng trong database có khóa không còn trong sổ đăng ký bị
BỎ QUA lúc nạp, không gây lỗi: xóa khóa khỏi mã mà database còn dòng cũ là
chuyện bình thường khi triển khai lại.

### 3. Mỗi tham số PHẢI có biên

Một tham số không có biên là một tham số ai đó sẽ đặt bằng 0 và làm sập một
thứ ở xa. Cỡ mẫu 0 nghĩa là chấm mọi gian hàng dù chỉ có một đơn.

### 4. Đọc KHÔNG BAO GIỜ trả lỗi, và KHÔNG khóa

Tham số được đọc trên đường phục vụ request. `Doc` trả về giá trị mặc định
đã biên dịch khi bộ đệm trống hoặc database không đọc được.

Hệ thống chạy tiếp bằng con số cũ — đúng bằng hành vi trước khi có tính
năng này. Hỏng theo hướng an toàn.

Bộ đệm là `atomic.Pointer` tới một map CHỈ ĐỌC, không phải RWMutex quanh
một map sửa tại chỗ: đường đọc chạy trên mọi request tính hiệu suất, và
một mutex dùng chung ở đó là điểm tranh chấp cho một thứ gần như không bao
giờ đổi. Ghi thì thay CẢ map, nên người đọc luôn thấy ảnh chụp nhất quán.

### 5. Mọi lần đổi ghi nhật ký, kèm giá trị CŨ

Đổi tham số vận hành ảnh hưởng tới người **ngoài** công ty: hạ hạn giao
hàng làm hàng loạt gian hàng đột ngột bị chấm là giao trễ, và điểm đó ảnh
hưởng tới việc họ thắng buy box.

Nói cách khác: đổi một con số ở đây làm thay đổi **thu nhập của người
khác**. Một lần đổi không có người chịu trách nhiệm và không có lý do thì
không giải thích được khi họ khiếu nại.

Giá trị CŨ nằm trong vết: "đổi thành 24" không trả lời được câu hỏi quan
trọng nhất khi điều tra — đổi từ bao nhiêu?

Ghi vết và ghi giá trị nằm trong **CÙNG một giao dịch**, và giá trị cũ đọc
dưới `pg_advisory_xact_lock` theo tên khóa.

Khóa theo TÊN chứ không theo hàng vì hàng có thể chưa tồn tại — trường hợp
mà `SELECT … FOR UPDATE` không khóa được gì. Không có nó, hai quản trị viên
đổi cùng lúc sẽ cùng đọc một điểm xuất phát, và một trong hai dòng nhật ký
ghi sai lịch sử: "đổi từ 48 thành 36" trong khi thực tế nó đi từ 24.

### 6. Quyền: CHỈ `ADMIN`

Hẹp hơn mọi nhóm quản trị khác. Người vận hành hàng hóa không cần đổi
những con số này; người muốn lách một ngưỡng thì lại rất cần.

### 7. Giao diện phải hiện HỆ QUẢ, không chỉ con số

Mỗi tham số mang một trường `impact` mô tả điều gì xảy ra khi đổi, và giao
diện hiện nó **trước** khi xác nhận.

Người đổi con số hiếm khi là người viết đoạn mã đọc nó. "48" không tự nói
rằng hạ nó xuống sẽ làm hàng loạt gian hàng đột ngột bị chấm là giao trễ.

## Đánh đổi đã chấp nhận

**Nhiều bản sao lệch nhau trong vài chục giây.** Ghi tham số chỉ nạp lại bộ
đệm của tiến trình đang ghi; bản sao khác biết ở lần nạp định kỳ tiếp theo.

Chấp nhận được vì đây là chính sách kinh doanh, không phải kiểm soát đúng
đắn — một khoảng lệch ngắn không gây hại. Đổi lại, đường đọc không tốn một
truy vấn nào.

`LISTEN/NOTIFY` của PostgreSQL sẽ chính xác hơn, nhưng thêm một kết nối
thường trực cho một nhu cầu không cần tới nó.

**Đổi tham số ảnh hưởng NGƯỢC VỀ QUÁ KHỨ.** Hạ SLA xuống 24 giờ làm những
đơn đã giao xong từ trước bị tính lại là trễ, vì chỉ số tính từ dữ liệu
thô mỗi lần hỏi.

Giữ nguyên như vậy, có chủ ý: lưu lại "ngưỡng tại thời điểm đó" cho từng
đơn là một read model riêng, và nó chỉ đáng dựng khi có tranh chấp thật.
Giao diện nói rõ điều này trước khi xác nhận.

## Liên quan

- `internal/platform/opsconfig/` — sổ đăng ký và store
- `migrations/000039_ops_config.up.sql`
- `internal/app/opsconfig_http.go` — API quản trị
- `gouse-web/apps/admin/src/app/config/page.tsx` — giao diện
- [ADR-0011](0011-audit-log.md) — nhật ký thao tác
