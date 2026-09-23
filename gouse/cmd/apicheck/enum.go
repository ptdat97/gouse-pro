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

// docEnumDacTa đọc mọi enum trong `api/`, khóa theo ĐƯỜNG DẪN.
//
// # Vì sao đường dẫn chứ không phải số dòng
//
// Một lần sửa mô tả ở trên làm mọi số dòng phía dưới trôi. Sổ ghi cặp phải
// sửa theo sau mỗi lần chỉnh chữ là sổ không ai giữ đúng.
//
// Đường dẫn dựng từ các khóa cha, bám theo THỤT LỀ — YAML của dự án này
// dùng thụt lề đều và do chính dự án viết ra.
func (c *checker) docEnumDacTa() (map[string][]string, error) {
	goc := filepath.Join(c.root, "api")
	out := map[string][]string{}

	err := filepath.WalkDir(goc, func(duong string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		noiDung, err := os.ReadFile(duong)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(goc, duong)
		docEnumMotFile(filepath.ToSlash(rel), strings.Split(string(noiDung), "\n"), out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

var (
	reKhoa     = regexp.MustCompile(`^(\s*)-?\s*([A-Za-z_][A-Za-z0-9_-]*):\s*(.*)$`)
	reMucEnum  = regexp.MustCompile(`^(\s*)-\s+(\S+)\s*$`)
	reEnumDong = regexp.MustCompile(`^\s*enum:\s*\[(.+)\]\s*$`)
)

// docEnumMotFile quét một file và ghi các enum tìm được vào `out`.
func docEnumMotFile(ten string, dong []string, out map[string][]string) {
	// nganh giữ chuỗi khóa cha đang mở, kèm thụt lề của từng khóa.
	type khoa struct {
		thut int
		ten  string
	}
	var nganh []khoa

	duongHienTai := func(thut int) string {
		var p []string
		for _, k := range nganh {
			if k.thut < thut {
				p = append(p, k.ten)
			}
		}
		return ten + "#/" + strings.Join(p, "/")
	}

	for i, l := range dong {
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}

		// Enum viết một dòng: `enum: [A, B, C]`.
		if m := reEnumDong.FindStringSubmatch(l); m != nil {
			thut := len(l) - len(strings.TrimLeft(l, " "))
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
				out[duongHienTai(thut)] = v
			}
			continue
		}

		m := reKhoa.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		thut, k, phanConLai := len(m[1]), m[2], strings.TrimSpace(m[3])

		// Cắt nhánh: mọi khóa cùng cấp hoặc sâu hơn đã đóng.
		for len(nganh) > 0 && nganh[len(nganh)-1].thut >= thut {
			nganh = nganh[:len(nganh)-1]
		}

		if k == "enum" && phanConLai == "" {
			var v []string
			for j := i + 1; j < len(dong); j++ {
				mm := reMucEnum.FindStringSubmatch(dong[j])
				if mm != nil && len(mm[1]) > thut-2 {
					v = append(v, strings.Trim(mm[2], `'"`))
					continue
				}
				if strings.TrimSpace(dong[j]) == "" ||
					strings.HasPrefix(strings.TrimSpace(dong[j]), "#") {
					continue
				}
				break
			}
			if len(v) >= 2 {
				out[duongHienTai(thut)] = v
			}
			continue
		}

		nganh = append(nganh, khoa{thut: thut, ten: k})
	}
}

// docHangGo đọc các hằng số chuỗi của MỘT kiểu trong MỘT gói.
//
// Đọc AST chứ không grep: một giá trị xuất hiện trong bình luận hay trong
// thông điệp lỗi không phải là một giá trị hợp lệ của kiểu.
func (c *checker) docHangGo(goi, kieu string) ([]string, error) {
	thuMuc := filepath.Join(c.root, "internal", filepath.FromSlash(goi))
	vao, err := os.ReadDir(thuMuc)
	if err != nil {
		return nil, fmt.Errorf("đọc gói %s: %w", goi, err)
	}

	var ra []string
	fset := token.NewFileSet()
	for _, e := range vao {
		ten := e.Name()
		if e.IsDir() || !strings.HasSuffix(ten, ".go") ||
			strings.HasSuffix(ten, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(thuMuc, ten), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("phân tích %s: %w", ten, err)
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
			if !ok || id.Name != kieu {
				return true
			}
			for _, v := range vs.Values {
				bl, ok := v.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				if s, err := strconv.Unquote(bl.Value); err == nil {
					ra = append(ra, s)
				}
			}
			return true
		})
	}
	if len(ra) == 0 {
		return nil, fmt.Errorf("không tìm thấy hằng số nào của kiểu %s.%s "+
			"— đổi tên kiểu thì phải sửa cả sổ `capEnumDaKiem`", goi, kieu)
	}
	sort.Strings(ra)
	return ra, nil
}

// doiChieuEnum so từng cặp đã khai, rồi canh số enum chưa gác.
func (c *checker) doiChieuEnum(dacTa map[string][]string) {
	daGhep := map[string]bool{}

	for _, cap := range c.capEnum {
		daGhep[cap.Duong] = true

		sp, co := dacTa[cap.Duong]
		if !co {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"ENUM sổ ghép trỏ tới %q (%s) mà đặc tả KHÔNG có enum ở đường "+
					"dẫn đó — đường dẫn đổi thì phải sửa `capEnumDaKiem`",
				cap.Duong, cap.LyDo))
			continue
		}

		gv, err := c.docHangGo(cap.GoiGo, cap.KieuGo)
		if err != nil {
			c.viPham = append(c.viPham, fmt.Sprintf("ENUM %s: %v", cap.LyDo, err))
			continue
		}

		tapGo := tapChuoi(gv)
		tapSpec := tapChuoi(sp)

		// Đặc tả khai giá trị Go KHÔNG có: client viết một nhánh không bao
		// giờ chạy tới. KHÔNG BAO GIỜ hợp lệ.
		var thua []string
		for _, v := range sp {
			if !tapGo[v] {
				thua = append(thua, v)
			}
		}
		sort.Strings(thua)
		if len(thua) > 0 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"ENUM %s (%s): đặc tả khai %s mà Go KHÔNG có. Client sẽ viết "+
					"một nhánh không bao giờ chạy tới, hoặc mời người dùng "+
					"chọn một giá trị luôn trả về rỗng.",
				cap.LyDo, cap.Duong, strings.Join(thua, ", ")))
		}

		// Go có mà đặc tả thiếu: client sinh kiểu từ đặc tả sẽ vỡ khi gặp
		// response thật. Hợp lệ KHI cặp khai `ChoPhepTapCon`.
		var thieu []string
		for _, v := range gv {
			if !tapSpec[v] {
				thieu = append(thieu, v)
			}
		}
		sort.Strings(thieu)
		if len(thieu) > 0 && !cap.ChoPhepTapCon {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"ENUM %s (%s ↔ %s.%s): Go có %s mà đặc tả THIẾU. Client sinh "+
					"kiểu từ đặc tả sẽ thu hẹp kiểu sai rồi vỡ khi gặp "+
					"response thật.",
				cap.LyDo, cap.Duong, cap.GoiGo, cap.KieuGo,
				strings.Join(thieu, ", ")))
		}
	}

	// Sổ miễn trừ trỏ tới thứ không còn tồn tại cũng là một dòng nói dối.
	for duong := range c.enumKhongGhep {
		daGhep[duong] = true
		if _, co := dacTa[duong]; !co {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"ENUM sổ `enumKhongGhep` còn dòng cho %q mà đặc tả không còn "+
					"enum ở đó — xóa đi", duong))
		}
	}

	// CHỐT MỘT CHIỀU cho phần chưa gác.
	chuaGac := 0
	for duong := range dacTa {
		if !daGhep[duong] {
			chuaGac++
		}
	}
	c.soEnumChuaGac = chuaGac

	switch {
	case chuaGac > c.nguongEnumChuaGac:
		c.viPham = append(c.viPham, fmt.Sprintf(
			"ENUM có %d enum chưa gác, vượt chốt %d. Enum mới thêm vào đặc "+
				"tả phải được QUYẾT: ghép cặp vào `capEnumDaKiem`, hoặc khai "+
				"lý do vào `enumKhongGhep`.",
			chuaGac, c.nguongEnumChuaGac))
	case chuaGac < c.nguongEnumChuaGac:
		c.viPham = append(c.viPham, fmt.Sprintf(
			"ENUM chỉ còn %d enum chưa gác, chốt đang để %d. Hạ chốt xuống "+
				"%d trong `enum_cap.go` — chốt này chỉ được GIẢM, và một chốt "+
				"cao hơn thực tế cho phép lặng lẽ thêm enum không ai gác.",
			chuaGac, c.nguongEnumChuaGac, chuaGac))
	}
}

func tapChuoi(v []string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, x := range v {
		m[x] = true
	}
	return m
}
