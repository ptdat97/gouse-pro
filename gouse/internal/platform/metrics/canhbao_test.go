package metrics_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

	// Quét MỌI nơi khai tên chỉ số, không riêng metrics.go.
	//
	// Giả định "mọi chỉ số nằm trong metrics.go" đã hết đúng từ khi
	// platform/database tự khai collector của nó. Một bài quét chỉ nhìn
	// một tệp sẽ báo thiếu cho chỉ số CÓ THẬT, và sửa nó bằng cách nới
	// điều kiện là đúng thứ làm bài test này vô dụng.
	nguonChiSo := []string{
		"metrics.go",
		"../database/metrics.go",
	}

	phat := map[string]bool{}
	for _, tep := range nguonChiSo {
		nguon, err := os.ReadFile(tep)
		if err != nil {
			t.Fatalf("đọc %s: %v", tep, err)
		}
		for _, m := range regexp.MustCompile(`"(gouse_[a-z_]+)"`).
			FindAllStringSubmatch(string(nguon), -1) {
			phat[m[1]] = true
		}
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
