package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

func chuoi(s string) *string { return &s }

// Sửa được ở DRAFT — kể cả nháp "bị trả về", vì Reject đưa về DRAFT.
func TestSuaNhapDoiDungTruongDuocGui(t *testing.T) {
	p := newTestProduct(t, nil)
	tenCu := p.Name()
	moTaCu := p.Description()

	err := p.SuaNhap(domain.SuaNhapParams{
		MaterialComposition: chuoi("100% lụa"),
	}, testNow)
	if err != nil {
		t.Fatalf("sửa nháp: %v", err)
	}
	if got := p.MaterialComposition(); got != "100% lụa" {
		t.Errorf("chất liệu = %q", got)
	}
	// PATCH: trường KHÔNG gửi thì GIỮ NGUYÊN. Một phép sửa xóa mất mô tả
	// chỉ vì người gọi không gửi lại nó là lỗi tệ nhất của PATCH.
	if p.Name() != tenCu || p.Description() != moTaCu {
		t.Errorf("trường không gửi bị đổi: tên %q→%q, mô tả %q→%q",
			tenCu, p.Name(), moTaCu, p.Description())
	}
}

// ẢNH thay THẾ cả danh sách — vì chỉ nối thêm thì ảnh sai không gỡ được.
func TestSuaNhapAnhThayCaDanhSach(t *testing.T) {
	p := newTestProduct(t, func(n *domain.NewProductParams) {
		n.Images = []string{"https://x/sai.jpg", "https://x/dung.jpg"}
	})

	moi := []string{"https://x/dung.jpg", "https://x/them.jpg"}
	if err := p.SuaNhap(domain.SuaNhapParams{Images: &moi}, testNow); err != nil {
		t.Fatalf("sửa ảnh: %v", err)
	}
	got := p.Images()
	if len(got) != 2 || got[0] != "https://x/dung.jpg" || got[1] != "https://x/them.jpg" {
		t.Errorf("ảnh sau khi sửa = %v — ảnh sai phải GỠ được, và thứ tự "+
			"phải giữ (ảnh đầu là ảnh bìa)", got)
	}
}

// TẤT CẢ HOẶC KHÔNG GÌ.
//
// Tên hợp lệ + loại sản phẩm hỏng trong CÙNG một lần sửa: không được đổi
// tên rồi mới phát hiện loại hỏng. Sản phẩm sửa dở — nửa ý cũ, nửa ý mới —
// là trạng thái không ai biết để sửa tiếp.
func TestSuaNhapHongMotTruongThiKhongDoiGiCa(t *testing.T) {
	p := newTestProduct(t, nil)
	tenCu := p.Name()

	sai := domain.ProductType("KHONG_CO")
	err := p.SuaNhap(domain.SuaNhapParams{
		Name:        chuoi("Tên mới hoàn toàn hợp lệ"),
		ProductType: &sai,
	}, testNow)
	if !errors.Is(err, domain.ErrInvalidProductType) {
		t.Fatalf("mong ErrInvalidProductType, nhận %v", err)
	}
	if p.Name() != tenCu {
		t.Errorf("tên đã bị đổi thành %q dù cả lần sửa bị từ chối", p.Name())
	}
}

// Gửi RỖNG khác với không gửi.
func TestSuaNhapGuiRongLaLoiKhongPhaiBoQua(t *testing.T) {
	for _, ca := range []struct {
		ten string
		in  domain.SuaNhapParams
		loi error
	}{
		{"tên rỗng", domain.SuaNhapParams{Name: chuoi("   ")}, domain.ErrEmptyName},
		{"slug rỗng", domain.SuaNhapParams{Slug: chuoi("")}, domain.ErrEmptySlug},
		{"ảnh rỗng trong danh sách", domain.SuaNhapParams{
			Images: &[]string{"https://x/a.jpg", " "}}, domain.ErrEmptyImageURL},
	} {
		t.Run(ca.ten, func(t *testing.T) {
			p := newTestProduct(t, nil)
			if err := p.SuaNhap(ca.in, testNow); !errors.Is(err, ca.loi) {
				t.Errorf("mong %v, nhận %v", ca.loi, err)
			}
		})
	}
}

// NGOÀI DRAFT thì không sửa được.
//
// PENDING_REVIEW: người duyệt đang xem, sửa lúc này là đổi thứ họ đang
// duyệt ngay dưới tay họ. ACTIVE: khách đang thấy — sửa không qua duyệt lại
// là cửa sau để tráo ảnh sau khi được duyệt.
func TestSuaNhapNgoaiDraftBiTuChoi(t *testing.T) {
	p := newTestProduct(t, nil)
	datDuDeGuiDuyet(t, p)
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("gửi duyệt: %v", err)
	}

	err := p.SuaNhap(domain.SuaNhapParams{Name: chuoi("Đổi sau khi gửi")}, testNow)
	if !errors.Is(err, domain.ErrInvalidStatus) {
		t.Errorf("sửa được sản phẩm đang CHỜ DUYỆT: %v", err)
	}

	if err := p.Approve(testNow); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	err = p.SuaNhap(domain.SuaNhapParams{Name: chuoi("Tráo sau khi duyệt")}, testNow)
	if !errors.Is(err, domain.ErrInvalidStatus) {
		t.Errorf("sửa được sản phẩm ĐANG BÁN mà không qua duyệt lại: %v", err)
	}
}

// Bị trả về → sửa theo lý do → gửi lại: đường đi tự nhiên phải đi được.
func TestNhapBiTraVeSuaDuocRoiGuiLai(t *testing.T) {
	p := newTestProduct(t, nil)
	datDuDeGuiDuyet(t, p)
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("gửi duyệt: %v", err)
	}
	if err := p.Reject("Ảnh bìa bị mờ", testNow); err != nil {
		t.Fatalf("từ chối: %v", err)
	}

	moi := []string{"https://x/bia-net.jpg"}
	if err := p.SuaNhap(domain.SuaNhapParams{Images: &moi}, testNow); err != nil {
		t.Fatalf("sửa nháp bị trả về: %v", err)
	}
	if err := p.SubmitForReview(testNow); err != nil {
		t.Errorf("gửi lại sau khi sửa: %v", err)
	}
}

// datDuDeGuiDuyet bổ sung mọi thứ `CheckReadyForReview` đòi.
func datDuDeGuiDuyet(t *testing.T, p *domain.Product) {
	t.Helper()
	anh := []string{"https://x/a.jpg"}
	if err := p.SuaNhap(domain.SuaNhapParams{
		Description:         chuoi("Mô tả đủ dài"),
		MaterialComposition: chuoi("100% cotton"),
		Images:              &anh,
	}, testNow); err != nil {
		t.Fatalf("bổ sung thông tin: %v", err)
	}
	v, err := domain.NewVariant(domain.NewVariantParams{
		Attributes: map[string]string{"color": "Đen", "size": "M"},
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("dựng biến thể: %v", err)
	}
	if err := p.AddVariant(v, testNow); err != nil {
		t.Fatalf("thêm biến thể: %v", err)
	}
}
