package testdb

import (
	"strings"
	"testing"
)

// HAI DSN TRỎ CÙNG DATABASE phải bị nhận ra, kể cả khi viết khác nhau.
//
// Đây là logic đứng sau hàng rào duy nhất ngăn `go test ./...` xóa sạch
// database phát triển. So chuỗi thuần thì hai DSN chỉ khác `sslmode` sẽ
// lọt qua — và hậu quả là mất dữ liệu, im lặng, không ai biết cho tới lần
// mở giao diện sau.
func TestNhanRaHaiDSNCungDatabase(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "y hệt nhau",
			a:    "postgres://postgres@127.0.0.1:5432/gouse?sslmode=disable",
			b:    "postgres://postgres@127.0.0.1:5432/gouse?sslmode=disable",
			want: true,
		},
		{
			name: "khác tham số truy vấn — VẪN là cùng database",
			a:    "postgres://postgres@127.0.0.1:5432/gouse?sslmode=disable",
			b:    "postgres://postgres@127.0.0.1:5432/gouse",
			want: true,
		},
		{
			name: "khác mật khẩu — vẫn cùng database",
			a:    "postgres://postgres:abc@127.0.0.1:5432/gouse",
			b:    "postgres://postgres:xyz@127.0.0.1:5432/gouse",
			want: true,
		},
		{
			name: "khác tên database",
			a:    "postgres://postgres@127.0.0.1:5432/gouse",
			b:    "postgres://postgres@127.0.0.1:5432/gouse_test",
			want: false,
		},
		{
			name: "khác cổng — hai máy chủ khác nhau",
			a:    "postgres://postgres@127.0.0.1:5432/gouse",
			b:    "postgres://postgres@127.0.0.1:5433/gouse",
			want: false,
		},
		{
			name: "tên database là tiền tố của tên kia",
			a:    "postgres://postgres@127.0.0.1:5432/gouse",
			b:    "postgres://postgres@127.0.0.1:5432/gouse_test_modules_order",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameDB(c.a, c.b); got != c.want {
				t.Errorf("sameDB = %v, muốn %v", got, c.want)
			}
		})
	}
}

// MỖI GÓI MỘT TÊN KHÁC NHAU — đây là toàn bộ lý do gói này tồn tại.
//
// Ba gói `interfaces/http` của cart, checkout và order đều cho ra tệp nhị
// phân tên `http.test`. Nếu đặt tên database theo đó, chúng dùng chung một
// database và mang lại đúng sự cố cần chữa.
func TestMoiGoiMotTenDatabase(t *testing.T) {
	got := map[string]string{}
	for _, dir := range []string{
		"/Users/ai/gouse/internal/modules/cart",
		"/Users/ai/gouse/internal/modules/order",
		"/Users/ai/gouse/internal/modules/cart/interfaces/http",
		"/Users/ai/gouse/internal/modules/checkout/interfaces/http",
		"/Users/ai/gouse/internal/modules/order/interfaces/http",
		"/Users/ai/gouse/internal/platform/audit",
		"/Users/ai/gouse/cmd/api",
	} {
		slug := slugify(dir)
		if prev, dup := got[slug]; dup {
			t.Errorf("hai gói dùng chung tên %q: %s và %s", slug, prev, dir)
		}
		got[slug] = dir
	}
}

// TÊN KHÔNG PHỤ THUỘC MÁY: đường dẫn tuyệt đối khác nhau ở mỗi máy, nhưng
// cùng một gói phải luôn ra cùng một tên database.
func TestTenKhongPhuThuocDuongDanTuyetDoi(t *testing.T) {
	a := slugify("/Users/ai/gouse/internal/modules/order")
	b := slugify("/home/ci/work/nen-tang/internal/modules/order")
	if a != b {
		t.Errorf("cùng gói ra hai tên: %q và %q", a, b)
	}
	if a != "modules_order" {
		t.Errorf("tên = %q, muốn modules_order", a)
	}
}

// MẬT KHẨU KHÔNG ĐƯỢC XUẤT HIỆN trong thông báo lỗi.
//
// Thông báo của hàng rào đi thẳng vào log CI, và log CI thường công khai
// hoặc chia sẻ rộng hơn nhiều so với biến môi trường.
func TestThongBaoLoiKhongLoMatKhau(t *testing.T) {
	got := redact("postgres://postgres:sieu-bi-mat@127.0.0.1:5432/gouse")
	if got == "postgres://postgres:sieu-bi-mat@127.0.0.1:5432/gouse" {
		t.Fatal("mật khẩu còn nguyên trong chuỗi đã che")
	}
	for _, leak := range []string{"sieu-bi-mat"} {
		if strings.Contains(got, leak) {
			t.Errorf("chuỗi đã che vẫn lộ %q: %s", leak, got)
		}
	}
	// Vẫn phải đọc được là DSN nào, nếu không thì thông báo lỗi vô dụng.
	if !strings.Contains(got, "127.0.0.1:5432") || !strings.Contains(got, "gouse") {
		t.Errorf("chuỗi đã che mất thông tin cần thiết: %s", got)
	}
}
