// Command eventcheck đối chiếu HỢP ĐỒNG EVENT với cài đặt thật.
//
// # Vì sao cần một công cụ riêng
//
// `archcheck` gác ranh giới module, `apicheck` gác hợp đồng API với thế
// giới bên ngoài. Giữa hai cái đó có một hợp đồng thứ ba KHÔNG ai gác:
// event là cách các module nói chuyện với nhau, và nó vô hình với cả hai.
//
// Một loại event khai ra rồi không ai phát trông y hệt một loại event
// đang chạy: nó có tên, có hằng số, có người viết chú thích bàn về nó.
// Người sau viết một bên nhận cho nó, và bên nhận ấy KHÔNG BAO GIỜ chạy —
// không lỗi, không log, không có gì để lần theo.
//
// Đó không phải giả thiết. Ngày 25/09/2026 công cụ này tìm ra BỐN loại
// event ở đúng tình trạng ấy, trong đó có `order.placed` — event trung
// tâm nhất của cả hệ thống, được nhắc tên trong chú thích của hai module
// giải thích vì sao họ "không nghe order.placed".
//
// # Ba quy tắc
//
//	R1  khai mà KHÔNG AI PHÁT   một cái tên không có sự kiện đằng sau
//	R2  khai mà KHÔNG AI NGHE   một sự thật không ai hành động
//	R3  NGHE mà không ai phát   bên nhận chết, chờ một event không tới
//
// R3 nghiêm trọng nhất và KHÔNG có ngoại lệ: một bên nhận không bao giờ
// chạy là mã người ta tin là đang chạy.
//
// R1 và R2 có ngoại lệ hợp lệ, và mỗi ngoại lệ phải có LÝ DO trong sổ
// `soEvent` — cùng khuôn với các sổ của `apicheck`. Miễn trừ phải là một
// quyết định có người ký, không phải một chỗ bị bỏ quên.
package main

import (
	"flag"
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

func main() {
	root := flag.String("root", ".", "thư mục gốc của module Go")
	verbose := flag.Bool("v", false, "in cả những thứ ĐÚNG, không chỉ vi phạm")
	lietKe := flag.Bool("liet-ke", false, "in bảng event rồi thoát")
	flag.Parse()

	c := &checker{
		root:      *root,
		khongPhat: khongAiPhat,
		khongNghe: khongAiNghe,
		khaiEvent: map[string]string{},
		noiPhat:   map[string][]string{},
		noiNghe:   map[string][]string{},
	}
	if err := c.run(); err != nil {
		fmt.Fprintln(os.Stderr, "eventcheck:", err)
		os.Exit(2)
	}

	if *lietKe {
		c.bang()
		return
	}
	c.report(*verbose)
	if len(c.viPham) > 0 {
		os.Exit(1)
	}
}

type checker struct {
	root string

	// khongPhat và khongNghe là hai SỔ MIỄN TRỪ, khóa là tên event.
	khongPhat map[string]string
	khongNghe map[string]string

	// khaiEvent: tên hằng (TypeOrderPlaced) -> giá trị ("order.placed").
	khaiEvent map[string]string

	// noiPhat và noiNghe: tên event -> các chỗ phát / các bên nhận.
	noiPhat map[string][]string
	noiNghe map[string][]string

	viPham []string
}

func (c *checker) run() error {
	if err := c.docKhaiBao(); err != nil {
		return err
	}
	if len(c.khaiEvent) == 0 {
		return fmt.Errorf("không tìm thấy hằng số loại event nào trong " +
			"internal/platform/eventbus — đổi chỗ khai thì phải sửa cả công cụ này")
	}
	if err := c.docDungGo(); err != nil {
		return err
	}
	c.doiChieu()
	return nil
}

// reTenEvent khớp dạng `mien.su_kien` — quy ước đặt tên của eventbus.
var reTenEvent = regexp.MustCompile(`^[a-z][a-z_]*\.[a-z][a-z_]*$`)

// docKhaiBao đọc bảng hằng số loại event.
//
// Đọc AST chứ không grep: một chuỗi `"order.placed"` trong chú thích hay
// trong thông điệp lỗi không phải một khai báo.
func (c *checker) docKhaiBao() error {
	thuMuc := filepath.Join(c.root, "internal", "platform", "eventbus")
	vao, err := os.ReadDir(thuMuc)
	if err != nil {
		return fmt.Errorf("đọc gói eventbus: %w", err)
	}

	fset := token.NewFileSet()
	for _, e := range vao {
		ten := e.Name()
		if e.IsDir() || !strings.HasSuffix(ten, ".go") ||
			strings.HasSuffix(ten, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(thuMuc, ten), nil, 0)
		if err != nil {
			return fmt.Errorf("phân tích %s: %w", ten, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil || !reTenEvent.MatchString(v) {
					continue
				}
				c.khaiEvent[name.Name] = v
			}
			return true
		})
	}
	return nil
}

// docDungGo tìm nơi PHÁT và nơi NGHE từng loại event.
//
// Bỏ qua `_test.go`: một loại event chỉ xuất hiện trong test là một loại
// event production không bao giờ phát — đúng thứ công cụ này tìm.
func (c *checker) docDungGo() error {
	fset := token.NewFileSet()

	return filepath.WalkDir(c.root, func(duong string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// `_`-prefix là thư mục nháp, Go cũng bỏ qua.
			if strings.HasPrefix(d.Name(), "_") || d.Name() == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") ||
			strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		// Chính gói eventbus khai các hằng ấy; nó không phải bên phát.
		if strings.Contains(filepath.ToSlash(duong), "/platform/eventbus/") {
			return nil
		}

		f, err := parser.ParseFile(fset, duong, nil, 0)
		if err != nil {
			return fmt.Errorf("phân tích %s: %w", duong, err)
		}
		rel, _ := filepath.Rel(c.root, duong)
		c.quetFile(filepath.ToSlash(rel), f)
		return nil
	})
}

func (c *checker) quetFile(ten string, f *ast.File) {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		// `EventTypes()` khai bên nhận này NGHE những loại nào.
		if fn.Name.Name == "EventTypes" && fn.Recv != nil {
			ben := tenBenNhan(fn)
			for _, ev := range c.hangTrongNode(fn.Body) {
				c.noiNghe[ev] = append(c.noiNghe[ev], ben)
			}
			continue
		}

		// Mọi chỗ khác: `eventbus.NewEvent(eventbus.TypeX, ...)` là PHÁT.
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !laNewEvent(call.Fun) || len(call.Args) == 0 {
				return true
			}
			for _, ev := range c.hangTrongNode(call.Args[0]) {
				c.noiPhat[ev] = append(c.noiPhat[ev],
					fmt.Sprintf("%s:%s", ten, fn.Name.Name))
			}
			return true
		})
	}
}

// hangTrongNode trả các GIÁ TRỊ event mà node nhắc tới qua `eventbus.TypeX`.
func (c *checker) hangTrongNode(n ast.Node) []string {
	var ra []string
	ast.Inspect(n, func(x ast.Node) bool {
		sel, ok := x.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "eventbus" {
			return true
		}
		if v, co := c.khaiEvent[sel.Sel.Name]; co {
			ra = append(ra, v)
		}
		return true
	})
	return ra
}

func laNewEvent(fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "NewEvent" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "eventbus"
}

func tenBenNhan(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "?"
	}
	switch t := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return "?"
}

func (c *checker) doiChieu() {
	for _, ev := range c.sapXep() {
		phat, nghe := c.noiPhat[ev], c.noiNghe[ev]

		// R1 — khai mà không ai phát.
		if len(phat) == 0 {
			if lyDo, co := c.khongPhat[ev]; co {
				_ = lyDo
			} else {
				c.viPham = append(c.viPham, fmt.Sprintf(
					"R1 %q khai trong eventbus mà KHÔNG chỗ nào phát. Một cái "+
						"tên không có sự kiện đằng sau: người sau viết bên nhận "+
						"cho nó và bên nhận ấy không bao giờ chạy. Xóa hằng số, "+
						"hoặc khai lý do vào `khongAiPhat`.", ev))
			}
		}

		// R2 — khai mà không ai nghe.
		if len(nghe) == 0 {
			if _, co := c.khongNghe[ev]; !co {
				c.viPham = append(c.viPham, fmt.Sprintf(
					"R2 %q không bên nhận nào nghe. Phát một sự thật mà không "+
						"ai hành động là tốn một hàng outbox mỗi lần. Khai lý "+
						"do vào `khongAiNghe` nếu đó là chủ ý.", ev))
			}
		}

		// R3 — nghe mà không ai phát. KHÔNG có ngoại lệ.
		if len(nghe) > 0 && len(phat) == 0 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"R3 %q có bên nhận (%s) mà KHÔNG chỗ nào phát. Bên nhận ấy "+
					"chờ một event không bao giờ tới — mã người ta tin là "+
					"đang chạy.", ev, strings.Join(nghe, ", ")))
		}
	}

	// Sổ miễn trừ trỏ tới thứ không còn tồn tại cũng là một dòng nói dối.
	for _, so := range []struct {
		ten string
		m   map[string]string
	}{{"khongAiPhat", c.khongPhat}, {"khongAiNghe", c.khongNghe}} {
		for ev := range so.m {
			if !c.coKhai(ev) {
				c.viPham = append(c.viPham, fmt.Sprintf(
					"sổ `%s` còn dòng cho %q mà eventbus không còn khai loại "+
						"event ấy — xóa đi", so.ten, ev))
			}
		}
	}
	sort.Strings(c.viPham)
}

func (c *checker) coKhai(ev string) bool {
	for _, v := range c.khaiEvent {
		if v == ev {
			return true
		}
	}
	return false
}

func (c *checker) sapXep() []string {
	ra := make([]string, 0, len(c.khaiEvent))
	for _, v := range c.khaiEvent {
		ra = append(ra, v)
	}
	sort.Strings(ra)
	return ra
}

func (c *checker) bang() {
	fmt.Printf("%-34s %-6s %-6s %s\n", "EVENT", "PHÁT", "NGHE", "BÊN NHẬN")
	for _, ev := range c.sapXep() {
		fmt.Printf("%-34s %-6d %-6d %s\n",
			ev, len(c.noiPhat[ev]), len(c.noiNghe[ev]),
			strings.Join(c.noiNghe[ev], ", "))
	}
}

func (c *checker) report(verbose bool) {
	if verbose {
		var phat, nghe int
		for _, ev := range c.sapXep() {
			if len(c.noiPhat[ev]) > 0 {
				phat++
			}
			if len(c.noiNghe[ev]) > 0 {
				nghe++
			}
		}
		fmt.Printf("eventcheck: %d loại event khai · %d có nơi phát · "+
			"%d có bên nghe · %d miễn trừ phát · %d miễn trừ nghe\n",
			len(c.khaiEvent), phat, nghe, len(c.khongPhat), len(c.khongNghe))
	}

	if len(c.viPham) == 0 {
		fmt.Printf("eventcheck: OK — %d loại event, mọi loại đều có nơi phát "+
			"và bên nghe (hoặc đã khai lý do)\n", len(c.khaiEvent))
		return
	}
	for _, v := range c.viPham {
		fmt.Printf("\n  %s\n", v)
	}
	fmt.Printf("\neventcheck: %d vi phạm\n", len(c.viPham))
}
