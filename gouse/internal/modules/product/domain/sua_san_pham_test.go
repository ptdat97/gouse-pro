package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

func chuoi(s string) *string { return &s }

// Sửa được ở DRAFT — kể cả nháp "bị trả về", vì Reject đưa về DRAFT.
func TestSuaDoiDungTruongDuocGui(t *testing.T) {
	p := newTestProduct(t, nil)
	tenCu := p.Name()
	moTaCu := p.Description()

	err := p.Sua(domain.SuaParams{
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
func TestSuaAnhThayCaDanhSach(t *testing.T) {
	p := newTestProduct(t, func(n *domain.NewProductParams) {
		n.Images = []string{"https://x/sai.jpg", "https://x/dung.jpg"}
	})

	moi := []string{"https://x/dung.jpg", "https://x/them.jpg"}
	if err := p.Sua(domain.SuaParams{Images: &moi}, testNow); err != nil {
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
func TestSuaHongMotTruongThiKhongDoiGiCa(t *testing.T) {
	p := newTestProduct(t, nil)
	tenCu := p.Name()

	sai := domain.ProductType("KHONG_CO")
	err := p.Sua(domain.SuaParams{
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
func TestSuaGuiRongLaLoiKhongPhaiBoQua(t *testing.T) {
	for _, ca := range []struct {
		ten string
		in  domain.SuaParams
		loi error
	}{
		{"tên rỗng", domain.SuaParams{Name: chuoi("   ")}, domain.ErrEmptyName},
		{"slug rỗng", domain.SuaParams{Slug: chuoi("")}, domain.ErrEmptySlug},
		{"ảnh rỗng trong danh sách", domain.SuaParams{
			Images: &[]string{"https://x/a.jpg", " "}}, domain.ErrEmptyImageURL},
	} {
		t.Run(ca.ten, func(t *testing.T) {
			p := newTestProduct(t, nil)
			if err := p.Sua(ca.in, testNow); !errors.Is(err, ca.loi) {
				t.Errorf("mong %v, nhận %v", ca.loi, err)
			}
		})
	}
}

// PENDING_REVIEW và INACTIVE thì KHÔNG sửa được.
//
// PENDING_REVIEW: người duyệt đang xem, sửa lúc này là đổi thứ họ đang
// duyệt ngay dưới tay họ. INACTIVE: bật lại được mà không cần duyệt vì nội
// dung không đổi — cho sửa ở đó sẽ phá đúng giả định ấy.
func TestSuaOTrangThaiKhongChoPhepBiTuChoi(t *testing.T) {
	t.Run("đang chờ duyệt", func(t *testing.T) {
		p := newTestProduct(t, nil)
		datDuDeGuiDuyet(t, p)
		if err := p.SubmitForReview(testNow); err != nil {
			t.Fatalf("gửi duyệt: %v", err)
		}
		err := p.Sua(domain.SuaParams{Name: chuoi("Đổi sau khi gửi")}, testNow)
		if !errors.Is(err, domain.ErrInvalidStatus) {
			t.Errorf("sửa được sản phẩm đang CHỜ DUYỆT: %v", err)
		}
	})

	t.Run("tạm ngừng bán", func(t *testing.T) {
		p := dungSanPhamDangBan(t)
		if err := p.Deactivate(testNow); err != nil {
			t.Fatalf("tạm ngừng: %v", err)
		}
		err := p.Sua(domain.SuaParams{Name: chuoi("Đổi khi ngừng bán")}, testNow)
		if !errors.Is(err, domain.ErrInvalidStatus) {
			t.Errorf("sửa được sản phẩm TẠM NGỪNG BÁN: %v", err)
		}
	})
}

// Sửa hàng ĐANG BÁN đưa nó về hàng chờ duyệt, tức TẠM ẨN khỏi cửa hàng.
//
// Đây là quyết định 23/09/2026. Không có bước này thì nội dung mới ra tới
// khách mà chưa ai duyệt — cửa sau để tráo ảnh sau khi đã được duyệt.
func TestSuaHangDangBanPhaiDuyetLai(t *testing.T) {
	p := dungSanPhamDangBan(t)
	if !p.IsVisibleToCustomer() {
		t.Fatal("dựng sai: sản phẩm chưa ở trạng thái đang bán")
	}

	if err := p.Sua(domain.SuaParams{
		Description: chuoi("Mô tả mới, thêm chi tiết chất liệu"),
	}, testNow); err != nil {
		t.Fatalf("sửa hàng đang bán: %v", err)
	}

	if got := p.Status(); got != domain.StatusPendingReview {
		t.Errorf("sau khi sửa, trạng thái = %q, mong PENDING_REVIEW", got)
	}
	if p.IsVisibleToCustomer() {
		t.Error("khách VẪN thấy nội dung chưa ai duyệt — đúng cửa sau mà " +
			"luật này sinh ra để đóng")
	}
}

// Lý do từ chối CŨ phải được xóa khi tự nguyện gửi lại.
//
// Để lại thì màn hình nhà bán hiện "Bị trả về" cho một sản phẩm vừa được
// gửi đi vì lý do hoàn toàn khác — và lý do hiện ra là chuyện của lượt
// duyệt trước, đã xử lý xong.
func TestSuaHangDangBanXoaLyDoTuChoiCu(t *testing.T) {
	p := newTestProduct(t, nil)
	datDuDeGuiDuyet(t, p)
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("gửi duyệt: %v", err)
	}
	if err := p.Reject("Ảnh bìa mờ", testNow); err != nil {
		t.Fatalf("từ chối: %v", err)
	}
	anh := []string{"https://x/bia-net.jpg"}
	if err := p.Sua(domain.SuaParams{Images: &anh}, testNow); err != nil {
		t.Fatalf("sửa: %v", err)
	}
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("gửi lại: %v", err)
	}
	if err := p.Approve(testNow); err != nil {
		t.Fatalf("duyệt: %v", err)
	}

	// Giờ sửa lại lần nữa khi đang bán.
	if err := p.Sua(domain.SuaParams{Description: chuoi("Bổ sung")}, testNow); err != nil {
		t.Fatalf("sửa hàng đang bán: %v", err)
	}
	if got := p.RejectionReason(); got != "" {
		t.Errorf("lý do từ chối cũ còn lại: %q — nhà bán sẽ thấy \"Bị trả "+
			"về\" cho một sản phẩm họ vừa tự gửi đi duyệt lại", got)
	}
}

// Sửa hàng đang bán mà làm nó THIẾU điều kiện duyệt thì bị TỪ CHỐI CẢ LƯỢT.
//
// Không có chốt này, gỡ hết ảnh sẽ đẩy sản phẩm vào hàng chờ duyệt ở trạng
// thái không bao giờ duyệt được: nhà bán mất hàng đang bán mà không hiểu
// vì sao.
func TestSuaHangDangBanLamThieuDieuKienThiTuChoi(t *testing.T) {
	p := dungSanPhamDangBan(t)

	trong := []string{}
	err := p.Sua(domain.SuaParams{Images: &trong}, testNow)
	if !errors.Is(err, domain.ErrNoImages) {
		t.Fatalf("mong ErrNoImages, nhận %v", err)
	}
	if got := p.Status(); got != domain.StatusActive {
		t.Errorf("sản phẩm rời khỏi ACTIVE dù lượt sửa bị từ chối: %q", got)
	}
	if len(p.Images()) == 0 {
		t.Error("ảnh đã bị gỡ dù lượt sửa bị từ chối")
	}
}

// dungSanPhamDangBan đưa một sản phẩm đi hết đường tới ACTIVE.
func dungSanPhamDangBan(t *testing.T) *domain.Product {
	t.Helper()
	p := newTestProduct(t, nil)
	datDuDeGuiDuyet(t, p)
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("gửi duyệt: %v", err)
	}
	if err := p.Approve(testNow); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	return p
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
	if err := p.Sua(domain.SuaParams{Images: &moi}, testNow); err != nil {
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
	if err := p.Sua(domain.SuaParams{
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
