package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

func newTestSKU(t *testing.T, code string) *domain.SKU {
	t.Helper()
	s, err := domain.NewSKU(domain.NewSKUParams{
		Code:       code,
		WeightGram: 320,
		Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewSKU(%q): %v", code, err)
	}
	return s
}

// Mã SKU được người thật gõ và quét ở nhiều nơi. Không chuẩn hóa thì tồn kho
// của cùng một mặt hàng sẽ âm thầm tách làm đôi.
func TestMaSKUDuocChuanHoa(t *testing.T) {
	cases := []struct{ vao, mong string }{
		{"sm-lin-oxf-wht-m", "SM-LIN-OXF-WHT-M"},
		{"  SM-LIN-OXF-WHT-M  ", "SM-LIN-OXF-WHT-M"},
		{"Sm-Lin-Oxf-Wht-M", "SM-LIN-OXF-WHT-M"},
	}
	for _, tc := range cases {
		s := newTestSKU(t, tc.vao)
		if s.Code() != tc.mong {
			t.Errorf("NewSKU(%q).Code() = %q, mong %q", tc.vao, s.Code(), tc.mong)
		}
	}
}

func TestNewSKUTuChoiDuLieuSai(t *testing.T) {
	cases := []struct {
		ten     string
		params  domain.NewSKUParams
		wantErr error
	}{
		{"mã rỗng", domain.NewSKUParams{Code: "   "}, domain.ErrEmptySKUCode},
		{"trọng lượng âm", domain.NewSKUParams{Code: "A-1", WeightGram: -1}, domain.ErrNegativeWeight},
		{
			"kích thước âm",
			domain.NewSKUParams{Code: "A-1", Dimensions: domain.Dimensions{LengthMM: -5}},
			domain.ErrNegativeDim,
		},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			if _, err := domain.NewSKU(tc.params); !errors.Is(err, tc.wantErr) {
				t.Errorf("lỗi = %v, mong %v", err, tc.wantErr)
			}
		})
	}
}

// Thiếu trọng lượng/kích thước thì hãng vận chuyển tính theo mặc định,
// thường cao hơn thực tế — chi phí âm thầm ăn vào biên lợi nhuận.
func TestCanShipCanDuTrongLuongVaKichThuoc(t *testing.T) {
	day := newTestSKU(t, "DU-THONG-TIN")
	if !day.CanShip() {
		t.Error("SKU đủ trọng lượng và kích thước phải giao được")
	}

	thieuCan, err := domain.NewSKU(domain.NewSKUParams{
		Code:       "THIEU-CAN",
		Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewSKU: %v", err)
	}
	if thieuCan.CanShip() {
		t.Error("SKU thiếu trọng lượng không được coi là giao được")
	}

	thieuKichThuoc, err := domain.NewSKU(domain.NewSKUParams{
		Code: "THIEU-KT", WeightGram: 320, Now: testNow,
	})
	if err != nil {
		t.Fatalf("NewSKU: %v", err)
	}
	if thieuKichThuoc.CanShip() {
		t.Error("SKU thiếu kích thước không được coi là giao được")
	}

	// Bổ sung thông tin sau thì phải giao được.
	if err := thieuCan.SetShippingInfo(320, domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40}, testNow); err != nil {
		t.Fatalf("SetShippingInfo: %v", err)
	}
	if !thieuCan.CanShip() {
		t.Error("sau khi bổ sung thông tin, SKU phải giao được")
	}
}

func TestNgungKinhDoanhSKU(t *testing.T) {
	s := newTestSKU(t, "NGUNG-BAN")
	if !s.IsSellable() {
		t.Fatal("SKU mới phải bán được")
	}

	s.Discontinue(testNow)
	if s.IsSellable() {
		t.Error("SKU đã ngừng kinh doanh không được bán")
	}
	// KHÔNG xóa dữ liệu: đơn hàng cũ vẫn trỏ tới SKU này.
	if s.Code() == "" || s.ID().IsZero() {
		t.Error("ngừng kinh doanh không được xóa dữ liệu SKU")
	}
}

// Quy tắc 1 (mục 12) ở phạm vi biến thể. Tính duy nhất toàn hệ thống do
// tầng application và ràng buộc database bảo đảm.
func TestKhongChoTrungMaSKUTrongBienThe(t *testing.T) {
	v := newTestVariant(t, map[string]string{"color": "Trắng", "size": "M"})

	if err := v.AddSKU(newTestSKU(t, "AO-WHT-M"), testNow); err != nil {
		t.Fatalf("AddSKU: %v", err)
	}

	// Khác kiểu chữ vẫn là cùng một mã sau khi chuẩn hóa.
	if err := v.AddSKU(newTestSKU(t, "ao-wht-m"), testNow); !errors.Is(err, domain.ErrDuplicateSKUCode) {
		t.Errorf("lỗi = %v, mong ErrDuplicateSKUCode", err)
	}
	if got := len(v.SKUs()); got != 1 {
		t.Errorf("số SKU = %d, mong 1", got)
	}
}

func TestAddSKUGanVaoBienThe(t *testing.T) {
	v := newTestVariant(t, map[string]string{"color": "Đen", "size": "L"})
	s := newTestSKU(t, "AO-BLK-L")

	if err := v.AddSKU(s, testNow); err != nil {
		t.Fatalf("AddSKU: %v", err)
	}
	if s.VariantID() != v.ID() {
		t.Errorf("variantID = %q, mong %q", s.VariantID(), v.ID())
	}

	got, ok := v.SKUByCode("ao-blk-l")
	if !ok {
		t.Fatal("không tìm thấy SKU theo mã (không phân biệt hoa thường)")
	}
	if got.ID() != s.ID() {
		t.Errorf("tìm sai SKU: %q, mong %q", got.ID(), s.ID())
	}
}

func TestDimensionsThuocTinh(t *testing.T) {
	var rong domain.Dimensions
	if !rong.IsZero() {
		t.Error("kích thước rỗng phải IsZero")
	}

	d := domain.Dimensions{LengthMM: 300, WidthMM: 200, HeightMM: 40}
	if d.IsZero() {
		t.Error("kích thước có giá trị không được IsZero")
	}
	if got, mong := d.VolumeMM3(), 300*200*40; got != mong {
		t.Errorf("thể tích = %d, mong %d", got, mong)
	}
}

func TestSKUCoTienToDung(t *testing.T) {
	s := newTestSKU(t, "TIEN-TO")
	if s.ID().Prefix() != ids.PrefixSKU {
		t.Errorf("tiền tố = %q, mong %q", s.ID().Prefix(), ids.PrefixSKU)
	}
}
