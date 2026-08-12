package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

// newTestProduct tạo sản phẩm hợp lệ tối thiểu để test.
func newTestProduct(t *testing.T, mutate func(*domain.NewProductParams)) *domain.Product {
	t.Helper()
	p := domain.NewProductParams{
		BrandID:      ids.MustNew(ids.PrefixBrand),
		CategoryID:   ids.MustNew(ids.PrefixCategory),
		SizeChartID:  ids.MustNew(ids.PrefixSizeChart),
		Name:         "Áo sơ mi linen Oxford",
		Slug:         "ao-so-mi-linen-oxford",
		ProductType:  domain.ProductTypeTop,
		GenderTarget: domain.GenderMen,
		Now:          testNow,
	}
	if mutate != nil {
		mutate(&p)
	}
	got, err := domain.NewProduct(p)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	return got
}

// newReadyProduct tạo sản phẩm đã đủ điều kiện gửi duyệt.
func newReadyProduct(t *testing.T) *domain.Product {
	t.Helper()
	p := newTestProduct(t, func(np *domain.NewProductParams) {
		np.Description = "Áo sơ mi vải linen, form suông"
		np.MaterialComposition = "80% cotton, 20% linen"
		np.Images = []string{"https://cdn.example.com/1.jpg"}
	})
	v := newTestVariant(t, map[string]string{"color": "Trắng", "size": "M"})
	if err := p.AddVariant(v, testNow); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}
	return p
}

func newTestVariant(t *testing.T, attrs map[string]string) *domain.Variant {
	t.Helper()
	v, err := domain.NewVariant(domain.NewVariantParams{Attributes: attrs, Now: testNow})
	if err != nil {
		t.Fatalf("NewVariant: %v", err)
	}
	return v
}

func TestNewProductBatDauOTrangThaiNhap(t *testing.T) {
	p := newTestProduct(t, nil)

	if p.Status() != domain.StatusDraft {
		t.Errorf("trạng thái = %q, mong DRAFT", p.Status())
	}
	// Sản phẩm nháp KHÔNG được hiển thị cho khách.
	if p.IsVisibleToCustomer() {
		t.Error("sản phẩm nháp không được hiển thị cho khách")
	}
	if p.ID().Prefix() != ids.PrefixProduct {
		t.Errorf("tiền tố id = %q, mong %q", p.ID().Prefix(), ids.PrefixProduct)
	}
}

func TestNewProductTuChoiDuLieuThieu(t *testing.T) {
	cases := []struct {
		ten     string
		mutate  func(*domain.NewProductParams)
		wantErr error
	}{
		{"thiếu tên", func(p *domain.NewProductParams) { p.Name = "  " }, domain.ErrEmptyName},
		{"thiếu slug", func(p *domain.NewProductParams) { p.Slug = "" }, domain.ErrEmptySlug},
		{"thiếu thương hiệu", func(p *domain.NewProductParams) { p.BrandID = "" }, domain.ErrMissingBrand},
		{"thiếu danh mục", func(p *domain.NewProductParams) { p.CategoryID = "" }, domain.ErrMissingCategory},
	}

	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			p := domain.NewProductParams{
				BrandID:     ids.MustNew(ids.PrefixBrand),
				CategoryID:  ids.MustNew(ids.PrefixCategory),
				Name:        "Áo",
				Slug:        "ao",
				ProductType: domain.ProductTypeTop,
				Now:         testNow,
			}
			tc.mutate(&p)
			if _, err := domain.NewProduct(p); !errors.Is(err, tc.wantErr) {
				t.Errorf("lỗi = %v, mong %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewProductTuChoiLoaiKhongHopLe(t *testing.T) {
	_, err := domain.NewProduct(domain.NewProductParams{
		BrandID:     ids.MustNew(ids.PrefixBrand),
		CategoryID:  ids.MustNew(ids.PrefixCategory),
		Name:        "Áo",
		Slug:        "ao",
		ProductType: domain.ProductType("KHONG_CO_THAT"),
		Now:         testNow,
	})
	if err == nil {
		t.Fatal("mong lỗi khi loại sản phẩm không hợp lệ")
	}
}

// Quy tắc 3 và 4 (mục 12): sản phẩm phải đủ ảnh, mô tả, bảng size trước khi
// được duyệt. Đây là hàng rào chính chống hàng đăng sơ sài.
func TestKhongGuiDuyetDuocKhiThieuThongTin(t *testing.T) {
	cases := []struct {
		ten     string
		build   func(*testing.T) *domain.Product
		wantErr error
	}{
		{
			ten: "thiếu mô tả",
			build: func(t *testing.T) *domain.Product {
				p := newReadyProduct(t)
				// Dựng lại không có mô tả.
				return rebuildWithout(t, p, "description")
			},
			wantErr: domain.ErrMissingDescription,
		},
		{
			ten: "thiếu ảnh",
			build: func(t *testing.T) *domain.Product {
				return rebuildWithout(t, newReadyProduct(t), "images")
			},
			wantErr: domain.ErrNoImages,
		},
		{
			ten: "thiếu biến thể",
			build: func(t *testing.T) *domain.Product {
				return rebuildWithout(t, newReadyProduct(t), "variants")
			},
			wantErr: domain.ErrNoVariants,
		},
		{
			ten: "thiếu bảng size",
			build: func(t *testing.T) *domain.Product {
				return rebuildWithout(t, newReadyProduct(t), "sizechart")
			},
			wantErr: domain.ErrMissingSizeChart,
		},
		{
			ten: "thiếu chất liệu",
			build: func(t *testing.T) *domain.Product {
				return rebuildWithout(t, newReadyProduct(t), "material")
			},
			wantErr: domain.ErrMissingMaterial,
		},
	}

	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			p := tc.build(t)
			if err := p.SubmitForReview(testNow); !errors.Is(err, tc.wantErr) {
				t.Errorf("lỗi = %v, mong %v", err, tc.wantErr)
			}
			// Quan trọng: thất bại KHÔNG được làm đổi trạng thái.
			if p.Status() != domain.StatusDraft {
				t.Errorf("trạng thái = %q sau khi gửi duyệt thất bại, mong giữ DRAFT", p.Status())
			}
		})
	}
}

// rebuildWithout dựng lại sản phẩm nhưng bỏ đi một phần dữ liệu.
func rebuildWithout(t *testing.T, p *domain.Product, missing string) *domain.Product {
	t.Helper()
	rp := domain.RestoreProductParams{
		ID:                  p.ID(),
		BrandID:             p.BrandID(),
		CategoryID:          p.CategoryID(),
		SizeChartID:         p.SizeChartID(),
		Name:                p.Name(),
		Slug:                p.Slug(),
		Description:         p.Description(),
		MaterialComposition: p.MaterialComposition(),
		ProductType:         p.Type(),
		GenderTarget:        p.GenderTarget(),
		Status:              p.Status(),
		Images:              p.Images(),
		Variants:            p.Variants(),
		CreatedAt:           p.CreatedAt(),
		UpdatedAt:           p.UpdatedAt(),
	}
	switch missing {
	case "description":
		rp.Description = ""
	case "images":
		rp.Images = nil
	case "variants":
		rp.Variants = nil
	case "sizechart":
		rp.SizeChartID = ""
	case "material":
		rp.MaterialComposition = ""
	default:
		t.Fatalf("không biết bỏ gì: %q", missing)
	}
	return domain.RestoreProduct(rp)
}

// Túi và phụ kiện không có size — không được bắt buộc bảng size, nếu không
// sẽ chặn việc đăng bán những mặt hàng hoàn toàn hợp lệ.
func TestTuiVaPhuKienKhongCanBangSize(t *testing.T) {
	for _, pt := range []domain.ProductType{domain.ProductTypeBag, domain.ProductTypeAccessory} {
		t.Run(string(pt), func(t *testing.T) {
			if pt.NeedsSizeChart() {
				t.Fatalf("%s không nên bắt buộc bảng size", pt)
			}

			p := newTestProduct(t, func(np *domain.NewProductParams) {
				np.ProductType = pt
				np.SizeChartID = "" // cố ý không có bảng size
				np.Description = "Túi da thật"
				np.MaterialComposition = "100% da bò"
				np.Images = []string{"https://cdn.example.com/tui.jpg"}
			})
			if err := p.AddVariant(newTestVariant(t, map[string]string{"color": "Đen"}), testNow); err != nil {
				t.Fatalf("AddVariant: %v", err)
			}

			if err := p.SubmitForReview(testNow); err != nil {
				t.Errorf("túi/phụ kiện phải gửi duyệt được dù không có bảng size, lỗi: %v", err)
			}
		})
	}

	// Ngược lại: áo quần thì BẮT BUỘC có bảng size.
	for _, pt := range []domain.ProductType{
		domain.ProductTypeTop, domain.ProductTypeBottom,
		domain.ProductTypeDress, domain.ProductTypeShoes,
	} {
		if !pt.NeedsSizeChart() {
			t.Errorf("%s phải bắt buộc bảng size", pt)
		}
	}
}

func TestVongDoiSanPhamDayDu(t *testing.T) {
	p := newReadyProduct(t)

	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if p.Status() != domain.StatusPendingReview {
		t.Fatalf("trạng thái = %q, mong PENDING_REVIEW", p.Status())
	}

	duyet := testNow.Add(2 * time.Hour)
	if err := p.Approve(duyet); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !p.IsVisibleToCustomer() {
		t.Error("sản phẩm ACTIVE phải hiển thị cho khách")
	}
	if !p.PublishedAt().Equal(duyet) {
		t.Errorf("publishedAt = %v, mong %v", p.PublishedAt(), duyet)
	}

	// Tạm ngừng rồi bán lại KHÔNG được ghi đè publishedAt — nếu ghi đè,
	// báo cáo "sản phẩm mới trong tháng" sẽ đếm sai.
	if err := p.Deactivate(testNow.Add(3 * time.Hour)); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if p.IsVisibleToCustomer() {
		t.Error("sản phẩm INACTIVE không được hiển thị")
	}
	if err := p.Reactivate(testNow.Add(4 * time.Hour)); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if !p.PublishedAt().Equal(duyet) {
		t.Errorf("publishedAt bị ghi đè thành %v, phải giữ %v", p.PublishedAt(), duyet)
	}
}

func TestTuChoiPhaiNeuLyDo(t *testing.T) {
	p := newReadyProduct(t)
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}

	// Từ chối không có lý do phải bị chặn: seller không biết sửa gì sẽ
	// gửi lại y nguyên và tốn thêm một vòng duyệt.
	if err := p.Reject("   ", testNow); err == nil {
		t.Error("mong lỗi khi từ chối không nêu lý do")
	}
	if p.Status() != domain.StatusPendingReview {
		t.Errorf("trạng thái = %q, từ chối thất bại không được đổi trạng thái", p.Status())
	}

	if err := p.Reject("Ảnh mờ, thiếu ảnh mặt sau", testNow); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if p.Status() != domain.StatusDraft {
		t.Errorf("trạng thái = %q, mong quay về DRAFT", p.Status())
	}
	if p.RejectionReason() == "" {
		t.Error("phải lưu lý do từ chối để seller biết sửa gì")
	}

	// Duyệt lại phải xóa lý do từ chối cũ.
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("SubmitForReview lần 2: %v", err)
	}
	if err := p.Approve(testNow); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if p.RejectionReason() != "" {
		t.Errorf("lý do từ chối = %q, phải rỗng sau khi được duyệt", p.RejectionReason())
	}
}

// ARCHIVED là trạng thái cuối. Cho phép bật lại sẽ phá vỡ giả định của các
// module khác (đơn hàng cũ, báo cáo).
func TestLuuTruLaTrangThaiCuoi(t *testing.T) {
	p := newReadyProduct(t)
	if err := p.Archive(testNow); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	cases := map[string]error{
		"bán lại":   p.Reactivate(testNow),
		"gửi duyệt": p.SubmitForReview(testNow),
		"tạm ngừng": p.Deactivate(testNow),
	}
	for ten, err := range cases {
		if !errors.Is(err, domain.ErrInvalidStatus) {
			t.Errorf("%s sau khi lưu trữ: lỗi = %v, mong ErrInvalidStatus", ten, err)
		}
	}
	if p.Status() != domain.StatusArchived {
		t.Errorf("trạng thái = %q, mong giữ ARCHIVED", p.Status())
	}
}

func TestKhongDuyetTrucTiepTuNhap(t *testing.T) {
	p := newReadyProduct(t)
	// Bỏ qua bước duyệt là lỗ hổng: sản phẩm chưa ai xem đã lên sàn.
	if err := p.Approve(testNow); !errors.Is(err, domain.ErrInvalidStatus) {
		t.Errorf("lỗi = %v, mong ErrInvalidStatus khi duyệt thẳng từ DRAFT", err)
	}
}

// Quy tắc 2 (mục 12): không trùng tổ hợp thuộc tính trong một Product.
func TestKhongChoTrungToHopThuocTinh(t *testing.T) {
	p := newTestProduct(t, nil)

	v1 := newTestVariant(t, map[string]string{"color": "Trắng", "size": "M"})
	if err := p.AddVariant(v1, testNow); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}

	// Cùng tổ hợp, khác thứ tự khai báo và khác kiểu chữ — vẫn phải bị chặn.
	v2 := newTestVariant(t, map[string]string{"size": "M", "COLOR": "trắng"})
	if err := p.AddVariant(v2, testNow); !errors.Is(err, domain.ErrDuplicateVariant) {
		t.Errorf("lỗi = %v, mong ErrDuplicateVariant", err)
	}

	if got := len(p.Variants()); got != 1 {
		t.Errorf("số biến thể = %d, mong 1", got)
	}

	// Khác size thì phải thêm được.
	v3 := newTestVariant(t, map[string]string{"color": "Trắng", "size": "L"})
	if err := p.AddVariant(v3, testNow); err != nil {
		t.Errorf("AddVariant size khác: %v", err)
	}
}

// Bên ngoài KHÔNG được sửa trạng thái nội bộ qua lát cắt trả về.
func TestKhongSuaDuocTrangThaiNoiBoQuaGetter(t *testing.T) {
	p := newReadyProduct(t)

	imgs := p.Images()
	imgs[0] = "https://ke-tan-cong.example.com/anh-gia.jpg"
	if p.Images()[0] == "https://ke-tan-cong.example.com/anh-gia.jpg" {
		t.Error("sửa được ảnh của sản phẩm từ bên ngoài qua Images()")
	}

	vars := p.Variants()
	vars[0] = nil
	if p.Variants()[0] == nil {
		t.Error("sửa được danh sách biến thể từ bên ngoài qua Variants()")
	}
}

func TestSanPhamNenTangVaSanPhamSeller(t *testing.T) {
	nenTang := newTestProduct(t, nil)
	if !nenTang.IsPlatformCatalog() {
		t.Error("sản phẩm không có seller phải là danh mục chuẩn của nền tảng")
	}

	cuaSeller := newTestProduct(t, func(p *domain.NewProductParams) {
		p.CreatedBySellerID = ids.MustNew(ids.PrefixSeller)
	})
	if cuaSeller.IsPlatformCatalog() {
		t.Error("sản phẩm do seller tạo không phải danh mục chuẩn")
	}
}
