package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// R10 — MICROKERNEL: module EXTENSION không được bên nào import.
//
// # Vì sao R5 không đủ
//
// R5 chặn phụ thuộc VÒNG. Nó không chặn một phụ thuộc MỘT CHIỀU
// `checkout → notification`: đồ thị ấy vẫn là DAG, nên R5 im lặng cho qua —
// và ranh giới microkernel bị phá mà không vi phạm quy tắc nào hiện có.
//
// Một dòng import như vậy không trông giống lỗi. Nó trông giống tiện:
// checkout muốn gửi thư xác nhận ngay, và `notification` có sẵn hàm ấy. Sau
// dòng đó, tháo `notification` ra làm `checkout` không biên dịch được, và câu
// "tháo extension ra thì khách vẫn mua được hàng" thành câu không đúng nữa.
//
// # Bản đầu của quy tắc này SAI, và nó sai theo cách đáng ghi
//
// Bản đầu suy tập extension bằng cách lấy bao đóng phụ thuộc từ `checkout` —
// cái gì ngoài bao đóng là extension. Nghe hợp lý, và nó là một lập luận
// VÒNG: `checkout` import `notification` thì `notification` vào bao đóng, tức
// thành kernel, tức không có vi phạm nào. Phép phá đầu tiên cho thấy đúng
// thế: thêm một dòng import vào `checkout/adapters.go` và công cụ báo OK.
//
// Một quy tắc suy ra ranh giới TỪ thứ nó đang gác thì không gác được gì.
//
// # Nên phân loại là QUYẾT ĐỊNH, còn tính nhất quán là phép ĐO
//
// Tập extension khai tường minh ở `laExtension` dưới đây — nó là một quyết
// định, ADR-0021 và ADR-0022 ghi nó. Thứ công cụ này đo là ba điều:
//
//	R10a  module khai EXTENSION phải KHÔNG BÊN NÀO import
//	R10b  module không khai gì và cũng không nằm trên đường mua hàng
//	      → chưa ai phân loại nó
//	R10c  sổ khai một module KHÔNG tồn tại → sổ đã mục
//
// R10a là quy tắc chính, và nó MẠNH HƠN "kernel không import extension": nó
// cũng chặn extension import extension — thứ làm hai extension dính nhau rồi
// không tháo riêng được cái nào.
//
// Xem docs/adr/0022-microkernel-kernel-on-dinh.md và
// docs/03-architecture/microkernel.md.

// moduleGoc là GỐC của bao đóng định nghĩa ĐƯỜNG MUA HÀNG.
//
// `checkout` chứ không `order`: một đơn hàng chỉ ra đời từ một phiên thanh
// toán hoàn tất, nên `checkout` là điểm vào thật của việc bán hàng.
//
// Bao đóng này KHÔNG dùng để suy tập extension (xem lý do ở trên). Nó chỉ
// dùng cho R10b: một module nằm trên đường mua hàng thì đã được phân loại
// bằng phép đo, không cần khai tay.
const moduleGoc = "checkout"

// nangLucCatNgang là các module tài liệu xếp KERNEL mà nằm NGOÀI đường mua
// hàng — tức phán đoán, không phải phép đo.
//
// Đây là chỗ DUY NHẤT của R10 dựa vào phán đoán, nên mỗi dòng phải nêu vì
// sao. Khách vãng lai mua được hàng mà không cần cả hai, nên bao đóng của
// `checkout` không chứa chúng; xếp vào kernel là phán đoán ADR-0022 ghi ra
// để bàn lại được.
//
// Thiếu một dòng ở đây thì R10c ĐỎ với chính module ấy — nên sổ này không
// mục trong im lặng, và nó cũng không thể dùng để miễn trừ một module nằm
// trên đường mua hàng (những module ấy đã được PHÉP ĐO phân loại).
var nangLucCatNgang = map[string]string{
	"identity": "ai đang gọi. Khách vãng lai mua được mà không đăng nhập, " +
		"nhưng nhà bán và quản trị thì không — mọi mô hình kinh doanh đều cần",

	"customer": "khách là ai: địa chỉ, đồng ý nhận thư, yêu thích. " +
		"`customer_consent` là bản ghi PHÁP LÝ — xem P3-54",
}

// Tầng của một module, đọc từ hàng `**Tầng (microkernel)**` của tài liệu.
type tangModule int

const (
	tangChuaBiet tangModule = iota
	tangKernel
	tangExtension
	tangChuaXep
)

// reTangMicrokernel tách giá trị hàng `| **Tầng (microkernel)** | ... |`.
//
// Hàng này nằm ở bảng đầu mỗi tài liệu module, cạnh `Bounded Context` và
// `Phân loại`. Hai dấu `**` quanh giá trị là tùy chọn.
var reTangMicrokernel = regexp.MustCompile(
	`(?m)^\|\s*\*\*Tầng \(microkernel\)\*\*\s*\|\s*\**([^|*]+?)\**\s*\|`)

// docTangModule đọc tầng của mọi module từ `docs/04-modules/*.md`.
//
// # Vì sao nguồn sự thật là TÀI LIỆU, không phải một sổ trong Go
//
// Cùng lý do R6 đọc mục "Dữ liệu sở hữu" của chính tài liệu: một bản sao thứ
// hai của bảng phân loại sớm muộn sẽ lệch với tài liệu, và khi ấy không biết
// bên nào đúng.
//
// Bản đầu của R10 giữ ba sổ trong file này, rồi tài liệu module cũng có hàng
// `Tầng (microkernel)` — hai nguồn cho cùng một phân loại, đúng dạng lỗi dự
// án này gác ở mọi chỗ khác. Bỏ sổ, đọc tài liệu.
//
// Trả LỖI (không phải vi phạm) khi tài liệu thiếu hoặc ghi giá trị lạ: không
// có bảng phân loại đáng tin thì R10 không kiểm được gì, và một quy tắc im
// lặng không kiểm gì vẫn in ra `OK` — thứ tệ nhất.
func (c *checker) docTangModule() (map[string]tangModule, error) {
	goc := filepath.Join(c.root, "..", "docs", "04-modules")
	vao, err := os.ReadDir(goc)
	if err != nil {
		return nil, fmt.Errorf("đọc tài liệu module: %w\n"+
			"(R10 đọc hàng \"Tầng (microkernel)\" của chính tài liệu — đổi "+
			"chỗ thư mục ấy thì phải sửa cả công cụ này)", err)
	}

	ra := map[string]tangModule{}
	for _, e := range vao {
		ten := e.Name()
		if e.IsDir() || !strings.HasSuffix(ten, ".md") || ten == "README.md" {
			continue
		}
		noiDung, err := os.ReadFile(filepath.Join(goc, ten))
		if err != nil {
			return nil, err
		}

		mod := strings.TrimSuffix(ten, ".md")
		if m, co := tenTaiLieuKhacTenModule[mod]; co {
			mod = m
		}

		m := reTangMicrokernel.FindSubmatch(noiDung)
		if m == nil {
			return nil, fmt.Errorf("tài liệu %s không có hàng \"**Tầng "+
				"(microkernel)**\" — R10 không biết module ấy là kernel hay "+
				"extension. Trả lời \"tháo nó ra thì mất gì\" rồi thêm hàng "+
				"ấy vào bảng đầu tài liệu (xem docs/04-modules/README.md)",
				ten)
		}

		gt := strings.TrimSpace(string(m[1]))
		switch {
		case strings.HasPrefix(gt, "KERNEL"):
			ra[mod] = tangKernel
		case strings.HasPrefix(gt, "EXTENSION"):
			ra[mod] = tangExtension
		case strings.HasPrefix(gt, "CHƯA XẾP ĐƯỢC"):
			ra[mod] = tangChuaXep
		default:
			return nil, fmt.Errorf("tài liệu %s ghi tầng %q — R10 chỉ hiểu "+
				"\"KERNEL …\", \"EXTENSION …\" hoặc \"CHƯA XẾP ĐƯỢC\"",
				ten, gt)
		}
	}
	return ra, nil
}

// kiemMicrokernel thực thi R10.
//
// Trả LỖI khi bảng phân loại không đọc được — xem `docTangModule`.
func (c *checker) kiemMicrokernel() error {
	// `goc` rỗng = R10 TẮT. Dùng bởi bộ test của công cụ: một dự án giả ba
	// module không có kernel để mà gác.
	if c.goc == "" {
		return nil
	}

	doThi, moduleCo := c.doThiModule()
	if len(moduleCo) == 0 {
		return nil
	}

	tang, err := c.docTangModule()
	if err != nil {
		return err
	}

	if !moduleCo[c.goc] {
		c.violations = append(c.violations, Violation{
			Rule: "R10",
			File: "internal/modules",
			Message: fmt.Sprintf(
				"không tìm thấy module gốc %q — R10b dùng bao đóng phụ thuộc "+
					"của nó để biết module nào đã phân loại bằng phép đo",
				c.goc),
			Hint: "đổi tên hoặc bỏ module ấy thì phải sửa cả `moduleGoc` " +
				"trong cmd/archcheck/microkernel.go",
		})
		return nil
	}

	nguoiDung := nguoiImport(doThi)
	duongMuaHang := baoDong(doThi, c.goc)

	ten := make([]string, 0, len(moduleCo))
	for m := range moduleCo {
		ten = append(ten, m)
	}
	sort.Strings(ten)

	var soKernel, soExt int
	for _, m := range ten {
		switch tang[m] {
		case tangKernel:
			soKernel++

		case tangExtension:
			soExt++

			// ---- R10a: extension phải KHÔNG BÊN NÀO import ----
			dung := nguoiDung[m]
			if len(dung) == 0 {
				continue
			}
			sort.Strings(dung)
			c.violations = append(c.violations, Violation{
				Rule: "R10",
				File: "internal/modules/" + dung[0],
				Message: fmt.Sprintf("module EXTENSION %q bị %s import",
					m, strings.Join(dung, ", ")),
				Hint: fmt.Sprintf(
					"tháo %q ra phải chỉ mất tính năng của nó, nhưng sau "+
						"dòng import này nó làm %s không biên dịch được. Đảo "+
						"chiều bằng event, hoặc để %q đọc ngược qua cổng "+
						"công khai (ADR-0022)",
					m, strings.Join(dung, ", "), m),
			})

		case tangChuaXep:
			// KHÔNG kết luận gì: xếp sai còn tệ hơn để trống, vì một module
			// gán nhãn "kernel" sẽ được người sau tin là ổn định.
			if c.verbose {
				fmt.Printf("  R10: %s CHƯA XẾP ĐƯỢC — bỏ qua\n", m)
			}

		default:
			// ---- R10b: module chưa được phân loại ----
			//
			// Chỉ xảy ra khi thư mục module không tra ra tài liệu —
			// `docSoHuuBang` của R6 đã trả LỖI cho trường hợp ấy, nên nhánh
			// này là lớp bảo vệ thứ hai, không phải đường thường đi.
			c.violations = append(c.violations, Violation{
				Rule: "R10",
				File: "internal/modules/" + m,
				Message: fmt.Sprintf(
					"module %q không tra ra tầng microkernel nào", m),
				Hint: "thêm hàng `| **Tầng (microkernel)** | KERNEL … |` " +
					"(hoặc EXTENSION …, CHƯA XẾP ĐƯỢC) vào bảng đầu " +
					"docs/04-modules/<module>.md",
			})
		}

		// ---- R10c: tài liệu nói KERNEL mà module nằm ngoài đường mua hàng
		// và không ai import — dấu hiệu phân loại đã cũ ----
		if tang[m] == tangKernel && !duongMuaHang[m] && len(nguoiDung[m]) == 0 {
			if _, catNgang := c.catNgang[m]; !catNgang {
				c.violations = append(c.violations, Violation{
					Rule: "R10",
					File: "docs/04-modules",
					Message: fmt.Sprintf(
						"tài liệu xếp %q là KERNEL, nhưng nó KHÔNG nằm trên "+
							"đường mua hàng và KHÔNG module nào import nó", m),
					Hint: "đó là hình dạng của một EXTENSION. Hoặc sửa tài " +
						"liệu, hoặc khai nó là năng lực cắt ngang trong " +
						"`nangLucCatNgang` (cmd/archcheck/microkernel.go) " +
						"kèm lý do mọi mô hình kinh doanh đều cần nó",
				})
			}
		}
	}

	// Lý do khai cho một năng lực cắt ngang KHÔNG tồn tại là một sổ đã mục.
	catNgang := make([]string, 0, len(c.catNgang))
	for m := range c.catNgang {
		catNgang = append(catNgang, m)
	}
	sort.Strings(catNgang)
	for _, m := range catNgang {
		if moduleCo[m] {
			continue
		}
		c.violations = append(c.violations, Violation{
			Rule: "R10",
			File: "cmd/archcheck/microkernel.go",
			Message: fmt.Sprintf("sổ `nangLucCatNgang` khai module %q mà "+
				"internal/modules KHÔNG có nó", m),
			Hint: "module đã đổi tên hoặc bị xóa; bỏ dòng ấy khỏi sổ",
		})
	}

	c.soKernel = soKernel
	c.soExtension = soExt
	return nil
}

// nguoiImport đảo đồ thị: module -> các module import nó.
func nguoiImport(doThi map[string]map[string]bool) map[string][]string {
	ra := map[string][]string{}
	for from, tos := range doThi {
		for to := range tos {
			ra[to] = append(ra[to], from)
		}
	}
	return ra
}

// doThiModule rút gọn đồ thị package thành đồ thị MODULE.
//
// Tách thành hàm riêng để R5 và R10 dùng CÙNG một phép rút gọn: hai bản cài
// của cùng một đồ thị sớm muộn sẽ khác nhau, và khi ấy hai quy tắc nói về hai
// hệ thống khác nhau.
func (c *checker) doThiModule() (map[string]map[string]bool, map[string]bool) {
	doThi := map[string]map[string]bool{}
	co := map[string]bool{}

	// Tập module lấy từ `pkgFiles` — MỌI package đã quét — không từ
	// `imports`.
	//
	// `imports` chỉ có package CÓ import nội bộ, nên một module mới chưa
	// import gì sẽ vô hình. Phép phá đầu tiên cho thấy đúng thế: dựng một
	// module `loyalty` chỉ có `public.go` không import gì, và R10b im lặng
	// cho qua — đúng lúc nó cần kêu nhất, vì module mới là lúc người ta
	// chưa phân loại.
	for pkg := range c.pkgFiles {
		if m := classify(c.modulePath, pkg).Module; m != "" {
			co[m] = true
			if doThi[m] == nil {
				doThi[m] = map[string]bool{}
			}
		}
	}

	for fromPkg, tos := range c.imports {
		from := classify(c.modulePath, fromPkg)
		if from.Module == "" {
			continue
		}
		co[from.Module] = true
		if doThi[from.Module] == nil {
			doThi[from.Module] = map[string]bool{}
		}

		for _, toPkg := range tos {
			to := classify(c.modulePath, toPkg)
			if to.Module == "" || to.Module == from.Module {
				continue
			}
			co[to.Module] = true
			doThi[from.Module][to.Module] = true
		}
	}
	return doThi, co
}

// baoDong trả bao đóng phụ thuộc (đóng kín theo chiều import) từ một gốc.
func baoDong(doThi map[string]map[string]bool, goc string) map[string]bool {
	ra := map[string]bool{}
	var di func(string)
	di = func(m string) {
		if ra[m] {
			return
		}
		ra[m] = true
		for to := range doThi[m] {
			di(to)
		}
	}
	di(goc)
	return ra
}
