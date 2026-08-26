package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// duongDaDangKy quét mã nguồn tìm MỌI route dưới /api/v1/admin/ và
// /api/v1/seller/.
//
// Quét nguồn thay vì hỏi mux vì `http.ServeMux` không cho liệt kê pattern
// đã đăng ký.
var duongDaDangKy = regexp.MustCompile(
	`mux\.Handle(?:Func)?\("([A-Z]+) (/api/v1/(?:admin|seller)/[^"]*)"`)

// TestMoiDuongCanQuyenDeuDuocKiem biến "phải nhớ" thành "test sẽ báo".
//
// # Vấn đề nó giải quyết
//
// `TestDuongQuanTriChanNguoiKhongCoQuyen` chỉ mạnh bằng danh sách nó duyệt.
// Thêm một route quản trị mới mà quên khai vào danh sách thì test vẫn xanh
// và route đó KHÔNG được ai kiểm — đúng loại lỗi mà cả bộ test này sinh ra
// để chặn.
//
// Bài này đọc mã nguồn, tìm mọi route dưới tiền tố cần quyền, rồi đối
// chiếu với danh sách. Thiếu một đường là đỏ, kèm tên đường.
//
// # Vì sao chấp nhận một test "biết về mã nguồn"
//
// Nó phá vỡ ranh giới thường thấy giữa test và cài đặt, và đó là đánh đổi
// có ý thức: cái giá của việc quên bọc `RequireRole` là mở toang một
// đường quản trị, còn cái giá của bài test này là phải sửa nó khi đổi cách
// đăng ký route. Vế thứ hai rẻ hơn nhiều.
func TestMoiDuongCanQuyenDeuDuocKiem(t *testing.T) {
	daKiem := map[string]bool{}
	for _, d := range duongCanQuyen() {
		daKiem[d.method+" "+mauDuong(d.path)] = true
	}

	// `/api/v1/admin/me` cố ý chỉ cần đăng nhập — có test riêng.
	daKiem["GET /api/v1/admin/me"] = true

	goc := []string{"internal/app", "internal/modules"}
	var thieu []string

	for _, g := range goc {
		err := filepath.Walk("../../"+g, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range duongDaDangKy.FindAllStringSubmatch(string(b), -1) {
				khoa := m[1] + " " + mauDuong(m[2])
				if !daKiem[khoa] {
					thieu = append(thieu, khoa+"  ("+filepath.Base(path)+")")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("quét %s: %v", g, err)
		}
	}

	if len(thieu) > 0 {
		t.Errorf("%d đường cần quyền KHÔNG có trong danh sách kiểm phân quyền "+
			"— thêm vào duongCanQuyen() ở api_auth_test.go:\n  %s",
			len(thieu), strings.Join(thieu, "\n  "))
	}
}

// mauDuong đổi giá trị thật trong đường dẫn về dạng mẫu `{}`.
//
// Danh sách kiểm dùng mã có thật (`ord_01J9…`) để gọi được, còn mã nguồn
// dùng mẫu (`{order_id}`). Chuẩn hóa cả hai về một dạng mới so sánh được.
func mauDuong(p string) string {
	phan := strings.Split(p, "/")
	for i, x := range phan {
		if strings.HasPrefix(x, "{") || regexp.MustCompile(`^[a-z]+_[0-9A-Za-z]{20,}$`).MatchString(x) {
			phan[i] = "{}"
		}
	}
	return strings.Join(phan, "/")
}
