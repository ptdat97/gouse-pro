package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/modules/catalog/infrastructure/postgres"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

// newPool mở kết nối tới database test.
//
// BỎ QUA test nếu không có DATABASE_URL: người phát triển không có
// PostgreSQL vẫn chạy được phần còn lại của bộ test. Nhưng CI PHẢI đặt
// biến này — nếu không, toàn bộ tầng kho lưu trữ sẽ không được kiểm chứng
// mà bộ test vẫn báo xanh.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL để chạy test với PostgreSQL thật")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("kết nối database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

// cleanCatalog xóa dữ liệu catalog để mỗi test bắt đầu từ trạng thái sạch.
//
// Xóa theo thứ tự ngược phụ thuộc khóa ngoại.
func cleanCatalog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{
		"size_chart_entry", "size_chart", "category",
		"collection", "brand_authorization", "brand",
	} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("dọn bảng %s: %v", tbl, err)
		}
	}
}

func newBrand(t *testing.T, slug string, mutate func(*domain.NewBrandParams)) *domain.Brand {
	t.Helper()
	p := domain.NewBrandParams{
		Name:            "Lumière",
		Slug:            slug,
		Description:     "Thương hiệu thiết kế",
		BrandType:       domain.BrandTypeOwn,
		ProtectionLevel: domain.ProtectionRestricted,
		CountryOfOrigin: "VN",
		Now:             testNow,
	}
	if mutate != nil {
		mutate(&p)
	}
	b, err := domain.NewBrand(p)
	if err != nil {
		t.Fatalf("NewBrand: %v", err)
	}
	return b
}

func TestLuuVaDocLaiThuongHieu(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewBrandStore(pool)
	b := newBrand(t, "lumiere", nil)

	if err := s.Save(ctx, b); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.FindByID(ctx, b.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	// Mọi trường phải đi qua vòng lưu/đọc nguyên vẹn. Sót một trường là
	// lỗi âm thầm: dữ liệu vẫn đọc được nhưng thiếu.
	if got.Name() != b.Name() || got.Slug() != b.Slug() {
		t.Errorf("tên/slug = %q/%q, mong %q/%q", got.Name(), got.Slug(), b.Name(), b.Slug())
	}
	if got.Type() != b.Type() || got.ProtectionLevel() != b.ProtectionLevel() {
		t.Errorf("loại/mức bảo vệ = %q/%q", got.Type(), got.ProtectionLevel())
	}
	if got.Status() != b.Status() || got.Description() != b.Description() {
		t.Errorf("trạng thái/mô tả sai")
	}
	if !got.CreatedAt().Equal(b.CreatedAt()) {
		t.Errorf("createdAt = %v, mong %v", got.CreatedAt(), b.CreatedAt())
	}
}

// Ràng buộc UNIQUE ở DATABASE — điều kho in-memory chỉ mô phỏng được.
func TestSlugTrungBiChanBoiDatabase(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewBrandStore(pool)
	if err := s.Save(ctx, newBrand(t, "trung-slug", nil)); err != nil {
		t.Fatalf("Save đầu tiên: %v", err)
	}

	err := s.Save(ctx, newBrand(t, "trung-slug", nil))
	if !errors.Is(err, domain.ErrSlugTaken) {
		t.Errorf("lỗi = %v, mong ErrSlugTaken", err)
	}
}

func TestKhongTimThayTraErrNotFound(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewBrandStore(pool)
	if _, err := s.FindByID(ctx, ids.MustNew(ids.PrefixBrand)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindByID: lỗi = %v, mong ErrNotFound", err)
	}
	if _, err := s.FindBySlug(ctx, "khong-co-that"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindBySlug: lỗi = %v, mong ErrNotFound", err)
	}
}

// Save phải là UPSERT: lưu lại cùng id thì cập nhật, không tạo bản ghi mới.
func TestLuuLaiLaCapNhatKhongPhaiThemMoi(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewBrandStore(pool)
	b := newBrand(t, "cap-nhat", nil)
	if err := s.Save(ctx, b); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := b.Rename("Tên mới", testNow.Add(time.Hour)); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := s.Save(ctx, b); err != nil {
		t.Fatalf("Save lần 2: %v", err)
	}

	all, err := s.List(ctx, domain.BrandFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("số thương hiệu = %d, mong 1 (Save phải là UPSERT)", len(all))
	}
	if all[0].Name() != "Tên mới" {
		t.Errorf("tên = %q, mong %q", all[0].Name(), "Tên mới")
	}
}

func TestLayTheoLoMotTruyVan(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewBrandStore(pool)
	b1 := newBrand(t, "thuong-hieu-1", nil)
	b2 := newBrand(t, "thuong-hieu-2", nil)
	for _, b := range []*domain.Brand{b1, b2} {
		if err := s.Save(ctx, b); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	khongCo := ids.MustNew(ids.PrefixBrand)
	got, err := s.FindByIDs(ctx, []ids.ID{b1.ID(), b2.ID(), khongCo})
	if err != nil {
		t.Fatalf("FindByIDs: %v", err)
	}
	// id không tồn tại bị bỏ qua, không làm hỏng cả lời gọi.
	if len(got) != 2 {
		t.Errorf("số kết quả = %d, mong 2", len(got))
	}
	if got[b1.ID()] == nil || got[b2.ID()] == nil {
		t.Error("thiếu thương hiệu trong kết quả")
	}

	// Danh sách rỗng không được gây lỗi.
	empty, err := s.FindByIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("danh sách rỗng: %d kết quả, lỗi %v", len(empty), err)
	}
}

func TestLocThuongHieu(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewBrandStore(pool)
	own := newBrand(t, "own-brand", nil)
	third := newBrand(t, "third-party", func(p *domain.NewBrandParams) {
		p.BrandType = domain.BrandTypeThirdParty
		p.ProtectionLevel = domain.ProtectionOpen
	})
	for _, b := range []*domain.Brand{own, third} {
		if err := s.Save(ctx, b); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := s.List(ctx, domain.BrandFilter{Type: domain.BrandTypeOwn})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID() != own.ID() {
		t.Errorf("lọc theo loại sai: %d kết quả", len(got))
	}

	// Không lọc thì trả hết.
	all, err := s.List(ctx, domain.BrandFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("không lọc = %d kết quả, mong 2", len(all))
	}

	// Limit có tác dụng.
	limited, err := s.List(ctx, domain.BrandFilter{Limit: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("Limit=1 trả %d kết quả", len(limited))
	}
}

// UNIQUE CÓ ĐIỀU KIỆN: chỉ một ủy quyền APPROVED cho mỗi (brand, seller),
// nhưng nhiều bản ghi ở trạng thái khác thì được.
//
// Đây là loại ràng buộc kho in-memory rất khó mô phỏng đúng.
func TestChiMotUyQuyenDangHieuLuc(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	brandStore := postgres.NewBrandStore(pool)
	b := newBrand(t, "co-uy-quyen", nil)
	if err := brandStore.Save(ctx, b); err != nil {
		t.Fatalf("Save brand: %v", err)
	}

	s := postgres.NewAuthorizationStore(pool)
	sellerID := ids.MustNew(ids.PrefixSeller)

	mkAuth := func(t *testing.T) *domain.BrandAuthorization {
		t.Helper()
		a, err := domain.NewBrandAuthorization(domain.NewAuthorizationParams{
			BrandID:     b.ID(),
			SellerID:    sellerID,
			DocumentURL: "https://cdn.example.com/uy-quyen.pdf",
			ValidFrom:   testNow,
			ValidUntil:  testNow.AddDate(1, 0, 0),
			Now:         testNow,
		})
		if err != nil {
			t.Fatalf("NewBrandAuthorization: %v", err)
		}
		return a
	}

	a1 := mkAuth(t)
	if err := a1.Approve("admin", testNow); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := s.Save(ctx, a1); err != nil {
		t.Fatalf("Save a1: %v", err)
	}

	// Ủy quyền APPROVED thứ hai cho cùng (brand, seller) phải bị chặn.
	a2 := mkAuth(t)
	if err := a2.Approve("admin", testNow); err != nil {
		t.Fatalf("Approve a2: %v", err)
	}
	if err := s.Save(ctx, a2); !errors.Is(err, domain.ErrDuplicateAuthorization) {
		t.Errorf("lỗi = %v, mong ErrDuplicateAuthorization", err)
	}

	// Nhưng bản ghi PENDING thì thêm được — UNIQUE chỉ áp cho APPROVED.
	a3 := mkAuth(t)
	if err := s.Save(ctx, a3); err != nil {
		t.Errorf("bản ghi PENDING bị chặn nhầm: %v", err)
	}

	// Tra ủy quyền đang hiệu lực phải ra đúng bản đã duyệt.
	got, err := s.FindActiveForSeller(ctx, b.ID(), sellerID)
	if err != nil {
		t.Fatalf("FindActiveForSeller: %v", err)
	}
	if got.ID() != a1.ID() {
		t.Errorf("tra ra ủy quyền sai: %q, mong %q", got.ID(), a1.ID())
	}
	if got.ApprovedBy() != "admin" {
		t.Errorf("approvedBy = %q, mong admin", got.ApprovedBy())
	}
	if got.ApprovedAt().IsZero() {
		t.Error("approvedAt không được rỗng sau khi duyệt")
	}
}

// Bảng size và các dòng của nó là MỘT aggregate — lưu trong một giao dịch.
func TestLuuBangSizeCungCacDong(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	brandStore := postgres.NewBrandStore(pool)
	b := newBrand(t, "co-bang-size", nil)
	if err := brandStore.Save(ctx, b); err != nil {
		t.Fatalf("Save brand: %v", err)
	}

	s := postgres.NewSizeChartStore(pool)
	sc, err := domain.NewSizeChart(domain.NewSizeChartParams{
		BrandID:     b.ID(),
		ProductType: domain.ProductTypeTop,
		System:      domain.SizeSystemAlpha,
		Note:        "Số đo cơ thể",
		Entries: []domain.SizeEntry{
			{Size: "S", Measurements: map[string]string{"chest_cm": "84-88"}},
			{Size: "M", Measurements: map[string]string{"chest_cm": "88-92"}},
			{Size: "L", Measurements: map[string]string{"chest_cm": "92-96"}},
		},
		Now: testNow,
	})
	if err != nil {
		t.Fatalf("NewSizeChart: %v", err)
	}
	if err := s.Save(ctx, sc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.FindForBrandAndType(ctx, b.ID(), domain.ProductTypeTop)
	if err != nil {
		t.Fatalf("FindForBrandAndType: %v", err)
	}
	if len(got.Entries()) != 3 {
		t.Fatalf("số dòng = %d, mong 3", len(got.Entries()))
	}
	// Số đo là JSONB — phải đi qua vòng mã hóa/giải mã nguyên vẹn.
	m, err := got.MeasurementsFor("M")
	if err != nil {
		t.Fatalf("MeasurementsFor: %v", err)
	}
	if m["chest_cm"] != "88-92" {
		t.Errorf("chest_cm = %q, mong 88-92", m["chest_cm"])
	}

	// Lưu lại với ít dòng hơn: dòng cũ phải bị xóa, không tích lũy.
	sc2, err := domain.NewSizeChart(domain.NewSizeChartParams{
		BrandID:     b.ID(),
		ProductType: domain.ProductTypeBottom,
		System:      domain.SizeSystemNumeric,
		Entries:     []domain.SizeEntry{{Size: "30", Measurements: map[string]string{"waist_cm": "76"}}},
		Now:         testNow,
	})
	if err != nil {
		t.Fatalf("NewSizeChart 2: %v", err)
	}
	if err := s.Save(ctx, sc2); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	got2, err := s.FindForBrandAndType(ctx, b.ID(), domain.ProductTypeBottom)
	if err != nil {
		t.Fatalf("FindForBrandAndType 2: %v", err)
	}
	if len(got2.Entries()) != 1 {
		t.Errorf("số dòng = %d, mong 1", len(got2.Entries()))
	}
}

// Một thương hiệu chỉ có MỘT bảng size cho mỗi loại sản phẩm.
func TestKhongTrungBangSizeChoCungLoai(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	brandStore := postgres.NewBrandStore(pool)
	b := newBrand(t, "trung-bang-size", nil)
	if err := brandStore.Save(ctx, b); err != nil {
		t.Fatalf("Save brand: %v", err)
	}

	s := postgres.NewSizeChartStore(pool)
	mk := func(t *testing.T) *domain.SizeChart {
		t.Helper()
		sc, err := domain.NewSizeChart(domain.NewSizeChartParams{
			BrandID:     b.ID(),
			ProductType: domain.ProductTypeTop,
			System:      domain.SizeSystemAlpha,
			Entries:     []domain.SizeEntry{{Size: "M", Measurements: map[string]string{"chest_cm": "88"}}},
			Now:         testNow,
		})
		if err != nil {
			t.Fatalf("NewSizeChart: %v", err)
		}
		return sc
	}

	if err := s.Save(ctx, mk(t)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, mk(t)); !errors.Is(err, domain.ErrDuplicateSizeChart) {
		t.Errorf("lỗi = %v, mong ErrDuplicateSizeChart", err)
	}
}

func TestCayDanhMuc(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	s := postgres.NewCategoryStore(pool)

	root, err := domain.NewCategory(domain.NewCategoryParams{
		Name: "Nam", Slug: "nam", ParentDepth: -1, DisplayOrder: 1, Now: testNow,
	})
	if err != nil {
		t.Fatalf("NewCategory: %v", err)
	}
	if err := s.Save(ctx, root); err != nil {
		t.Fatalf("Save root: %v", err)
	}

	child, err := domain.NewCategory(domain.NewCategoryParams{
		ParentID: root.ID(), ParentDepth: root.Depth(), Name: "Áo", Slug: "nam-ao", DisplayOrder: 1, Now: testNow,
	})
	if err != nil {
		t.Fatalf("NewCategory con: %v", err)
	}
	if err := s.Save(ctx, child); err != nil {
		t.Fatalf("Save con: %v", err)
	}

	all, err := s.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("số danh mục = %d, mong 2", len(all))
	}

	children, err := s.FindChildren(ctx, root.ID())
	if err != nil {
		t.Fatalf("FindChildren: %v", err)
	}
	if len(children) != 1 || children[0].ID() != child.ID() {
		t.Errorf("danh mục con sai: %d kết quả", len(children))
	}

	// Danh mục gốc có parentID rỗng — phải giữ nguyên qua vòng lưu/đọc,
	// không biến thành NULL rồi thành chuỗi lạ.
	got, err := s.FindByID(ctx, root.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.IsRoot() {
		t.Errorf("danh mục gốc mất parentID rỗng: %q", got.ParentID())
	}
}

func TestBoSuuTapTheoTrangThai(t *testing.T) {
	pool := newPool(t)
	cleanCatalog(t, pool)
	ctx := context.Background()

	brandStore := postgres.NewBrandStore(pool)
	b := newBrand(t, "co-bo-suu-tap", nil)
	if err := brandStore.Save(ctx, b); err != nil {
		t.Fatalf("Save brand: %v", err)
	}

	s := postgres.NewCollectionStore(pool)
	c, err := domain.NewCollection(domain.NewCollectionParams{
		BrandID:         b.ID(),
		Name:            "Thu Đông 2026",
		Slug:            "thu-dong-2026",
		Season:          "FW2026",
		LaunchDate:      testNow.AddDate(0, 1, 0),
		EndOfSeasonDate: testNow.AddDate(0, 4, 0),
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewCollection: %v", err)
	}
	if err := s.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	planned, err := s.FindByStatus(ctx, domain.CollectionPlanning)
	if err != nil {
		t.Fatalf("FindByStatus: %v", err)
	}
	if len(planned) != 1 {
		t.Fatalf("số bộ sưu tập PLANNING = %d, mong 1", len(planned))
	}

	// Mốc thời gian phải giữ nguyên qua vòng lưu/đọc — sai múi giờ ở đây
	// sẽ khiến bộ sưu tập ra mắt lệch ngày.
	got := planned[0]
	if !got.LaunchDate().Equal(c.LaunchDate()) {
		t.Errorf("launchDate = %v, mong %v", got.LaunchDate(), c.LaunchDate())
	}
	if !got.EndOfSeason().Equal(c.EndOfSeason()) {
		t.Errorf("endOfSeason = %v, mong %v", got.EndOfSeason(), c.EndOfSeason())
	}

	byBrand, err := s.FindByBrand(ctx, b.ID())
	if err != nil {
		t.Fatalf("FindByBrand: %v", err)
	}
	if len(byBrand) != 1 {
		t.Errorf("số bộ sưu tập theo thương hiệu = %d, mong 1", len(byBrand))
	}
}
