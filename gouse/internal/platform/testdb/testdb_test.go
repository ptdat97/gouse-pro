package testdb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/database"
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

// ------------------------------------------- Thử lại khi kết nối hết hạn

// Vì sao có vòng thử lại.
//
// `go test ./...` khởi động bằng việc hai mươi ba gói cùng chạy
// `DROP DATABASE` rồi `CREATE DATABASE ... TEMPLATE` — mỗi lệnh sau là một
// lần chép file của cả khuôn. Cùng lúc Go biên dịch và chạy test trên mọi
// lõi. Trong cửa sổ ấy, một lần bắt tay kết nối có thể vượt quá hạn chờ.
//
// Ngày 24/09/2026 chuyện đó xảy ra hai lần trong một buổi, ở hai gói khác
// nhau (`analytics`, `customer`), và cả hai lần chạy lại đều xanh. Một bộ
// test thỉnh thoảng đỏ dạy người ta bấm chạy lại thay vì đọc — nguy hiểm
// hơn một bộ test đỏ hẳn.
//
// ĐÃ LOẠI TRỪ: không phải chạm trần kết nối. Đo trong lúc chạy cả bộ,
// đỉnh là 22 trên `max_connections = 100`.

func rutNganNhipThuLai(t *testing.T) {
	t.Helper()
	nhip, lan := nhipThuLai, soLanThu
	nhipThuLai = time.Millisecond
	t.Cleanup(func() { nhipThuLai, soLanThu = nhip, lan })
}

// Hết hạn thì THỬ LẠI, và lần sau thành công thì trả về kết nối ấy.
func TestHetHanThiThuLai(t *testing.T) {
	rutNganNhipThuLai(t)

	goi := 0
	_, err := thuLai(func() (*database.DB, error) {
		goi++
		if goi < 3 {
			return nil, fmt.Errorf("bọc: %w", context.DeadlineExceeded)
		}
		return nil, nil // lần thứ ba "thành công"
	})
	if err != nil {
		t.Fatalf("lần thứ ba thành công thì không được trả lỗi: %v", err)
	}
	if goi != 3 {
		t.Errorf("gọi %d lần, mong 3", goi)
	}
}

// Lỗi KHÔNG phải hết hạn thì đỏ NGAY, không thử lại.
//
// Postgres không chạy, DSN sai, sai mật khẩu — lần sau vẫn thế. Thử lại
// chỉ làm mỗi gói treo thêm vài giây trước khi đỏ, nhân với hai mươi ba
// gói.
func TestLoiKhacHetHanThiKhongThuLai(t *testing.T) {
	rutNganNhipThuLai(t)

	goi := 0
	loiGoc := errors.New("database: DSN không hợp lệ")
	_, err := thuLai(func() (*database.DB, error) {
		goi++
		return nil, loiGoc
	})
	if goi != 1 {
		t.Errorf("gọi %d lần, mong 1 — lỗi cấu hình không đáng thử lại", goi)
	}
	if !errors.Is(err, loiGoc) {
		t.Errorf("phải trả NGUYÊN lỗi gốc để người đọc biết sửa gì, nhận %v", err)
	}
}

// Hết hạn mãi thì chịu thua, và thông báo phải nói rõ đã thử mấy lần.
func TestHetHanMaiThiChiuThua(t *testing.T) {
	rutNganNhipThuLai(t)

	goi := 0
	_, err := thuLai(func() (*database.DB, error) {
		goi++
		return nil, fmt.Errorf("bọc: %w", context.DeadlineExceeded)
	})
	if goi != soLanThu {
		t.Errorf("gọi %d lần, mong %d", goi, soLanThu)
	}
	if err == nil {
		t.Fatal("hết hạn mãi mà không báo lỗi")
	}
	if !strings.Contains(err.Error(), "3 lần") {
		t.Errorf("thông báo phải nói đã thử mấy lần, nhận: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("phải giữ lỗi gốc để phân biệt với lỗi cấu hình: %v", err)
	}
}
