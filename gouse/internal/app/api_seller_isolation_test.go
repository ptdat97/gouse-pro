package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	catalogapp "github.com/fashion-commerce/platform/internal/modules/catalog/application"
	catalogdom "github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	productapp "github.com/fashion-commerce/platform/internal/modules/product/application"
	productdom "github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// nhaBanThu là một nhà bán đã ACTIVE kèm tài khoản đăng nhập được.
type nhaBanThu struct {
	sellerID string
	token    string
}

// dungNhaBan đưa một nhà bán qua ĐỦ vòng đời rồi cấp token.
//
// Không ghi thẳng ACTIVE vào database: một nhà bán không bao giờ tồn tại
// được ngoài đời sẽ khiến bài test nói dối. Vai trò gán KÈM PHẠM VI — đó
// mới là thứ bài test này kiểm.
func dungNhaBan(t *testing.T, a *apiTest, ten string) nhaBanThu {
	t.Helper()
	ctx := context.Background()

	v, err := a.mods.seller.ApplyAsSeller(ctx, seller.ApplicationRequest{
		Name:             ten,
		Slug:             ten,
		SellerType:       "BUSINESS",
		LegalName:        "Công ty " + ten,
		TaxCode:          "0300000000",
		Email:            ten + "@apitest.local",
		Phone:            "+84900000000",
		CommissionRateBP: 1000,
	})
	if err != nil {
		t.Fatalf("nộp hồ sơ %s: %v", ten, err)
	}

	svc := a.mods.seller.Service()
	sid := ids.ID(v.ID)
	if _, err := svc.SubmitForReview(ctx, sid); err != nil {
		t.Fatalf("gửi duyệt: %v", err)
	}
	if _, err := svc.Approve(ctx, sid, "test"); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	if _, err := svc.VerifyBankAccount(ctx, sid); err != nil {
		t.Fatalf("xác minh ngân hàng: %v", err)
	}
	if _, err := svc.Activate(ctx, sid); err != nil {
		t.Fatalf("kích hoạt: %v", err)
	}

	email := ten + "-owner@apitest.local"
	const matKhau = "MatKhauDuDai@2026"
	u, err := a.mods.identity.Register(ctx, identity.RegisterRequest{
		Email: email, Password: matKhau,
	})
	if err != nil {
		t.Fatalf("tạo tài khoản: %v", err)
	}

	// PHẠM VI là thứ quyết định: vai trò không phạm vi nghĩa là người này
	// làm chủ MỌI gian hàng — đúng lỗ hổng bài test này tồn tại để chặn.
	if err := a.mods.identity.GrantRole(ctx, u.ID, "SELLER_OWNER", v.ID); err != nil {
		t.Fatalf("gán vai trò: %v", err)
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": matKhau}, nil)
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập %s: %s", email, res.raw)
	}
	return nhaBanThu{sellerID: v.ID, token: tok}
}

// TestNhaBanKhongChamDuocTaiNguyenCuaNhaBanKhac — bất biến BẢO MẬT.
//
// # Vì sao kiểm vai trò là KHÔNG ĐỦ
//
// Cả hai nhà bán đều có vai trò `SELLER_OWNER` hợp lệ, nên middleware
// `RequireRole` cho cả hai đi qua. Thứ phân biệt họ là PHẠM VI của vai trò
// và QUYỀN SỞ HỮU tài nguyên — hai lớp mà middleware không nhìn tới.
//
// Đây là lỗ hổng IDOR kinh điển của một cái chợ: đoán mã tài nguyên của
// đối thủ rồi đọc giá vốn, sửa giá bán, hoặc đổi số tồn kho của họ.
//
// # Vì sao 404 chứ không phải 403
//
// Phân biệt "không có quyền" với "không tồn tại" cho kẻ dò biết mã nào CÓ
// THẬT. Với một cái chợ, đó là rò rỉ danh sách hàng của đối thủ.
func TestNhaBanKhongChamDuocTaiNguyenCuaNhaBanKhac(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	A := dungNhaBan(t, a, "nhabana")
	B := dungNhaBan(t, a, "nhabanb")

	// Nhà bán A có một offer và một bản ghi tồn kho THẬT.
	//
	// SKU phải thuộc thương hiệu MỞ: dữ liệu mẫu chỉ có một sản phẩm và
	// nó thuộc thương hiệu RESTRICTED — chỉ chủ thương hiệu bán được. Đó
	// là quy tắc đúng, nên bài test dựng hàng của mình thay vì lách.
	skuID := dungSkuThuongHieuMo(t, a)

	gia, _ := money.New(199_000, money.VND)
	offer, err := a.mods.marketplace.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID: ids.ID(skuID), SellerID: ids.ID(A.sellerID), Price: gia,
		Condition: marketdom.ConditionNew, HandlingTimeHours: 24,
		MinOrderQuantity: 1, Activate: true,
	})
	if err != nil {
		t.Skipf("không tạo được offer cho nhà bán A: %v", err)
	}

	loc, err := a.mods.inventory.EnsureLocation(ctx, "Kho A", "SELLER-"+A.sellerID, "SELLER")
	if err != nil {
		t.Fatalf("tạo kho: %v", err)
	}
	if _, err := a.mods.inventory.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: loc,
		OwnerID:  inventory.OwnerForSeller(A.sellerID, false),
		Quantity: 25, PerformedBy: "test",
	}); err != nil {
		t.Fatalf("nhập kho cho A: %v", err)
	}

	// Mọi đường mà nhà bán B có thể thử chạm vào tài nguyên của A.
	thu := []struct {
		ten    string
		method string
		path   string
		body   any
	}{
		{
			"đọc offer của A",
			http.MethodPatch, "/api/v1/seller/offers/" + offer.ID().String(),
			map[string]any{"price": map[string]any{"amount": 1000, "currency": "VND"}},
		},
		{
			"đổi tồn kho của A",
			http.MethodPut, "/api/v1/seller/inventory/" + skuID,
			map[string]any{"quantity_available": 0, "reason": "pha hoai doi thu"},
		},
		{
			"ngừng bán offer của A",
			http.MethodPatch, "/api/v1/seller/offers/" + offer.ID().String(),
			map[string]any{"status": "ARCHIVED"},
		},
	}

	for _, tt := range thu {
		t.Run(tt.ten, func(t *testing.T) {
			h := khoaIdem()
			h["Authorization"] = "Bearer " + B.token
			got := a.call(tt.method, tt.path, tt.body, h)

			if got.code == http.StatusOK || got.code == http.StatusCreated {
				t.Fatalf("nhà bán B CHẠM ĐƯỢC tài nguyên của A: HTTP %d — %s",
					got.code, got.raw)
			}
			if got.code != http.StatusNotFound {
				t.Errorf("HTTP %d, cần 404 — phân biệt 403 với 404 cho kẻ dò "+
					"biết mã nào có thật: %s", got.code, got.raw)
			}
		})
	}

	// Và tài nguyên của A phải NGUYÊN VẸN sau mọi lần thử.
	t.Run("tài nguyên của A không đổi", func(t *testing.T) {
		sau, err := a.mods.marketplace.Service().GetOffer(ctx, offer.ID())
		if err != nil {
			t.Fatalf("đọc lại offer: %v", err)
		}
		if sau.Price().Amount() != 199_000 {
			t.Errorf("giá offer của A bị đổi thành %d", sau.Price().Amount())
		}
		if string(sau.Status()) != "ACTIVE" {
			t.Errorf("trạng thái offer của A bị đổi thành %s", sau.Status())
		}

		items, err := a.mods.inventory.GetItemsBySKUs(ctx, []string{skuID}, "")
		if err != nil {
			t.Fatalf("đọc tồn kho: %v", err)
		}
		for _, it := range items[skuID] {
			if it.OwnerID == inventory.OwnerForSeller(A.sellerID, false) &&
				it.Available != 25 {
				t.Errorf("tồn kho của A bị đổi thành %d, cần 25", it.Available)
			}
		}
	})
}

// dungSkuThuongHieuMo tạo một SKU mà NHÀ BÁN NGOÀI được phép chào bán.
func dungSkuThuongHieuMo(t *testing.T, a *apiTest) string {
	t.Helper()
	ctx := context.Background()

	brands, err := a.mods.catalog.Service().ListBrands(ctx, catalogdom.BrandFilter{Limit: 50})
	if err != nil {
		t.Fatalf("đọc thương hiệu: %v", err)
	}
	var brandID ids.ID
	for _, b := range brands {
		if string(b.ProtectionLevel()) == "OPEN" {
			brandID = b.ID()
			break
		}
	}
	if brandID.IsZero() {
		t.Skip("dữ liệu mẫu không có thương hiệu OPEN nào")
	}

	// Bảng size là BẮT BUỘC với hàng thời trang, và phải thuộc CHÍNH
	// thương hiệu đó — "M" của hãng này không phải "M" của hãng kia.
	//
	// DÙNG LẠI bảng đã có nếu có: mỗi thương hiệu chỉ được một bảng cho
	// mỗi loại sản phẩm, nên helper này gọi lần thứ hai trong cùng một
	// database sẽ va vào ràng buộc đó. Hai bài test khác nhau đều cần một
	// SKU thuộc thương hiệu mở là chuyện bình thường.
	chart, err := a.mods.catalog.Service().CreateSizeChart(ctx,
		catalogapp.CreateSizeChartInput{
			BrandID: brandID, ProductType: catalogdom.ProductTypeTop,
			System: catalogdom.SizeSystemAlpha,
			Entries: []catalogdom.SizeEntry{
				{Size: "M", Measurements: map[string]string{"nguc": "90-94"}},
			},
		})
	if err != nil {
		san, loiTim := a.mods.catalog.Service().GetSizeChartFor(
			ctx, brandID, catalogdom.ProductTypeTop)
		if loiTim != nil {
			t.Fatalf("tạo bảng size: %v", err)
		}
		chart = san
	}

	cats, err := a.mods.catalog.GetCategoryTree(ctx)
	if err != nil || len(cats) == 0 {
		t.Skip("không có danh mục")
	}

	psvc := a.mods.product.Service()
	p, err := psvc.CreateProduct(ctx, productapp.CreateProductInput{
		BrandID: brandID, CategoryID: ids.ID(cats[0].ID),
		Name: "Hàng thử cách ly",
		// Lấy ĐUÔI của ULID chứ không phải đầu: mười ký tự đầu là dấu
		// thời gian, nên hai lần gọi cách nhau vài mili-giây cho cùng
		// một chuỗi và slug bị trùng.
		Slug: "hang-thu-cach-ly-" + strings.ToLower(
			ids.MustNew(ids.PrefixRequest).String()[22:]),
		Description:         "Dựng riêng cho bài test cách ly nhà bán.",
		MaterialComposition: "100% cotton",
		CareInstructions:    "Giặt máy 30°C",
		OriginCountry:       "VN",
		ProductType:         productdom.ProductTypeTop,
		GenderTarget:        productdom.GenderUnisex,
		Images:              []string{"https://cdn.example.com/t.jpg"},
		SizeChartID:         chart.ID(),
	})
	if err != nil {
		t.Fatalf("tạo sản phẩm: %v", err)
	}
	if _, err := psvc.AddVariant(ctx, productapp.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Đen", "size": "M"},
		Images:     []string{"https://cdn.example.com/t.jpg"},
		SKUs: []productapp.NewSKUInput{{
			// Đuôi ULID, cùng lý do với slug ở trên.
			Code: "ISO-" + ids.MustNew(ids.PrefixRequest).String()[22:], WeightGram: 200,
			Dimensions: productdom.Dimensions{LengthMM: 200, WidthMM: 150, HeightMM: 20},
		}},
	}); err != nil {
		t.Fatalf("thêm biến thể: %v", err)
	}
	if _, err := psvc.SubmitForReview(ctx, p.ID()); err != nil {
		t.Fatalf("gửi duyệt: %v", err)
	}
	if _, err := psvc.Approve(ctx, p.ID()); err != nil {
		t.Fatalf("duyệt: %v", err)
	}

	saved, err := psvc.GetProduct(ctx, p.ID())
	if err != nil {
		t.Fatalf("đọc lại sản phẩm: %v", err)
	}
	return saved.SKUs()[0].ID().String()
}
