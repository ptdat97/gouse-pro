# Fashion Commerce — Web

Tầng trình bày của nền tảng. Backend Go nằm ở repo anh em [`../gouse`](../gouse).

```text
gouse-pro/
├── gouse/       ← Go backend, đặc tả OpenAPI, tài liệu kiến trúc
└── gouse-web/   ← repo này
```

## Ranh giới nghiêm ngặt nhất

> **Next.js KHÔNG BAO GIỜ truy cập database, KHÔNG chứa logic nghiệp vụ.**

```text
Next.js  →  HTTP/OpenAPI  →  Go  →  Domain/Application  →  Database
```

Không phải:

```text
Next.js  →  Database              ✗
Next.js  →  Logic nghiệp vụ nhân bản  ✗
```

Ngay cả phép cộng đơn giản cũng không làm ở đây — quy tắc giảm giá sẽ phức
tạp dần, và khi đó hai nơi tính ra hai kết quả khác nhau. Xem
[`../gouse/docs/08-frontend/frontend-architecture.md`](../gouse/docs/08-frontend/frontend-architecture.md).

## Cấu trúc

```text
apps/
├── admin/            — giao diện vận hành nội bộ (cổng 3000)
└── storefront/       — cửa hàng cho khách (cổng 3001)

packages/
├── types/            — kiểu SINH TỪ ../gouse/api/openapi.yaml
├── api-client/       — gọi API, xử lý lỗi và làm mới token
├── design-tokens/    — màu, khoảng cách, typography
└── ui/               — component trình bày dùng chung
```

Chưa có `storefront`, `seller-center`, `creator-center` — chúng thuộc P2-5,
P2-6 trong [backlog](../gouse/docs/10-roadmap/backlog.md). Cũng chưa có
`ui-commerce`: admin gần như không dùng component thương mại.

## Bắt đầu

```bash
npm install
npm run types      # sinh kiểu từ đặc tả của backend
cp .env.example .env
npm run dev        # cần backend chạy ở cổng 8080
```

Chạy backend:

```bash
cd ../gouse
make migrate-up
APP_ENV=development MODULES_STORAGE=postgres \
  DATABASE_URL="postgres://postgres@127.0.0.1:5432/gouse?sslmode=disable" \
  make run
```

## Phụ thuộc vào repo anh em

`npm run types` đọc `../gouse/api/openapi.yaml`. **Hai repo phải nằm cạnh
nhau.** Đường dẫn đặt ở `package.json` → `config.spec`, đổi được nếu bố cục
thư mục khác.

Đây là ràng buộc có chủ ý: đặc tả OpenAPI là **nguồn sự thật duy nhất** về
hợp đồng API. Sao chép nó sang đây sẽ tạo bản thứ hai, và hai bản sẽ lệch
nhau.

```bash
npm run types:check   # CI: đỏ nếu kiểu đã commit lệch với đặc tả
```

## Lệnh

```bash
npm run dev         # chạy admin
npm run typecheck   # kiểm tra kiểu toàn bộ workspace
npm run build
```


## Cửa hàng (`apps/storefront`)

```bash
npm run dev:storefront   # cổng 3001
npm run dev:admin        # cổng 3000
```

### Khách VÃNG LAI là mặc định, đăng nhập là TÙY CHỌN

Không trang nào ở đường mua hàng yêu cầu đăng nhập. Danh tính người mua đến
từ cookie `shopper_session` do backend cấp ở request đầu tiên, và trình
duyệt gửi lại nhờ `credentials: "include"` trong `ApiClient`.

Đăng nhập chỉ THÊM: giỏ theo người thay vì theo trình duyệt, cùng trang hồ
sơ, sổ địa chỉ và yêu thích.

**GỘP GIỎ ngay sau khi đăng nhập là bắt buộc.** `ShopProvider.login` gọi
`mergeCartOnLogin` trước khi đọc hồ sơ. Không gộp thì khách thêm hàng lúc
chưa đăng nhập, đăng nhập xong thấy giỏ trống — và họ nghĩ hệ thống mất dữ
liệu của mình. Cảnh báo gộp (`warnings`) hiển thị ở trang giỏ.

**Email đã từng đặt hàng vãng lai sẽ KHÔNG đăng ký được** (`409`). Hồ sơ cũ
chứa lịch sử mua hàng và địa chỉ nhà; gắn nó vào tài khoản vừa đăng ký nghĩa
là ai biết email người khác đều đọc được. Backend phân biệt sẵn hai lý do
trong `message`, nên trang đăng ký hiển thị nguyên văn thay vì tự đoán.

### Luồng mua hàng

```text
/                      danh sách sản phẩm
/products/{id}         chọn NHÀ BÁN (offer), thêm vào giỏ
/cart                  sửa số lượng, xóa món — nhóm theo nhà bán
/checkout              MỞ PHIÊN (giữ hàng 15 phút) → địa chỉ → vận chuyển → đặt
/orders                tra cứu bằng mã đơn + số điện thoại
/orders/{key}          chi tiết đơn + TIẾN ĐỘ GIAO theo từng gói

/dang-nhap             đăng nhập → GỘP GIỎ → hồ sơ
/dang-ky               đăng ký → tự đăng nhập
/tai-khoan             hồ sơ · sổ địa chỉ · yêu thích
```

**Trang chi tiết đơn ghép HAI nguồn.** `order` giữ dòng hàng và tiền,
`fulfillment` giữ tiến độ giao — hai module không gọi được nhau. Trang gọi
hai endpoint rồi khớp `order_line_ids` với `lines` đã có, nên không cần lượt
gọi thứ ba để lấy tên sản phẩm.

**Phiên thanh toán mở LÚC VÀO `/checkout`**, không phải lúc bấm "Đặt hàng".
Mở phiên là giữ tồn kho: mở sớm hơn (ở trang giỏ) là khóa hàng của người
khác trong khi khách còn lưỡng lự; mở muộn hơn thì khách điền xong địa chỉ
mới biết hết hàng. Đồng hồ đếm ngược luôn hiển thị.

### Ba điều trang này KHÔNG làm

1. **Không tự cộng tiền.** Mọi tổng do backend trả. Cộng lại ở đây nghĩa là
   hai nơi cùng tính một con số, và khách thấy một số ở giỏ, số khác ở bước
   thanh toán.
2. **Không giữ bản sao giỏ hàng cục bộ.** Mọi thao tác trả về giỏ đầy đủ và
   ta thay nguyên trạng thái — giá và tình trạng hàng đổi ở server.
3. **Không gửi phí vận chuyển lên.** Chỉ gửi TÊN phương thức; phí do máy chủ
   tra. Gửi phí lên là để khách tự đặt phí ship 0đ cho mình.

### Chưa có ở đợt này

- Quên mật khẩu, đổi mật khẩu, xác minh email.
- Tìm kiếm, lọc theo danh mục/thương hiệu, đánh giá sản phẩm.
