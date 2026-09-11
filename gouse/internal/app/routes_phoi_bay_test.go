package app

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestMoiTuyenCuaModuleDeuDuocPhoiBay.
//
// # Vì sao cần hàng rào này
//
// Một tuyến phải khai ở HAI chỗ mới tới được khách:
//
//	module/interfaces/http  Register()   → gắn vào mux RIÊNG của module
//	internal/app/shopper.go              → liệt kê lại để bọc middleware
//
// Danh sách thứ hai tồn tại có lý do: mỗi nhóm tuyến cần chuỗi middleware
// khác nhau (ResolveShopper, RequireIdempotencyKey, RateLimit), và bọc cả
// mux con thì không phân biệt được. Nhưng cái giá là hai danh sách phải
// khớp nhau bằng tay.
//
// Quên danh sách thứ hai KHÔNG gây lỗi biên dịch, không gây lỗi lúc khởi
// động, và không một test nào của module bắt được — test của module gọi
// thẳng mux của module, nơi tuyến CÓ mặt. Nó chỉ hiện ra dưới dạng 404 với
// khách thật. Đã xảy ra khi thêm `DELETE .../coupon`.
//
// Bài này đọc MÃ NGUỒN vì đó là nơi sự thật nằm: `http.ServeMux` không cho
// liệt kê lại các mẫu đã đăng ký.
func TestMoiTuyenCuaModuleDeuDuocPhoiBay(t *testing.T) {
	// Chỉ những module gắn vào mux CON mới có nguy cơ này. Module gắn
	// thẳng vào mux của app (catalog, product, marketplace) không có
	// danh sách thứ hai để lệch.
	muxCon := map[string]string{
		"cart":     "internal/modules/cart/interfaces/http",
		"checkout": "internal/modules/checkout/interfaces/http",
		"customer": "internal/modules/customer/interfaces/http",
		"identity": "internal/modules/identity/interfaces/http",
	}

	phoiBay := docMauTuyen(t, "internal/app")

	for module, thuMuc := range muxCon {
		for mau := range docMauTuyen(t, thuMuc) {
			if phoiBay[mau] {
				continue
			}
			t.Errorf("module %s đăng ký %q nhưng internal/app KHÔNG phơi "+
				"bày nó — tuyến này trả 404 với khách thật.\n"+
				"    Thêm dòng: mux.Handle(%q, h)", module, mau, mau)
		}
	}
}

// gocRepo tìm gốc module Go bằng cách đi ngược tới khi thấy go.mod.
//
// Test chạy với thư mục làm việc là thư mục của CHÍNH gói, nên đường dẫn
// tương đối tới gói khác không dùng được. Đi ngược thay vì viết "../.."
// cứng: bài kiểm này sẽ sống sót khi gói được chuyển chỗ.
func gocRepo(t *testing.T) string {
	t.Helper()

	d, err := os.Getwd()
	if err != nil {
		t.Fatalf("đọc thư mục làm việc: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		cha := filepath.Dir(d)
		if cha == d {
			t.Fatal("không tìm thấy go.mod từ thư mục làm việc trở lên")
		}
		d = cha
	}
}

// docMauTuyen đọc mọi mẫu tuyến `mux.Handle("METHOD /đường/dẫn"` trong một
// thư mục, bỏ qua file test.
//
// Trả về tập hợp để bên gọi tra nhanh; thứ tự không có ý nghĩa.
func docMauTuyen(t *testing.T, thuMuc string) map[string]bool {
	t.Helper()

	re := regexp.MustCompile(`mux\.Handle\(\s*"([A-Z]+ /[^"]*)"`)
	ra := map[string]bool{}

	entries, err := os.ReadDir(filepath.Join(gocRepo(t), thuMuc))
	if err != nil {
		t.Fatalf("đọc thư mục %s: %v", thuMuc, err)
	}
	var soFile int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(gocRepo(t), thuMuc, name))
		if err != nil {
			t.Fatalf("đọc %s: %v", name, err)
		}
		soFile++
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			ra[m[1]] = true
		}
	}

	// Hàng rào cho chính bài kiểm: thư mục không có file nào, hoặc biểu
	// thức chính quy không khớp gì, sẽ làm bài này XANH mà không kiểm gì.
	if soFile == 0 {
		t.Fatalf("%s không có file .go nào — đường dẫn đã đổi?", thuMuc)
	}
	if len(ra) == 0 {
		t.Fatalf("%s không có mẫu tuyến nào — cách đăng ký tuyến đã đổi "+
			"và bài kiểm này không còn kiểm gì", thuMuc)
	}
	return ra
}

// TestDanhSachPhoiBayKhongThua là chiều NGƯỢC lại.
//
// Một dòng ỦY QUYỀN trong shopper.go trỏ tới tuyến mà module không đăng ký
// là tuyến CHẾT: request đi vào mux con và nhận 404 ở đó. Dạng này sinh ra
// khi đổi tên đường dẫn ở module mà quên sửa danh sách.
//
// Chỉ xét dạng `mux.Handle("…", h)` — ủy quyền cho mux con. Các tuyến app
// TỰ phục vụ (/metrics, /health, cấu hình vận hành) truyền handler khác và
// không có danh sách thứ hai để lệch.
func TestDanhSachPhoiBayKhongThua(t *testing.T) {
	dangKy := map[string]bool{}
	for _, thuMuc := range []string{
		"internal/modules/cart/interfaces/http",
		"internal/modules/checkout/interfaces/http",
		"internal/modules/customer/interfaces/http",
		"internal/modules/identity/interfaces/http",
		"internal/modules/order/interfaces/http",

		// returns gắn vào mux CON của order: tuyến trả hàng nằm dưới
		// /orders/{id}/returns nên chúng đi cùng chuỗi middleware của đơn.
		"internal/modules/returns/interfaces/http",
	} {
		for mau := range docMauTuyen(t, thuMuc) {
			dangKy[mau] = true
		}
	}

	b, err := os.ReadFile(filepath.Join(gocRepo(t), "internal/app/shopper.go"))
	if err != nil {
		t.Fatalf("đọc shopper.go: %v", err)
	}
	re := regexp.MustCompile(`mux\.Handle\(\s*"([A-Z]+ /[^"]*)",\s*h\)`)
	khop := re.FindAllStringSubmatch(string(b), -1)
	if len(khop) == 0 {
		t.Fatal("shopper.go không có dòng ủy quyền nào — cách gắn tuyến đã " +
			"đổi và bài kiểm này không còn kiểm gì")
	}

	var thua []string
	for _, m := range khop {
		if !dangKy[m[1]] {
			thua = append(thua, m[1])
		}
	}
	sort.Strings(thua)

	for _, mau := range thua {
		t.Errorf("shopper.go ủy quyền %q nhưng KHÔNG module nào đăng ký "+
			"nó — request sẽ đi vào mux con rồi nhận 404 ở đó", mau)
	}
}

// TestDacTaVaMaKhongTroiXaNhau.
//
// # Khoảng cách phải CÓ CHỦ Ý, không phải trôi dần
//
// Đặc tả API là bản thiết kế cho cả lộ trình, nên nó khai cả những đường
// dẫn thuộc giai đoạn sau. Điều đó hợp lý — nhưng chỉ khi người đọc phân
// biệt được cái nào chạy được.
//
// Đo lúc thêm bài này: 13 trong 79 đường dẫn không có tuyến nào. Mười cái
// thuộc creator/content (Phase 2), hai cái thuộc chuỗi cung ứng (Phase 3),
// và `POST /api/v1/admin/payouts` — thao tác chuyển tiền thật ra ngoài,
// thuộc Phase 2 vì 2FA và tích hợp ngân hàng đều chưa có.
//
// Không có bài kiểm này thì con số 13 chỉ lớn dần, và người tích hợp không
// có cách nào biết endpoint nào gọi được.
//
// # Canh HAI chiều
//
//	thiếu tuyến, KHÔNG có nhãn  → đỏ. Hoặc xây, hoặc nói rõ là giai đoạn sau.
//	CÓ nhãn nhưng đã có tuyến   → đỏ. Nhãn cũ nói dối về thứ đã chạy được.
//
// Chiều thứ hai quan trọng ngang chiều đầu: một nhãn "Phase 2" còn sót lại
// sau khi tính năng đã xong sẽ khiến người đọc bỏ qua một endpoint dùng
// được.
func TestDacTaVaMaKhongTroiXaNhau(t *testing.T) {
	goc := gocRepo(t)

	duongDan := docDuongDanDacTa(t, goc)
	if len(duongDan) == 0 {
		t.Fatal("không đọc được đường dẫn nào từ đặc tả — cách khai đã đổi " +
			"và bài kiểm này không còn kiểm gì")
	}

	tuyen := map[string]bool{}
	for _, thuMuc := range thuMucInterfaceHTTP(t, goc) {
		for mau := range docMauTuyen(t, thuMuc) {
			// Bỏ phương thức, chỉ giữ đường dẫn.
			if i := strings.IndexByte(mau, ' '); i > 0 {
				tuyen[chuanHoaDuongDan(mau[i+1:])] = true
			}
		}
	}
	// internal/app tự phục vụ một số đường (cấu hình vận hành, sức khỏe).
	for mau := range docMauTuyen(t, "internal/app") {
		if i := strings.IndexByte(mau, ' '); i > 0 {
			tuyen[chuanHoaDuongDan(mau[i+1:])] = true
		}
	}

	for _, d := range duongDan {
		coTuyen := tuyen[chuanHoaDuongDan(d.duongDan)]
		switch {
		case !coTuyen && d.giaiDoan == 0:
			t.Errorf("đặc tả khai %q nhưng KHÔNG có tuyến nào, và cũng "+
				"không có nhãn `x-phase`.\n"+
				"    Hoặc xây nó, hoặc thêm `x-phase: 2` vào khối %q ở "+
				"api/paths/ để nói rõ đây là giai đoạn sau.",
				d.duongDan, d.khoi)
		case coTuyen && d.giaiDoan > 0:
			t.Errorf("khối %q vẫn mang nhãn `x-phase: %d` nhưng %q ĐÃ có "+
				"tuyến — gỡ nhãn đi, nếu không người đọc đặc tả sẽ bỏ qua "+
				"một endpoint dùng được", d.khoi, d.giaiDoan, d.duongDan)
		}
	}
}

// mucDacTa là một đường dẫn trong đặc tả, kèm giai đoạn nếu có nhãn.
type mucDacTa struct {
	duongDan string
	khoi     string // ví dụ "admin.yaml#/payouts"
	giaiDoan int    // 0 = không có nhãn, nghĩa là "phải chạy được hôm nay"
}

// chuanHoaDuongDan bỏ TÊN tham số đường dẫn.
//
// Đặc tả viết `{product_id}`, mã viết `{product_id}` — hôm nay giống nhau,
// nhưng đổi tên tham số ở một bên là chuyện thường và không đáng làm bài
// kiểm đỏ.
func chuanHoaDuongDan(p string) string {
	return regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(p, "{}")
}

// docDuongDanDacTa đọc đường dẫn từ openapi.yaml và tra nhãn x-phase.
func docDuongDanDacTa(t *testing.T, goc string) []mucDacTa {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(goc, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("đọc openapi.yaml: %v", err)
	}

	re := regexp.MustCompile(`(?m)^  (/[^\s:]+):\s*\n\s*\$ref:\s*'\./paths/([^#]+)#/(\w+)'`)
	var out []mucDacTa
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		out = append(out, mucDacTa{
			duongDan: m[1],
			khoi:     m[2] + "#/" + m[3],
			giaiDoan: docNhanGiaiDoan(t, goc, m[2], m[3]),
		})
	}
	return out
}

// docNhanGiaiDoan tìm `x-phase` trong MỘT khối của tệp đường dẫn.
//
// Đọc theo khối chứ không theo cả tệp: một tệp có hàng chục khối, và nhãn
// của khối này không nói gì về khối kia.
func docNhanGiaiDoan(t *testing.T, goc, tep, khoi string) int {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(goc, "api/paths", tep))
	if err != nil {
		t.Fatalf("đọc %s: %v", tep, err)
	}

	than := string(b)
	dau := strings.Index(than, "\n"+khoi+":\n")
	if dau < 0 {
		return 0
	}
	than = than[dau+1:]
	// Khối kết thúc ở khóa cấp cao nhất tiếp theo.
	if cuoi := regexp.MustCompile(`\n[A-Za-z_]\w*:`).FindStringIndex(than[len(khoi)+2:]); cuoi != nil {
		than = than[:len(khoi)+2+cuoi[0]]
	}

	m := regexp.MustCompile(`x-phase:\s*(\d+)`).FindStringSubmatch(than)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("nhãn x-phase của khối %s không phải số: %q", khoi, m[1])
	}
	return n
}

// thuMucInterfaceHTTP liệt kê mọi thư mục interfaces/http của module.
func thuMucInterfaceHTTP(t *testing.T, goc string) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(goc, "internal/modules"))
	if err != nil {
		t.Fatalf("đọc internal/modules: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join("internal/modules", e.Name(), "interfaces/http")
		if _, err := os.Stat(filepath.Join(goc, d)); err == nil {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		t.Fatal("không tìm thấy thư mục interfaces/http nào — cấu trúc đã đổi")
	}
	return out
}
