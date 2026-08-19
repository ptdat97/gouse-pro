package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	producthttp "github.com/fashion-commerce/platform/internal/modules/product/interfaces/http"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

// catalogOK cho phép mọi thao tác — test HTTP tập trung vào tầng trình bày.
type catalogOK struct{}

func (catalogOK) BrandExists(context.Context, ids.ID) (bool, error) { return true, nil }
func (catalogOK) CanSellerSellBrand(context.Context, ids.ID, ids.ID) (bool, string, error) {
	return true, "OK", nil
}
func (catalogOK) SizeChartExistsFor(context.Context, ids.ID, string) (ids.ID, bool, error) {
	return "", false, nil
}

func newServer(t *testing.T) (*http.ServeMux, *application.Service) {
	t.Helper()
	// Dựng service qua application, KHÔNG import infrastructure trực tiếp:
	// tầng interfaces không được biết tới kho lưu trữ (quy tắc R8).
	svc := application.NewInMemoryService(catalogOK{}, application.FixedClock{T: testNow})
	mux := http.NewServeMux()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	producthttp.NewHandler(svc, log).Register(mux)
	return mux, svc
}

// taoSanPham tạo sản phẩm, tùy chọn duyệt luôn.
func taoSanPham(t *testing.T, svc *application.Service, slug string, duyet bool) *domain.Product {
	t.Helper()
	ctx := context.Background()

	p, err := svc.CreateProduct(ctx, application.CreateProductInput{
		BrandID:             ids.MustNew(ids.PrefixBrand),
		CategoryID:          ids.MustNew(ids.PrefixCategory),
		SizeChartID:         ids.MustNew(ids.PrefixSizeChart),
		Name:                "Áo sơ mi linen Oxford",
		Slug:                slug,
		Description:         "Áo sơ mi vải linen, form suông",
		MaterialComposition: "80% cotton, 20% linen",
		CareInstructions:    "Giặt máy ở 30°C",
		OriginCountry:       "VN",
		ProductType:         domain.ProductTypeTop,
		GenderTarget:        domain.GenderMen,
		Images:              []string{"https://cdn.example.com/1.jpg"},
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	if _, err := svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Trắng", "size": "M"},
		Images:     []string{"https://cdn.example.com/trang.jpg"},
		SKUs: []application.NewSKUInput{{
			Code:       "SKU-" + slug + "-M",
			WeightGram: 320,
			Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		}},
	}); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}

	if duyet {
		if _, err := svc.SubmitForReview(ctx, p.ID()); err != nil {
			t.Fatalf("SubmitForReview: %v", err)
		}
		if _, err := svc.Approve(ctx, p.ID()); err != nil {
			t.Fatalf("Approve: %v", err)
		}
	}

	got, err := svc.GetProduct(ctx, p.ID())
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	return got
}

func do(t *testing.T, mux *http.ServeMux, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSON không hợp lệ: %v\nnội dung: %s", err, rec.Body.String())
		}
	}
	return rec, body
}

// errPayload lấy phần `error` trong response lỗi.
//
// Đặc tả bọc lỗi trong `{"error": {...}, "request_id": "..."}` —
// xem api/components/common.yaml#/schemas/Error.
func errPayload(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, ok := body["error"]
	if !ok {
		t.Fatalf("response lỗi thiếu trường `error`: %v", body)
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("`error` không phải object: %v", raw)
	}
	// request_id LUÔN phải có — dùng để tra ngược khi hỗ trợ khách hàng.
	if body["request_id"] == nil {
		t.Error("response lỗi thiếu request_id")
	}
	return payload
}

func TestChiTietSanPhamKhopDacTa(t *testing.T) {
	mux, svc := newServer(t)
	p := taoSanPham(t, svc, "ao-so-mi", true)

	rec, body := do(t, mux, "/api/v1/products/"+p.ID().String())
	if rec.Code != http.StatusOK {
		t.Fatalf("mã = %d, mong 200. Nội dung: %s", rec.Code, rec.Body.String())
	}

	// Tên trường phải khớp đặc tả (snake_case), không phải tên struct Go.
	for _, field := range []string{
		"id", "name", "material_composition", "care_instructions",
		"origin_country", "product_type", "gender_target", "variants",
	} {
		if _, ok := body[field]; !ok {
			t.Errorf("thiếu trường %q trong response", field)
		}
	}

	// Ba trường đặc thù thời trang phải có giá trị thật.
	if body["material_composition"] != "80% cotton, 20% linen" {
		t.Errorf("material_composition = %v", body["material_composition"])
	}

	variants, ok := body["variants"].([]any)
	if !ok || len(variants) != 1 {
		t.Fatalf("variants = %v", body["variants"])
	}
	v := variants[0].(map[string]any)
	if v["color"] != "Trắng" {
		t.Errorf("color = %v, mong Trắng", v["color"])
	}

	skus, ok := v["skus"].([]any)
	if !ok || len(skus) != 1 {
		t.Fatalf("skus = %v", v["skus"])
	}
	sku := skus[0].(map[string]any)
	// Size phải được truyền xuống mức SKU để client dựng danh sách chọn size.
	if sku["size"] != "M" {
		t.Errorf("size = %v, mong M", sku["size"])
	}
	if sku["sku_code"] == "" || sku["sku_code"] == nil {
		t.Error("thiếu sku_code")
	}

	// KHÔNG được bịa trường thuộc module khác.
	for _, notYet := range []string{"price_from", "buy_box_offer", "rating"} {
		if _, ok := body[notYet]; ok {
			t.Errorf("trả trường %q khi module sở hữu chưa tồn tại", notYet)
		}
	}
	if _, ok := sku["available"]; ok {
		t.Error("trả `available` khi chưa có module inventory — khách sẽ đặt hàng không có thật")
	}
}

// Sản phẩm chưa duyệt phải trả 404, KHÔNG phải 403.
//
// 403 xác nhận tài nguyên tồn tại — đủ để đối thủ dò id và biết ta sắp bán gì.
func TestSanPhamChuaDuyetTra404(t *testing.T) {
	mux, svc := newServer(t)
	p := taoSanPham(t, svc, "con-nhap", false)

	rec, body := do(t, mux, "/api/v1/products/"+p.ID().String())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mã = %d, mong 404", rec.Code)
	}
	payload := errPayload(t, body)
	if payload["code"] != "NOT_FOUND" {
		t.Errorf("code = %v, mong NOT_FOUND", payload["code"])
	}

	// Thông báo lỗi không được lộ việc sản phẩm có tồn tại.
	msg, _ := payload["message"].(string)
	for _, cam := range []string{"nháp", "DRAFT", "chưa duyệt", "chưa xuất bản"} {
		if strings.Contains(msg, cam) {
			t.Errorf("thông báo lỗi %q lộ trạng thái sản phẩm", msg)
		}
	}
}

// Tên thương hiệu thuộc module catalog. Trả `"name": ""` tệ hơn là bỏ hẳn
// trường: chuỗi rỗng trông như dữ liệu hợp lệ và hiển thị thành khoảng
// trắng trên trang, còn trường thiếu thì client biết phải lấy từ nguồn khác.
func TestKhongTraTenRongChoThamChieuNgoaiModule(t *testing.T) {
	mux, svc := newServer(t)
	p := taoSanPham(t, svc, "kiem-tra-ten", true)

	_, body := do(t, mux, "/api/v1/products/"+p.ID().String())

	brand, ok := body["brand"].(map[string]any)
	if !ok {
		t.Fatalf("thiếu brand: %v", body["brand"])
	}
	if brand["id"] == nil || brand["id"] == "" {
		t.Error("brand phải có id để client tra tên")
	}
	if name, có := brand["name"]; có && name == "" {
		t.Error(`trả "name": "" — phải bỏ hẳn trường khi chưa biết tên`)
	}
}

func TestIDSaiDinhDangTra400(t *testing.T) {
	mux, _ := newServer(t)

	for _, id := range []string{"khong-phai-id", "brd_01J9XABC123DEF456GHJKMNPQR", "123"} {
		rec, body := do(t, mux, "/api/v1/products/"+id)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("id %q: mã = %d, mong 400", id, rec.Code)
			continue
		}
		if got := errPayload(t, body)["code"]; got != "VALIDATION_FAILED" {
			t.Errorf("id %q: code = %v, mong VALIDATION_FAILED", id, got)
		}
	}
}

func TestKhongTimThayTra404(t *testing.T) {
	mux, _ := newServer(t)
	rec, _ := do(t, mux, "/api/v1/products/"+ids.MustNew(ids.PrefixProduct).String())
	if rec.Code != http.StatusNotFound {
		t.Errorf("mã = %d, mong 404", rec.Code)
	}
}

// Endpoint công khai KHÔNG được cho lọc theo trạng thái qua query string —
// `?status=DRAFT` sẽ lộ toàn bộ hàng chưa duyệt của mọi seller.
func TestDanhSachKhongLoSanPhamChuaDuyet(t *testing.T) {
	mux, svc := newServer(t)
	taoSanPham(t, svc, "da-duyet", true)
	taoSanPham(t, svc, "con-nhap", false)

	for _, path := range []string{
		"/api/v1/products",
		"/api/v1/products?status=DRAFT",
		"/api/v1/products?only_visible=false",
	} {
		rec, body := do(t, mux, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: mã = %d", path, rec.Code)
		}
		data, ok := body["data"].([]any)
		if !ok {
			t.Fatalf("%s: thiếu mảng data", path)
		}
		if len(data) != 1 {
			t.Errorf("%s: %d sản phẩm, mong 1 — lộ hàng chưa duyệt", path, len(data))
		}
		for _, item := range data {
			m := item.(map[string]any)
			if m["slug"] == "con-nhap" {
				t.Errorf("%s: lộ sản phẩm chưa duyệt", path)
			}
		}
	}
}

func TestDanhSachTraMangRongChuKhongPhaiNull(t *testing.T) {
	mux, _ := newServer(t)

	rec, body := do(t, mux, "/api/v1/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("mã = %d", rec.Code)
	}
	// `null` bắt client phải kiểm tra thêm; `[]` thì lặp thẳng được.
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("data = %v, mong mảng rỗng", body["data"])
	}
	if len(data) != 0 {
		t.Errorf("số phần tử = %d, mong 0", len(data))
	}
}

func TestLocSaiDinhDangTra400(t *testing.T) {
	mux, _ := newServer(t)

	for _, q := range []string{"brand_id=sai", "category_id=sai", "collection_id=sai"} {
		rec, _ := do(t, mux, "/api/v1/products?"+q)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: mã = %d, mong 400", q, rec.Code)
		}
	}
}

// Sản phẩm có NHIỀU biến thể phải trả đủ, mỗi biến thể kèm size của nó.
//
// Đây là dữ liệu client dùng để dựng bộ chọn màu/size — sai ở đây thì
// khách không chọn được size, hoặc chọn nhầm.
func TestNhieuBienTheTraDuMauVaSize(t *testing.T) {
	mux, svc := newServer(t)
	ctx := context.Background()
	p := taoSanPham(t, svc, "nhieu-bien-the", false)

	// Thêm biến thể thứ hai: khác màu, khác size.
	if _, err := svc.AddVariant(ctx, application.AddVariantInput{
		ProductID:  p.ID(),
		Attributes: map[string]string{"color": "Đen", "size": "L"},
		SKUs: []application.NewSKUInput{{
			Code: "SKU-NHIEU-BIEN-THE-L", WeightGram: 340,
			Dimensions: domain.Dimensions{LengthMM: 300, WidthMM: 220, HeightMM: 40},
		}},
	}); err != nil {
		t.Fatalf("AddVariant: %v", err)
	}
	if _, err := svc.SubmitForReview(ctx, p.ID()); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if _, err := svc.Approve(ctx, p.ID()); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	rec, body := do(t, mux, "/api/v1/products/"+p.ID().String())
	if rec.Code != http.StatusOK {
		t.Fatalf("mã = %d", rec.Code)
	}

	variants := body["variants"].([]any)
	if len(variants) != 2 {
		t.Fatalf("số biến thể = %d, mong 2", len(variants))
	}

	// Mỗi biến thể phải mang đúng size của MÌNH, không phải size của biến
	// thể đầu tiên.
	theoMau := map[string]string{}
	for _, raw := range variants {
		v := raw.(map[string]any)
		skus := v["skus"].([]any)
		if len(skus) != 1 {
			t.Fatalf("biến thể %v có %d SKU, mong 1", v["color"], len(skus))
		}
		theoMau[v["color"].(string)] = skus[0].(map[string]any)["size"].(string)
	}
	if theoMau["Trắng"] != "M" {
		t.Errorf("size của màu Trắng = %q, mong M", theoMau["Trắng"])
	}
	if theoMau["Đen"] != "L" {
		t.Errorf("size của màu Đen = %q, mong L", theoMau["Đen"])
	}

	// Danh sách phải gom được màu có sẵn.
	_, list := do(t, mux, "/api/v1/products")
	item := list["data"].([]any)[0].(map[string]any)
	colors := item["available_colors"].([]any)
	if len(colors) != 2 {
		t.Errorf("số màu = %d, mong 2", len(colors))
	}
}

// ---------------------------------------------------- Tra theo lô bằng `ids`

// TRA NHIỀU SẢN PHẨM trong MỘT lượt gọi.
//
// Không có đường này thì trang có danh sách mã (rõ nhất là yêu thích) phải
// gọi getProduct cho từng mã — 30 món là 30 lượt đi-về, đúng vấn đề N+1 mà
// GetProductsByIDs sinh ra để tránh.
func TestTraNhieuSanPhamTheoMa(t *testing.T) {
	mux, svc := newServer(t)

	a := taoSanPham(t, svc, "ao-so-mi-a", true)
	b := taoSanPham(t, svc, "ao-so-mi-b", true)

	_, body := do(t, mux, "/api/v1/products?ids="+a.ID().String()+","+b.ID().String())
	data := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("trả về %d sản phẩm, mong 2", len(data))
	}
}

// GIỮ ĐÚNG THỨ TỰ mã được hỏi.
//
// Bên gọi đã sắp xếp danh sách của họ (yêu thích theo thời gian thêm). Trả
// về thứ tự khác buộc họ sắp lại — hoặc tệ hơn, họ không nhận ra và hiển
// thị sai thứ tự.
func TestGiuDungThuTuMaDuocHoi(t *testing.T) {
	mux, svc := newServer(t)

	// SÁU sản phẩm, không phải hai. Duyệt map trong Go trả thứ tự ngẫu
	// nhiên, nên với hai phần tử thì một cài đặt SAI vẫn đúng 50% số lần —
	// test sẽ chập chờn thay vì bắt lỗi. Sáu phần tử thì xác suất trùng
	// ngẫu nhiên là 1/720.
	const n = 6
	made := make([]string, 0, n)
	for i := 0; i < n; i++ {
		p := taoSanPham(t, svc, "thu-tu-"+strconv.Itoa(i), true)
		made = append(made, p.ID().String())
	}

	// Hỏi NGƯỢC thứ tự tạo.
	want := make([]string, 0, n)
	for i := n - 1; i >= 0; i-- {
		want = append(want, made[i])
	}

	_, body := do(t, mux, "/api/v1/products?ids="+strings.Join(want, ","))
	data := body["data"].([]any)
	if len(data) != n {
		t.Fatalf("trả về %d sản phẩm, mong %d", len(data), n)
	}

	for i, item := range data {
		got := item.(map[string]any)["id"]
		if got != want[i] {
			t.Fatalf("vị trí %d: %v, mong %v — thứ tự không theo mã được hỏi",
				i, got, want[i])
		}
	}
}

// SẢN PHẨM CHƯA DUYỆT không lọt ra, kể cả khi hỏi ĐÍCH DANH mã của nó.
//
// Đây là điểm dễ sót nhất của đường tra theo lô: bộ lọc `OnlyVisible` nằm ở
// truy vấn danh sách, còn đường này đi thẳng vào FindByIDs và bỏ qua nó.
func TestSanPhamChuaDuyetKhongLotQuaTraTheoMa(t *testing.T) {
	mux, svc := newServer(t)

	nhap := taoSanPham(t, svc, "con-nhap", false)
	daDuyet := taoSanPham(t, svc, "da-duyet", true)

	_, body := do(t, mux,
		"/api/v1/products?ids="+nhap.ID().String()+","+daDuyet.ID().String())
	data := body["data"].([]any)

	if len(data) != 1 {
		t.Fatalf("trả về %d sản phẩm, mong 1 — hàng chưa duyệt đã lọt ra", len(data))
	}
	if got := data[0].(map[string]any)["id"]; got != daDuyet.ID().String() {
		t.Errorf("sản phẩm trả về = %v, mong %v", got, daDuyet.ID())
	}
}

// MÃ KHÔNG TỒN TẠI thì VẮNG MẶT, không phải ô rỗng.
//
// Trả về phần tử rỗng buộc mọi vòng lặp ở giao diện phải kiểm tra null.
func TestMaKhongTonTaiThiVangMat(t *testing.T) {
	mux, svc := newServer(t)

	a := taoSanPham(t, svc, "co-that", true)
	khong := ids.MustNew(ids.PrefixProduct)

	_, body := do(t, mux, "/api/v1/products?ids="+a.ID().String()+","+khong.String())
	if data := body["data"].([]any); len(data) != 1 {
		t.Errorf("trả về %d sản phẩm, mong 1", len(data))
	}
}

// MÃ SAI ĐỊNH DẠNG bị từ chối thay vì âm thầm bỏ qua.
func TestMaSaiDinhDangBiTuChoi(t *testing.T) {
	mux, _ := newServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products?ids=khong-phai-ma", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, mong 400", rec.Code)
	}
}

// QUÁ NHIỀU MÃ bị từ chối.
//
// Không giới hạn thì một request kéo cả bảng sản phẩm, và mệnh đề IN với
// hàng nghìn phần tử là truy vấn chậm mà không chỉ mục nào cứu được.
func TestQuaNhieuMaBiTuChoi(t *testing.T) {
	mux, _ := newServer(t)

	list := make([]string, 0, 101)
	for i := 0; i < 101; i++ {
		list = append(list, ids.MustNew(ids.PrefixProduct).String())
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/products?ids="+strings.Join(list, ","), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, mong 400", rec.Code)
	}
}
