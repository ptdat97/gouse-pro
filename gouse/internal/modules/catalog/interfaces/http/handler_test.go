package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

type fixture struct {
	h            http.Handler
	brandID      ids.ID
	launchedID   ids.ID
	unlaunchedID ids.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()

	// Dựng service qua application, KHÔNG import trực tiếp infrastructure —
	// tầng interfaces không được phép biết infrastructure (quy tắc R8).
	svc := application.NewInMemoryService(application.FixedClock{T: testNow})

	brand, err := svc.CreateBrand(ctx, application.CreateBrandInput{
		Name:            "Lumière",
		Slug:            "lumiere",
		Description:     "Thương hiệu thiết kế của nền tảng",
		LogoURL:         "https://cdn.example.com/l.png",
		BrandType:       domain.BrandTypeOwn,
		ProtectionLevel: domain.ProtectionRestricted,
		CountryOfOrigin: "VN",
	})
	if err != nil {
		t.Fatalf("tạo thương hiệu: %v", err)
	}

	launched, err := svc.CreateCollection(ctx, application.CreateCollectionInput{
		BrandID:         brand.ID(),
		Name:            "Thu Đông 2026",
		Slug:            "thu-dong-2026",
		Season:          "FW2026",
		Theme:           "Tông đất",
		LaunchDate:      testNow.Add(-24 * time.Hour),
		EndOfSeasonDate: testNow.Add(90 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("tạo bộ sưu tập: %v", err)
	}
	if _, err := svc.LaunchCollection(ctx, launched.ID()); err != nil {
		t.Fatalf("ra mắt bộ sưu tập: %v", err)
	}

	unlaunched, err := svc.CreateCollection(ctx, application.CreateCollectionInput{
		BrandID:         brand.ID(),
		Name:            "Xuân Hè 2027",
		Slug:            "xuan-he-2027",
		Season:          "SS2027",
		LaunchDate:      testNow.Add(120 * 24 * time.Hour),
		EndOfSeasonDate: testNow.Add(240 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("tạo bộ sưu tập chưa ra mắt: %v", err)
	}

	root, err := svc.CreateCategory(ctx, application.CreateCategoryInput{
		Name: "Nữ", Slug: "nu", DisplayOrder: 1,
	})
	if err != nil {
		t.Fatalf("tạo danh mục: %v", err)
	}
	if _, err := svc.CreateCategory(ctx, application.CreateCategoryInput{
		ParentID: root.ID(), Name: "Váy", Slug: "nu-vay", DisplayOrder: 1,
	}); err != nil {
		t.Fatalf("tạo danh mục con: %v", err)
	}

	mux := http.NewServeMux()
	NewHandler(svc, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(mux)

	return fixture{
		h:            mux,
		brandID:      brand.ID(),
		launchedID:   launched.ID(),
		unlaunchedID: unlaunched.ID(),
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response phải là JSON hợp lệ: %v\nnội dung: %s", err, rec.Body.String())
	}
	return body
}

func TestGetBrand(t *testing.T) {
	f := newFixture(t)

	rec := get(t, f.h, "/api/v1/brands/"+f.brandID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("mong 200, nhận %d: %s", rec.Code, rec.Body.String())
	}

	body := decode(t, rec)

	// Tên trường phải khớp đặc tả OpenAPI, không phải tên trường Go.
	for _, key := range []string{"id", "name", "slug", "logo_url", "description", "country_of_origin", "collections"} {
		if _, ok := body[key]; !ok {
			t.Errorf("thiếu trường %q trong response (đặc tả khai báo)", key)
		}
	}
	if body["id"] != f.brandID.String() {
		t.Errorf("id sai: %v", body["id"])
	}
	if body["country_of_origin"] != "VN" {
		t.Errorf("country_of_origin sai: %v", body["country_of_origin"])
	}
}

// TestGetBrandKhongLoBoSuuTapChuaRaMat là test QUAN TRỌNG NHẤT của file này.
//
// Lịch ra mắt bộ sưu tập là thông tin kinh doanh nhạy cảm. Rò rỉ qua endpoint
// công khai cho đối thủ biết trước chúng ta sắp bán gì.
func TestGetBrandKhongLoBoSuuTapChuaRaMat(t *testing.T) {
	f := newFixture(t)

	rec := get(t, f.h, "/api/v1/brands/"+f.brandID.String())
	raw := rec.Body.String()

	if !strings.Contains(raw, "Thu Đông 2026") {
		t.Errorf("bộ sưu tập ĐÃ ra mắt phải xuất hiện, không thấy: %s", raw)
	}
	if strings.Contains(raw, "Xuân Hè 2027") || strings.Contains(raw, f.unlaunchedID.String()) {
		t.Errorf("RÒ RỈ bộ sưu tập chưa ra mắt trong response công khai: %s", raw)
	}
}

func TestGetBrandKhongTonTaiTra404(t *testing.T) {
	f := newFixture(t)

	// ID đúng định dạng nhưng không tồn tại.
	rec := get(t, f.h, "/api/v1/brands/"+ids.MustNew(ids.PrefixBrand).String())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mong 404, nhận %d: %s", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "NOT_FOUND" {
		t.Errorf("mong code=NOT_FOUND, nhận %q", code)
	}
}

func TestGetBrandIDSaiDinhDangTra400(t *testing.T) {
	f := newFixture(t)

	for _, bad := range []string{
		"khong-phai-id",
		"col_01J0000000000000000000000A", // đúng dạng nhưng SAI tiền tố
		"brd_qua-ngan",
	} {
		rec := get(t, f.h, "/api/v1/brands/"+bad)
		// 400 chứ không phải 404: lỗi nằm ở request, không phải ở việc
		// tài nguyên không tồn tại.
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: mong 400, nhận %d", bad, rec.Code)
		}
	}
}

func TestGetCollectionDaRaMat(t *testing.T) {
	f := newFixture(t)

	rec := get(t, f.h, "/api/v1/collections/"+f.launchedID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("mong 200, nhận %d: %s", rec.Code, rec.Body.String())
	}

	body := decode(t, rec)
	if body["season"] != "FW2026" {
		t.Errorf("season sai: %v", body["season"])
	}

	// Đặc tả khai báo format: date — chỉ ngày, KHÔNG kèm giờ.
	launch, _ := body["launch_date"].(string)
	if len(launch) != len("2006-01-02") {
		t.Errorf("launch_date phải theo dạng YYYY-MM-DD, nhận %q", launch)
	}

	brand, ok := body["brand"].(map[string]any)
	if !ok {
		t.Fatalf("thiếu trường brand: %v", body)
	}
	if brand["id"] != f.brandID.String() {
		t.Errorf("brand.id sai: %v", brand["id"])
	}
}

// TestGetCollectionChuaRaMatTra404 kiểm tra rằng bộ sưu tập chưa ra mắt trả
// 404 chứ không phải 403.
//
// 403 XÁC NHẬN tài nguyên tồn tại — đủ để đối thủ dò ID và biết chúng ta
// đang chuẩn bị bộ sưu tập.
func TestGetCollectionChuaRaMatTra404(t *testing.T) {
	f := newFixture(t)

	rec := get(t, f.h, "/api/v1/collections/"+f.unlaunchedID.String())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mong 404, nhận %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Xuân Hè 2027") {
		t.Errorf("response 404 vẫn lộ tên bộ sưu tập: %s", rec.Body.String())
	}
}

func TestGetCategoryTree(t *testing.T) {
	f := newFixture(t)

	rec := get(t, f.h, "/api/v1/categories")
	if rec.Code != http.StatusOK {
		t.Fatalf("mong 200, nhận %d: %s", rec.Code, rec.Body.String())
	}

	body := decode(t, rec)
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("response phải có mảng `data` theo đặc tả: %v", body)
	}
	if len(data) != 1 {
		t.Fatalf("mong 1 danh mục gốc, nhận %d", len(data))
	}

	root, _ := data[0].(map[string]any)
	if root["name"] != "Nữ" {
		t.Errorf("tên danh mục gốc sai: %v", root["name"])
	}

	// Cây phải TRẢ CẢ CẤP CON trong một lời gọi — nếu không, client phải
	// gọi tuần tự từng cấp để dựng thanh điều hướng.
	children, ok := root["children"].([]any)
	if !ok || len(children) != 1 {
		t.Fatalf("danh mục gốc phải có 1 con, nhận %v", root["children"])
	}
	child, _ := children[0].(map[string]any)
	if child["name"] != "Váy" {
		t.Errorf("tên danh mục con sai: %v", child["name"])
	}
}

// TestLoiTheoDinhDangDacTa kiểm tra response lỗi khớp
// api/components/common.yaml#/schemas/Error.
func TestLoiTheoDinhDangDacTa(t *testing.T) {
	f := newFixture(t)

	rec := get(t, f.h, "/api/v1/brands/khong-phai-id")

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type phải là application/json, nhận %q", ct)
	}

	body := decode(t, rec)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("lỗi phải bọc trong trường `error`: %v", body)
	}
	for _, key := range []string{"code", "message"} {
		if _, ok := errObj[key]; !ok {
			t.Errorf("thiếu trường error.%s", key)
		}
	}
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := decode(t, rec)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := errObj["code"].(string)
	return code
}
