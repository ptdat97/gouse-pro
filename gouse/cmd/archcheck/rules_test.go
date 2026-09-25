package main

import "testing"

const mod = "github.com/fashion-commerce/platform"

func p(importPath string) pkgInfo { return classify(mod, importPath) }

func TestClassify(t *testing.T) {
	cases := []struct {
		path       string
		wantModule string
		wantLayer  Layer
		wantKernel bool
		wantPlat   bool
	}{
		{mod + "/internal/kernel/money", "", "", true, false},
		{mod + "/internal/platform/database", "", "", false, true},
		{mod + "/internal/modules/order", "order", LayerPublic, false, false},
		{mod + "/internal/modules/order/domain", "order", LayerDomain, false, false},
		{mod + "/internal/modules/order/application", "order", LayerApplication, false, false},
		{mod + "/internal/modules/order/infrastructure", "order", LayerInfrastructure, false, false},
		{mod + "/internal/modules/order/interfaces", "order", LayerInterfaces, false, false},
		{mod + "/internal/modules/inventory/domain/sub", "inventory", LayerDomain, false, false},
		{"github.com/other/lib", "", "", false, false},
	}
	for _, tc := range cases {
		got := classify(mod, tc.path)
		if got.Module != tc.wantModule || got.Layer != tc.wantLayer ||
			got.IsKernel != tc.wantKernel || got.IsPlatform != tc.wantPlat {
			t.Errorf("classify(%q) = {module:%q layer:%q kernel:%v platform:%v}, mong {%q %q %v %v}",
				tc.path, got.Module, got.Layer, got.IsKernel, got.IsPlatform,
				tc.wantModule, tc.wantLayer, tc.wantKernel, tc.wantPlat)
		}
	}
}

// TestR1_NoDeepImportIntoOtherModule là quy tắc quan trọng nhất.
//
// Import sâu vào module khác phá vỡ tính đóng gói — mọi thay đổi nội bộ
// của module đó trở thành thay đổi phá vỡ, và không tách service được.
func TestR1_NoDeepImportIntoOtherModule(t *testing.T) {
	from := p(mod + "/internal/modules/order/application")

	// CẤM: import vào domain của module khác
	rule, _, _ := checkImport(from, p(mod+"/internal/modules/inventory/domain"))
	if rule != "R1" {
		t.Errorf("import sâu vào inventory/domain phải vi phạm R1, nhận %q", rule)
	}

	// CẤM: import vào infrastructure của module khác
	rule, _, _ = checkImport(from, p(mod+"/internal/modules/inventory/infrastructure"))
	if rule != "R1" {
		t.Errorf("import sâu vào inventory/infrastructure phải vi phạm R1, nhận %q", rule)
	}

	// ĐƯỢC: import public.go của module khác
	rule, _, _ = checkImport(from, p(mod+"/internal/modules/inventory"))
	if rule != "" {
		t.Errorf("import public API của inventory phải hợp lệ, nhận vi phạm %q", rule)
	}

	// ĐƯỢC: import tầng khác trong CÙNG module
	rule, _, _ = checkImport(from, p(mod+"/internal/modules/order/domain"))
	if rule != "" {
		t.Errorf("import domain cùng module phải hợp lệ, nhận vi phạm %q", rule)
	}
}

// TestR2_DomainLayerIsClean kiểm chứng điều kiện để test domain không cần
// database, không cần HTTP.
func TestR2_DomainLayerIsClean(t *testing.T) {
	from := p(mod + "/internal/modules/order/domain")

	// CẤM: domain import platform (database, http, event bus)
	rule, _, _ := checkImport(from, p(mod+"/internal/platform/database"))
	if rule != "R2" {
		t.Errorf("domain import platform phải vi phạm R2, nhận %q", rule)
	}

	// CẤM: domain import module khác (kể cả public API)
	rule, _, _ = checkImport(from, p(mod+"/internal/modules/inventory"))
	if rule != "R2" {
		t.Errorf("domain import module khác phải vi phạm R2, nhận %q", rule)
	}

	// ĐƯỢC: domain import kernel
	rule, _, _ = checkImport(from, p(mod+"/internal/kernel/money"))
	if rule != "" {
		t.Errorf("domain import kernel phải hợp lệ, nhận vi phạm %q", rule)
	}
}

// TestR3_PlatformKnowsNothingAboutBusiness — nếu platform biết về "order"
// hay "seller", nó không còn trung lập và trở thành điểm phụ thuộc toàn cục.
func TestR3_PlatformKnowsNothingAboutBusiness(t *testing.T) {
	from := p(mod + "/internal/platform/eventbus")

	rule, _, _ := checkImport(from, p(mod+"/internal/modules/order"))
	if rule != "R3" {
		t.Errorf("platform import module nghiệp vụ phải vi phạm R3, nhận %q", rule)
	}

	// ĐƯỢC: platform import kernel
	rule, _, _ = checkImport(from, p(mod+"/internal/kernel/ids"))
	if rule != "" {
		t.Errorf("platform import kernel phải hợp lệ, nhận vi phạm %q", rule)
	}
}

// TestR4_KernelIsMinimal — kernel là phụ thuộc của TOÀN BỘ hệ thống,
// phải giữ tối thiểu tuyệt đối.
func TestR4_KernelIsMinimal(t *testing.T) {
	from := p(mod + "/internal/kernel/money")

	rule, _, _ := checkImport(from, p(mod+"/internal/platform/database"))
	if rule != "R4" {
		t.Errorf("kernel import platform phải vi phạm R4, nhận %q", rule)
	}

	rule, _, _ = checkImport(from, p(mod+"/internal/modules/order"))
	if rule != "R4" {
		t.Errorf("kernel import module phải vi phạm R4, nhận %q", rule)
	}

	// ĐƯỢC: kernel import kernel khác
	rule, _, _ = checkImport(from, p(mod+"/internal/kernel/types"))
	if rule != "" {
		t.Errorf("kernel import kernel phải hợp lệ, nhận vi phạm %q", rule)
	}
}

// TestR8_LayerDirection kiểm chứng chiều phụ thuộc trong module:
// interfaces → application → domain ← infrastructure
func TestR8_LayerDirection(t *testing.T) {
	// CẤM: interfaces gọi thẳng infrastructure (bỏ qua application)
	rule, _, _ := checkImport(
		p(mod+"/internal/modules/order/interfaces"),
		p(mod+"/internal/modules/order/infrastructure"),
	)
	if rule != "R8" {
		t.Errorf("interfaces → infrastructure phải vi phạm R8, nhận %q", rule)
	}

	// CẤM: infrastructure import ngược lên application
	rule, _, _ = checkImport(
		p(mod+"/internal/modules/order/infrastructure"),
		p(mod+"/internal/modules/order/application"),
	)
	if rule != "R8" {
		t.Errorf("infrastructure → application phải vi phạm R8, nhận %q", rule)
	}

	// ĐƯỢC: infrastructure cài đặt port do domain định nghĩa
	rule, _, _ = checkImport(
		p(mod+"/internal/modules/order/infrastructure"),
		p(mod+"/internal/modules/order/domain"),
	)
	if rule != "" {
		t.Errorf("infrastructure → domain phải hợp lệ, nhận vi phạm %q", rule)
	}

	// ĐƯỢC: interfaces → application
	rule, _, _ = checkImport(
		p(mod+"/internal/modules/order/interfaces"),
		p(mod+"/internal/modules/order/application"),
	)
	if rule != "" {
		t.Errorf("interfaces → application phải hợp lệ, nhận vi phạm %q", rule)
	}
}

func TestAllowedCommonCases(t *testing.T) {
	// Các trường hợp hợp lệ điển hình không được báo vi phạm.
	cases := []struct {
		name string
		from string
		to   string
	}{
		{"application dùng kernel", "/internal/modules/order/application", "/internal/kernel/money"},
		{"application dùng platform", "/internal/modules/order/application", "/internal/platform/database"},
		{"application gọi public API module khác", "/internal/modules/checkout/application", "/internal/modules/inventory"},
		{"infrastructure dùng platform", "/internal/modules/order/infrastructure", "/internal/platform/database"},
		{"cmd dùng module", "/cmd/api", "/internal/modules/order"},
		{"cmd dùng platform", "/cmd/api", "/internal/platform/httpserver"},
		{"public.go dùng kernel", "/internal/modules/order", "/internal/kernel/ids"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rule, msg, _ := checkImport(p(mod+tc.from), p(mod+tc.to)); rule != "" {
				t.Errorf("phải hợp lệ nhưng vi phạm %s: %s", rule, msg)
			}
		})
	}
}

func TestExternalImportsIgnored(t *testing.T) {
	// Import thư viện bên thứ ba không bị kiểm tra.
	from := p(mod + "/internal/modules/order/domain")
	if rule, _, _ := checkImport(from, p("github.com/lib/pq")); rule != "" {
		t.Errorf("import bên thứ ba không được báo vi phạm, nhận %q", rule)
	}
}

func TestViolationStringIncludesHint(t *testing.T) {
	v := Violation{
		Rule:    "R1",
		File:    "internal/modules/order/application/place_order.go",
		Line:    42,
		Message: "import sâu vào module inventory",
		Hint:    "chỉ được import internal/modules/inventory",
	}
	s := v.String()
	for _, want := range []string{"R1", "place_order.go", "42", "inventory", "→"} {
		if !contains(s, want) {
			t.Errorf("thông báo vi phạm thiếu %q:\n%s", want, s)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestR9_KhongSDKNgoaiTrongKernelVaDomain.
//
// # Vì sao quy tắc này cần bài test riêng
//
// R4 nói "kernel chỉ dùng thư viện chuẩn" và R2 nói "domain chỉ phụ thuộc
// chính nó và kernel". Cả hai câu ấy đúng với mã hiện tại nhưng KHÔNG được
// cưỡng chế suốt một thời gian dài: vòng quét ở `main.go` bỏ qua mọi import
// không bắt đầu bằng đường dẫn module, nên `checkImport` không bao giờ
// NHÌN THẤY một thư viện bên thứ ba.
//
// Hai quy tắc hứa nhiều hơn thứ chúng kiểm là đúng dạng lỗi tệ nhất của
// một công cụ kiểm: nó tạo cảm giác an toàn giả.
func TestR9_KhongSDKNgoaiTrongKernelVaDomain(t *testing.T) {
	kernel := p(mod + "/internal/kernel/money")
	domain := p(mod + "/internal/modules/order/domain")
	infra := p(mod + "/internal/modules/order/infrastructure/postgres")

	// CẤM ở kernel và domain.
	for _, tc := range []struct {
		ten  string
		from pkgInfo
	}{{"kernel", kernel}, {"domain", domain}} {
		rule, _, _ := KiemThuVienNgoai(tc.from, "github.com/stripe/stripe-go/v76")
		if rule != "R9" {
			t.Errorf("%s import SDK ngoài phải vi phạm R9, nhận %q", tc.ten, rule)
		}
	}

	// ĐƯỢC ở infrastructure: đó chính là chỗ SDK phải sống.
	if rule, _, _ := KiemThuVienNgoai(infra, "github.com/jackc/pgx/v5"); rule != "" {
		t.Errorf("infrastructure import SDK phải hợp lệ, nhận %q", rule)
	}
}

// Nhận diện thư viện ngoài KHÔNG được nhầm với thư viện chuẩn.
//
// Nhầm theo chiều "chuẩn thành ngoài" sẽ làm mọi file domain đỏ và người ta
// tắt hẳn công cụ. Nhầm theo chiều ngược lại thì quy tắc không gác gì.
func TestNhanDienThuVienNgoai(t *testing.T) {
	cases := []struct {
		importPath string
		ngoai      bool
	}{
		// Thư viện chuẩn: đoạn đầu KHÔNG có dấu chấm.
		{"time", false},
		{"net/http", false},
		{"encoding/json", false},
		{"crypto/sha256", false},

		// Chính dự án này — cũng bắt đầu bằng github.com, phải loại ra.
		{mod + "/internal/kernel/ids", false},
		{mod + "/internal/modules/order/domain", false},

		// Bên thứ ba.
		{"github.com/jackc/pgx/v5", true},
		{"golang.org/x/crypto/bcrypt", true},
		{"gopkg.in/yaml.v3", true},
		{"go.uber.org/zap", true},
	}
	for _, tc := range cases {
		if got := LaThuVienNgoai(mod, tc.importPath); got != tc.ngoai {
			t.Errorf("LaThuVienNgoai(%q) = %v, mong %v", tc.importPath, got, tc.ngoai)
		}
	}
}
