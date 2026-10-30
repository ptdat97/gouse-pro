package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

// TestMaMauChuanHoaVeCHUHOA.
//
// "#ff0000" và "#FF0000" là MỘT màu. Để hai cách viết cùng tồn tại nghĩa
// là lọc theo mã màu bỏ sót một nửa, và hai biến thể cùng màu trông như
// hai màu khác nhau trên trang.
func TestColorHexNormalizesToUppercase(t *testing.T) {
	v, err := domain.NewVariant(domain.NewVariantParams{
		Attributes: map[string]string{
			domain.AttrColor:    "Đỏ",
			domain.AttrColorHex: "#ff0000",
		},
		Now: types.Now(),
	})
	if err != nil {
		t.Fatalf("NewVariant: %v", err)
	}
	if got := v.ColorHex(); got != "#FF0000" {
		t.Errorf("mã màu = %q, mong #FF0000", got)
	}
}

// TestMaMauSaiDinhDangBiTUCHOI.
//
// Một mã màu hỏng hiện ra thành ô đen hoặc trong suốt trên trang, và
// khách chọn theo thứ NHÌN THẤY — tệ hơn hẳn việc không có ô màu nào.
//
// Dạng rút gọn ba ký tự (#FFF) cũng bị từ chối dù hợp lệ trong CSS: nó
// tạo hai cách viết cho cùng một màu.
func TestMalformedColorHexIsRejected(t *testing.T) {
	for _, colorCode := range []string{
		"FF0000",   // thiếu dấu thăng
		"#FFF",     // dạng rút gọn
		"#GG0000",  // ký tự không phải hex
		"#FF00000", // bảy ký tự
		"đỏ",       // tên màu, không phải mã
	} {
		_, err := domain.NewVariant(domain.NewVariantParams{
			Attributes: map[string]string{
				domain.AttrColor:    "Đỏ",
				domain.AttrColorHex: colorCode,
			},
			Now: types.Now(),
		})
		if !errors.Is(err, domain.ErrInvalidColorHex) {
			t.Errorf("mã %q: lỗi = %v, mong ErrInvalidColorHex", colorCode, err)
		}
	}
}

// TestMaMauLaTUYCHON.
//
// Bắt buộc sẽ chặn mọi sản phẩm hiện có và mọi nhà bán chưa kịp lấy mã
// màu. Thiếu hex thì giao diện hiện ô chữ như cũ.
func TestColorHexIsOptional(t *testing.T) {
	v, err := domain.NewVariant(domain.NewVariantParams{
		Attributes: map[string]string{domain.AttrColor: "Đỏ"},
		Now:        types.Now(),
	})
	if err != nil {
		t.Fatalf("biến thể không có mã màu phải tạo được: %v", err)
	}
	if got := v.ColorHex(); got != "" {
		t.Errorf("mã màu = %q, mong rỗng", got)
	}
}

// TestMaMauKHONGvaoKhoaDinhDanh — bài dễ bỏ sót nhất.
//
// Hai biến thể cùng màu "Đen" mà mã màu khác nhau KHÔNG phải hai biến
// thể: đó là một lần nhập sai. Đưa hex vào khóa định danh sẽ cho cả hai
// cùng tồn tại, và khách thấy hai ô "Đen" cạnh nhau.
//
// Cùng nguyên tắc với `color_family`, thứ đã bị loại khỏi khóa từ trước.
func TestColorHexStaysOutOfTheIdentityKey(t *testing.T) {
	makeVariant := func(hex string) *domain.Variant {
		v, err := domain.NewVariant(domain.NewVariantParams{
			Attributes: map[string]string{
				domain.AttrColor:    "Đen",
				domain.AttrSize:     "M",
				domain.AttrColorHex: hex,
			},
			Now: types.Now(),
		})
		if err != nil {
			t.Fatalf("NewVariant(%s): %v", hex, err)
		}
		return v
	}

	a, b := makeVariant("#000000"), makeVariant("#111111")
	if a.AttributeKey() != b.AttributeKey() {
		t.Errorf("khóa khác nhau (%q vs %q) — mã màu đang được coi là "+
			"thuộc tính phân biệt", a.AttributeKey(), b.AttributeKey())
	}
}
