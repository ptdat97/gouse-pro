// Dựng NHÀ BÁN THỨ HAI kèm hàng của họ, để thử luồng nhiều nhà bán.
//
// # Vì sao cần công cụ này
//
// Dữ liệu mẫu chỉ có MỘT nhà bán: Lumière, loại INTERNAL (own brand). Với
// một nhà bán thì không kiểm được thứ làm nên cái chợ — đơn trộn hàng của
// nhiều bên, tách đơn thực hiện theo nguồn, và cách ly tồn kho giữa các
// chủ sở hữu.
//
// Hai rào cản thật, và cả hai đều ĐÚNG chứ không phải chỗ cần lách:
//
//  1. Thương hiệu Lumière ở mức RESTRICTED — chỉ chủ thương hiệu được
//     bán. Nên nhà bán mới cần một sản phẩm thuộc thương hiệu OPEN.
//  2. Nhà bán chỉ ACTIVE khi tài khoản ngân hàng đã xác minh (quy tắc 1
//     của seller.md). Duyệt hồ sơ thôi chưa đủ.
//
// CHỈ dùng khi phát triển.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/catalog"
	catalogapp "github.com/fashion-commerce/platform/internal/modules/catalog/application"
	catalogdom "github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	"github.com/fashion-commerce/platform/internal/modules/product"
	productapp "github.com/fashion-commerce/platform/internal/modules/product/application"
	productdom "github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/token"
)

const (
	emailNhaBan = "nhaban2@example.com"
	tenNhaBan   = "Xưởng May Bảy"
	slugNhaBan  = "xuong-may-bay"
)

func main() {
	if env := os.Getenv("APP_ENV"); env != "" && env != "development" {
		fmt.Fprintf(os.Stderr,
			"từ chối chạy với APP_ENV=%s — công cụ này CHỈ dùng khi phát triển\n", env)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{DSN: os.Getenv("DATABASE_URL")})
	if err != nil {
		panic(err)
	}
	defer db.Close()

	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		secret = "development-only-jwt-secret-do-not-use-in-production"
	}
	issuer, err := token.NewIssuer(token.Config{Secret: secret})
	if err != nil {
		panic(err)
	}

	idm, err := identity.New(identity.Config{Storage: "postgres", DB: db, Issuer: issuer})
	if err != nil {
		panic(err)
	}
	sel, err := seller.New(seller.Config{Storage: "postgres", DB: db})
	if err != nil {
		panic(err)
	}
	cat, err := catalog.New(catalog.Config{Storage: "postgres", DB: db})
	if err != nil {
		panic(err)
	}
	prod, err := product.New(product.Config{Storage: "postgres", DB: db, Catalog: cat})
	if err != nil {
		panic(err)
	}
	inv, err := inventory.New(inventory.Config{Storage: "postgres", DB: db})
	if err != nil {
		panic(err)
	}
	mkt, err := marketplace.New(marketplace.Config{
		Storage: "postgres", DB: db,
		Product: prod, Seller: sel, Catalog: cat, Inventory: inv,
	})
	if err != nil {
		panic(err)
	}

	sellerID := dungNhaBan(ctx, sel)
	fmt.Println("nhà bán :", sellerID, tenNhaBan)

	ganVaiTro(ctx, idm, sellerID)

	skuID := dungSanPham(ctx, cat, prod)
	fmt.Println("sku     :", skuID)

	dungOffer(ctx, mkt, inv, sellerID, skuID)
}

// dungNhaBan đưa nhà bán qua ĐỦ vòng đời tới ACTIVE.
//
// Bốn bước, không tắt bước nào: nộp hồ sơ → duyệt → xác minh ngân hàng →
// kích hoạt. Ghi thẳng ACTIVE vào database sẽ tạo ra một nhà bán không
// bao giờ tồn tại được ngoài đời, và test dựa trên nó sẽ nói dối.
func dungNhaBan(ctx context.Context, sel *seller.Module) string {
	// Đã có thì dùng lại — công cụ này chạy lại nhiều lần.
	if cu, err := sel.Service().GetSellerBySlug(ctx, slugNhaBan); err == nil {
		fmt.Println("nhà bán đã có, dùng lại")
		kichHoat(ctx, sel, cu.ID().String())
		return cu.ID().String()
	}

	v, err := sel.ApplyAsSeller(ctx, seller.ApplicationRequest{
		Name:             tenNhaBan,
		Slug:             slugNhaBan,
		SellerType:       "BUSINESS",
		LegalName:        "Công ty TNHH Xưởng May Bảy",
		TaxCode:          "0312345678",
		Email:            emailNhaBan,
		Phone:            "+84907070707",
		CommissionRateBP: 1200,
	})
	if err != nil {
		panic(fmt.Sprintf("nộp hồ sơ: %v", err))
	}
	kichHoat(ctx, sel, v.ID)
	return v.ID
}

// kichHoat đưa nhà bán tới ACTIVE nếu chưa.
func kichHoat(ctx context.Context, sel *seller.Module, id string) {
	svc := sel.Service()
	sid, err := ids.Parse(id, ids.PrefixSeller)
	if err != nil {
		panic(err)
	}

	cur, err := svc.GetSeller(ctx, sid)
	if err != nil {
		panic(err)
	}
	if string(cur.Status()) == "ACTIVE" {
		return
	}

	// Bỏ qua lỗi ở từng bước: chạy lại lần hai thì nhà bán có thể đã ở
	// bước sau, và "chuyển trạng thái không hợp lệ" khi đó là bình thường.
	_, _ = svc.SubmitForReview(ctx, sid)
	_, _ = svc.Approve(ctx, sid, "dev-tool")
	if _, err := svc.VerifyBankAccount(ctx, sid); err != nil {
		panic(fmt.Sprintf("xác minh ngân hàng: %v", err))
	}
	if _, err := svc.Activate(ctx, sid); err != nil {
		panic(fmt.Sprintf("kích hoạt: %v", err))
	}
}

// ganVaiTro gắn vai trò SELLER_OWNER kèm PHẠM VI là gian hàng.
//
// Phạm vi là thứ quyết định: vai trò không có phạm vi nghĩa là người này
// làm chủ MỌI gian hàng. Đó đúng là lỗ hổng mà cách ly giữa các nhà bán
// tồn tại để chặn.
func ganVaiTro(ctx context.Context, idm *identity.Module, sellerID string) {
	res, err := idm.Login(ctx, identity.LoginRequest{
		Email: emailNhaBan, Password: "mat-khau-du-dai-123",
	})
	if err != nil {
		fmt.Printf("CẢNH BÁO không đăng nhập được %s: %v\n", emailNhaBan, err)
		return
	}
	if err := idm.GrantRole(ctx, res.User.ID, "SELLER_OWNER", sellerID); err != nil {
		fmt.Printf("CẢNH BÁO gán vai trò: %v\n", err)
		return
	}
	fmt.Printf("vai trò : SELLER_OWNER cho %s, phạm vi %s\n", emailNhaBan, sellerID)
}

// dungSanPham tạo sản phẩm thuộc thương hiệu OPEN.
//
// KHÔNG dùng thương hiệu của dữ liệu mẫu: Lumière ở mức RESTRICTED nên chỉ
// chủ thương hiệu bán được. Nhà bán ngoài cần hàng của thương hiệu mở.
func dungSanPham(
	ctx context.Context, cat *catalog.Module, prod *product.Module,
) string {
	const slug = "ao-thun-co-tron-basics"

	brandID := os.Getenv("BRAND_ID_OPEN")
	if brandID == "" {
		panic("cần BRAND_ID_OPEN — mã thương hiệu ở mức OPEN")
	}
	b, err := cat.GetBrand(ctx, brandID)
	if err != nil {
		panic(fmt.Sprintf("đọc thương hiệu: %v", err))
	}
	if b.ProtectionLevel != "OPEN" {
		panic(fmt.Sprintf(
			"thương hiệu %s ở mức %s — nhà bán ngoài KHÔNG bán được",
			b.Name, b.ProtectionLevel))
	}

	// Bảng size TRƯỚC sản phẩm.
	//
	// Kiểm tra của domain là "thương hiệu này đã có bảng size cho loại
	// hàng đó chưa", nên bảng phải tồn tại trước khi gửi duyệt — kể cả khi
	// sản phẩm đã được tạo từ lần chạy trước.
	chartID := dungBangSize(ctx, cat, brandID)

	svc := prod.Service()

	if cu, err := svc.GetProductBySlug(ctx, slug); err == nil {
		if skus := cu.SKUs(); len(skus) > 0 {
			// Đảm bảo ĐÃ DUYỆT, không chỉ đã tồn tại: lần chạy trước có
			// thể dừng giữa chừng và để lại sản phẩm ở trạng thái nháp.
			duyet(ctx, svc, cu.ID())
			fmt.Println("sản phẩm đã có, dùng lại")
			return skus[0].ID().String()
		}
	}

	cats, err := cat.GetCategoryTree(ctx)
	if err != nil || len(cats) == 0 {
		panic("không có danh mục nào")
	}

	p, err := svc.CreateProduct(ctx, productapp.CreateProductInput{
		BrandID:             ids.ID(brandID),
		CategoryID:          ids.ID(cats[0].ID),
		Name:                "Áo thun cổ tròn Basics",
		Slug:                slug,
		Description:         "Áo thun cotton 100%, form regular, mặc hằng ngày.",
		MaterialComposition: "100% cotton",
		CareInstructions:    "Giặt máy ở 30°C, không dùng chất tẩy",
		OriginCountry:       "VN",
		ProductType:         productdom.ProductTypeTop,
		GenderTarget:        productdom.GenderUnisex,
		Images:              []string{"https://cdn.example.com/products/ao-thun-1.jpg"},
		SizeChartID:         ids.ID(chartID),
	})
	if err != nil {
		panic(fmt.Sprintf("tạo sản phẩm: %v", err))
	}

	if _, err := svc.AddVariant(ctx, productapp.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Đen", "size": "M"},
		Images:     []string{"https://cdn.example.com/products/ao-thun-1.jpg"},
		SKUs: []productapp.NewSKUInput{{
			Code:       "BSC-TEE-BLK-M",
			WeightGram: 200,
			Dimensions: productdom.Dimensions{LengthMM: 250, WidthMM: 200, HeightMM: 30},
		}},
	}); err != nil {
		panic(fmt.Sprintf("thêm biến thể: %v", err))
	}

	duyet(ctx, svc, p.ID())

	saved, err := svc.GetProduct(ctx, p.ID())
	if err != nil {
		panic(err)
	}
	skus := saved.SKUs()
	if len(skus) == 0 {
		panic("sản phẩm vừa tạo không có SKU")
	}
	return skus[0].ID().String()
}

// dungBangSize tạo bảng size cho thương hiệu nếu chưa có.
//
// Số đo THẬT, không chỉ ký hiệu S/M/L: sai size là nguyên nhân hoàn hàng
// số một trong thời trang, và ký hiệu không nói được gì về vòng ngực.
func dungBangSize(ctx context.Context, cat *catalog.Module, brandID string) string {
	if sc, err := cat.GetSizeChartFor(ctx, brandID, "TOP"); err == nil && sc != nil {
		return sc.ID
	}

	sc, err := cat.Service().CreateSizeChart(ctx, catalogapp.CreateSizeChartInput{
		BrandID:     ids.ID(brandID),
		ProductType: catalogdom.ProductTypeTop,
		System:      catalogdom.SizeSystemAlpha,
		Note:        "Số đo cơ thể, đơn vị cm",
		Entries: []catalogdom.SizeEntry{
			{Size: "S", Measurements: map[string]string{"nguc": "86-90", "eo": "70-74"}},
			{Size: "M", Measurements: map[string]string{"nguc": "90-94", "eo": "74-78"}},
			{Size: "L", Measurements: map[string]string{"nguc": "94-98", "eo": "78-82"}},
		},
	})
	if err != nil {
		panic(fmt.Sprintf("tạo bảng size: %v", err))
	}
	fmt.Println("bảng size:", sc.ID().String())
	return sc.ID().String()
}

// duyet đưa sản phẩm tới trạng thái bán được.
//
// Bỏ qua lỗi ở từng bước: chạy lại thì sản phẩm có thể đã ở bước sau, và
// "chuyển trạng thái không hợp lệ" khi đó là bình thường.
func duyet(ctx context.Context, svc *productapp.Service, id ids.ID) {
	// GHI LẠI lỗi thay vì nuốt: nếu cả hai bước đều hỏng, sản phẩm ở lại
	// DRAFT và lỗi hiện ra tận bước tạo offer với thông điệp "SKU không
	// còn được kinh doanh" — chẳng liên quan gì tới nguyên nhân thật.
	if _, err := svc.SubmitForReview(ctx, id); err != nil {
		fmt.Printf("  gửi duyệt: %v\n", err)
	}
	if _, err := svc.Approve(ctx, id); err != nil {
		fmt.Printf("  duyệt: %v\n", err)
	}
}

// dungOffer cho nhà bán chào bán SKU đó, kèm hàng trong kho.
//
// Chủ sở hữu tồn kho là CHÍNH nhà bán (họ không phải seller nội bộ), theo
// đúng quy tắc ở inventory.OwnerForSeller — ADR-0012.
func dungOffer(
	ctx context.Context, mkt *marketplace.Module, inv *inventory.Module,
	sellerID, skuID string,
) {
	sid := ids.ID(sellerID)
	sku := ids.ID(skuID)

	gia, err := money.New(259_000, money.VND)
	if err != nil {
		panic(err)
	}

	o, err := mkt.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID:             sku,
		SellerID:          sid,
		Price:             gia,
		Condition:         marketdom.ConditionNew,
		HandlingTimeHours: 48,
		MinOrderQuantity:  1,
		MaxOrderQuantity:  5,
		Activate:          true,
	})
	switch {
	case err == nil:
		fmt.Println("offer   :", o.ID().String(), "259.000đ")
	case errors.Is(err, marketdom.ErrDuplicateActiveOffer):
		fmt.Println("offer đã có, dùng lại")
	default:
		panic(fmt.Sprintf("tạo offer: %v", err))
	}

	loc, err := inv.EnsureLocation(ctx,
		"Kho "+tenNhaBan, "SELLER-"+sellerID, "SELLER")
	if err != nil {
		panic(err)
	}
	if _, err := inv.Receive(ctx, inventory.ReceiveRequest{
		SKUID:       skuID,
		LocationID:  loc,
		OwnerID:     inventory.OwnerForSeller(sellerID, false),
		Quantity:    40,
		ReferenceID: "dev-tool",
		PerformedBy: "dev-tool",
	}); err != nil {
		panic(fmt.Sprintf("nhập kho: %v", err))
	}
	fmt.Println("tồn kho : 40 (chủ sở hữu = chính nhà bán)")
}
