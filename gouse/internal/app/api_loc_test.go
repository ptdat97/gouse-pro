package app

import (
	"context"
	"net/http"
	"testing"
)

// TestFilteringTheCatalogBySizeAndColor.
//
// # Vì sao đây là tính năng cốt lõi, không phải tiện ích
//
// Khách chỉ mặc vừa một hai size. Danh mục không lọc được size bắt họ mở
// từng sản phẩm để biết có mua được không — và họ bỏ đi trước khi mở tới
// cái thứ năm.
//
// Đặc tả có hai tham số này từ đầu (`listProducts`), nhưng chỉ ba bộ lọc
// theo mã được cài. Bài này khóa lại phần còn thiếu.
func TestFilteringTheCatalogBySizeAndColor(t *testing.T) {
	a := newAPITest(t)

	all := a.countProducts(t, "")
	if all == 0 {
		t.Skip("danh mục trống")
	}

	// Tìm một size và một nhóm màu CÓ THẬT trong dữ liệu.
	size, colorFamily := a.sampleSizeAndColorFamily(t)
	if size == "" || colorFamily == "" {
		t.Skip("dữ liệu mẫu không có biến thể nào mang size và nhóm màu")
	}

	// --- lọc theo size ---
	bySize := a.countProducts(t, "size="+size)
	if bySize == 0 {
		t.Errorf("lọc size=%s trả 0 sản phẩm dù có biến thể mang size đó", size)
	}
	if bySize > all {
		t.Errorf("lọc size=%s trả %d, nhiều hơn tổng %d — bộ lọc nhân đôi "+
			"sản phẩm có nhiều biến thể khớp", size, bySize, all)
	}

	// Size KHÔNG tồn tại phải trả rỗng, không phải trả tất cả.
	if n := a.countProducts(t, "size=SIZE-KHONG-CO-THAT"); n != 0 {
		t.Errorf("lọc size không tồn tại trả %d sản phẩm, cần 0 — "+
			"điều kiện lọc bị bỏ qua", n)
	}

	// --- lọc theo nhóm màu ---
	byPattern := a.countProducts(t, "color="+colorFamily)
	if byPattern == 0 {
		t.Errorf("lọc color=%s trả 0 sản phẩm dù có biến thể thuộc nhóm đó", colorFamily)
	}
	if n := a.countProducts(t, "color=KHONGCOMAUNAY"); n != 0 {
		t.Errorf("lọc nhóm màu không tồn tại trả %d, cần 0", n)
	}

	// --- nhiều giá trị: khớp BẤT KỲ, không phải khớp TẤT CẢ ---
	many := a.countProducts(t, "size="+size+",SIZE-KHONG-CO-THAT")
	if many != bySize {
		t.Errorf("lọc nhiều size trả %d, cần %d — nhiều giá trị phải khớp "+
			"BẤT KỲ, không phải khớp tất cả", many, bySize)
	}
}

// countProducts đếm sản phẩm trả về với một chuỗi truy vấn.
func (a *apiTest) countProducts(t *testing.T, query string) int {
	t.Helper()
	route := "/api/v1/products?limit=100"
	if query != "" {
		route += "&" + query
	}
	res := a.call(http.MethodGet, route, nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d — %s", route, res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	return len(ds)
}

// sampleSizeAndColorFamily lấy một size và một nhóm màu có thật trong database.
func (a *apiTest) sampleSizeAndColorFamily(t *testing.T) (string, string) {
	t.Helper()
	var size, group string
	err := a.db.Pool().QueryRow(context.Background(), `
		SELECT v.attributes->>'size', v.attributes->>'color_family'
		  FROM variant v
		  JOIN product p ON p.id = v.product_id
		 WHERE p.status = 'ACTIVE'
		   AND v.attributes ? 'size'
		   AND v.attributes ? 'color_family'
		 LIMIT 1`).Scan(&size, &group)
	if err != nil {
		return "", ""
	}
	return size, group
}
