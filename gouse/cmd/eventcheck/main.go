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
//	R4  BỘ BÊN NHẬN lệch nhau   test chạy với hệ thống khác production
//
// R3 nghiêm trọng nhất và KHÔNG có ngoại lệ: một bên nhận không bao giờ
// chạy là mã người ta tin là đang chạy.
//
// R4 gác tầng NỐI DÂY thay vì hợp đồng event: xem `bo_ben_nhan.go`. Nó ra
// đời 01/10/2026, sau khi một phép phá KHÔNG ĐỎ cho thấy bộ bên nhận của
// test tích hợp thiếu đường ra của kho suốt nhiều tháng (P3-86).
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
	listAll := flag.Bool("liet-ke", false, "in bảng event rồi thoát")
	flag.Parse()

	c := moiChecker(*root)
	if err := c.run(); err != nil {
		fmt.Fprintln(os.Stderr, "eventcheck:", err)
		os.Exit(2)
	}

	if *listAll {
		c.tables()
		return
	}
	c.report(*verbose)
	if len(c.violations) > 0 {
		os.Exit(1)
	}
}

// moiChecker dựng checker với BỘ SỔ THẬT của dự án.
//
// Một hàm dựng duy nhất, dùng bởi `main` và bởi `TestTheRealProjectIsClean`. Dựng
// bằng tay ở hai chỗ là hai chỗ để quên một sổ — và nó đã xảy ra: thêm sổ
// của R4 làm bài test repo thật đỏ vì nó dựng riêng và không có sổ ấy.
func moiChecker(root string) *checker {
	return &checker{
		root:           root,
		noPublisher:    nobodyPublishes,
		noListener:     nobodyListens,
		chiOWorker:     receiversOnlyInWorker,
		declaredEvents: map[string]string{},
		publishedIn:    map[string][]string{},
		listenedIn:     map[string][]string{},
	}
}

type checker struct {
	root string

	// noPublisher và noListener là hai SỔ MIỄN TRỪ, khóa là tên event.
	noPublisher map[string]string
	noListener  map[string]string

	// chiOWorker là SỔ MIỄN TRỪ của R4, khóa là tên hàm tạo bên nhận.
	//
	// Là TRƯỜNG chứ không đọc thẳng biến gói, cùng lý do hai sổ trên: bộ
	// test của công cụ đo phép đối chiếu, không đo nội dung sổ của dự án
	// thật — nên nó truyền sổ RỖNG vào.
	chiOWorker map[string]string

	// declaredEvents: tên hằng (TypeOrderPlaced) -> giá trị ("order.placed").
	declaredEvents map[string]string

	// publishedIn và listenedIn: tên event -> các chỗ phát / các bên nhận.
	publishedIn map[string][]string
	listenedIn  map[string][]string

	violations []string

	// workerReceiverCount là số bên nhận worker đăng ký — chỉ để in báo cáo.
	workerReceiverCount int
}

func (c *checker) run() error {
	if err := c.readDeclarations(); err != nil {
		return err
	}
	if len(c.declaredEvents) == 0 {
		return fmt.Errorf("không tìm thấy hằng số loại event nào trong " +
			"internal/platform/eventbus — đổi chỗ khai thì phải sửa cả công cụ này")
	}
	if err := c.readGoConsts(); err != nil {
		return err
	}
	c.compare()
	return c.compareReceiverSets()
}

// reTenEvent khớp dạng `mien.su_kien` — quy ước đặt tên của eventbus.
var reTenEvent = regexp.MustCompile(`^[a-z][a-z_]*\.[a-z][a-z_]*$`)

// readDeclarations đọc bảng hằng số loại event.
//
// Đọc AST chứ không grep: một chuỗi `"order.placed"` trong chú thích hay
// trong thông điệp lỗi không phải một khai báo.
func (c *checker) readDeclarations() error {
	dir := filepath.Join(c.root, "internal", "platform", "eventbus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("đọc gói eventbus: %w", err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		ten := e.Name()
		if e.IsDir() || !strings.HasSuffix(ten, ".go") ||
			strings.HasSuffix(ten, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, ten), nil, 0)
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
				c.declaredEvents[name.Name] = v
			}
			return true
		})
	}
	return nil
}

// readGoConsts tìm nơi PHÁT và nơi NGHE từng loại event.
//
// Bỏ qua `_test.go`: một loại event chỉ xuất hiện trong test là một loại
// event production không bao giờ phát — đúng thứ công cụ này tìm.
func (c *checker) readGoConsts() error {
	fset := token.NewFileSet()

	return filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
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
		if strings.Contains(filepath.ToSlash(path), "/platform/eventbus/") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("phân tích %s: %w", path, err)
		}
		rel, _ := filepath.Rel(c.root, path)
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
			receiver := receiverName(fn)
			for _, ev := range c.rowsInNode(fn.Body) {
				c.listenedIn[ev] = append(c.listenedIn[ev], receiver)
			}
			continue
		}

		// Mọi chỗ khác: `eventbus.NewEvent(eventbus.TypeX, ...)` là PHÁT.
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !laNewEvent(call.Fun) || len(call.Args) == 0 {
				return true
			}
			for _, ev := range c.rowsInNode(call.Args[0]) {
				c.publishedIn[ev] = append(c.publishedIn[ev],
					fmt.Sprintf("%s:%s", ten, fn.Name.Name))
			}
			return true
		})
	}
}

// rowsInNode trả các GIÁ TRỊ event mà node nhắc tới qua `eventbus.TypeX`.
func (c *checker) rowsInNode(n ast.Node) []string {
	var ra []string
	ast.Inspect(n, func(x ast.Node) bool {
		sel, ok := x.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "eventbus" {
			return true
		}
		if v, co := c.declaredEvents[sel.Sel.Name]; co {
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

func receiverName(fn *ast.FuncDecl) string {
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

func (c *checker) compare() {
	for _, ev := range c.sortLines() {
		publish, listen := c.publishedIn[ev], c.listenedIn[ev]

		// R1 — khai mà không ai phát.
		if len(publish) == 0 {
			if reason, co := c.noPublisher[ev]; co {
				_ = reason
			} else {
				c.violations = append(c.violations, fmt.Sprintf(
					"R1 %q khai trong eventbus mà KHÔNG chỗ nào phát. Một cái "+
						"tên không có sự kiện đằng sau: người sau viết bên nhận "+
						"cho nó và bên nhận ấy không bao giờ chạy. Xóa hằng số, "+
						"hoặc khai lý do vào `nobodyPublishes`.", ev))
			}
		}

		// R2 — khai mà không ai nghe.
		if len(listen) == 0 {
			if _, co := c.noListener[ev]; !co {
				c.violations = append(c.violations, fmt.Sprintf(
					"R2 %q không bên nhận nào nghe. Phát một sự thật mà không "+
						"ai hành động là tốn một hàng outbox mỗi lần. Khai lý "+
						"do vào `nobodyListens` nếu đó là chủ ý.", ev))
			}
		}

		// R3 — nghe mà không ai phát. KHÔNG có ngoại lệ.
		if len(listen) > 0 && len(publish) == 0 {
			c.violations = append(c.violations, fmt.Sprintf(
				"R3 %q có bên nhận (%s) mà KHÔNG chỗ nào phát. Bên nhận ấy "+
					"chờ một event không bao giờ tới — mã người ta tin là "+
					"đang chạy.", ev, strings.Join(listen, ", ")))
		}
	}

	// Sổ miễn trừ trỏ tới thứ không còn tồn tại cũng là một dòng nói dối.
	for _, overrides := range []struct {
		ten string
		m   map[string]string
	}{{"nobodyPublishes", c.noPublisher}, {"nobodyListens", c.noListener}} {
		for ev := range overrides.m {
			if !c.hasDeclaration(ev) {
				c.violations = append(c.violations, fmt.Sprintf(
					"sổ `%s` còn dòng cho %q mà eventbus không còn khai loại "+
						"event ấy — xóa đi", overrides.ten, ev))
			}
		}
	}
	sort.Strings(c.violations)
}

func (c *checker) hasDeclaration(ev string) bool {
	for _, v := range c.declaredEvents {
		if v == ev {
			return true
		}
	}
	return false
}

func (c *checker) sortLines() []string {
	ra := make([]string, 0, len(c.declaredEvents))
	for _, v := range c.declaredEvents {
		ra = append(ra, v)
	}
	sort.Strings(ra)
	return ra
}

func (c *checker) tables() {
	fmt.Printf("%-34s %-6s %-6s %s\n", "EVENT", "PHÁT", "NGHE", "BÊN NHẬN")
	for _, ev := range c.sortLines() {
		fmt.Printf("%-34s %-6d %-6d %s\n",
			ev, len(c.publishedIn[ev]), len(c.listenedIn[ev]),
			strings.Join(c.listenedIn[ev], ", "))
	}
}

func (c *checker) report(verbose bool) {
	if verbose {
		var publish, listen int
		for _, ev := range c.sortLines() {
			if len(c.publishedIn[ev]) > 0 {
				publish++
			}
			if len(c.listenedIn[ev]) > 0 {
				listen++
			}
		}
		fmt.Printf("eventcheck: %d loại event khai · %d có nơi phát · "+
			"%d có bên nghe · %d miễn trừ phát · %d miễn trừ nghe\n",
			len(c.declaredEvents), publish, listen, len(c.noPublisher), len(c.noListener))
	}

	if len(c.violations) == 0 {
		fmt.Printf("eventcheck: OK — %d loại event, mọi loại đều có nơi phát "+
			"và bên nghe (hoặc đã khai lý do) · %d bên nhận của worker đều "+
			"có trong bộ của test (hoặc đã khai lý do)\n",
			len(c.declaredEvents), c.workerReceiverCount)
		return
	}
	for _, v := range c.violations {
		fmt.Printf("\n  %s\n", v)
	}
	fmt.Printf("\neventcheck: %d vi phạm\n", len(c.violations))
}
