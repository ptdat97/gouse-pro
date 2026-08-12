package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

// Không chuẩn hóa khóa thuộc tính thì "Color" và "color" thành hai thuộc
// tính khác nhau, và bộ lọc theo màu sẽ bỏ sót hàng.
func TestThuocTinhDuocChuanHoa(t *testing.T) {
	v := newTestVariant(t, map[string]string{
		"  COLOR  ": "  Trắng  ",
		"Size":      "M",
	})

	if got := v.Color(); got != "Trắng" {
		t.Errorf("Color() = %q, mong %q", got, "Trắng")
	}
	if got := v.Size(); got != "M" {
		t.Errorf("Size() = %q, mong %q", got, "M")
	}

	// Tra cứu không phân biệt hoa thường.
	if _, ok := v.Attribute("COLOR"); !ok {
		t.Error("không tra được thuộc tính bằng chữ hoa")
	}
}

func TestNewVariantTuChoiDuLieuSai(t *testing.T) {
	if _, err := domain.NewVariant(domain.NewVariantParams{Now: testNow}); !errors.Is(err, domain.ErrNoAttributes) {
		t.Errorf("lỗi = %v, mong ErrNoAttributes", err)
	}

	_, err := domain.NewVariant(domain.NewVariantParams{
		Attributes: map[string]string{"color": "   "},
		Now:        testNow,
	})
	if err == nil {
		t.Error("mong lỗi khi giá trị thuộc tính rỗng")
	}
}

// AttributeKey phải ỔN ĐỊNH: map trong Go duyệt theo thứ tự ngẫu nhiên,
// nếu không sắp xếp khóa thì việc phát hiện trùng lặp sẽ chập chờn.
func TestAttributeKeyOnDinhQuaNhieuLanGoi(t *testing.T) {
	v := newTestVariant(t, map[string]string{
		"color":    "Trắng",
		"size":     "M",
		"material": "Linen",
		"pattern":  "Trơn",
		"fit":      "Suông",
	})

	dau := v.AttributeKey()
	for i := 0; i < 100; i++ {
		if got := v.AttributeKey(); got != dau {
			t.Fatalf("AttributeKey đổi giữa các lần gọi: %q rồi %q", dau, got)
		}
	}
}

func TestSameAttributesAsKhongPhuThuocThuTuVaKieuChu(t *testing.T) {
	v1 := newTestVariant(t, map[string]string{"color": "Trắng", "size": "M"})
	v2 := newTestVariant(t, map[string]string{"size": "M", "COLOR": "trắng"})
	v3 := newTestVariant(t, map[string]string{"color": "Trắng", "size": "L"})

	if !v1.SameAttributesAs(v2) {
		t.Error("cùng tổ hợp khác thứ tự/kiểu chữ phải được coi là trùng")
	}
	if v1.SameAttributesAs(v3) {
		t.Error("khác size không được coi là trùng")
	}
	if v1.SameAttributesAs(nil) {
		t.Error("so với nil phải trả false")
	}
}

// Map là kiểu tham chiếu — trả thẳng sẽ cho bên ngoài sửa trạng thái nội bộ.
func TestAttributesTraVeBanSao(t *testing.T) {
	v := newTestVariant(t, map[string]string{"color": "Trắng", "size": "M"})

	attrs := v.Attributes()
	attrs["color"] = "ĐÃ BỊ SỬA"
	attrs["size"] = "XXL"

	if v.Color() != "Trắng" {
		t.Errorf("Color() = %q, sửa được thuộc tính từ bên ngoài", v.Color())
	}
	if v.Size() != "M" {
		t.Errorf("Size() = %q, sửa được thuộc tính từ bên ngoài", v.Size())
	}
}

func TestAddVariantGanVaoSanPham(t *testing.T) {
	p := newTestProduct(t, nil)
	v := newTestVariant(t, map[string]string{"color": "Xanh", "size": "S"})

	if err := p.AddVariant(v, testNow); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}
	if v.ProductID() != p.ID() {
		t.Errorf("productID = %q, mong %q", v.ProductID(), p.ID())
	}

	got, ok := p.VariantByID(v.ID())
	if !ok || got.ID() != v.ID() {
		t.Error("không tìm được biến thể vừa thêm theo id")
	}
}

func TestSanPhamGomSKUCuaMoiBienThe(t *testing.T) {
	p := newTestProduct(t, nil)

	v1 := newTestVariant(t, map[string]string{"color": "Trắng", "size": "M"})
	if err := v1.AddSKU(newTestSKU(t, "AO-WHT-M"), testNow); err != nil {
		t.Fatalf("AddSKU: %v", err)
	}
	v2 := newTestVariant(t, map[string]string{"color": "Trắng", "size": "L"})
	if err := v2.AddSKU(newTestSKU(t, "AO-WHT-L"), testNow); err != nil {
		t.Fatalf("AddSKU: %v", err)
	}

	for _, v := range []*domain.Variant{v1, v2} {
		if err := p.AddVariant(v, testNow); err != nil {
			t.Fatalf("AddVariant: %v", err)
		}
	}

	if got := len(p.SKUs()); got != 2 {
		t.Errorf("số SKU của sản phẩm = %d, mong 2", got)
	}
}

func TestVariantCoTienToDung(t *testing.T) {
	v := newTestVariant(t, map[string]string{"color": "Đỏ"})
	if v.ID().Prefix() != ids.PrefixVariant {
		t.Errorf("tiền tố = %q, mong %q", v.ID().Prefix(), ids.PrefixVariant)
	}
}

func TestTamNgungBienThe(t *testing.T) {
	v := newTestVariant(t, map[string]string{"color": "Đỏ", "size": "M"})
	if v.Status() != domain.StatusActive {
		t.Fatalf("trạng thái = %q, mong ACTIVE", v.Status())
	}

	v.Deactivate(testNow)
	if v.Status() != domain.StatusInactive {
		t.Errorf("trạng thái = %q, mong INACTIVE", v.Status())
	}
}
