package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Tầng thứ tư của hợp đồng API: GIÁ TRỊ CHO PHÉP.
//
// # Hai lần lệch đã xảy ra thật
//
//	account_type   đặc tả khai 9 tài khoản, `domain.AccountType` có 13.
//	               `ACCOUNTS_RECEIVABLE` ĐÃ có mặt trong sổ cái, nên một
//	               client sinh kiểu từ đặc tả sẽ vỡ khi gặp response thật.
//	roles          đặc tả thiếu `CUSTOMER`, `SELLER_OWNER`, `SELLER_STAFF`,
//	               `CREATOR` — bốn vai trò phổ biến NHẤT. Giao diện phải
//	               viết `includes(r as never)` để đi qua trình biên dịch,
//	               tức tắt kiểu ở đúng chỗ kiểm phân quyền.
//	color_family   đặc tả nói `GRAY`, máy chủ lưu `GREY`. Bộ lọc theo màu
//	               im lặng trả rỗng.
//
// Cả ba đều tìm ra bằng tay. Phép kiểm này để lần sau máy tìm.

// readSpecEnums đọc mọi enum trong `api/`, khóa theo ĐƯỜNG DẪN.
//
// # Vì sao đường dẫn chứ không phải số dòng
//
// Một lần sửa mô tả ở trên làm mọi số dòng phía dưới trôi. Sổ ghi cặp phải
// sửa theo sau mỗi lần chỉnh chữ là sổ không ai giữ đúng.
//
// Đường dẫn dựng từ các khóa cha, bám theo THỤT LỀ — YAML của dự án này
// dùng thụt lề đều và do chính dự án viết ra.
func (c *checker) readSpecEnums() (map[string][]string, error) {
	root2 := filepath.Join(c.root, "api")
	out := map[string][]string{}

	err := filepath.WalkDir(root2, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root2, path)
		readEnumsInFile(filepath.ToSlash(rel), strings.Split(string(content), "\n"), out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

var (
	reKey       = regexp.MustCompile(`^(\s*)-?\s*([A-Za-z_][A-Za-z0-9_-]*):\s*(.*)$`)
	reEnumEntry = regexp.MustCompile(`^(\s*)-\s+(\S+)\s*$`)
	reEnumLine  = regexp.MustCompile(`^\s*enum:\s*\[(.+)\]\s*$`)
)

// readEnumsInFile quét một file và ghi các enum tìm được vào `out`.
func readEnumsInFile(label string, line []string, out map[string][]string) {
	// branches giữ chuỗi khóa cha đang mở, kèm thụt lề của từng khóa.
	type key struct {
		indent int
		label  string
	}
	var branches []key

	currentPath := func(indent int) string {
		var p []string
		for _, k := range branches {
			if k.indent < indent {
				p = append(p, k.label)
			}
		}
		return label + "#/" + strings.Join(p, "/")
	}

	for i, l := range line {
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}

		// Enum viết một dòng: `enum: [A, B, C]`.
		if m := reEnumLine.FindStringSubmatch(l); m != nil {
			indent := len(l) - len(strings.TrimLeft(l, " "))
			var v []string
			for _, x := range strings.Split(m[1], ",") {
				x = strings.Trim(strings.TrimSpace(x), `'"`)
				if x != "" {
					v = append(v, x)
				}
			}
			if len(v) >= 2 {
				// CÙNG đường dẫn với nhánh enum nhiều dòng: hai cách viết
				// YAML của một thứ phải cho một khóa, nếu không sổ ghép
				// phải biết đặc tả viết kiểu nào — một chi tiết không ai
				// nhớ và sẽ sai.
				out[currentPath(indent)] = v
			}
			continue
		}

		m := reKey.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		indent, k, rest := len(m[1]), m[2], strings.TrimSpace(m[3])

		// Cắt nhánh: mọi khóa cùng cấp hoặc sâu hơn đã đóng.
		for len(branches) > 0 && branches[len(branches)-1].indent >= indent {
			branches = branches[:len(branches)-1]
		}

		if k == "enum" && rest == "" {
			var v []string
			for j := i + 1; j < len(line); j++ {
				mm := reEnumEntry.FindStringSubmatch(line[j])
				if mm != nil && len(mm[1]) > indent-2 {
					v = append(v, strings.Trim(mm[2], `'"`))
					continue
				}
				if strings.TrimSpace(line[j]) == "" ||
					strings.HasPrefix(strings.TrimSpace(line[j]), "#") {
					continue
				}
				break
			}
			if len(v) >= 2 {
				out[currentPath(indent)] = v
			}
			continue
		}

		branches = append(branches, key{indent: indent, label: k})
	}
}

// readGoRows đọc các hằng số chuỗi của MỘT kiểu trong MỘT gói.
//
// Đọc AST chứ không grep: một giá trị xuất hiện trong bình luận hay trong
// thông điệp lỗi không phải là một giá trị hợp lệ của kiểu.
func (c *checker) readGoRows(pkg, kind string) ([]string, error) {
	dir := filepath.Join(c.root, "internal", filepath.FromSlash(pkg))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("đọc gói %s: %w", pkg, err)
	}

	var collected []string
	fset := token.NewFileSet()
	for _, e := range entries {
		label := e.Name()
		if e.IsDir() || !strings.HasSuffix(label, ".go") ||
			strings.HasSuffix(label, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, label), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("phân tích %s: %w", label, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			// Kiểu khai TRÊN dòng (`X Kieu = "..."`), hoặc thừa kế từ dòng
			// trước trong cùng khối `const` — trường hợp sau không đọc được
			// từ ValueSpec, nên chỉ nhận dạng thứ nhất. Mọi enum của dự án
			// này đều khai kiểu trên từng dòng.
			id, ok := vs.Type.(*ast.Ident)
			if !ok || id.Name != kind {
				return true
			}
			for _, v := range vs.Values {
				bl, ok := v.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				if s, err := strconv.Unquote(bl.Value); err == nil {
					collected = append(collected, s)
				}
			}
			return true
		})
	}
	if len(collected) == 0 {
		return nil, fmt.Errorf("không tìm thấy hằng số nào của kiểu %s.%s "+
			"— đổi tên kiểu thì phải sửa cả sổ `checkedEnumPairs`", pkg, kind)
	}
	sort.Strings(collected)
	return collected, nil
}

// compareEnums so từng cặp đã khai, rồi canh số enum chưa gác.
func (c *checker) compareEnums(spec map[string][]string) {
	paired := map[string]bool{}

	for _, pair := range c.enumPair {
		paired[pair.Path] = true

		sp, co := spec[pair.Path]
		if !co {
			c.violations = append(c.violations, fmt.Sprintf(
				"ENUM sổ ghép trỏ tới %q (%s) mà đặc tả KHÔNG có enum ở đường "+
					"dẫn đó — đường dẫn đổi thì phải sửa `checkedEnumPairs`",
				pair.Path, pair.Reason))
			continue
		}

		gv, err := c.readGoRows(pair.GoPkg, pair.GoType)
		if err != nil {
			c.violations = append(c.violations, fmt.Sprintf("ENUM %s: %v", pair.Reason, err))
			continue
		}

		goSet := stringSet(gv)
		specSet := stringSet(sp)

		// Đặc tả khai giá trị Go KHÔNG có: client viết một nhánh không bao
		// giờ chạy tới. KHÔNG BAO GIỜ hợp lệ.
		var extra []string
		for _, v := range sp {
			if !goSet[v] {
				extra = append(extra, v)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			c.violations = append(c.violations, fmt.Sprintf(
				"ENUM %s (%s): đặc tả khai %s mà Go KHÔNG có. Client sẽ viết "+
					"một nhánh không bao giờ chạy tới, hoặc mời người dùng "+
					"chọn một giá trị luôn trả về rỗng.",
				pair.Reason, pair.Path, strings.Join(extra, ", ")))
		}

		// Go có mà đặc tả thiếu: client sinh kiểu từ đặc tả sẽ vỡ khi gặp
		// response thật. Hợp lệ KHI cặp khai `AllowSubset`.
		var missing []string
		for _, v := range gv {
			if !specSet[v] {
				missing = append(missing, v)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 && !pair.AllowSubset {
			c.violations = append(c.violations, fmt.Sprintf(
				"ENUM %s (%s ↔ %s.%s): Go có %s mà đặc tả THIẾU. Client sinh "+
					"kiểu từ đặc tả sẽ thu hẹp kiểu sai rồi vỡ khi gặp "+
					"response thật.",
				pair.Reason, pair.Path, pair.GoPkg, pair.GoType,
				strings.Join(missing, ", ")))
		}
	}

	// Sổ miễn trừ trỏ tới thứ không còn tồn tại cũng là một dòng nói dối.
	for path := range c.unpairedEnums {
		paired[path] = true
		if _, co := spec[path]; !co {
			c.violations = append(c.violations, fmt.Sprintf(
				"ENUM sổ `unpairedEnums` còn dòng cho %q mà đặc tả không còn "+
					"enum ở đó — xóa đi", path))
		}
	}

	// CHỐT MỘT CHIỀU cho phần chưa gác.
	ungated := 0
	for path := range spec {
		if !paired[path] {
			ungated++
		}
	}
	c.ungatedEnums = ungated

	switch {
	case ungated > c.ungatedEnumLedger:
		c.violations = append(c.violations, fmt.Sprintf(
			"ENUM có %d enum chưa gác, vượt chốt %d. Enum mới thêm vào đặc "+
				"tả phải được QUYẾT: ghép cặp vào `checkedEnumPairs`, hoặc khai "+
				"lý do vào `unpairedEnums`.",
			ungated, c.ungatedEnumLedger))
	case ungated < c.ungatedEnumLedger:
		c.violations = append(c.violations, fmt.Sprintf(
			"ENUM chỉ còn %d enum chưa gác, chốt đang để %d. Hạ chốt xuống "+
				"%d trong `enum_cap.go` — chốt này chỉ được GIẢM, và một chốt "+
				"cao hơn thực tế cho phép lặng lẽ thêm enum không ai gác.",
			ungated, c.ungatedEnumLedger, ungated))
	}
}

func stringSet(v []string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, x := range v {
		m[x] = true
	}
	return m
}
