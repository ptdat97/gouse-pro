package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/modules/product/infrastructure/inmemory"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

// fakeCatalog giả lập module catalog.
//
// Viết tay thay vì dùng thư viện sinh mã: port chỉ có 3 phương thức, và
// bản giả viết tay cho phép mô tả rõ TÌNH HUỐNG NGHIỆP VỤ đang thử
// ("giấy ủy quyền hết hạn") thay vì chỉ khai báo giá trị trả về.
type fakeCatalog struct {
	brandExists   bool
	brandErr      error
	allowSell     bool
	denyReason    string
	sellErr       error
	sizeChartID   ids.ID
	sizeChartOK   bool
	sellCallCount int
}

func (f *fakeCatalog) BrandExists(context.Context, ids.ID) (bool, error) {
	return f.brandExists, f.brandErr
}

func (f *fakeCatalog) CanSellerSellBrand(context.Context, ids.ID, ids.ID) (bool, string, error) {
	f.sellCallCount++
	return f.allowSell, f.denyReason, f.sellErr
}

func (f *fakeCatalog) SizeChartExistsFor(context.Context, ids.ID, string) (ids.ID, bool, error) {
	return f.sizeChartID, f.sizeChartOK, nil
}

func newCatalogOK() *fakeCatalog {
	return &fakeCatalog{brandExists: true, allowSell: true}
}

func newService(t *testing.T, cat application.CatalogPort) *application.Service {
	t.Helper()
	return application.NewService(application.Deps{
		Products: inmemory.NewProductStore(),
		Catalog:  cat,
		Clock:    &fixedClock{t: testNow},
	})
}

func baseInput() application.CreateProductInput {
	return application.CreateProductInput{
		BrandID:             ids.MustNew(ids.PrefixBrand),
		CategoryID:          ids.MustNew(ids.PrefixCategory),
		SizeChartID:         ids.MustNew(ids.PrefixSizeChart),
		Name:                "Áo sơ mi linen Oxford",
		Slug:                "ao-so-mi-linen-oxford",
		Description:         "Áo sơ mi vải linen",
		MaterialComposition: "80% cotton, 20% linen",
		ProductType:         domain.ProductTypeTop,
		GenderTarget:        domain.GenderMen,
		Images:              []string{"https://cdn.example.com/1.jpg"},
	}
}

func TestTaoSanPham(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())

	p, err := svc.CreateProduct(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if p.Status() != domain.StatusDraft {
		t.Errorf("trạng thái = %q, mong DRAFT", p.Status())
	}
	if !p.IsPlatformCatalog() {
		t.Error("không truyền SellerID thì phải là danh mục chuẩn của nền tảng")
	}

	// Phải lưu thật, không chỉ trả về.
	got, err := svc.GetProduct(ctx, p.ID())
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got.ID() != p.ID() {
		t.Error("sản phẩm không được lưu")
	}
}

func TestTaoSanPhamTuChoiThuongHieuKhongTonTai(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, &fakeCatalog{brandExists: false})

	if _, err := svc.CreateProduct(ctx, baseInput()); !errors.Is(err, application.ErrBrandNotFound) {
		t.Errorf("lỗi = %v, mong ErrBrandNotFound", err)
	}
}

// HÀNG RÀO CHỐNG HÀNG GIẢ. Seller không có ủy quyền không được tạo sản phẩm
// mang thương hiệu được bảo vệ.
func TestSellerKhongCoUyQuyenKhongTaoDuocSanPham(t *testing.T) {
	ctx := context.Background()
	cat := &fakeCatalog{
		brandExists: true,
		allowSell:   false,
		denyReason:  "NO_AUTHORIZATION",
	}
	svc := newService(t, cat)

	in := baseInput()
	in.SellerID = ids.MustNew(ids.PrefixSeller)

	_, err := svc.CreateProduct(ctx, in)
	if !errors.Is(err, application.ErrNotAuthorized) {
		t.Fatalf("lỗi = %v, mong NotAuthorizedError", err)
	}

	// Lý do phải đi kèm để giao diện hiển thị hành động cụ thể.
	var authErr *application.NotAuthorizedError
	if !errors.As(err, &authErr) {
		t.Fatal("không lấy được chi tiết lỗi")
	}
	if authErr.Reason != "NO_AUTHORIZATION" {
		t.Errorf("lý do = %q, mong NO_AUTHORIZATION", authErr.Reason)
	}

	// Và KHÔNG được lưu gì.
	list, err := svc.ListProducts(ctx, domain.Filter{})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("đã lưu %d sản phẩm dù bị từ chối quyền bán", len(list))
	}
}

// Nền tảng tự tạo sản phẩm cho thương hiệu của mình thì KHÔNG cần kiểm tra
// ủy quyền — bắt buộc sẽ chặn chính own brand của nền tảng.
func TestNenTangTaoSanPhamKhongCanKiemTraUyQuyen(t *testing.T) {
	ctx := context.Background()
	cat := &fakeCatalog{brandExists: true, allowSell: false, denyReason: "NO_AUTHORIZATION"}
	svc := newService(t, cat)

	if _, err := svc.CreateProduct(ctx, baseInput()); err != nil {
		t.Fatalf("nền tảng phải tạo được sản phẩm: %v", err)
	}
	if cat.sellCallCount != 0 {
		t.Errorf("đã gọi kiểm tra quyền bán %d lần, mong 0", cat.sellCallCount)
	}
}

// Bảng size gắn với (thương hiệu, loại sản phẩm) và catalog đã biết. Bắt
// người đăng bán tự tìm id là nguồn gốc của việc gắn sai hoặc bỏ trống.
func TestTuTraBangSizeKhiKhongChiDinh(t *testing.T) {
	ctx := context.Background()
	sizeChartID := ids.MustNew(ids.PrefixSizeChart)
	cat := &fakeCatalog{
		brandExists: true, allowSell: true,
		sizeChartID: sizeChartID, sizeChartOK: true,
	}
	svc := newService(t, cat)

	in := baseInput()
	in.SizeChartID = "" // cố ý không chỉ định

	p, err := svc.CreateProduct(ctx, in)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if p.SizeChartID() != sizeChartID {
		t.Errorf("sizeChartID = %q, mong %q", p.SizeChartID(), sizeChartID)
	}
}

func TestThemBienTheVaSKU(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())

	p, err := svc.CreateProduct(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	got, err := svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Trắng", "size": "M"},
		SKUs: []application.NewSKUInput{{
			Code:       "AO-WHT-M",
			WeightGram: 320,
			Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		}},
	})
	if err != nil {
		t.Fatalf("AddVariant: %v", err)
	}
	if len(got.Variants()) != 1 || len(got.SKUs()) != 1 {
		t.Fatalf("biến thể = %d, SKU = %d, mong 1 và 1", len(got.Variants()), len(got.SKUs()))
	}

	// Tra ngược từ mã SKU phải ra đúng sản phẩm.
	bySKU, err := svc.GetProductBySKUCode(ctx, "AO-WHT-M")
	if err != nil {
		t.Fatalf("GetProductBySKUCode: %v", err)
	}
	if bySKU.ID() != p.ID() {
		t.Error("tra ngược mã SKU ra sản phẩm sai")
	}
}

// Quy tắc 1: mã SKU duy nhất toàn hệ thống, kể cả giữa hai sản phẩm khác nhau.
func TestKhongThemDuocSKUTrungMaOSanPhamKhac(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())

	p1, err := svc.CreateProduct(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateProduct p1: %v", err)
	}
	if _, err := svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p1.ID(),
		Attributes: map[string]string{"color": "Trắng", "size": "M"},
		SKUs:       []application.NewSKUInput{{Code: "TRUNG-MA", WeightGram: 300}},
	}); err != nil {
		t.Fatalf("AddVariant p1: %v", err)
	}

	in2 := baseInput()
	in2.Slug = "san-pham-hai"
	p2, err := svc.CreateProduct(ctx, in2)
	if err != nil {
		t.Fatalf("CreateProduct p2: %v", err)
	}

	_, err = svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p2.ID(),
		Attributes: map[string]string{"color": "Đen", "size": "L"},
		SKUs:       []application.NewSKUInput{{Code: "TRUNG-MA", WeightGram: 300}},
	})
	if !errors.Is(err, domain.ErrSKUCodeTaken) {
		t.Errorf("lỗi = %v, mong ErrSKUCodeTaken", err)
	}
}

func TestVongDoiXuatBanQuaService(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())

	p := taoSanPhamDuDieuKien(t, svc)

	if _, err := svc.SubmitForReview(ctx, p.ID()); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	got, err := svc.Approve(ctx, p.ID())
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !got.IsVisibleToCustomer() {
		t.Error("sản phẩm đã duyệt phải hiển thị cho khách")
	}

	// Trạng thái phải được LƯU, không chỉ đổi trong bộ nhớ.
	reload, err := svc.GetProduct(ctx, p.ID())
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if reload.Status() != domain.StatusActive {
		t.Errorf("trạng thái đã lưu = %q, mong ACTIVE", reload.Status())
	}
}

// Giữa lúc gửi duyệt và lúc duyệt có thể đã nhiều ngày — giấy ủy quyền có
// thể đã hết hạn. Không kiểm tra lại nghĩa là hàng không còn được phép bán
// vẫn lên sàn.
func TestKiemTraLaiQuyenBanTruocKhiXuatBan(t *testing.T) {
	ctx := context.Background()
	cat := newCatalogOK()
	svc := newService(t, cat)

	in := baseInput()
	in.SellerID = ids.MustNew(ids.PrefixSeller)
	p, err := svc.CreateProduct(ctx, in)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Trắng", "size": "M"},
		SKUs:       []application.NewSKUInput{{Code: "AO-M", WeightGram: 300}},
	}); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}
	if _, err := svc.SubmitForReview(ctx, p.ID()); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}

	// Giấy ủy quyền hết hạn TRONG LÚC chờ duyệt.
	cat.allowSell = false
	cat.denyReason = "AUTHORIZATION_EXPIRED"

	if _, err := svc.Approve(ctx, p.ID()); !errors.Is(err, application.ErrNotAuthorized) {
		t.Fatalf("lỗi = %v, mong NotAuthorizedError", err)
	}

	// Và sản phẩm KHÔNG được lên sàn.
	reload, err := svc.GetProduct(ctx, p.ID())
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if reload.IsVisibleToCustomer() {
		t.Error("sản phẩm hết hạn ủy quyền vẫn lên sàn")
	}
}

func TestTuChoiVaGuiLai(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())
	p := taoSanPhamDuDieuKien(t, svc)

	if _, err := svc.SubmitForReview(ctx, p.ID()); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	got, err := svc.Reject(ctx, p.ID(), "Ảnh mờ, thiếu ảnh mặt sau")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if got.Status() != domain.StatusDraft {
		t.Errorf("trạng thái = %q, mong DRAFT", got.Status())
	}
	if got.RejectionReason() == "" {
		t.Error("phải lưu lý do từ chối")
	}

	// Sửa xong gửi lại được.
	if _, err := svc.SubmitForReview(ctx, p.ID()); err != nil {
		t.Errorf("gửi lại sau khi bị từ chối: %v", err)
	}
}

// Sản phẩm chưa duyệt KHÔNG được lọt vào danh sách hiển thị cho khách.
func TestSanPhamChuaDuyetKhongLotRaTrangKhach(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())
	coll := ids.MustNew(ids.PrefixCollection)

	// Một sản phẩm đã duyệt.
	daDuyet := taoSanPhamDuDieuKien(t, svc, func(in *application.CreateProductInput) {
		in.Slug = "da-duyet"
		in.CollectionID = coll
	})
	if _, err := svc.SubmitForReview(ctx, daDuyet.ID()); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if _, err := svc.Approve(ctx, daDuyet.ID()); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Một sản phẩm còn nháp trong CÙNG bộ sưu tập.
	taoSanPhamDuDieuKien(t, svc, func(in *application.CreateProductInput) {
		in.Slug = "con-nhap"
		in.CollectionID = coll
	})

	got, err := svc.ListVisibleByCollection(ctx, coll)
	if err != nil {
		t.Fatalf("ListVisibleByCollection: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("số sản phẩm hiển thị = %d, mong 1", len(got))
	}
	if got[0].Slug() != "da-duyet" {
		t.Errorf("lộ sản phẩm chưa duyệt: %q", got[0].Slug())
	}
}

// Quên truyền sellerID phải thành LỖI RÕ RÀNG, không được âm thầm trả về
// dữ liệu của tất cả seller.
func TestThieuSellerIDTraLoiChuKhongTraToanBo(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())
	taoSanPhamDuDieuKien(t, svc)

	got, err := svc.ListSellerProducts(ctx, "", "")
	if !errors.Is(err, application.ErrSellerRequired) {
		t.Errorf("lỗi = %v, mong ErrSellerRequired", err)
	}
	if len(got) != 0 {
		t.Errorf("trả về %d sản phẩm dù thiếu sellerID", len(got))
	}
}

func TestSellerChiThaySanPhamCuaMinh(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, newCatalogOK())

	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	taoSanPhamDuDieuKien(t, svc, func(in *application.CreateProductInput) {
		in.Slug = "cua-a"
		in.SellerID = sellerA
	})
	taoSanPhamDuDieuKien(t, svc, func(in *application.CreateProductInput) {
		in.Slug = "cua-b"
		in.SellerID = sellerB
	})

	got, err := svc.ListSellerProducts(ctx, sellerA, "")
	if err != nil {
		t.Fatalf("ListSellerProducts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("số sản phẩm = %d, mong 1", len(got))
	}
	if got[0].Slug() != "cua-a" {
		t.Errorf("lộ sản phẩm của seller khác: %q", got[0].Slug())
	}
}

// taoSanPhamDuDieuKien tạo sản phẩm đã đủ điều kiện gửi duyệt.
func taoSanPhamDuDieuKien(
	t *testing.T, svc *application.Service, mutate ...func(*application.CreateProductInput),
) *domain.Product {
	t.Helper()
	ctx := context.Background()

	in := baseInput()
	for _, m := range mutate {
		m(&in)
	}
	// Slug và mã SKU phải khác nhau giữa các lần gọi.
	p, err := svc.CreateProduct(ctx, in)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	if _, err := svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Trắng", "size": "M"},
		SKUs: []application.NewSKUInput{{
			Code:       "SKU-" + string(p.ID()),
			WeightGram: 320,
			Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		}},
	}); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}

	got, err := svc.GetProduct(ctx, p.ID())
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	return got
}
