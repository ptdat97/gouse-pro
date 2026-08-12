package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	c := &checker{
		root:       root,
		modulePath: mod,
		imports:    make(map[string][]string),
		pkgFiles:   make(map[string][]string),
	}
	if err := c.run(); err != nil {
		t.Fatalf("checker.run: %v", err)
	}
	return c.violations
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

	root := writeFiles(t, map[string]string{
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

	root := writeFiles(t, map[string]string{
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

	root := writeFiles(t, map[string]string{
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
