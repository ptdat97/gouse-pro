package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultModulePath = "github.com/fashion-commerce/platform"

func main() {
	var (
		root       = flag.String("root", ".", "thư mục gốc dự án")
		modulePath = flag.String("module", defaultModulePath, "đường dẫn module Go")
		verbose    = flag.Bool("v", false, "in chi tiết quá trình kiểm tra")
	)
	flag.Parse()

	c := &checker{
		root:       *root,
		modulePath: *modulePath,
		verbose:    *verbose,
		imports:    make(map[string][]string),
		pkgFiles:   make(map[string][]string),
	}

	if err := c.run(); err != nil {
		fmt.Fprintf(os.Stderr, "archcheck: %v\n", err)
		os.Exit(2)
	}

	c.report()
	if len(c.violations) > 0 {
		os.Exit(1)
	}
}

type checker struct {
	root       string
	modulePath string
	verbose    bool

	imports    map[string][]string // package → danh sách import nội bộ
	pkgFiles   map[string][]string // package → file thuộc package
	violations []Violation
	filesRead  int
}

func (c *checker) run() error {
	if err := c.checkForbiddenDirs(); err != nil {
		return err
	}
	if err := c.scanPackages(); err != nil {
		return err
	}
	c.checkDependencyCycles()
	return nil
}

// checkForbiddenDirs thực thi R7: cấm thư mục bãi rác (nguyên tắc P12).
func (c *checker) checkForbiddenDirs() error {
	return filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" || name == "node_modules" || name == "vendor" {
			return filepath.SkipDir
		}
		for _, forbidden := range forbiddenDirs {
			if name == forbidden {
				rel, _ := filepath.Rel(c.root, path)
				c.violations = append(c.violations, Violation{
					Rule: "R7",
					File: rel,
					Line: 0,
					Message: fmt.Sprintf(
						"thư mục %q bị cấm — trở thành bãi rác phụ thuộc", name),
					Hint: "phân loại rõ: kernel/ (khái niệm domain chung), " +
						"platform/ (hạ tầng trung lập), pkg/ (tiện ích thuần)",
				})
			}
		}
		return nil
	})
}

// scanPackages phân tích mọi file Go, thu thập import và áp dụng quy tắc.
func (c *checker) scanPackages() error {
	fset := token.NewFileSet()

	return filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("phân tích %s: %w", rel, err)
		}
		c.filesRead++

		fromPkg := c.importPathOf(filepath.Dir(rel))
		from := classify(c.modulePath, fromPkg)
		c.pkgFiles[fromPkg] = append(c.pkgFiles[fromPkg], rel)

		for _, spec := range f.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(importPath, c.modulePath) {
				continue // thư viện chuẩn hoặc bên thứ ba
			}

			c.imports[fromPkg] = appendUnique(c.imports[fromPkg], importPath)

			to := classify(c.modulePath, importPath)
			if rule, msg, hint := checkImport(from, to); rule != "" {
				pos := fset.Position(spec.Pos())
				c.violations = append(c.violations, Violation{
					Rule:    rule,
					File:    rel,
					Line:    pos.Line,
					Message: msg,
					Hint:    hint,
				})
			}
		}
		return nil
	})
}

// importPathOf chuyển đường dẫn thư mục tương đối thành import path.
func (c *checker) importPathOf(dir string) string {
	if dir == "." {
		return c.modulePath
	}
	return c.modulePath + "/" + filepath.ToSlash(dir)
}

// checkDependencyCycles thực thi R5: đồ thị phụ thuộc module phải là DAG.
//
// Phụ thuộc vòng làm module không thể kiểm thử độc lập, không thể tách,
// và làm thay đổi lan truyền không kiểm soát.
func (c *checker) checkDependencyCycles() {
	// Rút gọn đồ thị package thành đồ thị MODULE.
	moduleGraph := make(map[string]map[string]struct{})
	for fromPkg, tos := range c.imports {
		from := classify(c.modulePath, fromPkg)
		if from.Module == "" {
			continue
		}
		for _, toPkg := range tos {
			to := classify(c.modulePath, toPkg)
			if to.Module == "" || to.Module == from.Module {
				continue
			}
			if moduleGraph[from.Module] == nil {
				moduleGraph[from.Module] = make(map[string]struct{})
			}
			moduleGraph[from.Module][to.Module] = struct{}{}
		}
	}

	const (
		white = 0 // chưa thăm
		gray  = 1 // đang trong ngăn xếp đệ quy
		black = 2 // đã xong
	)
	state := make(map[string]int)
	var stack []string

	var visit func(string)
	visit = func(mod string) {
		state[mod] = gray
		stack = append(stack, mod)

		deps := make([]string, 0, len(moduleGraph[mod]))
		for d := range moduleGraph[mod] {
			deps = append(deps, d)
		}
		sort.Strings(deps) // thứ tự xác định để thông báo lỗi ổn định

		for _, dep := range deps {
			switch state[dep] {
			case white:
				visit(dep)
			case gray:
				// Tìm thấy chu trình: cắt ngăn xếp từ dep tới hiện tại.
				cycleStart := 0
				for i, m := range stack {
					if m == dep {
						cycleStart = i
						break
					}
				}
				cycle := append(append([]string{}, stack[cycleStart:]...), dep)
				c.violations = append(c.violations, Violation{
					Rule:    "R5",
					File:    "internal/modules/" + mod,
					Line:    0,
					Message: "phụ thuộc vòng: " + strings.Join(cycle, " → "),
					Hint: "giải bằng domain event (đảo một chiều), trích xuất phần chung " +
						"xuống tầng thấp hơn, hoặc xem lại ranh giới module",
				})
			}
		}
		stack = stack[:len(stack)-1]
		state[mod] = black
	}

	mods := make([]string, 0, len(moduleGraph))
	for m := range moduleGraph {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		if state[m] == white {
			visit(m)
		}
	}
}

func (c *checker) report() {
	if c.verbose {
		fmt.Printf("Đã đọc %d file Go, %d package nội bộ\n", c.filesRead, len(c.pkgFiles))
	}

	if len(c.violations) == 0 {
		fmt.Printf("archcheck: OK — %d file, không vi phạm ranh giới\n", c.filesRead)
		return
	}

	// Nhóm theo quy tắc để dễ đọc.
	sort.Slice(c.violations, func(i, j int) bool {
		if c.violations[i].Rule != c.violations[j].Rule {
			return c.violations[i].Rule < c.violations[j].Rule
		}
		if c.violations[i].File != c.violations[j].File {
			return c.violations[i].File < c.violations[j].File
		}
		return c.violations[i].Line < c.violations[j].Line
	})

	fmt.Fprintf(os.Stderr, "\narchcheck: PHÁT HIỆN %d VI PHẠM RANH GIỚI\n\n", len(c.violations))
	for _, v := range c.violations {
		fmt.Fprintln(os.Stderr, v)
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr, "Quy tắc: xem docs/adr/0005-module-boundaries.md")
	fmt.Fprintln(os.Stderr, "Ngoại lệ phải được ghi trong ADR và đánh dấu tường minh.")
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
