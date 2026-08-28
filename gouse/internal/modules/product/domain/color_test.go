package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

func TestSuyRaNhomMau(t *testing.T) {
	cac := map[string]domain.NhomMau{
		"Trắng":        domain.MauTrang,
		"Trắng ngà":    domain.MauTrang,
		"Đen":          domain.MauDen,
		"Xanh navy":    domain.MauXanh,
		"Xanh dương":   domain.MauXanh,
		"Denim wash":   domain.MauXanh,
		"Đỏ đô":        domain.MauDo,
		"Hồng pastel":  domain.MauHong,
		"Nâu bò":       domain.MauNau,
		"Be":           domain.MauBe,
		"":             domain.MauKhac,
		"Họa tiết hoa": domain.MauKhac,
	}
	for ten, mong := range cac {
		if got := domain.SuyRaNhomMau(ten); got != mong {
			t.Errorf("SuyRaNhomMau(%q) = %q, mong %q", ten, got, mong)
		}
	}
}

// TestXanhLaKhongRoiVaoNhomXanhDuong — bẫy của phép so khớp chuỗi con.
//
// "Xanh lá cây" chứa cả "xanh lá" lẫn "xanh". Không xét từ ghép trước thì
// mọi màu xanh lá bị xếp vào nhóm xanh dương, và bộ lọc trả về sai hàng —
// thứ khách nhận ra ngay còn hệ thống thì không.
func TestXanhLaKhongRoiVaoNhomXanhDuong(t *testing.T) {
	for _, ten := range []string{"Xanh lá", "Xanh lá cây", "Xanh la ma"} {
		if got := domain.SuyRaNhomMau(ten); got != domain.MauXanhLa {
			t.Errorf("SuyRaNhomMau(%q) = %q, mong %q — từ ghép phải xét trước",
				ten, got, domain.MauXanhLa)
		}
	}
}
