package app

import (
	"context"
	"net/http"
	"testing"
)

// TestLocDanhMucTheoSizeVaMau.
//
// # Vì sao đây là tính năng cốt lõi, không phải tiện ích
//
// Khách chỉ mặc vừa một hai size. Danh mục không lọc được size bắt họ mở
// từng sản phẩm để biết có mua được không — và họ bỏ đi trước khi mở tới
// cái thứ năm.
//
// Đặc tả có hai tham số này từ đầu (`listProducts`), nhưng chỉ ba bộ lọc
// theo mã được cài. Bài này khóa lại phần còn thiếu.
func TestLocDanhMucTheoSizeVaMau(t *testing.T) {
	a := newAPITest(t)

	tatCa := a.demSanPham(t, "")
	if tatCa == 0 {
		t.Skip("danh mục trống")
	}

	// Tìm một size và một nhóm màu CÓ THẬT trong dữ liệu.
	size, nhomMau := a.mauSizeVaNhomMau(t)
	if size == "" || nhomMau == "" {
		t.Skip("dữ liệu mẫu không có biến thể nào mang size và nhóm màu")
	}

	// --- lọc theo size ---
	theoSize := a.demSanPham(t, "size="+size)
	if theoSize == 0 {
		t.Errorf("lọc size=%s trả 0 sản phẩm dù có biến thể mang size đó", size)
	}
	if theoSize > tatCa {
		t.Errorf("lọc size=%s trả %d, nhiều hơn tổng %d — bộ lọc nhân đôi "+
			"sản phẩm có nhiều biến thể khớp", size, theoSize, tatCa)
	}

	// Size KHÔNG tồn tại phải trả rỗng, không phải trả tất cả.
	if n := a.demSanPham(t, "size=SIZE-KHONG-CO-THAT"); n != 0 {
		t.Errorf("lọc size không tồn tại trả %d sản phẩm, cần 0 — "+
			"điều kiện lọc bị bỏ qua", n)
	}

	// --- lọc theo nhóm màu ---
	theoMau := a.demSanPham(t, "color="+nhomMau)
	if theoMau == 0 {
		t.Errorf("lọc color=%s trả 0 sản phẩm dù có biến thể thuộc nhóm đó", nhomMau)
	}
	if n := a.demSanPham(t, "color=KHONGCOMAUNAY"); n != 0 {
		t.Errorf("lọc nhóm màu không tồn tại trả %d, cần 0", n)
	}

	// --- nhiều giá trị: khớp BẤT KỲ, không phải khớp TẤT CẢ ---
	nhieu := a.demSanPham(t, "size="+size+",SIZE-KHONG-CO-THAT")
	if nhieu != theoSize {
		t.Errorf("lọc nhiều size trả %d, cần %d — nhiều giá trị phải khớp "+
			"BẤT KỲ, không phải khớp tất cả", nhieu, theoSize)
	}
}

// demSanPham đếm sản phẩm trả về với một chuỗi truy vấn.
func (a *apiTest) demSanPham(t *testing.T, query string) int {
	t.Helper()
	duong := "/api/v1/products?limit=100"
	if query != "" {
		duong += "&" + query
	}
	res := a.call(http.MethodGet, duong, nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d — %s", duong, res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	return len(ds)
}

// mauSizeVaNhomMau lấy một size và một nhóm màu có thật trong database.
func (a *apiTest) mauSizeVaNhomMau(t *testing.T) (string, string) {
	t.Helper()
	var size, nhom string
	err := a.db.Pool().QueryRow(context.Background(), `
		SELECT v.attributes->>'size', v.attributes->>'color_family'
		  FROM variant v
		  JOIN product p ON p.id = v.product_id
		 WHERE p.status = 'ACTIVE'
		   AND v.attributes ? 'size'
		   AND v.attributes ? 'color_family'
		 LIMIT 1`).Scan(&size, &nhom)
	if err != nil {
		return "", ""
	}
	return size, nhom
}
