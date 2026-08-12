package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

// SeedResult là các định danh vừa tạo.
//
// Người phát triển cần ID thật để gọi thử endpoint — ULID sinh ngẫu nhiên
// mỗi lần khởi động nên không thể đoán hay ghi sẵn vào tài liệu.
type SeedResult struct {
	OwnBrandID      string
	ThirdPartyID    string
	LaunchedColID   string
	UnlaunchedColID string

	// MenTopsCategoryID là một danh mục lá, để module khác gắn sản phẩm mẫu
	// vào một danh mục CÓ THẬT thay vì tự bịa định danh.
	MenTopsCategoryID string

	// AlreadySeeded báo dữ liệu đã có sẵn nên không nạp lại.
	//
	// Cần thiết với PostgreSQL: dữ liệu sống qua lần khởi động lại, và nạp
	// lại sẽ vi phạm ràng buộc UNIQUE khiến tiến trình không khởi động được.
	AlreadySeeded bool
}

// SeedDemo nạp dữ liệu mẫu tối thiểu để chạy thử khi dùng kho in-memory.
//
// CHỈ dùng cho môi trường phát triển. Kho in-memory mất dữ liệu khi tiến
// trình dừng, nên không có cách nào tạo dữ liệu ban đầu ngoài việc nạp lúc
// khởi động. Khi có PostgreSQL, việc này chuyển sang migration/fixture và
// hàm này biến mất.
//
// Dữ liệu phản ánh mô hình kinh doanh thật: một thương hiệu của nền tảng
// (BrandTypeOwn, được bảo vệ) và một thương hiệu mở cho seller bán.
func SeedDemo(ctx context.Context, m *Module) (SeedResult, error) {
	var out SeedResult
	svc := m.svc
	now := svc.Now()

	// Với PostgreSQL, dữ liệu SỐNG QUA lần khởi động lại. Nạp lại sẽ vi phạm
	// ràng buộc UNIQUE trên slug và làm tiến trình không khởi động được —
	// nên phải kiểm tra trước.
	//
	// Kho in-memory luôn rỗng lúc khởi động nên nhánh này không ảnh hưởng.
	existing, err := svc.ListBrands(ctx, domain.BrandFilter{Limit: 1})
	if err != nil {
		return out, fmt.Errorf("kiểm tra dữ liệu sẵn có: %w", err)
	}
	if len(existing) > 0 {
		return SeedResult{AlreadySeeded: true}, nil
	}

	own, err := svc.CreateBrand(ctx, application.CreateBrandInput{
		Name:            "Lumière",
		Slug:            "lumiere",
		Description:     "Thương hiệu thiết kế của nền tảng, sản xuất theo lô nhỏ.",
		LogoURL:         "https://cdn.example.com/brands/lumiere.png",
		BrandType:       domain.BrandTypeOwn,
		ProtectionLevel: domain.ProtectionRestricted,
		CountryOfOrigin: "VN",
	})
	if err != nil {
		return out, fmt.Errorf("nạp thương hiệu của nền tảng: %w", err)
	}

	out.OwnBrandID = own.ID().String()

	thirdParty, err := svc.CreateBrand(ctx, application.CreateBrandInput{
		Name:            "Basics Co",
		Slug:            "basics-co",
		Description:     "Thương hiệu phổ thông, seller đã xác minh được bán.",
		BrandType:       domain.BrandTypeThirdParty,
		ProtectionLevel: domain.ProtectionOpen,
		CountryOfOrigin: "VN",
	})
	if err != nil {
		return out, fmt.Errorf("nạp thương hiệu bên thứ ba: %w", err)
	}
	out.ThirdPartyID = thirdParty.ID().String()

	// Một bộ sưu tập ĐÃ ra mắt và một bộ CHƯA — để kiểm chứng rằng bộ chưa
	// ra mắt không lộ ra API công khai.
	launched, err := svc.CreateCollection(ctx, application.CreateCollectionInput{
		BrandID:         own.ID(),
		Name:            "Thu Đông 2026",
		Slug:            "thu-dong-2026",
		Season:          "FW2026",
		Theme:           "Tối giản, tông đất",
		LaunchDate:      now.Add(-24 * time.Hour),
		EndOfSeasonDate: now.Add(90 * 24 * time.Hour),
	})
	if err != nil {
		return out, fmt.Errorf("nạp bộ sưu tập đã ra mắt: %w", err)
	}
	if _, err := svc.LaunchCollection(ctx, launched.ID()); err != nil {
		return out, fmt.Errorf("ra mắt bộ sưu tập: %w", err)
	}
	out.LaunchedColID = launched.ID().String()

	unlaunched, err := svc.CreateCollection(ctx, application.CreateCollectionInput{
		BrandID:         own.ID(),
		Name:            "Xuân Hè 2027",
		Slug:            "xuan-he-2027",
		Season:          "SS2027",
		Theme:           "Chưa công bố",
		LaunchDate:      now.Add(120 * 24 * time.Hour),
		EndOfSeasonDate: now.Add(240 * 24 * time.Hour),
	})
	if err != nil {
		return out, fmt.Errorf("nạp bộ sưu tập chưa ra mắt: %w", err)
	}
	out.UnlaunchedColID = unlaunched.ID().String()

	// Cây danh mục hai cấp.
	women, err := svc.CreateCategory(ctx, application.CreateCategoryInput{
		Name: "Nữ", Slug: "nu", DisplayOrder: 1,
	})
	if err != nil {
		return out, fmt.Errorf("nạp danh mục gốc: %w", err)
	}
	men, err := svc.CreateCategory(ctx, application.CreateCategoryInput{
		Name: "Nam", Slug: "nam", DisplayOrder: 2,
	})
	if err != nil {
		return out, fmt.Errorf("nạp danh mục gốc: %w", err)
	}

	for _, c := range []application.CreateCategoryInput{
		{ParentID: women.ID(), Name: "Áo", Slug: "nu-ao", DisplayOrder: 1},
		{ParentID: women.ID(), Name: "Váy", Slug: "nu-vay", DisplayOrder: 2},
		{ParentID: men.ID(), Name: "Áo", Slug: "nam-ao", DisplayOrder: 1},
		{ParentID: men.ID(), Name: "Quần", Slug: "nam-quan", DisplayOrder: 2},
	} {
		created, err := svc.CreateCategory(ctx, c)
		if err != nil {
			return out, fmt.Errorf("nạp danh mục con %q: %w", c.Slug, err)
		}
		// Trả về một danh mục lá để module product gắn sản phẩm mẫu vào.
		// Sản phẩm phải trỏ tới danh mục CÓ THẬT, nên không thể tự bịa id.
		if c.Slug == "nam-ao" {
			out.MenTopsCategoryID = created.ID().String()
		}
	}

	// Bảng size với SỐ ĐO THỰC TẾ — không chỉ ký hiệu S/M/L.
	if _, err := svc.CreateSizeChart(ctx, application.CreateSizeChartInput{
		BrandID:     own.ID(),
		ProductType: domain.ProductTypeTop,
		System:      domain.SizeSystemAlpha,
		Note:        "Số đo cơ thể, không phải số đo sản phẩm.",
		Entries: []domain.SizeEntry{
			{Size: "S", Measurements: map[string]string{"chest_cm": "84-88", "waist_cm": "64-68"}},
			{Size: "M", Measurements: map[string]string{"chest_cm": "88-92", "waist_cm": "68-72"}},
			{Size: "L", Measurements: map[string]string{"chest_cm": "92-96", "waist_cm": "72-76"}},
		},
	}); err != nil {
		return out, fmt.Errorf("nạp bảng size: %w", err)
	}

	return out, nil
}
