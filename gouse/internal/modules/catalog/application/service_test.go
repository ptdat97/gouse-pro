package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/modules/catalog/infrastructure/inmemory"
)

// fixedClock cho phép test kiểm soát thời gian — cần cho test về hạn hiệu
// lực giấy ủy quyền và lịch ra mắt bộ sưu tập.
type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time          { return c.t }
func (c *fixedClock) advance(d time.Duration) { c.t = c.t.Add(d) }

var baseTime = time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)

func newService(t *testing.T) (*application.Service, *fixedClock) {
	t.Helper()
	clock := &fixedClock{t: baseTime}
	svc := application.NewService(application.Deps{
		Brands:      inmemory.NewBrandStore(),
		Auths:       inmemory.NewAuthorizationStore(),
		Collections: inmemory.NewCollectionStore(),
		Categories:  inmemory.NewCategoryStore(),
		SizeCharts:  inmemory.NewSizeChartStore(),
		Clock:       clock,
	})
	return svc, clock
}

func TestCreateAndGetBrand(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	b, err := svc.CreateBrand(ctx, application.CreateBrandInput{
		Name: "Thương hiệu A", Slug: "thuong-hieu-a",
	})
	if err != nil {
		t.Fatalf("tạo brand lỗi: %v", err)
	}

	got, err := svc.GetBrand(ctx, b.ID())
	if err != nil {
		t.Fatalf("lấy brand lỗi: %v", err)
	}
	if got.Name() != "Thương hiệu A" {
		t.Errorf("tên: mong %q, nhận %q", "Thương hiệu A", got.Name())
	}

	// Lấy theo slug cũng phải được — dùng cho URL thân thiện
	bySlug, err := svc.GetBrandBySlug(ctx, "thuong-hieu-a")
	if err != nil || bySlug.ID() != b.ID() {
		t.Errorf("lấy theo slug thất bại: %v", err)
	}
}

func TestBrandSlugMustBeUnique(t *testing.T) {
	// Slug là định danh trong URL — hai brand cùng slug nghĩa là hai
	// trang cùng đường dẫn.
	ctx := context.Background()
	svc, _ := newService(t)

	_, err := svc.CreateBrand(ctx, application.CreateBrandInput{Name: "A", Slug: "trung"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateBrand(ctx, application.CreateBrandInput{Name: "B", Slug: "trung"})
	if !errors.Is(err, domain.ErrSlugTaken) {
		t.Fatalf("slug trùng phải bị từ chối, nhận %v", err)
	}
}

// TestCanSellerSellBrand là test QUAN TRỌNG NHẤT của module.
//
// Đây là cơ chế chống hàng giả — rủi ro sống còn của marketplace thời trang.
func TestCanSellerSellBrand(t *testing.T) {
	ctx := context.Background()
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	t.Run("thương hiệu OPEN: ai cũng bán được", func(t *testing.T) {
		svc, _ := newService(t)
		b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{
			Name: "Mở", Slug: "mo", ProtectionLevel: domain.ProtectionOpen,
		})

		res, err := svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Allowed {
			t.Errorf("thương hiệu OPEN phải cho phép, lý do: %s", res.Reason)
		}
	})

	t.Run("VERIFIED_ONLY không có ủy quyền: bị chặn", func(t *testing.T) {
		svc, _ := newService(t)
		b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{
			Name: "Bảo vệ", Slug: "bao-ve",
			ProtectionLevel: domain.ProtectionVerifiedOnly,
		})

		res, _ := svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if res.Allowed {
			t.Error("không có ủy quyền phải bị CHẶN")
		}
		if res.Reason != application.ReasonNoAuthorization {
			t.Errorf("lý do: mong %q, nhận %q", application.ReasonNoAuthorization, res.Reason)
		}
		// Phải nói seller cần làm gì — không chỉ từ chối
		if res.RequiredAction != "UPLOAD_AUTHORIZATION" {
			t.Errorf("phải chỉ ra hành động cần làm, nhận %q", res.RequiredAction)
		}
	})

	t.Run("VERIFIED_ONLY có ủy quyền đã duyệt: cho phép", func(t *testing.T) {
		svc, _ := newService(t)
		b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{
			Name: "Bảo vệ", Slug: "bao-ve",
			ProtectionLevel: domain.ProtectionVerifiedOnly,
		})

		a, err := svc.GrantAuthorization(ctx, application.GrantAuthorizationInput{
			BrandID: b.ID(), SellerID: sellerA,
			DocumentURL: "https://cdn/uy-quyen.pdf",
			ValidFrom:   baseTime,
			ValidUntil:  baseTime.Add(365 * 24 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}

		// CHƯA duyệt → vẫn bị chặn
		res, _ := svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if res.Allowed {
			t.Error("ủy quyền CHƯA DUYỆT không được cho phép bán")
		}

		if _, err := svc.ApproveAuthorization(ctx, a.ID(), "nv.hoa"); err != nil {
			t.Fatal(err)
		}

		res, _ = svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if !res.Allowed {
			t.Errorf("ủy quyền đã duyệt phải cho phép, lý do: %s", res.Reason)
		}

		// Seller KHÁC vẫn bị chặn — ủy quyền không lây sang seller khác
		res, _ = svc.CanSellerSellBrand(ctx, b.ID(), sellerB)
		if res.Allowed {
			t.Error("ủy quyền của seller A không được áp dụng cho seller B")
		}
	})

	t.Run("ủy quyền HẾT HẠN tự động chặn", func(t *testing.T) {
		// Đây là điểm mấu chốt: không cần ai nhớ thu hồi thủ công.
		svc, clock := newService(t)
		b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{
			Name: "Bảo vệ", Slug: "bao-ve",
			ProtectionLevel: domain.ProtectionVerifiedOnly,
		})
		a, _ := svc.GrantAuthorization(ctx, application.GrantAuthorizationInput{
			BrandID: b.ID(), SellerID: sellerA,
			DocumentURL: "https://cdn/x.pdf",
			ValidFrom:   baseTime,
			ValidUntil:  baseTime.Add(30 * 24 * time.Hour),
		})
		_, _ = svc.ApproveAuthorization(ctx, a.ID(), "nv.hoa")

		res, _ := svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if !res.Allowed {
			t.Fatal("trong hạn phải cho phép")
		}

		// Tua thời gian qua ngày hết hạn
		clock.advance(31 * 24 * time.Hour)

		res, _ = svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if res.Allowed {
			t.Error("ủy quyền HẾT HẠN phải TỰ ĐỘNG chặn")
		}
		if res.Reason != application.ReasonAuthExpired {
			t.Errorf("lý do: mong %q, nhận %q", application.ReasonAuthExpired, res.Reason)
		}
		if res.RequiredAction != "RENEW_AUTHORIZATION" {
			t.Errorf("phải hướng dẫn gia hạn, nhận %q", res.RequiredAction)
		}
	})

	t.Run("RESTRICTED: chỉ seller được chỉ định", func(t *testing.T) {
		svc, _ := newService(t)
		b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{
			Name: "Hạn chế", Slug: "han-che",
			ProtectionLevel: domain.ProtectionRestricted,
			OwnerSellerID:   sellerA,
		})

		res, _ := svc.CanSellerSellBrand(ctx, b.ID(), sellerA)
		if !res.Allowed {
			t.Error("seller được chỉ định phải được bán")
		}

		res, _ = svc.CanSellerSellBrand(ctx, b.ID(), sellerB)
		if res.Allowed {
			t.Error("seller khác phải bị chặn với RESTRICTED")
		}
		if res.Reason != application.ReasonRestrictedToOwner {
			t.Errorf("lý do: nhận %q", res.Reason)
		}
	})

	t.Run("thương hiệu không tồn tại", func(t *testing.T) {
		svc, _ := newService(t)
		res, err := svc.CanSellerSellBrand(ctx, ids.MustNew(ids.PrefixBrand), sellerA)
		if err != nil {
			t.Fatalf("brand không tồn tại KHÔNG nên trả lỗi hệ thống: %v", err)
		}
		if res.Allowed || res.Reason != application.ReasonBrandNotFound {
			t.Errorf("mong BRAND_NOT_FOUND, nhận %+v", res)
		}
	})
}

func TestExpiringAuthorizationWarning(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	seller := ids.MustNew(ids.PrefixSeller)

	b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{Name: "A", Slug: "a"})

	// Giấy hết hạn sau 20 ngày
	a, _ := svc.GrantAuthorization(ctx, application.GrantAuthorizationInput{
		BrandID: b.ID(), SellerID: seller,
		DocumentURL: "https://cdn/x.pdf",
		ValidFrom:   baseTime.Add(-24 * time.Hour),
		ValidUntil:  baseTime.Add(20 * 24 * time.Hour),
	})
	_, _ = svc.ApproveAuthorization(ctx, a.ID(), "nv.hoa")

	expiring, err := svc.FindExpiringAuthorizations(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(expiring) != 1 {
		t.Fatalf("mong 1 giấy sắp hết hạn trong 30 ngày, nhận %d", len(expiring))
	}
}

// TestProcessScheduledCollections kiểm chứng cơ chế công bố theo lịch.
func TestProcessScheduledCollections(t *testing.T) {
	ctx := context.Background()
	svc, clock := newService(t)

	b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{Name: "A", Slug: "a"})

	launch := baseTime.Add(7 * 24 * time.Hour)
	end := launch.Add(90 * 24 * time.Hour)
	c, err := svc.CreateCollection(ctx, application.CreateCollectionInput{
		BrandID: b.ID(), Name: "Thu Đông 2026", Slug: "fw2026",
		Season: "FW2026", LaunchDate: launch, EndOfSeasonDate: end,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Chưa tới ngày → không ra mắt
	launched, ending, err := svc.ProcessScheduledCollections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if launched != 0 || ending != 0 {
		t.Errorf("chưa tới lịch: mong 0/0, nhận %d/%d", launched, ending)
	}

	got, _ := svc.GetCollection(ctx, c.ID())
	if got.IsVisibleToCustomer() {
		t.Error("bộ sưu tập chưa ra mắt không được hiển thị")
	}

	// Tới ngày ra mắt
	clock.advance(7 * 24 * time.Hour)
	launched, _, _ = svc.ProcessScheduledCollections(ctx)
	if launched != 1 {
		t.Errorf("mong 1 bộ sưu tập ra mắt, nhận %d", launched)
	}

	got, _ = svc.GetCollection(ctx, c.ID())
	if !got.IsVisibleToCustomer() {
		t.Error("bộ sưu tập đã ra mắt phải hiển thị")
	}

	// Chạy lại không ra mắt lần hai (idempotent)
	launched, _, _ = svc.ProcessScheduledCollections(ctx)
	if launched != 0 {
		t.Errorf("chạy lại không được ra mắt lại, nhận %d", launched)
	}

	// Tới cuối mùa
	clock.advance(90 * 24 * time.Hour)
	_, ending, _ = svc.ProcessScheduledCollections(ctx)
	if ending != 1 {
		t.Errorf("mong 1 bộ sưu tập chuyển ENDING, nhận %d", ending)
	}
}

func TestCategoryTree(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	nu, err := svc.CreateCategory(ctx, application.CreateCategoryInput{
		Name: "Nữ", Slug: "nu",
	})
	if err != nil {
		t.Fatal(err)
	}
	ao, _ := svc.CreateCategory(ctx, application.CreateCategoryInput{
		ParentID: nu.ID(), Name: "Áo", Slug: "nu-ao",
	})
	_, _ = svc.CreateCategory(ctx, application.CreateCategoryInput{
		ParentID: ao.ID(), Name: "Áo sơ mi", Slug: "nu-ao-so-mi",
	})

	if ao.Depth() != 1 {
		t.Errorf("độ sâu danh mục con: mong 1, nhận %d", ao.Depth())
	}

	tree, err := svc.GetCategoryTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 {
		t.Fatalf("mong 1 nút gốc, nhận %d", len(tree))
	}
	if tree[0].Category.Name() != "Nữ" {
		t.Errorf("nút gốc: nhận %q", tree[0].Category.Name())
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("Nữ phải có 1 con, nhận %d", len(tree[0].Children))
	}
	if len(tree[0].Children[0].Children) != 1 {
		t.Error("Áo phải có 1 con (Áo sơ mi)")
	}
}

func TestSizeChartWithRealMeasurements(t *testing.T) {
	// Bảng size có SỐ ĐO THỰC TẾ là một trong ba yếu tố giảm trực tiếp
	// tỷ lệ hoàn hàng — vấn đề kinh tế lớn nhất của thương mại thời trang.
	ctx := context.Background()
	svc, _ := newService(t)

	b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{Name: "A", Slug: "a"})

	sc, err := svc.CreateSizeChart(ctx, application.CreateSizeChartInput{
		BrandID:     b.ID(),
		ProductType: domain.ProductTypeTop,
		System:      domain.SizeSystemAlpha,
		Note:        "Bảng size khác nhau theo thương hiệu",
		Entries: []domain.SizeEntry{
			{Size: "S", Measurements: map[string]string{"chest_cm": "92-96", "length_cm": "68"}},
			{Size: "M", Measurements: map[string]string{"chest_cm": "96-100", "length_cm": "70"}},
			{Size: "L", Measurements: map[string]string{"chest_cm": "100-104", "length_cm": "72"}},
		},
	})
	if err != nil {
		t.Fatalf("tạo bảng size lỗi: %v", err)
	}

	// Tra được bảng size theo (brand, loại sản phẩm) — dùng khi hiển thị
	// trang sản phẩm
	got, err := svc.GetSizeChartFor(ctx, b.ID(), domain.ProductTypeTop)
	if err != nil {
		t.Fatalf("tra bảng size lỗi: %v", err)
	}
	if got.ID() != sc.ID() {
		t.Error("trả về sai bảng size")
	}

	m, err := got.MeasurementsFor("M")
	if err != nil {
		t.Fatal(err)
	}
	if m["chest_cm"] != "96-100" {
		t.Errorf("số đo ngực size M: nhận %q", m["chest_cm"])
	}

	if !got.HasSize("L") {
		t.Error("bảng phải có size L")
	}
	if got.HasSize("XXL") {
		t.Error("bảng không có size XXL")
	}
}

func TestSizeChartRejectsDuplicates(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{Name: "A", Slug: "a"})

	_, err := svc.CreateSizeChart(ctx, application.CreateSizeChartInput{
		BrandID:     b.ID(),
		ProductType: domain.ProductTypeTop,
		Entries: []domain.SizeEntry{
			{Size: "M"}, {Size: "M"},
		},
	})
	if !errors.Is(err, domain.ErrDuplicateSize) {
		t.Fatalf("size trùng phải bị từ chối, nhận %v", err)
	}
}

func TestCreateCollectionRequiresExistingBrand(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	_, err := svc.CreateCollection(ctx, application.CreateCollectionInput{
		BrandID: ids.MustNew(ids.PrefixBrand), // không tồn tại
		Name:    "X", Slug: "x",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bộ sưu tập của brand không tồn tại phải lỗi, nhận %v", err)
	}
}
