package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// registeredPaths quét mã nguồn tìm MỌI route dưới /api/v1/admin/ và
// /api/v1/seller/.
//
// Quét nguồn thay vì hỏi mux vì `http.ServeMux` không cho liệt kê pattern
// đã đăng ký.
var registeredPaths = regexp.MustCompile(
	`mux\.Handle(?:Func)?\("([A-Z]+) (/api/v1/(?:admin|seller)/[^"]*)"`)

// TestEveryAuthenticatedPathIsChecked biến "phải nhớ" thành "test sẽ báo".
//
// # Vấn đề nó giải quyết
//
// `TestAdminPathsBlockTheUnprivileged` chỉ mạnh bằng danh sách nó duyệt.
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
func TestEveryAuthenticatedPathIsChecked(t *testing.T) {
	checked := map[string]bool{}
	for _, d := range pathsNeedingAuth() {
		checked[d.method+" "+pathPattern(d.path)] = true
	}

	// `/api/v1/admin/me` cố ý chỉ cần đăng nhập — có test riêng.
	checked["GET /api/v1/admin/me"] = true

	roots := []string{"internal/app", "internal/modules"}
	var missingImg []string

	for _, g := range roots {
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
			for _, m := range registeredPaths.FindAllStringSubmatch(string(b), -1) {
				key := m[1] + " " + pathPattern(m[2])
				if !checked[key] {
					missingImg = append(missingImg, key+"  ("+filepath.Base(path)+")")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("quét %s: %v", g, err)
		}
	}

	if len(missingImg) > 0 {
		t.Errorf("%d đường cần quyền KHÔNG có trong danh sách kiểm phân quyền "+
			"— thêm vào pathsNeedingAuth() ở api_auth_test.go:\n  %s",
			len(missingImg), strings.Join(missingImg, "\n  "))
	}
}

// pathPattern đổi giá trị thật trong đường dẫn về dạng mẫu `{}`.
//
// Danh sách kiểm dùng mã có thật (`ord_01J9…`) để gọi được, còn mã nguồn
// dùng mẫu (`{order_id}`). Chuẩn hóa cả hai về một dạng mới so sánh được.
var (
	// reULID khớp một định danh có tiền tố: `ord_01J9X...`.
	reULID = regexp.MustCompile(`^[a-z]+_[0-9A-Za-z]{20,}$`)

	// configKey khớp khóa tham số vận hành: `fulfillment.min_sample_size`.
	//
	// Dấu CHẤM là dấu hiệu đủ đặc thù: không đoạn đường nào khác trong API
	// này chứa dấu chấm, còn mọi khóa cấu hình đều có dạng `module.tên`.
	configKey = regexp.MustCompile(`^[a-z_]+\.[a-z_]+$`)
)

func pathPattern(p string) string {
	parts := strings.Split(p, "/")
	for i, x := range parts {
		if strings.HasPrefix(x, "{") || reULID.MatchString(x) ||
			configKey.MatchString(x) {
			parts[i] = "{}"
		}
	}
	return strings.Join(parts, "/")
}
