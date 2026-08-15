// Package testdb cấp cho MỖI GÓI TEST một database riêng.
//
// # Hai sự cố có thật mà gói này ngăn
//
//  1. **Test xóa sạch database phát triển.** Test dọn dữ liệu bằng TRUNCATE.
//     Nếu chúng chạy trên cùng database mà máy chủ đang dùng, một lần
//     `go test ./...` là mất toàn bộ dữ liệu mẫu — và cách hỏng thì im lặng:
//     lần sau mở giao diện mới thấy trống trơn.
//
//  2. **Test đỏ khi chạy song song.** `go test ./...` chạy các gói SONG SONG.
//     `order`, `fulfillment` và `checkout` cùng TRUNCATE bảng `"order"`, nên
//     gói này xóa dữ liệu gói kia đang dùng dở. Trước khi có gói này, chạy
//     ba gói đó cùng lúc hỏng 3/3 lần.
//
// Cách chữa cũ là cờ `-p 1` trong Makefile: chạy nối tiếp từng gói. Nó có
// tác dụng, nhưng là một QUY ƯỚC — ai gõ `go test ./...` thẳng, hoặc chạy
// `make test-race` (không có cờ đó), lại gặp đúng sự cố. Và nó không chữa
// được sự cố thứ nhất chút nào.
//
// # Cách làm
//
//	TEST_DATABASE_URL  →  .../gouse_test        (database KHUÔN, đã migrate)
//	                         ↓ CREATE DATABASE ... TEMPLATE
//	gói order          →  .../gouse_test_modules_order
//	gói cart           →  .../gouse_test_modules_cart
//
// `CREATE DATABASE ... TEMPLATE` là phép sao chép ở tầng PostgreSQL: nhanh
// hơn nhiều so với chạy lại 22 migration cho mỗi gói.
//
// Mỗi tiến trình test là MỘT gói, nên `sync.Once` ở đây tự nhiên có nghĩa
// "một lần cho mỗi gói".
//
// # Vẫn cần dọn dữ liệu giữa các test
//
// Gói này cô lập giữa các GÓI, không giữa các HÀM test trong cùng gói. Các
// lệnh TRUNCATE sẵn có trong từng test vẫn cần thiết — tạo database mới cho
// mỗi hàm test thì quá chậm.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/platform/database"
)

// EnvVar là biến môi trường trỏ tới database KHUÔN.
//
// CỐ TÌNH khác DATABASE_URL. Dùng chung một biến nghĩa là chỉ cần quên xuất
// biến môi trường là test chạy thẳng lên database phát triển — và không có
// gì báo cho tới khi dữ liệu đã mất.
const EnvVar = "TEST_DATABASE_URL"

var (
	once     sync.Once
	perPkg   string
	provErr  error
	provOnce string // tên database đã tạo, để thông báo lỗi rõ ràng
)

// Open trả kết nối tới database RIÊNG của gói test đang gọi.
//
// Bỏ qua test nếu chưa đặt TEST_DATABASE_URL — không phải ai chạy test cũng
// có PostgreSQL, và test đơn vị thuần vẫn phải chạy được.
//
// Kết nối tự đóng khi test kết thúc.
func Open(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.Open(context.Background(), database.Config{DSN: DSN(t)})
	if err != nil {
		t.Fatalf("mở database test %q: %v", provOnce, err)
	}
	t.Cleanup(db.Close)
	return db
}

// DSN trả chuỗi kết nối tới database riêng của gói, dựng nó nếu chưa có.
//
// Dùng khi test cần tự cấu hình pool — ví dụ test tranh chấp cần nhiều kết
// nối hơn mặc định để các goroutine thật sự chạy song song thay vì xếp hàng.
func DSN(t *testing.T) string {
	t.Helper()

	template := strings.TrimSpace(os.Getenv(EnvVar))
	if template == "" {
		t.Skipf("bỏ qua: cần %s để chạy test với PostgreSQL thật "+
			"(chạy `make test-db` một lần để tạo)", EnvVar)
	}

	// HÀNG RÀO: không bao giờ chạy test trên database của máy chủ.
	//
	// Đây là chốt chặn duy nhất đứng giữa `go test ./...` và việc mất sạch
	// dữ liệu phát triển. Nó phải là lỗi DỪNG HẲN, không phải cảnh báo.
	if dev := strings.TrimSpace(os.Getenv("DATABASE_URL")); dev != "" && sameDB(dev, template) {
		t.Fatalf("%s trỏ cùng database với DATABASE_URL (%s).\n"+
			"Test dọn dữ liệu bằng TRUNCATE — chạy tiếp là xóa sạch dữ liệu "+
			"phát triển.\nChạy `make test-db` để tạo database test riêng.",
			EnvVar, redact(template))
	}

	// slug tính TRƯỚC sync.Once: hàm truyền vào Once chạy trong khung ngăn
	// xếp khác, nên runtime.Caller ở đó không thấy được bên gọi.
	slug := packageSlug()

	once.Do(func() {
		perPkg, provOnce, provErr = provision(template, slug)
	})
	if provErr != nil {
		t.Fatalf("dựng database test %q: %v", provOnce, provErr)
	}

	return perPkg
}

// provision tạo database riêng cho gói, sao từ khuôn.
//
// XÓA TRƯỚC KHI TẠO chứ không dọn lúc kết thúc: chạy trước thì lần chạy sau
// vẫn sạch kể cả khi lần trước bị giết giữa chừng, và database còn lại sau
// khi test đỏ là thứ soi được để tìm nguyên nhân.
func provision(template, slug string) (dsn, name string, err error) {
	tmplName, err := dbName(template)
	if err != nil {
		return "", "", err
	}
	name = tmplName + "_" + slug

	// PostgreSQL giới hạn tên database 63 byte. Cắt phần ĐẦU chứ không phải
	// phần đuôi: đuôi là phần riêng biệt nhất (`interfaces_http`), đầu là
	// phần lặp lại ở mọi gói (`gouse_test_modules_`).
	if len(name) > 63 {
		name = name[len(name)-63:]
	}

	admin, err := withDB(template, "postgres")
	if err != nil {
		return "", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, admin)
	if err != nil {
		return "", name, fmt.Errorf("kết nối database quản trị: %w", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`"`); err != nil {
		return "", name, fmt.Errorf("xóa database cũ: %w", err)
	}

	// Thử lại vài lần: `CREATE DATABASE ... TEMPLATE` thất bại khi có kết
	// nối khác đang mở tới khuôn. Với `go test ./...`, nhiều gói cùng sao
	// chép từ một khuôn tại cùng thời điểm.
	createSQL := `CREATE DATABASE "` + name + `" TEMPLATE "` + tmplName + `"`
	for attempt := 0; ; attempt++ {
		_, err = pool.Exec(ctx, createSQL)
		if err == nil {
			break
		}
		if attempt >= 20 || ctx.Err() != nil {
			return "", name, fmt.Errorf(
				"tạo database từ khuôn %q: %w\n"+
					"(khuôn đã tồn tại và đã migrate chưa? chạy `make test-db`)",
				tmplName, err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	dsn, err = withDB(template, name)
	if err != nil {
		return "", name, err
	}
	return dsn, name, nil
}

// packageSlug đặt tên theo THƯ MỤC của gói test đang gọi.
//
// Dùng đường dẫn chứ không dùng tên tệp nhị phân (`os.Args[0]`): ba gói
// `cart/interfaces/http`, `checkout/interfaces/http` và `order/interfaces/http`
// đều cho ra `http.test`, nên chúng sẽ dùng chung một database và mang lại
// đúng sự cố mà gói này sinh ra để chữa.
func packageSlug() string {
	// Đi ngược ngăn xếp tới khung ĐẦU TIÊN nằm ngoài gói này.
	//
	// Không đếm số khung cố định: Open gọi DSN, nên độ sâu khác nhau tùy
	// bên gọi vào bằng đường nào — và một con số cứng sẽ lặng lẽ trả về
	// slug "testdb", khiến mọi gói dùng chung một database.
	var dir string
	for i := 1; i < 12; i++ {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}
		d := filepath.ToSlash(filepath.Dir(file))
		if strings.HasSuffix(d, "/platform/testdb") {
			continue
		}
		dir = d
		break
	}
	return slugify(dir)
}

// slugify đổi đường dẫn thư mục thành tên database dùng được.
//
// Tách khỏi packageSlug để kiểm chứng được: phần đi ngược ngăn xếp phụ
// thuộc vào ai gọi, phần đổi tên thì không.
func slugify(dir string) string {
	if dir == "" {
		return "unknown"
	}

	// Cắt tới sau /internal/ hoặc /cmd/: phần trước đó là đường dẫn tuyệt
	// đối trên máy người chạy, khác nhau ở mỗi máy và không mang thông tin.
	for _, anchor := range []string{"/internal/", "/cmd/"} {
		if i := strings.LastIndex(dir, anchor); i >= 0 {
			dir = dir[i+len(anchor):]
			break
		}
	}

	var b strings.Builder
	for _, r := range strings.ToLower(dir) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	slug := strings.Trim(b.String(), "_")
	if slug == "" {
		return "unknown"
	}
	return slug
}

// dbName lấy tên database từ DSN.
func dbName(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("%s không phải URL hợp lệ: %w", EnvVar, err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("%s thiếu tên database: %s", EnvVar, redact(dsn))
	}
	return name, nil
}

// withDB trả DSN trỏ tới một database khác, giữ nguyên mọi tham số còn lại.
func withDB(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("%s không phải URL hợp lệ: %w", EnvVar, err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// sameDB cho biết hai DSN có trỏ cùng một database không.
//
// So sánh máy chủ VÀ tên database. So mỗi chuỗi thì hai DSN chỉ khác nhau ở
// tham số `sslmode` sẽ lọt qua hàng rào.
func sameDB(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return ua.Host == ub.Host && ua.Path == ub.Path
}

// redact bỏ mật khẩu khỏi DSN trước khi đưa vào thông báo lỗi.
func redact(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "<dsn không đọc được>"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.String()
}
