package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeProject gọi writeFiles rồi TỰ THÊM tài liệu cho mọi module nó dựng.
//
// R6 đỏ khi một thư mục module không tra ra tài liệu — chủ ý, để lệch tên
// phải kêu. Nhưng mọi bài test cũ dựng module mà không dựng tài liệu, nên
// nếu bắt từng bài tự thêm thì mỗi bài mới lại vỡ một lần vì cùng lý do.
func fakeProject(t *testing.T, files map[string]string) string {
	t.Helper()

	mods := map[string]bool{}
	for rel := range files {
		if m := reModuleInPath.FindStringSubmatch(rel); m != nil {
			mods[m[1]] = true
		}
	}
	for m := range mods {
		if label, than := moduleDoc(m); files[label] == "" {
			files[label] = than
		}
	}
	return writeFiles(t, files)
}

var reModuleInPath = regexp.MustCompile(`^internal/modules/([a-z][a-z0-9_]*)/`)

// writeFiles tạo cây thư mục tạm với nội dung cho trước.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("tạo thư mục %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("ghi %s: %v", path, err)
		}
	}
	return root
}

func runChecker(t *testing.T, root string) []Violation {
	t.Helper()
	vs, err := collectViolations(root)
	if err != nil {
		t.Fatalf("checker.run: %v", err)
	}
	return vs
}

// chayChecker chạy công cụ và trả CẢ lỗi, để bài test đo được lỗi cấu hình.
//
// R6 đọc `docs/04-modules/` và trả LỖI (không phải vi phạm) khi tài liệu
// thiếu — không có bảng sở hữu đáng tin thì không kiểm được gì. Bài test
// nào đo chuyện đó cần nhìn thấy lỗi ấy.
func collectViolations(root string) ([]Violation, error) {
	c := &checker{
		root:       root,
		modulePath: mod,
		imports:    make(map[string][]string),
		pkgFiles:   make(map[string][]string),
	}
	if err := c.run(); err != nil {
		return nil, err
	}
	return c.violations, nil
}

// moduleDoc dựng một file tài liệu tối thiểu có mục "Dữ liệu sở hữu".
//
// Mọi dự án giả phải có nó cho MỌI thư mục module, vì R6 đỏ khi một module
// không tra ra tài liệu — đó là chủ ý: lệch tên phải kêu, không được im.
func moduleDoc(mod string, tables ...string) (string, string) {
	than := "## 1. Dữ liệu sở hữu\n\n```sql\n"
	for _, b := range tables {
		than += b + "\n"
	}
	than += "```\n"
	return "../docs/04-modules/" + mod + ".md", than
}

func rulesFound(vs []Violation) map[string]bool {
	m := make(map[string]bool, len(vs))
	for _, v := range vs {
		m[v.Rule] = true
	}
	return m
}

// TestDetectsEachViolationType kiểm chứng công cụ THỰC SỰ bắt được vi phạm.
//
// Không có test này, archcheck có thể im lặng bỏ sót và tạo cảm giác an toàn
// giả — nguy hiểm hơn không có công cụ.
func TestDetectsEachViolationType(t *testing.T) {
	imp := func(p string) string {
		return "package x\nimport _ \"" + mod + p + "\"\n"
	}

	root := fakeProject(t, map[string]string{
		// R4: kernel import platform
		"internal/kernel/money/money.go": imp("/internal/platform/database"),
		// R3: platform import module nghiệp vụ
		"internal/platform/database/db.go": imp("/internal/modules/order"),
		// R2: domain import platform
		"internal/modules/order/domain/order.go": imp("/internal/platform/database"),
		// R1: import sâu vào module khác
		"internal/modules/order/application/place.go": imp("/internal/modules/inventory/domain"),
		// R8: interfaces → infrastructure
		"internal/modules/order/interfaces/http.go": imp("/internal/modules/order/infrastructure"),
		// R7: thư mục bị cấm
		"internal/common/util.go": "package common\n",

		// file phụ trợ để import phân giải được
		"internal/modules/order/public.go":           "package order\n",
		"internal/modules/order/infrastructure/r.go": "package infrastructure\n",
		"internal/modules/inventory/domain/inv.go":   "package domain\n",
	})

	got := rulesFound(runChecker(t, root))
	for _, want := range []string{"R1", "R2", "R3", "R4", "R7", "R8"} {
		if !got[want] {
			t.Errorf("KHÔNG bắt được vi phạm %s — công cụ tạo cảm giác an toàn giả", want)
		}
	}
}

// TestDetectsDependencyCycle — phụ thuộc vòng làm module không tách được
// và thay đổi lan truyền không kiểm soát.
func TestDetectsDependencyCycle(t *testing.T) {
	imp := func(p string) string {
		return "package application\nimport _ \"" + mod + p + "\"\n"
	}

	root := fakeProject(t, map[string]string{
		"internal/modules/order/public.go":   "package order\n",
		"internal/modules/loyalty/public.go": "package loyalty\n",
		"internal/modules/catalog/public.go": "package catalog\n",

		// order → loyalty → catalog → order
		"internal/modules/order/application/a.go":   imp("/internal/modules/loyalty"),
		"internal/modules/loyalty/application/a.go": imp("/internal/modules/catalog"),
		"internal/modules/catalog/application/a.go": imp("/internal/modules/order"),
	})

	vs := runChecker(t, root)
	if !rulesFound(vs)["R5"] {
		t.Fatal("KHÔNG phát hiện phụ thuộc vòng 3 bước")
	}

	// Thông báo phải nêu đủ các module trong vòng để người sửa biết cắt ở đâu.
	var cycleMsg string
	for _, v := range vs {
		if v.Rule == "R5" {
			cycleMsg = v.Message
		}
	}
	for _, m := range []string{"order", "loyalty", "catalog"} {
		if !strings.Contains(cycleMsg, m) {
			t.Errorf("thông báo chu trình thiếu module %q: %s", m, cycleMsg)
		}
	}
}

// TestValidStructureProducesNoViolations — cấu trúc đúng chuẩn phải sạch.
// Nếu công cụ báo lỗi giả, người ta sẽ tắt nó đi.
func TestValidStructureProducesNoViolations(t *testing.T) {
	imp := func(pkg string, paths ...string) string {
		s := "package " + pkg + "\n"
		for _, p := range paths {
			s += "import _ \"" + mod + p + "\"\n"
		}
		return s
	}

	root := fakeProject(t, map[string]string{
		"internal/kernel/money/money.go":   "package money\n",
		"internal/kernel/types/t.go":       imp("types", "/internal/kernel/money"),
		"internal/platform/database/db.go": imp("database", "/internal/kernel/ids"),
		"internal/kernel/ids/ids.go":       "package ids\n",

		"internal/modules/inventory/public.go": "package inventory\n",

		// domain chỉ dùng kernel
		"internal/modules/order/domain/order.go": imp("domain", "/internal/kernel/money"),
		// application dùng domain, kernel, platform, và public API module khác
		"internal/modules/order/application/place.go": imp("application",
			"/internal/modules/order/domain",
			"/internal/kernel/money",
			"/internal/platform/database",
			"/internal/modules/inventory",
		),
		// infrastructure cài đặt port của domain
		"internal/modules/order/infrastructure/repo.go": imp("infrastructure",
			"/internal/modules/order/domain",
			"/internal/platform/database",
		),
		// interfaces gọi application
		"internal/modules/order/interfaces/http.go": imp("interfaces",
			"/internal/modules/order/application",
		),
		"internal/modules/order/public.go": imp("order", "/internal/kernel/ids"),

		// cmd được dùng mọi thứ
		"cmd/api/main.go": imp("main",
			"/internal/modules/order",
			"/internal/platform/httpserver",
		),
		"internal/platform/httpserver/s.go": "package httpserver\n",
	})

	if vs := runChecker(t, root); len(vs) > 0 {
		for _, v := range vs {
			t.Errorf("BÁO LỖI GIẢ trên cấu trúc hợp lệ:\n%s", v)
		}
	}
}

// TestSelfCheck — codebase thật phải luôn sạch, với ĐỦ BỘ QUY TẮC.
//
// `moiChecker` chứ không `chayChecker`: bản sau để `goc` rỗng, tức TẮT R10.
// Đó là đúng cho dự án giả — ba module không có kernel để mà gác — nhưng ở
// đây nó sẽ làm R10 không bao giờ chạy trong `go test`, và một quy tắc chỉ
// chạy khi gọi tay là quy tắc sẽ bị bỏ qua.
func TestSelfCheck(t *testing.T) {
	c := newChecker("../..", mod)
	if err := c.run(); err != nil {
		t.Fatalf("checker.run trên repo thật: %v", err)
	}
	for _, v := range c.violations {
		t.Errorf("codebase vi phạm ranh giới:\n%s", v)
	}
}

// ------------------------------------------------- R6: sở hữu bảng

// R6 bắt module chạm bảng của module KHÁC.
//
// # Vì sao quy tắc này cần tồn tại
//
// R1 chặn module A import mã của module B. Nhưng nó KHÔNG chặn A viết
// `SELECT ... FROM bang_cua_B` — cùng một sự ghép nối, đi bằng đường khác,
// và là đường khó thấy hơn nhiều: không có import nào để đọc.
//
// Hậu quả nặng hơn ghép nối thường: B đổi lược đồ bảng của mình mà không
// biết A đang đọc nó, nên một migration đúng theo mọi nghĩa vẫn làm A vỡ.
func TestR6_AModuleMustNotTouchAnothersTable(t *testing.T) {
	tenOrder, thanOrder := moduleDoc("order", `"order"`, "order_line")
	tenSeller, thanSeller := moduleDoc("seller", "seller", "seller_bank_account")

	root := writeFiles(t, map[string]string{
		tenOrder:  thanOrder,
		tenSeller: thanSeller,

		// order đọc bảng của CHÍNH MÌNH — hợp lệ.
		"internal/modules/order/infrastructure/postgres/store.go": "package postgres\n" +
			"const q = `SELECT id FROM order_line WHERE order_id = $1`\n",

		// order đọc bảng của seller — VI PHẠM.
		"internal/modules/order/infrastructure/postgres/xau.go": "package postgres\n" +
			"const q2 = `SELECT name FROM seller WHERE id = $1`\n",
	})

	vs, err := collectViolations(root)
	if err != nil {
		t.Fatalf("chayChecker: %v", err)
	}
	if !rulesFound(vs)["R6"] {
		t.Fatalf("phải bắt R6, nhận: %v", vs)
	}

	var r6Count int
	for _, v := range vs {
		if v.Rule == "R6" {
			r6Count++
			if !strings.Contains(v.Message, "seller") {
				t.Errorf("thông báo phải gọi tên bảng và module chủ: %s", v.Message)
			}
		}
	}
	if r6Count != 1 {
		t.Errorf("mong đúng 1 vi phạm R6 (bảng của chính mình không tính), nhận %d", r6Count)
	}
}

// Bảng của platform thì MỌI module được chạm.
//
// Đó là hạ tầng dùng chung và không mang khái niệm nghiệp vụ nào để rò rỉ.
// Không có ngoại lệ này thì mọi module ghi audit đều đỏ, và người ta tắt
// hẳn quy tắc.
func TestR6_APlatformTableIsReachableByEveryone(t *testing.T) {
	label, than := moduleDoc("order", `"order"`)
	root := writeFiles(t, map[string]string{
		label: than,
		"internal/modules/order/infrastructure/postgres/store.go": "package postgres\n" +
			"const q = `INSERT INTO audit_log (id) VALUES ($1)`\n" +
			"const q2 = `INSERT INTO event_outbox (id) VALUES ($1)`\n",
	})

	vs, err := collectViolations(root)
	if err != nil {
		t.Fatalf("chayChecker: %v", err)
	}
	if rulesFound(vs)["R6"] {
		t.Fatalf("bảng platform không được tính là vi phạm: %v", vs)
	}
}

// Ba cách LÀM HỎNG BẢNG SỞ HỮU phải thành LỖI, không phải vi phạm.
//
// Không có bảng sở hữu đáng tin thì R6 không kiểm được gì — và một quy tắc
// im lặng không kiểm gì là thứ tệ nhất: nó vẫn in ra OK.
func TestR6_ABrokenOwnershipLedgerErrors(t *testing.T) {
	tenOrder, thanOrder := moduleDoc("order", `"order"`)

	t.Run("hai module cùng khai một bảng", func(t *testing.T) {
		tenSeller, thanSeller := moduleDoc("seller", "seller", `"order"`)
		root := writeFiles(t, map[string]string{
			tenOrder:                       thanOrder,
			tenSeller:                      thanSeller,
			"internal/modules/order/x.go":  "package order\n",
			"internal/modules/seller/x.go": "package seller\n",
		})
		_, err := collectViolations(root)
		if err == nil || !strings.Contains(err.Error(), "HAI module") {
			t.Fatalf("khai trùng phải là lỗi, nhận: %v", err)
		}
	})

	t.Run("module không có tài liệu", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			tenOrder:                      thanOrder,
			"internal/modules/order/x.go": "package order\n",
			"internal/modules/laKia/x.go": "package lakia\n",
		})
		_, err := collectViolations(root)
		if err == nil || !strings.Contains(err.Error(), "không có tài liệu") {
			t.Fatalf("module thiếu tài liệu phải là lỗi, nhận: %v", err)
		}
	})

	t.Run("tài liệu mất mục Dữ liệu sở hữu", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"../docs/04-modules/order.md": "# Order\n\nKhông có mục sở hữu.\n",
			"internal/modules/order/x.go": "package order\n",
		})
		_, err := collectViolations(root)
		if err == nil || !strings.Contains(err.Error(), "Dữ liệu sở hữu") {
			t.Fatalf("thiếu mục sở hữu phải là lỗi, nhận: %v", err)
		}
	})
}

// ------------------------------------------- R10: microkernel

// microkernelProject dựng dự án giả CÓ bật R10.
//
// `chayChecker` để `goc` rỗng, tức tắt R10 — đúng cho các bài kiểm R1–R9
// nhưng vô dụng ở đây. Hàm này bật R10 với sổ năng lực cắt ngang truyền vào.
//
// `tang` là tầng của từng module, và nó được ghi vào TÀI LIỆU module chứ
// không truyền thẳng vào checker: R10 đọc tài liệu, nên bộ test phải đi qua
// đúng đường ấy — bỏ qua nó thì bài test kiểm một đường mã không ai chạy.
func microkernelProject(
	t *testing.T, files map[string]string,
	tier map[string]string, crossCutting map[string]string,
) []Violation {
	t.Helper()

	mods := map[string]bool{}
	for rel := range files {
		if m := reModuleInPath.FindStringSubmatch(rel); m != nil {
			mods[m[1]] = true
		}
	}
	for m := range mods {
		label, than := moduleDoc(m)
		if t, found := tier[m]; found {
			than = "| **Tầng (microkernel)** | **" + t + "** |\n\n" + than
		}
		if files[label] == "" {
			files[label] = than
		}
	}
	// Tài liệu cho module KHÔNG có mã — dùng để kiểm sổ đã mục.
	for m, t := range tier {
		if mods[m] {
			continue
		}
		label, than := moduleDoc(m)
		files[label] = "| **Tầng (microkernel)** | **" + t + "** |\n\n" + than
	}

	root := writeFiles(t, files)

	c := &checker{
		root:         root,
		modulePath:   mod,
		root2:        "checkout",
		crossCutting: crossCutting,
		imports:      make(map[string][]string),
		pkgFiles:     make(map[string][]string),
	}
	if err := c.run(); err != nil {
		t.Fatalf("checker.run: %v", err)
	}
	return c.violations
}

// runMicrokernelError chạy như trên nhưng ĐÒI một lỗi khởi động.
func runMicrokernelError(t *testing.T, files map[string]string) error {
	t.Helper()
	root := writeFiles(t, files)
	c := &checker{
		root:       root,
		modulePath: mod,
		root2:      "checkout",
		imports:    make(map[string][]string),
		pkgFiles:   make(map[string][]string),
	}
	err := c.run()
	if err == nil {
		t.Fatalf("mong LỖI khởi động, nhận vi phạm: %v", c.violations)
	}
	return err
}

// onlyR10 lọc lấy vi phạm R10 — các bài dưới đây không quan tâm quy tắc khác.
func onlyR10(vs []Violation) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Rule == "R10" {
			out = append(out, v)
		}
	}
	return out
}

const checkoutSource = `package checkout

type API interface{ Mua() }
`

// R10a — module KERNEL import module EXTENSION.
//
// # Vì sao R5 không bắt được
//
// R5 chặn phụ thuộc VÒNG. `checkout → notification` là một chiều, nên đồ thị
// vẫn là DAG và R5 im lặng. Sau dòng import ấy, tháo `notification` ra làm
// `checkout` không biên dịch được — tức câu "tháo extension ra thì khách vẫn
// mua được hàng" thành câu không đúng.
func TestR10_TheKernelMustNotImportAnExtension(t *testing.T) {
	vs := onlyR10(microkernelProject(t, map[string]string{
		"internal/modules/checkout/public.go": `package checkout

import _ "` + mod + `/internal/modules/notification"

type API interface{ Mua() }
`,
		"internal/modules/notification/public.go": `package notification

type API interface{ Gui() }
`,
	}, map[string]string{
		"checkout":     "KERNEL — đường mua hàng",
		"notification": "EXTENSION — đã dựng",
	}, nil))

	if len(vs) != 1 {
		t.Fatalf("mong 1 vi phạm R10, nhận %d: %v", len(vs), vs)
	}
	for _, can := range []string{"notification", "checkout", "EXTENSION"} {
		if !strings.Contains(vs[0].String(), can) {
			t.Errorf("thiếu %q trong: %s", can, vs[0])
		}
	}
}

// R10a cũng chặn EXTENSION import EXTENSION.
//
// Mạnh hơn "kernel không import extension" có chủ ý: hai extension dính nhau
// thì không tháo riêng được cái nào, và câu "tháo nó ra thì chỉ mất tính năng
// của nó" thành sai cho cả hai.
func TestR10_AnExtensionMustNotImportAnExtension(t *testing.T) {
	vs := onlyR10(microkernelProject(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
		"internal/modules/returns/public.go": `package returns

import _ "` + mod + `/internal/modules/analytics"

type API interface{ Tra() }
`,
		"internal/modules/analytics/public.go": `package analytics

type API interface{ Ghi() }
`,
	}, map[string]string{
		"checkout":  "KERNEL — đường mua hàng",
		"returns":   "EXTENSION — đã dựng",
		"analytics": "EXTENSION — đã dựng",
	}, nil))

	if len(vs) != 1 {
		t.Fatalf("mong 1 vi phạm R10, nhận %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].String(), "analytics") ||
		!strings.Contains(vs[0].String(), "returns") {
		t.Errorf("không chỉ ra cặp extension dính nhau: %s", vs[0])
	}
}

// R10c — tài liệu xếp KERNEL mà module có HÌNH DẠNG của extension.
//
// Ngoài đường mua hàng VÀ không ai import: đó đúng là hình dạng một
// extension. Phân loại cũ sẽ làm người sau tin module ấy ổn định.
func TestR10_ADocMisfilingAModuleAsKernelIsRed(t *testing.T) {
	vs := onlyR10(microkernelProject(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
		"internal/modules/analytics/public.go": `package analytics

type API interface{ Ghi() }
`,
	}, map[string]string{
		"checkout":  "KERNEL — đường mua hàng",
		"analytics": "KERNEL — đường mua hàng",
	}, nil))

	if len(vs) != 1 {
		t.Fatalf("mong 1 vi phạm R10, nhận %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].String(), "analytics") {
		t.Errorf("không chỉ ra module xếp sai: %s", vs[0])
	}
}

// Khai năng lực CẮT NGANG thì R10c im — đó là đường miễn trừ có lý do.
func TestR10_CrossCuttingCapabilitiesAreExempt(t *testing.T) {
	vs := onlyR10(microkernelProject(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
		"internal/modules/identity/public.go": `package identity

type API interface{ Ai() }
`,
	}, map[string]string{
		"checkout": "KERNEL — đường mua hàng",
		"identity": "KERNEL — năng lực cắt ngang",
	}, map[string]string{
		"identity": "mọi mô hình kinh doanh đều cần biết ai đang gọi",
	}))

	if len(vs) != 0 {
		t.Fatalf("năng lực cắt ngang đã khai mà vẫn vi phạm: %v", vs)
	}
}

// Module CHƯA XẾP ĐƯỢC không bị kết luận theo chiều nào.
//
// `pricing` là trường hợp thật: tài liệu nói nó sở hữu `price_constraint`
// nhưng `marketplace` không import nó, nên khung giá ấy không cưỡng chế gì.
// Xếp sai còn tệ hơn để trống — một module gán nhãn "kernel" sẽ được người
// sau tin là ổn định.
func TestR10_AnUnassignedModuleDrawsNoConclusion(t *testing.T) {
	vs := onlyR10(microkernelProject(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
		"internal/modules/pricing/public.go": `package pricing

type API interface{ Gia() }
`,
	}, map[string]string{
		"checkout": "KERNEL — đường mua hàng",
		"pricing":  "CHƯA XẾP ĐƯỢC",
	}, nil))

	if len(vs) != 0 {
		t.Fatalf("module chưa xếp được bị kết luận: %v", vs)
	}
}

// Tài liệu THIẾU hàng tầng → LỖI khởi động, không phải vi phạm.
//
// Cùng cách R6 xử lý tài liệu thiếu mục sở hữu: không có bảng phân loại đáng
// tin thì R10 không kiểm được gì, và một quy tắc im lặng không kiểm gì vẫn
// in ra `OK` — thứ tệ nhất.
func TestR10_AMissingTierRowIsAStartupError(t *testing.T) {
	label, than := moduleDoc("checkout")
	err := runMicrokernelError(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
		label:                                 than, // KHÔNG có hàng tầng
	})
	if !strings.Contains(err.Error(), "Tầng (microkernel)") {
		t.Errorf("lỗi không nêu hàng thiếu: %v", err)
	}
}

// Tài liệu ghi giá trị LẠ → LỖI khởi động.
//
// "Quan trọng" không phải một tầng. Nhận im lặng rồi coi là chưa phân loại
// sẽ biến một lỗi đánh máy thành một module mất hàng rào.
func TestR10_AnUnknownTierIsAStartupError(t *testing.T) {
	label, than := moduleDoc("checkout")
	err := runMicrokernelError(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
		label:                                 "| **Tầng (microkernel)** | **Quan trọng** |\n\n" + than,
	})
	if !strings.Contains(err.Error(), "Quan trọng") {
		t.Errorf("lỗi không nêu giá trị lạ: %v", err)
	}
}

// Sổ `crossCuttingCapabilities` trỏ tới module KHÔNG tồn tại là một sổ đã mục.
func TestR10_CatchesARottenCrossCuttingOverride(t *testing.T) {
	vs := onlyR10(microkernelProject(t, map[string]string{
		"internal/modules/checkout/public.go": checkoutSource,
	}, map[string]string{
		"checkout": "KERNEL — đường mua hàng",
	}, map[string]string{
		"da_doi_ten": "lý do cũ cho một tên không còn tồn tại",
	}))

	if len(vs) != 1 {
		t.Fatalf("mong 1 vi phạm R10, nhận %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].String(), "da_doi_ten") {
		t.Errorf("không chỉ ra dòng sổ đã mục: %s", vs[0])
	}
}
