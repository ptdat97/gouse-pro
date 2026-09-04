package app

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// TestMoiDuongDanhSachChanTranTrang.
//
// # Vì sao phải chặn CẢ HAI đầu
//
// Hàm `limitOr` ở tầng kho lưu trữ chỉ xử lý cận DƯỚI: nó thay số ≤ 0 bằng
// mặc định và để nguyên mọi số lớn. Nên tầng HTTP là chỗ duy nhất chặn
// được cận trên, và quên nó thì `limit=100000000` đi thẳng vào câu SQL.
//
// Hai endpoint quản trị (`/admin/orders`, `/admin/sellers`) đã quên đúng
// chỗ đó, trong khi những đường khác cùng dự án đều có trần.
//
// Chúng chỉ dành cho nhân viên nên đây không phải cửa cho người ngoài —
// nhưng một lần gõ nhầm hay một script sai là đủ làm sập API bằng cách
// kéo cả bảng vào bộ nhớ, và không có gì cản.
func TestMoiDuongDanhSachChanTranTrang(t *testing.T) {
	a := newAPITest(t)

	duong := []struct {
		path   string
		vaiTro string
	}{
		{"/api/v1/admin/orders", identity.RoleOpsSupport},
		{"/api/v1/admin/sellers", identity.RoleOpsMerchandising},
		{"/api/v1/seller/offers", identity.RoleSellerOwner},
		{"/api/v1/seller/fulfillment-orders", identity.RoleSellerOwner},
		{"/api/v1/admin/audit-log", identity.RoleAdmin},
	}

	for _, d := range duong {
		t.Run(d.path, func(t *testing.T) {
			tok := a.taoTaiKhoanVaiTro(t, d.vaiTro)
			h := map[string]string{"Authorization": "Bearer " + tok}

			// Ngay TRÊN trần đúng một bậc: bắt được cả lỗi lệch một
			// đơn vị, không chỉ con số lố bịch.
			got := a.call(http.MethodGet, d.path+"?limit=101", nil, h)
			if got.code != http.StatusBadRequest {
				t.Errorf("limit=101 được chấp nhận: HTTP %d — đặc tả khai "+
					"tối đa 100: %s", got.code, got.raw)
			}

			// Trần: một con số không ai gõ có chủ ý.
			got = a.call(http.MethodGet, d.path+"?limit=100000000", nil, h)
			if got.code != http.StatusBadRequest {
				t.Errorf("limit=100000000 được chấp nhận: HTTP %d — "+
					"câu SQL sẽ kéo cả bảng vào bộ nhớ: %s",
					got.code, got.raw)
			}

			// Vẫn phải nhận giá trị hợp lý, kể cả ĐÚNG trần.
			if g := a.call(http.MethodGet, d.path+"?limit=100", nil, h); g.code == http.StatusBadRequest {
				t.Errorf("limit=100 (đúng trần) bị từ chối: %s", g.raw)
			}
			got = a.call(http.MethodGet, d.path+"?limit=20", nil, h)
			if got.code == http.StatusBadRequest {
				t.Errorf("limit=20 bị từ chối: %s", got.raw)
			}
		})
	}
}

// TestMoiChoDocLimitDeuCoTran quét MÃ NGUỒN, không gọi endpoint.
//
// Bài trên chỉ kiểm những đường có trong danh sách của nó. Đường danh sách
// MỚI thêm sẽ không nằm trong danh sách đó — và đường không ai nghĩ tới
// chính là đường bị quên.
//
// Bài này bắt ở mức mã: chỗ nào đọc `limit` từ query thì phải có so sánh
// cận trên ngay gần đó.
func TestMoiChoDocLimitDeuCoTran(t *testing.T) {
	var thieu []string

	err := filepath.WalkDir("..", func(duong string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(duong, ".go") ||
			strings.HasSuffix(duong, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(duong)
		if err != nil {
			return err
		}
		nguon := string(b)

		// Quét theo DÒNG, không theo cửa sổ byte.
		//
		// Bản đầu cắt 400 byte sau chỗ đọc. Không đủ: chú thích tiếng Việt
		// mỗi dấu tốn 2–3 byte, nên cửa sổ ấy dừng giữa phần giải thích và
		// không bao giờ tới dòng kiểm tham số. Nó báo thiếu cho cả bốn tệp
		// VỪA được sửa.
		dong := strings.Split(nguon, "\n")
		for k, d := range dong {
			if !strings.Contains(d, `Get("limit")`) {
				continue
			}
			het := k + 25
			if het > len(dong) {
				het = len(dong)
			}
			doan := strings.Join(dong[k:het], "\n")

			// Cần một so sánh cận TRÊN. `n < 1` là cận dưới, không tính.
			if !strings.Contains(doan, "n > max") &&
				!strings.Contains(doan, "> 100") &&
				!strings.Contains(doan, "maxLimit") {
				thieu = append(thieu, duong)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("quét mã nguồn: %v", err)
	}

	if len(thieu) > 0 {
		t.Errorf("%d chỗ đọc limit KHÔNG chặn cận trên — `limitOr` ở tầng "+
			"kho lưu trữ chỉ xử lý cận dưới, nên tầng HTTP là chỗ duy nhất "+
			"chặn được:\n  %s", len(thieu), strings.Join(thieu, "\n  "))
	}
}
