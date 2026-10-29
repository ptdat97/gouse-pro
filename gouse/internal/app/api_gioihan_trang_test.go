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

// TestEveryListPathEnforcesThePageCap.
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
func TestEveryListPathEnforcesThePageCap(t *testing.T) {
	a := newAPITest(t)

	route := []struct {
		path string
		role string
	}{
		{"/api/v1/admin/orders", identity.RoleOpsSupport},
		{"/api/v1/admin/sellers", identity.RoleOpsMerchandising},
		{"/api/v1/seller/offers", identity.RoleSellerOwner},
		{"/api/v1/seller/fulfillment-orders", identity.RoleSellerOwner},
		{"/api/v1/admin/audit-log", identity.RoleAdmin},
	}

	for _, d := range route {
		t.Run(d.path, func(t *testing.T) {
			tok := a.createAccountWithRole(t, d.role)
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

// TestEveryPlaceThatReadsLimitHasACap quét MÃ NGUỒN, không gọi endpoint.
//
// Bài trên chỉ kiểm những đường có trong danh sách của nó. Đường danh sách
// MỚI thêm sẽ không nằm trong danh sách đó — và đường không ai nghĩ tới
// chính là đường bị quên.
//
// Bài này bắt ở mức mã: chỗ nào đọc `limit` từ query thì phải có so sánh
// cận trên ngay gần đó.
func TestEveryPlaceThatReadsLimitHasACap(t *testing.T) {
	var missingImg []string

	err := filepath.WalkDir("..", func(route string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(route, ".go") ||
			strings.HasSuffix(route, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(route)
		if err != nil {
			return err
		}
		source := string(b)

		// Quét theo DÒNG, không theo cửa sổ byte.
		//
		// Bản đầu cắt 400 byte sau chỗ đọc. Không đủ: chú thích tiếng Việt
		// mỗi dấu tốn 2–3 byte, nên cửa sổ ấy dừng giữa phần giải thích và
		// không bao giờ tới dòng kiểm tham số. Nó báo thiếu cho cả bốn tệp
		// VỪA được sửa.
		line := strings.Split(source, "\n")
		for k, d := range line {
			if !strings.Contains(d, `Get("limit")`) {
				continue
			}
			end := k + 25
			if end > len(line) {
				end = len(line)
			}
			chunk := strings.Join(line[k:end], "\n")

			// Cần một so sánh cận TRÊN. `n < 1` là cận dưới, không tính.
			if !strings.Contains(chunk, "n > max") &&
				!strings.Contains(chunk, "> 100") &&
				!strings.Contains(chunk, "maxLimit") {
				missingImg = append(missingImg, route)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("quét mã nguồn: %v", err)
	}

	if len(missingImg) > 0 {
		t.Errorf("%d chỗ đọc limit KHÔNG chặn cận trên — `limitOr` ở tầng "+
			"kho lưu trữ chỉ xử lý cận dưới, nên tầng HTTP là chỗ duy nhất "+
			"chặn được:\n  %s", len(missingImg), strings.Join(missingImg, "\n  "))
	}
}
