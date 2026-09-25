package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// duAnGia gọi writeFiles rồi TỰ THÊM tài liệu cho mọi module nó dựng.
//
// R6 đỏ khi một thư mục module không tra ra tài liệu — chủ ý, để lệch tên
// phải kêu. Nhưng mọi bài test cũ dựng module mà không dựng tài liệu, nên
// nếu bắt từng bài tự thêm thì mỗi bài mới lại vỡ một lần vì cùng lý do.
func duAnGia(t *testing.T, files map[string]string) string {
	t.Helper()

	mods := map[string]bool{}
	for rel := range files {
		if m := reModuleTrongDuong.FindStringSubmatch(rel); m != nil {
			mods[m[1]] = true
		}
	}
	for m := range mods {
		if ten, than := taiLieuModule(m); files[ten] == "" {
			files[ten] = than
		}
	}
	return writeFiles(t, files)
}

var reModuleTrongDuong = regexp.MustCompile(`^internal/modules/([a-z][a-z0-9_]*)/`)

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
	vs, err := chayChecker(root)
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
func chayChecker(root string) ([]Violation, error) {
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

// taiLieuModule dựng một file tài liệu tối thiểu có mục "Dữ liệu sở hữu".
//
// Mọi dự án giả phải có nó cho MỌI thư mục module, vì R6 đỏ khi một module
// không tra ra tài liệu — đó là chủ ý: lệch tên phải kêu, không được im.
func taiLieuModule(mod string, bang ...string) (string, string) {
	than := "## 1. Dữ liệu sở hữu\n\n```sql\n"
	for _, b := range bang {
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

	root := duAnGia(t, map[string]string{
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

	root := duAnGia(t, map[string]string{
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

	root := duAnGia(t, map[string]string{
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

// TestSelfCheck — codebase thật phải luôn sạch.
func TestSelfCheck(t *testing.T) {
	if vs := runChecker(t, "../.."); len(vs) > 0 {
		for _, v := range vs {
			t.Errorf("codebase vi phạm ranh giới:\n%s", v)
		}
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
func TestR6_ModuleKhongChamBangCuaModuleKhac(t *testing.T) {
	tenOrder, thanOrder := taiLieuModule("order", `"order"`, "order_line")
	tenSeller, thanSeller := taiLieuModule("seller", "seller", "seller_bank_account")

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

	vs, err := chayChecker(root)
	if err != nil {
		t.Fatalf("chayChecker: %v", err)
	}
	if !rulesFound(vs)["R6"] {
		t.Fatalf("phải bắt R6, nhận: %v", vs)
	}

	var soR6 int
	for _, v := range vs {
		if v.Rule == "R6" {
			soR6++
			if !strings.Contains(v.Message, "seller") {
				t.Errorf("thông báo phải gọi tên bảng và module chủ: %s", v.Message)
			}
		}
	}
	if soR6 != 1 {
		t.Errorf("mong đúng 1 vi phạm R6 (bảng của chính mình không tính), nhận %d", soR6)
	}
}

// Bảng của platform thì MỌI module được chạm.
//
// Đó là hạ tầng dùng chung và không mang khái niệm nghiệp vụ nào để rò rỉ.
// Không có ngoại lệ này thì mọi module ghi audit đều đỏ, và người ta tắt
// hẳn quy tắc.
func TestR6_BangPlatformThiAiCungChamDuoc(t *testing.T) {
	ten, than := taiLieuModule("order", `"order"`)
	root := writeFiles(t, map[string]string{
		ten: than,
		"internal/modules/order/infrastructure/postgres/store.go": "package postgres\n" +
			"const q = `INSERT INTO audit_log (id) VALUES ($1)`\n" +
			"const q2 = `INSERT INTO event_outbox (id) VALUES ($1)`\n",
	})

	vs, err := chayChecker(root)
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
func TestR6_BangSoHuuHongThiBaoLoi(t *testing.T) {
	tenOrder, thanOrder := taiLieuModule("order", `"order"`)

	t.Run("hai module cùng khai một bảng", func(t *testing.T) {
		tenSeller, thanSeller := taiLieuModule("seller", "seller", `"order"`)
		root := writeFiles(t, map[string]string{
			tenOrder:                       thanOrder,
			tenSeller:                      thanSeller,
			"internal/modules/order/x.go":  "package order\n",
			"internal/modules/seller/x.go": "package seller\n",
		})
		_, err := chayChecker(root)
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
		_, err := chayChecker(root)
		if err == nil || !strings.Contains(err.Error(), "không có tài liệu") {
			t.Fatalf("module thiếu tài liệu phải là lỗi, nhận: %v", err)
		}
	})

	t.Run("tài liệu mất mục Dữ liệu sở hữu", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"../docs/04-modules/order.md": "# Order\n\nKhông có mục sở hữu.\n",
			"internal/modules/order/x.go": "package order\n",
		})
		_, err := chayChecker(root)
		if err == nil || !strings.Contains(err.Error(), "Dữ liệu sở hữu") {
			t.Fatalf("thiếu mục sở hữu phải là lỗi, nhận: %v", err)
		}
	})
}
