package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	catalogapp "github.com/fashion-commerce/platform/internal/modules/catalog/application"
	catalogdom "github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

// TestNhaBanDangBanSanPhamQuaAPI — P3-25 đi trọn một vòng.
//
// # Vì sao bài này tồn tại
//
// Trước hôm nay, hàng hóa chỉ vào hệ thống được qua `seed.go`: module
// product có đủ máy trạng thái DRAFT → PENDING_REVIEW → ACTIVE cùng
// `SubmitForReview`/`Approve`/`Reject`, đủ use case ở tầng application, và
// KHÔNG một endpoint nào. Lần thứ mười một của dạng lỗi "khai mà không ai
// dùng".
//
// Bài này đi hết đường mà đặc tả
// `docs/07-workflows/product-publishing.md` mục 4 mô tả, qua HTTP thật:
//
//	nhà bán tạo → thêm biến thể → gửi duyệt → vận hành duyệt → khách thấy
//
// Khẳng định KHẢ KIẾN ở từng chặng, không chỉ mã HTTP: một chuỗi trả 200
// ở mọi bước vẫn có thể để hàng chưa duyệt lọt ra cửa hàng.
func TestNhaBanDangBanSanPhamQuaAPI(t *testing.T) {
	a := newAPITest(t)

	sellerID := a.mauNhaBanCoDon(t)
	if sellerID == "" {
		sellerID = a.baoDamCoDonThucHien(t)
	}
	tokNB := a.taoTokenNhaBan(t, sellerID)
	nbHeaders := hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokNB})

	brandID, chartID, catID := a.thuongHieuMoVaBangSize(t)

	// ---- Nhà bán TẠO sản phẩm ----
	slug := "ao-thu-dang-ban-" + strings.ToLower(
		ids.MustNew(ids.PrefixRequest).String()[22:])
	res := a.call(http.MethodPost, "/api/v1/seller/products", map[string]any{
		"brand_id": brandID, "category_id": catID, "size_chart_id": chartID,
		"name": "Áo thử đăng bán", "slug": slug,
		"description":          "Sản phẩm dựng cho bài test đăng bán qua API.",
		"material_composition": "100% cotton",
		"care_instructions":    "Giặt máy 30°C",
		"origin_country":       "VN",
		"product_type":         "TOP",
		"gender_target":        "UNISEX",
		"images":               []string{"https://cdn.example.com/a.jpg"},
	}, nbHeaders)
	if res.code != http.StatusCreated {
		t.Fatalf("tạo sản phẩm: HTTP %d — %s", res.code, res.raw)
	}
	maSP, _ := res.body["id"].(string)
	if maSP == "" {
		t.Fatalf("không trả mã sản phẩm: %s", res.raw)
	}
	if res.body["status"] != "DRAFT" {
		t.Errorf("trạng thái = %v, cần DRAFT — hàng mới tạo không được ra "+
			"cửa hàng ngay", res.body["status"])
	}

	// Khách CHƯA thấy: hàng nháp không được lộ.
	if got := a.call(http.MethodGet, "/api/v1/products/"+maSP, nil, nil); got.code != http.StatusNotFound {
		t.Errorf("khách xem hàng DRAFT: HTTP %d, cần 404", got.code)
	}

	// ---- Gửi duyệt khi CHƯA có biến thể: phải bị chặn ----
	res = a.call(http.MethodPost, "/api/v1/seller/products/"+maSP+"/submit", nil, nbHeaders)
	if res.code < 400 {
		t.Errorf("gửi duyệt khi chưa có biến thể: HTTP %d, cần lỗi — "+
			"CheckReadyForReview tồn tại để chặn đúng chỗ này", res.code)
	}

	// ---- Thêm biến thể, KÈM MÃ MÀU (P3-22) ----
	//
	// Mã màu là nửa còn lại của P3-22, và nó bị chặn cho tới khi có chỗ
	// nhập liệu — tức là cho tới chính commit này. Cố ý nhập chữ THƯỜNG
	// để kiểm luôn phép chuẩn hóa.
	res = a.call(http.MethodPost, "/api/v1/seller/products/"+maSP+"/variants", map[string]any{
		"attributes": map[string]string{
			"color": "Đen", "size": "M", "color_hex": "#1b2a49",
		},
		"images": []string{"https://cdn.example.com/a.jpg"},
		"skus": []map[string]any{{
			"sku_code":    "DB-" + ids.MustNew(ids.PrefixRequest).String()[22:],
			"weight_gram": 200, "length_mm": 200, "width_mm": 150, "height_mm": 20,
		}},
	}, nbHeaders)
	if res.code != http.StatusOK {
		t.Fatalf("thêm biến thể: HTTP %d — %s", res.code, res.raw)
	}

	// ---- Gửi duyệt ----
	res = a.call(http.MethodPost, "/api/v1/seller/products/"+maSP+"/submit", nil, nbHeaders)
	if res.code != http.StatusOK {
		t.Fatalf("gửi duyệt: HTTP %d — %s", res.code, res.raw)
	}
	if res.body["status"] != "PENDING_REVIEW" {
		t.Errorf("trạng thái = %v, cần PENDING_REVIEW", res.body["status"])
	}

	// Vẫn CHƯA ra cửa hàng: chờ duyệt không phải đã duyệt.
	if got := a.call(http.MethodGet, "/api/v1/products/"+maSP, nil, nil); got.code != http.StatusNotFound {
		t.Errorf("khách xem hàng PENDING_REVIEW: HTTP %d, cần 404", got.code)
	}

	// ---- Vận hành DUYỆT ----
	tokVH := a.taoTaiKhoanVaiTro(t, "OPS_MERCHANDISING")
	vhHeaders := hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokVH})

	res = a.call(http.MethodGet, "/api/v1/admin/products/pending", nil, vhHeaders)
	if res.code != http.StatusOK {
		t.Fatalf("xem hàng chờ duyệt: HTTP %d — %s", res.code, res.raw)
	}
	if !coTrongDanhSach(res, maSP) {
		t.Errorf("sản phẩm vừa gửi KHÔNG có trong danh sách chờ duyệt")
	}

	res = a.call(http.MethodPost, "/api/v1/admin/products/"+maSP+"/approve", nil, vhHeaders)
	if res.code != http.StatusOK {
		t.Fatalf("duyệt: HTTP %d — %s", res.code, res.raw)
	}
	if res.body["status"] != "ACTIVE" {
		t.Errorf("trạng thái = %v, cần ACTIVE", res.body["status"])
	}

	// ---- Khách NAY mới thấy, KÈM ô màu thật ----
	got := a.call(http.MethodGet, "/api/v1/products/"+maSP, nil, nil)
	if got.code != http.StatusOK {
		t.Fatalf("khách xem hàng ĐÃ DUYỆT: HTTP %d, cần 200 — %s", got.code, got.raw)
	}

	bienThe, _ := got.body["variants"].([]any)
	if len(bienThe) != 1 {
		t.Fatalf("variants = %v, mong 1", got.body["variants"])
	}
	bt, _ := bienThe[0].(map[string]any)
	if bt["color_hex"] != "#1B2A49" {
		t.Errorf("color_hex = %v, mong #1B2A49 (chữ HOA) — mã màu nhà bán "+
			"nhập phải tới được khách", bt["color_hex"])
	}
}

// TestNhaBanKhongSuaDuocHangCuaNhaBanKhac.
//
// Ranh giới bảo mật quan trọng nhất của mặt GHI. Nó nằm ở tầng application
// (`kiemChuSoHuu`) chứ không ở HTTP: tầng HTTP lấy seller_id từ token,
// nhưng nếu việc so khớp chỉ nằm ở đó thì một đường thêm sau này quên so
// là đủ để nhà bán A sửa hàng của B.
//
// Trả 404 chứ không 403: 403 xác nhận sản phẩm tồn tại, và đó là đủ để dò
// mã hàng chưa phát hành của đối thủ.
func TestNhaBanKhongSuaDuocHangCuaNhaBanKhac(t *testing.T) {
	a := newAPITest(t)

	sellerA := a.mauNhaBanCoDon(t)
	if sellerA == "" {
		sellerA = a.baoDamCoDonThucHien(t)
	}
	tokA := a.taoTokenNhaBan(t, sellerA)
	brandID, chartID, catID := a.thuongHieuMoVaBangSize(t)

	slug := "hang-cua-a-" + strings.ToLower(ids.MustNew(ids.PrefixRequest).String()[22:])
	res := a.call(http.MethodPost, "/api/v1/seller/products", map[string]any{
		"brand_id": brandID, "category_id": catID, "size_chart_id": chartID,
		"name": "Hàng của A", "slug": slug,
		"description":          "Hàng của gian A.",
		"material_composition": "100% cotton",
		"product_type":         "TOP", "gender_target": "UNISEX",
		"images": []string{"https://cdn.example.com/a.jpg"},
	}, hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokA}))
	if res.code != http.StatusCreated {
		t.Fatalf("A tạo sản phẩm: HTTP %d — %s", res.code, res.raw)
	}
	maSP, _ := res.body["id"].(string)

	// Gian hàng B cầm token của mình, thử đụng vào hàng của A.
	sellerB := ids.MustNew(ids.PrefixSeller).String()
	tokB := a.taoTokenNhaBan(t, sellerB)
	bHeaders := hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokB})

	for _, buoc := range []struct {
		ten string
		goi func() reply
	}{
		{"gửi duyệt", func() reply {
			return a.call(http.MethodPost,
				"/api/v1/seller/products/"+maSP+"/submit", nil, bHeaders)
		}},
		{"thêm biến thể", func() reply {
			return a.call(http.MethodPost,
				"/api/v1/seller/products/"+maSP+"/variants", map[string]any{
					"attributes": map[string]string{"color": "Đỏ", "size": "L"},
					"skus": []map[string]any{{
						"sku_code": "B-" + ids.MustNew(ids.PrefixRequest).String()[22:],
					}},
				}, bHeaders)
		}},
	} {
		got := buoc.goi()
		if got.code != http.StatusNotFound {
			t.Errorf("B %s trên hàng của A: HTTP %d, cần 404 — %s",
				buoc.ten, got.code, got.raw)
		}
	}
}

// thuongHieuMoVaBangSize trả thương hiệu OPEN kèm bảng size và danh mục.
func (a *apiTest) thuongHieuMoVaBangSize(t *testing.T) (brandID, chartID, catID string) {
	t.Helper()
	ctx := context.Background()

	brands, err := a.mods.catalog.Service().ListBrands(ctx, catalogdom.BrandFilter{Limit: 50})
	if err != nil {
		t.Fatalf("đọc thương hiệu: %v", err)
	}
	var bid ids.ID
	for _, b := range brands {
		if string(b.ProtectionLevel()) == "OPEN" {
			bid = b.ID()
			break
		}
	}
	if bid.IsZero() {
		t.Skip("dữ liệu mẫu không có thương hiệu OPEN nào")
	}

	// Mỗi thương hiệu chỉ một bảng size cho mỗi loại hàng — dùng lại nếu có.
	chart, err := a.mods.catalog.Service().CreateSizeChart(ctx,
		catalogapp.CreateSizeChartInput{
			BrandID: bid, ProductType: catalogdom.ProductTypeTop,
			System: catalogdom.SizeSystemAlpha,
			Entries: []catalogdom.SizeEntry{
				{Size: "M", Measurements: map[string]string{"nguc": "90-94"}},
			},
		})
	if err != nil {
		san, loiTim := a.mods.catalog.Service().GetSizeChartFor(
			ctx, bid, catalogdom.ProductTypeTop)
		if loiTim != nil {
			t.Fatalf("tạo bảng size: %v", err)
		}
		chart = san
	}

	cats, err := a.mods.catalog.GetCategoryTree(ctx)
	if err != nil || len(cats) == 0 {
		t.Skip("không có danh mục")
	}
	return bid.String(), chart.ID().String(), cats[0].ID
}

func coTrongDanhSach(res reply, maSP string) bool {
	ds, _ := res.body["data"].([]any)
	for _, item := range ds {
		m, _ := item.(map[string]any)
		if m["id"] == maSP {
			return true
		}
	}
	return false
}
