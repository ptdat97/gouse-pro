package product

import (
	"context"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

// SeedInput là dữ liệu tham chiếu từ catalog.
//
// Sản phẩm phải trỏ tới thương hiệu và danh mục CÓ THẬT, nếu không hàng rào
// kiểm tra thương hiệu sẽ chặn ngay. Vì vậy phải nạp catalog trước.
type SeedInput struct {
	BrandID      string
	CollectionID string
	CategoryID   string
}

// SeedResult là các định danh vừa tạo.
//
// Người phát triển cần ID thật để gọi thử endpoint — ULID sinh ngẫu nhiên
// mỗi lần khởi động nên không thể đoán hay ghi sẵn vào tài liệu.
type SeedResult struct {
	ActiveProductID string
	DraftProductID  string
	SKUCode         string

	// SKUIDs là định danh các SKU vừa tạo, để module pricing gắn giá vào
	// SKU CÓ THẬT thay vì tự bịa định danh.
	SKUIDs []string
}

// SeedDemo nạp dữ liệu mẫu tối thiểu để chạy thử khi dùng kho in-memory.
//
// CHỈ dùng cho môi trường phát triển. Khi có PostgreSQL, việc này chuyển
// sang migration/fixture và hàm này biến mất.
//
// Nạp CẢ sản phẩm đã duyệt lẫn sản phẩm còn nháp: có cả hai mới kiểm chứng
// được rằng hàng chưa duyệt KHÔNG lọt ra endpoint công khai.
func SeedDemo(ctx context.Context, m *Module, in SeedInput) (SeedResult, error) {
	var out SeedResult

	brandID, err := ids.Parse(in.BrandID, ids.PrefixBrand)
	if err != nil {
		return out, fmt.Errorf("brand_id không hợp lệ: %w", err)
	}
	categoryID, err := ids.Parse(in.CategoryID, ids.PrefixCategory)
	if err != nil {
		return out, fmt.Errorf("category_id không hợp lệ: %w", err)
	}
	var collectionID ids.ID
	if in.CollectionID != "" {
		if collectionID, err = ids.Parse(in.CollectionID, ids.PrefixCollection); err != nil {
			return out, fmt.Errorf("collection_id không hợp lệ: %w", err)
		}
	}

	svc := m.svc

	// Sản phẩm đã xuất bản, đủ hai màu và hai size.
	active, err := svc.CreateProduct(ctx, application.CreateProductInput{
		BrandID:      brandID,
		CollectionID: collectionID,
		CategoryID:   categoryID,
		Name:         "Áo sơ mi linen Oxford",
		Slug:         "ao-so-mi-linen-oxford",
		Description:  "Áo sơ mi vải linen pha cotton, form suông, phù hợp khí hậu nóng ẩm.",
		// Ba trường đặc thù thời trang — thiếu chúng làm tăng tỷ lệ hoàn hàng.
		MaterialComposition: "80% cotton, 20% linen",
		CareInstructions:    "Giặt máy ở 30°C, không dùng chất tẩy, ủi nhiệt độ trung bình",
		OriginCountry:       "VN",
		ProductType:         domain.ProductTypeTop,
		GenderTarget:        domain.GenderMen,
		Images: []string{
			"https://cdn.example.com/products/so-mi-linen-1.jpg",
			"https://cdn.example.com/products/so-mi-linen-2.jpg",
		},
	})
	if err != nil {
		return out, fmt.Errorf("nạp sản phẩm: %w", err)
	}

	variants := []struct {
		color string
		size  string
		code  string
		gram  int
	}{
		{"Trắng", "M", "SM-LIN-OXF-WHT-M", 320},
		{"Trắng", "L", "SM-LIN-OXF-WHT-L", 340},
		{"Xanh navy", "M", "SM-LIN-OXF-NVY-M", 320},
	}
	for _, v := range variants {
		if _, err := svc.AddVariant(ctx, application.AddVariantInput{
			ProductID:  active.ID(),
			Attributes: map[string]string{"color": v.color, "size": v.size},
			Images:     []string{"https://cdn.example.com/products/so-mi-linen-1.jpg"},
			SKUs: []application.NewSKUInput{{
				Code:       v.code,
				WeightGram: v.gram,
				Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
			}},
		}); err != nil {
			return out, fmt.Errorf("nạp biến thể %s/%s: %w", v.color, v.size, err)
		}
	}

	if _, err := svc.SubmitForReview(ctx, active.ID()); err != nil {
		return out, fmt.Errorf("gửi duyệt sản phẩm mẫu: %w", err)
	}
	if _, err := svc.Approve(ctx, active.ID()); err != nil {
		return out, fmt.Errorf("duyệt sản phẩm mẫu: %w", err)
	}
	out.ActiveProductID = active.ID().String()
	out.SKUCode = variants[0].code

	// Đọc lại sản phẩm để lấy định danh SKU đã sinh — module pricing cần
	// chúng để gắn giá vào SKU có thật.
	saved, err := svc.GetProduct(ctx, active.ID())
	if err != nil {
		return out, fmt.Errorf("đọc lại sản phẩm mẫu: %w", err)
	}
	for _, sku := range saved.SKUs() {
		out.SKUIDs = append(out.SKUIDs, sku.ID().String())
	}

	// Sản phẩm còn NHÁP — dùng để kiểm chứng hàng chưa duyệt không lọt ra
	// endpoint công khai.
	draft, err := svc.CreateProduct(ctx, application.CreateProductInput{
		BrandID:             brandID,
		CategoryID:          categoryID,
		Name:                "Quần âu chưa xuất bản",
		Slug:                "quan-au-chua-xuat-ban",
		Description:         "Sản phẩm còn nháp, không được hiển thị cho khách.",
		MaterialComposition: "65% polyester, 35% viscose",
		ProductType:         domain.ProductTypeBottom,
		GenderTarget:        domain.GenderMen,
		Images:              []string{"https://cdn.example.com/products/quan-au.jpg"},
	})
	if err != nil {
		return out, fmt.Errorf("nạp sản phẩm nháp: %w", err)
	}
	out.DraftProductID = draft.ID().String()

	return out, nil
}
