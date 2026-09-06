# Kiểm thử đầu-cuối qua trình duyệt

```bash
npm run e2e          # chạy tất cả
npm run e2e:ui       # chế độ tương tác, xem từng bước
```

## Cần gì trước khi chạy

Bộ test này chạy trên **stack đang chạy**, không tự dựng:

```bash
cd ../gouse
export DATABASE_URL="postgres://postgres@127.0.0.1:5432/gouse?sslmode=disable"
MODULES_STORAGE=postgres go run ./cmd/api      # API      :8080
MODULES_STORAGE=postgres go run ./cmd/worker   # worker — BẮT BUỘC, xem dưới
cd ../gouse-web
npm run dev:storefront                         # cửa hàng :3001
npm run dev:seller                             # nhà bán  :3002
```

`DATABASE_URL` phải đặt tay: `make run` không đặt nó và backend không có
`.env`, nên thiếu là API chết ngay với `api: database: thiếu DSN`.

**`cmd/worker` không phải tùy chọn.** Không có nó thì outbox không được
vét, `checkout.completed` không bao giờ thành đơn thực hiện, và bài "bàn
giao vận chuyển" đỏ với *"worker chưa tách đơn thực hiện sau khi đặt
hàng"* — trông hệt như hồi quy của thay đổi đang làm, trong khi chỉ là
thiếu một tiến trình.

Tự dựng cả bốn sẽ cần database riêng, dữ liệu mẫu riêng và khoảng một phút
mỗi lần chạy. Đổi lại, test **phải tự tạo dữ liệu của mình** qua giao diện
và không được giả định trạng thái sẵn có — trừ danh mục sản phẩm.

Và vì stack là stack CHUNG, bài test đổi trạng thái phải **trả lại** trạng
thái đó ở cuối (nhập lại tồn kho sau khi đưa về 0, chẳng hạn). Không trả
lại thì nó thành cái bẫy cho bài chạy sau.

Không hardcode mã ULID: dữ liệu mẫu sinh mã mới mỗi lần nạp.

## Phạm vi: chỉ những gì CHỈ trình duyệt mới thấy

Logic nghiệp vụ đã có test ở backend, nhanh hơn hai bậc độ lớn. Lặp lại
chúng qua trình duyệt là trả giá đắt cho cùng một câu trả lời.

Bốn loại lỗi mà **chỉ** chỗ này bắt được — cả bốn đều đã xảy ra thật:

| Loại | Đã xảy ra |
|---|---|
| CORS thiếu origin hoặc thiếu header | 3 lần: cổng 3001, `X-Guest-Phone`, cổng 3002 |
| Trường đặc tả khai nhưng máy chủ không trả | nút "Thêm vào giỏ" khóa vĩnh viễn |
| Trạng thái React bị tháo mất | thông báo kiểm kê biến mất ngay sau khi thành công |
| Lỗi lúc dựng sẵn trang | `useSearchParams` không bọc Suspense |

Điểm chung của cả bốn: **log máy chủ hoàn toàn sạch**. Backend xanh,
TypeScript xanh, và ứng dụng hỏng.

## Vì sao không khẳng định "console không có lỗi nào"

Bản đầu tiên làm vậy và đỏ ngay vì ảnh mẫu trỏ tới `cdn.example.com` không
phân giải được, cùng với 401 của đường dò phiên khách vãng lai — cả hai
đều đúng như thiết kế.

Test đỏ vì lý do sai sẽ bị bỏ qua, rồi lần đỏ THẬT cũng bị bỏ qua theo.
`loi-mang.ts` vì vậy chỉ bắt hai thứ: lời gọi hỏng ở **tầng mạng** (hình
dạng của CORS trong trình duyệt) và **5xx** (máy chủ tự nhận mình hỏng).
4xx không bị bắt — chúng thường là câu trả lời hợp lệ cho câu hỏi hợp lệ.
