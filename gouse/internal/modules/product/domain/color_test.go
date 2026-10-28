package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

func TestInferColorFamily(t *testing.T) {
	byName := map[string]domain.ColorFamily{
		"Trắng":        domain.ColorWhite,
		"Trắng ngà":    domain.ColorWhite,
		"Đen":          domain.ColorBlack,
		"Xanh navy":    domain.ColorBlue,
		"Xanh dương":   domain.ColorBlue,
		"Denim wash":   domain.ColorBlue,
		"Đỏ đô":        domain.ColorRed,
		"Hồng pastel":  domain.ColorPink,
		"Nâu bò":       domain.ColorBrown,
		"Be":           domain.ColorBeige,
		"":             domain.ColorOther,
		"Họa tiết hoa": domain.ColorOther,
	}
	for name, expected := range byName {
		if got := domain.InferColorFamily(name); got != expected {
			t.Errorf("InferColorFamily(%q) = %q, mong %q", name, got, expected)
		}
	}
}

// TestGreenDoesNotFallIntoTheBlueFamily — bẫy của phép so khớp chuỗi con.
//
// "Xanh lá cây" chứa cả "xanh lá" lẫn "xanh". Không xét từ ghép trước thì
// mọi màu xanh lá bị xếp vào nhóm xanh dương, và bộ lọc trả về sai hàng —
// thứ khách nhận ra ngay còn hệ thống thì không.
func TestGreenDoesNotFallIntoTheBlueFamily(t *testing.T) {
	for _, name := range []string{"Xanh lá", "Xanh lá cây", "Xanh la ma"} {
		if got := domain.InferColorFamily(name); got != domain.ColorGreen {
			t.Errorf("InferColorFamily(%q) = %q, mong %q — từ ghép phải xét trước",
				name, got, domain.ColorGreen)
		}
	}
}
