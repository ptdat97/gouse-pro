package metrics_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// duongDeploy trỏ tới thư mục cấu hình Prometheus từ gói này.
func duongDeploy(t *testing.T, ten string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "deploy", "prometheus", ten)
}

func doc(t *testing.T, ten string) string {
	t.Helper()
	b, err := os.ReadFile(duongDeploy(t, ten))
	if err != nil {
		t.Fatalf("đọc %s: %v", ten, err)
	}
	return string(b)
}

// TestNhanJobTrongCanhBaoKhopCauHinhThuThap.
//
// # Vì sao bài này tồn tại
//
// Nhãn `job` KHÔNG do ứng dụng phát ra — nó do cấu hình thu thập của
// Prometheus đặt. Biểu thức `up{job="gouse-worker"} == 0` chỉ đúng khi
// scrape config đặt đúng tên ấy.
//
// Đổi tên job trong prometheus.yml thành thứ khác thì:
//
//	promtool check config   → VẪN xanh
//	promtool check rules    → VẪN xanh
//	promtool test rules     → VẪN xanh (bài test tự nạp chuỗi có sẵn nhãn)
//
// và cảnh báo "worker chết" im lặng vĩnh viễn. Đã kiểm bằng cách phá: đổi
// `job_name: gouse-worker` thành `job_name: worker` thì cả ba lệnh trên
// đều báo thành công.
//
// Không công cụ nào của Prometheus bắt được, vì không công cụ nào nối hai
// file đó lại với nhau. Bài này làm đúng việc ấy.
func TestNhanJobTrongCanhBaoKhopCauHinhThuThap(t *testing.T) {
	canhBao := doc(t, "alerts.yml")
	cauHinh := doc(t, "prometheus.yml")

	// Tên job mà cấu hình thu thập ĐẶT RA.
	dat := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*-?\s*job_name:\s*(\S+)`).
		FindAllStringSubmatch(cauHinh, -1) {
		dat[m[1]] = true
	}
	if len(dat) == 0 {
		t.Fatal("prometheus.yml không khai job_name nào")
	}

	// Tên job mà cảnh báo LỌC THEO.
	loc := map[string]bool{}
	for _, m := range regexp.MustCompile(`job\s*=\s*"([^"]+)"`).
		FindAllStringSubmatch(canhBao, -1) {
		loc[m[1]] = true
	}

	var thieu []string
	for j := range loc {
		if !dat[j] {
			thieu = append(thieu, j)
		}
	}
	sort.Strings(thieu)

	if len(thieu) > 0 {
		t.Errorf("cảnh báo lọc theo job %q nhưng prometheus.yml không đặt tên đó "+
			"(đang có: %v) — cảnh báo sẽ KHÔNG BAO GIỜ kêu", thieu, tenSapXep(dat))
	}
}

// TestMoiChiSoTrongCanhBaoDeuCoTrongCode.
//
// Một biểu thức trỏ vào chỉ số không tồn tại vẫn hợp lệ với promtool: nó
// chạy, không khớp dòng nào, và không kêu. Đổi tên một chỉ số trong code
// mà quên sửa cảnh báo là cách dễ nhất để mất giám sát trong im lặng.
func TestMoiChiSoTrongCanhBaoDeuCoTrongCode(t *testing.T) {
	canhBao := doc(t, "alerts.yml")

	// Quét CẢ CÂY internal/ để tìm tên chỉ số.
	//
	// Bản trước liệt kê tay từng tệp, và trong một buổi tôi phải sửa danh
	// sách đó HAI lần vì thêm tệp chỉ số mới. Danh sách thủ công ở đây
	// không sai một cách âm thầm — bài test kêu ngay — nhưng nó biến việc
	// thêm chỉ số thành việc phải nhớ sửa một tệp không liên quan, và
	// người sửa dễ chọn cách nhanh hơn: nới điều kiện cho qua.
	phat := map[string]bool{}
	tenChiSo := regexp.MustCompile(`"(gouse_[a-z_]+)"`)
	err := filepath.WalkDir("..", func(duong string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(duong, ".go") ||
			strings.HasSuffix(duong, "_test.go") {
			return nil
		}
		nguon, err := os.ReadFile(duong)
		if err != nil {
			return err
		}
		for _, m := range tenChiSo.FindAllStringSubmatch(string(nguon), -1) {
			phat[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("quét mã nguồn: %v", err)
	}

	// Chống xanh rỗng: quét hỏng (sai đường dẫn, regexp lệch) sẽ cho tập
	// RỖNG, và khi đó mọi chỉ số đều bị báo thiếu — dễ thấy. Nhưng quét ra
	// một tập nhỏ bất thường thì không ai để ý, nên khẳng định luôn.
	if len(phat) < 10 {
		t.Fatalf("chỉ tìm thấy %d tên chỉ số trong internal/ — "+
			"bộ quét nhiều khả năng đang hỏng", len(phat))
	}

	for _, m := range regexp.MustCompile(`\b(gouse_[a-z_]+)`).
		FindAllStringSubmatch(canhBao, -1) {
		ten := m[1]
		if phat[ten] {
			continue
		}
		// Histogram sinh thêm _count, _sum, _bucket. Cắt hậu tố rồi thử lại.
		goc := regexp.MustCompile(`_(count|sum|bucket)$`).ReplaceAllString(ten, "")
		if phat[goc] {
			continue
		}
		t.Errorf("cảnh báo dùng chỉ số %q mà code không phát — "+
			"biểu thức sẽ không bao giờ khớp dòng nào", ten)
	}
}

func tenSapXep(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
