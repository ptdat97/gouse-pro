package inmemory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/modules/product/infrastructure/inmemory"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

type buildOpts struct {
	slug     string
	skuCode  string
	sellerID ids.ID
	brandID  ids.ID
	collID   ids.ID
	status   domain.Status
}

// buildProduct dựng sản phẩm có đủ variant và SKU.
func buildProduct(t *testing.T, o buildOpts) *domain.Product {
	t.Helper()

	if o.slug == "" {
		o.slug = "ao-so-mi"
	}
	if o.skuCode == "" {
		o.skuCode = "AO-WHT-M"
	}
	if o.brandID.IsZero() {
		o.brandID = ids.MustNew(ids.PrefixBrand)
	}

	p, err := domain.NewProduct(domain.NewProductParams{
		BrandID:             o.brandID,
		CollectionID:        o.collID,
		CategoryID:          ids.MustNew(ids.PrefixCategory),
		SizeChartID:         ids.MustNew(ids.PrefixSizeChart),
		Name:                "Áo sơ mi linen",
		Slug:                o.slug,
		Description:         "Mô tả",
		MaterialComposition: "80% cotton, 20% linen",
		ProductType:         domain.ProductTypeTop,
		GenderTarget:        domain.GenderMen,
		CreatedBySellerID:   o.sellerID,
		Images:              []string{"https://cdn.example.com/1.jpg"},
		Now:                 testNow,
	})
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}

	v, err := domain.NewVariant(domain.NewVariantParams{
		Attributes: map[string]string{"color": "Trắng", "size": "M"},
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewVariant: %v", err)
	}
	sku, err := domain.NewSKU(domain.NewSKUParams{
		Code: o.skuCode, WeightGram: 320,
		Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewSKU: %v", err)
	}
	if err := v.AddSKU(sku, testNow); err != nil {
		t.Fatalf("AddSKU: %v", err)
	}
	if err := p.AddVariant(v, testNow); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}

	if o.status == domain.StatusActive {
		if err := p.SubmitForReview(testNow); err != nil {
			t.Fatalf("SubmitForReview: %v", err)
		}
		if err := p.Approve(testNow); err != nil {
			t.Fatalf("Approve: %v", err)
		}
	}
	return p
}

func TestLuuVaDocLai(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()
	p := buildProduct(t, buildOpts{})

	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Name() != p.Name() || got.Slug() != p.Slug() {
		t.Errorf("đọc ra sản phẩm khác: %q/%q", got.Name(), got.Slug())
	}
	if len(got.Variants()) != 1 || len(got.SKUs()) != 1 {
		t.Errorf("số biến thể = %d, số SKU = %d, mong 1 và 1", len(got.Variants()), len(got.SKUs()))
	}
}

func TestKhongTimThayTraErrNotFound(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	if _, err := s.FindByID(ctx, ids.MustNew(ids.PrefixProduct)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindByID: lỗi = %v, mong ErrNotFound", err)
	}
	if _, err := s.FindBySlug(ctx, "khong-co-that"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindBySlug: lỗi = %v, mong ErrNotFound", err)
	}
	if _, err := s.FindBySKUCode(ctx, "KHONG-CO"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindBySKUCode: lỗi = %v, mong ErrNotFound", err)
	}
}

// Kho phải hành xử GIỐNG database: sửa aggregate sau khi Save không được
// làm đổi dữ liệu đã lưu. Nếu lưu con trỏ, test này sẽ trượt.
func TestSuaSauKhiLuuKhongAnhHuongDuLieuDaLuu(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()
	p := buildProduct(t, buildOpts{})

	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Sửa aggregate SAU khi lưu.
	if err := p.AddImage("https://cdn.example.com/anh-them.jpg", testNow); err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	if err := p.SubmitForReview(testNow); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}

	got, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(got.Images()) != 1 {
		t.Errorf("số ảnh đã lưu = %d, mong 1 — sửa sau khi Save đã ảnh hưởng kho", len(got.Images()))
	}
	if got.Status() != domain.StatusDraft {
		t.Errorf("trạng thái đã lưu = %q, mong DRAFT", got.Status())
	}
}

// Bản chụp phải đi SÂU: sửa Variant/SKU sau khi Save cũng không được ảnh
// hưởng kho. Chụp nông sẽ bỏ lọt đúng loại lỗi này.
func TestSuaVariantVaSKUSauKhiLuuKhongAnhHuongKho(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()
	p := buildProduct(t, buildOpts{})

	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Sửa sâu bên trong aggregate.
	v := p.Variants()[0]
	if err := v.AddImage("https://cdn.example.com/variant-moi.jpg", testNow); err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	v.Deactivate(testNow)
	p.SKUs()[0].Discontinue(testNow)

	got, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	gotVariant := got.Variants()[0]
	if len(gotVariant.Images()) != 0 {
		t.Errorf("ảnh biến thể đã lưu = %d, mong 0", len(gotVariant.Images()))
	}
	if gotVariant.Status() != domain.StatusActive {
		t.Errorf("trạng thái biến thể đã lưu = %q, mong ACTIVE", gotVariant.Status())
	}
	if !got.SKUs()[0].IsSellable() {
		t.Error("SKU đã lưu bị đổi thành ngừng kinh doanh")
	}
}

// Sửa aggregate vừa ĐỌC RA cũng không được làm hỏng dữ liệu trong kho.
func TestSuaBanDocRaKhongAnhHuongKho(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()
	p := buildProduct(t, buildOpts{})
	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	first, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	first.Variants()[0].Deactivate(testNow)
	if err := first.AddImage("https://cdn.example.com/x.jpg", testNow); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	second, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID lần 2: %v", err)
	}
	if second.Variants()[0].Status() != domain.StatusActive {
		t.Error("sửa bản đọc ra đã làm đổi dữ liệu trong kho")
	}
	if len(second.Images()) != 1 {
		t.Errorf("số ảnh = %d, mong 1 — sửa bản đọc ra đã ảnh hưởng kho", len(second.Images()))
	}
}

// Quy tắc 1: mã SKU duy nhất TOÀN HỆ THỐNG, không chỉ trong một sản phẩm.
func TestMaSKUDuyNhatToanHeThong(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	p1 := buildProduct(t, buildOpts{slug: "san-pham-1", skuCode: "TRUNG-MA"})
	if err := s.Save(ctx, p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}

	// Sản phẩm KHÁC dùng lại mã SKU đó.
	p2 := buildProduct(t, buildOpts{slug: "san-pham-2", skuCode: "TRUNG-MA"})
	if err := s.Save(ctx, p2); !errors.Is(err, domain.ErrSKUCodeTaken) {
		t.Errorf("lỗi = %v, mong ErrSKUCodeTaken", err)
	}

	// Lưu lại CHÍNH sản phẩm đó thì không được coi là trùng.
	if err := s.Save(ctx, p1); err != nil {
		t.Errorf("lưu lại chính sản phẩm: %v", err)
	}
}

func TestSlugDuyNhat(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	p1 := buildProduct(t, buildOpts{slug: "trung-slug", skuCode: "MA-1"})
	if err := s.Save(ctx, p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}

	p2 := buildProduct(t, buildOpts{slug: "trung-slug", skuCode: "MA-2"})
	if err := s.Save(ctx, p2); !errors.Is(err, domain.ErrSlugTaken) {
		t.Errorf("lỗi = %v, mong ErrSlugTaken", err)
	}
}

func TestTraNguocTuMaSKU(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()
	p := buildProduct(t, buildOpts{skuCode: "QUET-MA-VACH"})
	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Quét mã vạch có thể ra chữ thường — vẫn phải tìm được.
	for _, code := range []string{"QUET-MA-VACH", "quet-ma-vach", "  Quet-Ma-Vach  "} {
		got, err := s.FindBySKUCode(ctx, code)
		if err != nil {
			t.Errorf("FindBySKUCode(%q): %v", code, err)
			continue
		}
		if got.ID() != p.ID() {
			t.Errorf("FindBySKUCode(%q) ra sản phẩm sai", code)
		}
	}
}

func TestTraNguocTuDanhSachSKUID(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	p1 := buildProduct(t, buildOpts{slug: "sp-1", skuCode: "MA-1"})
	p2 := buildProduct(t, buildOpts{slug: "sp-2", skuCode: "MA-2"})
	for _, p := range []*domain.Product{p1, p2} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	skuID1 := p1.SKUs()[0].ID()
	skuID2 := p2.SKUs()[0].ID()
	khongCo := ids.MustNew(ids.PrefixSKU)

	got, err := s.FindBySKUIDs(ctx, []ids.ID{skuID1, skuID2, khongCo})
	if err != nil {
		t.Fatalf("FindBySKUIDs: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("số kết quả = %d, mong 2 (bỏ qua id không tồn tại)", len(got))
	}
	if got[skuID1].ID() != p1.ID() || got[skuID2].ID() != p2.ID() {
		t.Error("tra ngược sku_id ra sản phẩm sai")
	}
}

// Khi cập nhật sản phẩm, ánh xạ SKU cũ phải được dọn — nếu không, mã của
// SKU đã bị gỡ sẽ bị chiếm mãi mãi.
func TestCapNhatDonAnhXaSKUCu(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	p := buildProduct(t, buildOpts{slug: "sp-goc", skuCode: "MA-CU"})
	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Dựng lại sản phẩm CÙNG id nhưng không còn SKU cũ.
	thayThe := domain.RestoreProduct(domain.RestoreProductParams{
		ID:         p.ID(),
		BrandID:    p.BrandID(),
		CategoryID: p.CategoryID(),
		Name:       p.Name(),
		Slug:       p.Slug(),
		Status:     p.Status(),
		CreatedAt:  p.CreatedAt(),
		UpdatedAt:  testNow,
	})
	if err := s.Save(ctx, thayThe); err != nil {
		t.Fatalf("Save bản thay thế: %v", err)
	}

	// Mã cũ phải được giải phóng.
	if _, err := s.FindBySKUCode(ctx, "MA-CU"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("lỗi = %v, mong ErrNotFound — mã SKU cũ chưa được giải phóng", err)
	}

	// Và sản phẩm khác phải dùng lại được mã đó.
	khac := buildProduct(t, buildOpts{slug: "sp-khac", skuCode: "MA-CU"})
	if err := s.Save(ctx, khac); err != nil {
		t.Errorf("sản phẩm khác không dùng lại được mã đã giải phóng: %v", err)
	}
}

// Lọc theo seller phải nằm trong TRUY VẤN. Đây là hàng rào chống rò rỉ dữ
// liệu giữa các seller (rủi ro #5 trong deliverables.md).
func TestLocTheoSellerKhongLoDuLieuSellerKhac(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	pA := buildProduct(t, buildOpts{slug: "cua-a", skuCode: "MA-A", sellerID: sellerA})
	pB := buildProduct(t, buildOpts{slug: "cua-b", skuCode: "MA-B", sellerID: sellerB})
	nenTang := buildProduct(t, buildOpts{slug: "cua-nen-tang", skuCode: "MA-NT"})
	for _, p := range []*domain.Product{pA, pB, nenTang} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := s.List(ctx, domain.Filter{SellerID: sellerA})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("số kết quả = %d, mong 1", len(got))
	}
	if got[0].ID() != pA.ID() {
		t.Errorf("trả về sản phẩm của seller khác: %q", got[0].Slug())
	}
}

func TestLocTheoNhieuTieuChi(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	brand := ids.MustNew(ids.PrefixBrand)
	coll := ids.MustNew(ids.PrefixCollection)

	hienThi := buildProduct(t, buildOpts{
		slug: "hien-thi", skuCode: "MA-1",
		brandID: brand, collID: coll, status: domain.StatusActive,
	})
	nhap := buildProduct(t, buildOpts{
		slug: "con-nhap", skuCode: "MA-2", brandID: brand, collID: coll,
	})
	brandKhac := buildProduct(t, buildOpts{
		slug: "brand-khac", skuCode: "MA-3", status: domain.StatusActive,
	})
	for _, p := range []*domain.Product{hienThi, nhap, brandKhac} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// OnlyVisible phải loại sản phẩm còn nháp — đây là hàng rào chặn hàng
	// chưa duyệt lọt ra trang khách.
	got, err := s.List(ctx, domain.Filter{BrandID: brand, OnlyVisible: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID() != hienThi.ID() {
		t.Errorf("lọc OnlyVisible sai: được %d kết quả", len(got))
	}

	// Lọc theo bộ sưu tập.
	byColl, err := s.FindByCollection(ctx, coll)
	if err != nil {
		t.Fatalf("FindByCollection: %v", err)
	}
	if len(byColl) != 2 {
		t.Errorf("số sản phẩm trong bộ sưu tập = %d, mong 2", len(byColl))
	}
}

// Duyệt map trong Go có thứ tự ngẫu nhiên. Không sắp xếp thì phân trang
// sẽ trả kết quả khác nhau mỗi lần và khách thấy sản phẩm nhảy giữa các trang.
func TestKetQuaOnDinhQuaNhieuLanGoi(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewProductStore()

	for i, slug := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		p := buildProduct(t, buildOpts{
			slug:    slug,
			skuCode: "MA-" + slug,
			status:  domain.StatusActive,
		})
		_ = i
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	first, err := s.List(ctx, domain.Filter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := s.List(ctx, domain.Filter{Limit: 3})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for j := range got {
			if got[j].ID() != first[j].ID() {
				t.Fatalf("thứ tự đổi giữa các lần gọi ở vị trí %d", j)
			}
		}
	}

	// Phân trang không được trùng lặp giữa các trang.
	trang2, err := s.List(ctx, domain.Filter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("List trang 2: %v", err)
	}
	for _, a := range first {
		for _, b := range trang2 {
			if a.ID() == b.ID() {
				t.Errorf("sản phẩm %q xuất hiện ở cả hai trang", a.Slug())
			}
		}
	}

	// Offset vượt quá số bản ghi trả rỗng, không panic.
	if got, err := s.List(ctx, domain.Filter{Offset: 999}); err != nil || len(got) != 0 {
		t.Errorf("offset vượt quá: %d kết quả, lỗi %v", len(got), err)
	}
}
