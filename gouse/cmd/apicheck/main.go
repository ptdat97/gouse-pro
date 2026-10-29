// apicheck đối chiếu ĐẶC TẢ OpenAPI với TUYẾN đã đăng ký trong mã Go.
//
// # Vì sao cần một công cụ riêng
//
// Backlog mục 2.4 ghi "OpenAPI là nguồn sự thật DUY NHẤT" và đánh dấu XANH
// vì CI có `types:check`. Nhưng `types:check` chỉ so ĐẶC TẢ với TypeScript
// sinh ra từ chính nó — nó không biết gì về route đã đăng ký trong Go.
//
// Ngày 17/09/2026, phép so thủ công tìm ra SÁU tuyến sống ngoài hợp đồng,
// trong đó có cả luồng trả hàng phía nhà bán: duyệt, từ chối, nhận hàng.
// Chúng chạy được, có test, và `openapi-typescript` không sinh kiểu cho
// chúng — nên giao diện nhà bán không gọi được theo cách có kiểu.
//
// Một dòng xanh cho một phép kiểm không kiểm thứ nó nói là tệ hơn không có
// dòng nào: nó khiến người đọc thôi nhìn.
//
// # Hai hướng lệch, và chúng KHÔNG đối xứng
//
//	tuyến có, đặc tả không   LỖI. Endpoint ngoài hợp đồng là endpoint
//	                         client không gọi được và không ai rà soát.
//	đặc tả có, tuyến không   phải KHAI vào `notImplemented` kèm lý do. Phần lớn
//	                         là Phase 2/3 — hợp lệ, nhưng phải nói ra.
//
// Nhờ hướng thứ hai mà câu "24 thao tác chưa cài, 20 thuộc Phase 2/3"
// thôi là một dòng văn trong tài liệu và thành một danh sách máy giữ.
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
	var (
		root    = flag.String("root", ".", "thư mục gốc dự án")
		verbose = flag.Bool("v", false, "in chi tiết quá trình kiểm tra")

		// listEnums in đường dẫn của MỌI enum trong đặc tả.
		//
		// Sổ `checkedEnumPairs` khóa theo đường dẫn, và đường dẫn là thứ không
		// đoán được — nó do bộ đọc dựng ra từ thụt lề. Không có cờ này thì
		// mỗi lần thêm một cặp là một vòng đoán rồi chạy rồi sửa.
		listEnums = flag.Bool("liet-ke-enum", false,
			"in đường dẫn và giá trị của mọi enum trong đặc tả, rồi thoát")
	)
	flag.Parse()

	if *listEnums {
		c := &checker{root: *root}
		ds, err := c.readSpecEnums()
		if err != nil {
			fmt.Fprintf(os.Stderr, "apicheck: %v\n", err)
			os.Exit(2)
		}
		path := make([]string, 0, len(ds))
		for d := range ds {
			path = append(path, d)
		}
		sort.Strings(path)
		for _, d := range path {
			fmt.Printf("%-78s %s\n", d, strings.Join(ds[d], ", "))
		}
		fmt.Printf("\n%d enum\n", len(ds))
		return
	}

	c := &checker{
		root: *root, verbose: *verbose,
		notImplemented: notImplemented, outsideContract: outsideContract,
		headersOutsideSpec:   headersOutsideSpec,
		headersNotViaBrowser: headersNotViaBrowser,
		enumPair:             checkedEnumPairs,
		unpairedEnums:        unpairedEnums,
		ungatedEnumLedger:    ungatedEnums,
		dtoPair:              checkedDTOPairs,
		ungatedDTOLedger:     ungatedDTOs,
	}
	if err := c.run(); err != nil {
		fmt.Fprintf(os.Stderr, "apicheck: %v\n", err)
		os.Exit(2)
	}

	c.report()
	if len(c.violations) > 0 {
		os.Exit(1)
	}
}

// operation là một cặp (method, đường dẫn).
type operation struct {
	Method string
	Path   string
}

func (t operation) String() string { return t.Method + " " + t.Path }

type checker struct {
	root    string
	verbose bool

	// Hai sổ khai báo, truyền vào chứ không đọc biến gói.
	//
	// Nhờ vậy bài test dựng được một dự án giả với sổ RỖNG: nếu công cụ
	// đọc thẳng sổ thật thì mọi bài test đều kèm theo mười chín vi phạm
	// của dự án thật, và không bài nào đo được thứ nó muốn đo.
	notImplemented  map[string]string
	outsideContract map[string]string

	// Hai sổ cho tầng HEADER — xem header.go.
	headersOutsideSpec   map[string]string
	headersNotViaBrowser map[string]string

	// Hai sổ + một chốt cho tầng ENUM — xem enum.go.
	enumPair          []enumPair
	unpairedEnums     map[string]string
	ungatedEnumLedger int
	ungatedEnums      int

	dtoPair          []dtoPair
	ungatedDTOLedger int
	ungatedDTOs      int

	spec  map[operation]string // thao tác -> file đặc tả khai nó
	route map[operation]string // tuyến   -> file Go đăng ký nó

	specHeaders map[string]string // header -> file đặc tả khai nó
	headerCORS  map[string]bool   // header CORS cho phép

	violations []string
}

func (c *checker) run() error {
	var err error
	if c.spec, err = c.readSpec(); err != nil {
		return err
	}
	if c.route, err = c.readRoutes(); err != nil {
		return err
	}
	if len(c.spec) == 0 || len(c.route) == 0 {
		return fmt.Errorf("đọc được %d thao tác và %d tuyến — quá ít để "+
			"kết luận gì; kiểm lại -root", len(c.spec), len(c.route))
	}

	if c.specHeaders, err = c.readSpecHeaders(); err != nil {
		return err
	}
	if c.headerCORS, err = c.readCORSHeaders(); err != nil {
		return err
	}

	specEnums, err := c.readSpecEnums()
	if err != nil {
		return err
	}

	dtoProperties, err := c.readSchemaProperties()
	if err != nil {
		return err
	}

	c.compare()
	c.compareHeaders(c.specHeaders, c.headerCORS)
	c.compareEnums(specEnums)
	c.compareDTO(dtoProperties)
	c.warnUngatedDTOs(dtoProperties)
	return nil
}

// warnUngatedDTOs là CHỐT MỘT CHIỀU cho các schema chưa ghép cặp.
//
// Chỉ đếm schema CÓ thuộc tính: một schema chỉ có `type: string` hay
// `$ref` không có hình dạng để mà so.
func (c *checker) warnUngatedDTOs(properties map[string][]string) {
	paired := map[string]bool{}
	for _, pair := range c.dtoPair {
		paired[pair.Schema] = true
	}

	ungated := 0
	for ten, props := range properties {
		if len(props) > 0 && !paired[ten] {
			ungated++
		}
	}
	c.ungatedDTOs = ungated

	switch {
	case ungated > c.ungatedDTOLedger:
		c.violations = append(c.violations, fmt.Sprintf(
			"DTO có %d schema chưa gác hình dạng, vượt chốt %d. Schema mới "+
				"thêm vào đặc tả phải được QUYẾT: ghép cặp vào `checkedDTOPairs`, "+
				"hoặc hạ chốt nếu nó thật sự chưa có struct Go nào phục vụ.",
			ungated, c.ungatedDTOLedger))
	case ungated < c.ungatedDTOLedger:
		c.violations = append(c.violations, fmt.Sprintf(
			"DTO chỉ còn %d schema chưa gác, chốt đang để %d. Hạ chốt xuống "+
				"%d trong `dto_cap.go` — chốt này chỉ được GIẢM, và một chốt "+
				"cao hơn thực tế cho phép lặng lẽ thêm schema không ai gác.",
			ungated, c.ungatedDTOLedger, ungated))
	}
}

// ------------------------------------------------------------------ Đặc tả

var (
	// Dòng khai một đường dẫn trong openapi.yaml: hai dấu cách rồi "/...:".
	rePath = regexp.MustCompile(`^  (/\S*):\s*$`)

	// Dòng $ref trỏ tới khối trong file con.
	reRef = regexp.MustCompile(`^\s+\$ref:\s*'\./paths/([^#']+)#/([^']+)'\s*$`)

	// Khóa method bên trong một khối thao tác.
	reMethod = regexp.MustCompile(`^  (get|post|put|patch|delete):\s*$`)
)

// readSpec đọc mọi thao tác khai trong api/openapi.yaml.
//
// # Vì sao TỰ PHÂN TÍCH thay vì dùng thư viện YAML
//
// Dự án có đúng bốn phụ thuộc trực tiếp, và thêm cái thứ năm cho một công
// cụ kiểm tra là một cái giá không đổi lấy gì: cấu trúc file đặc tả đều
// đặn và do chính dự án viết ra.
//
// Cái giá phải trả là bộ đọc này KHÔNG được dễ dãi. Gặp thứ nó không hiểu
// thì nó BÁO LỖI chứ không bỏ qua — một bộ đọc âm thầm bỏ qua sẽ dựng lại
// đúng vấn đề mà công cụ này sinh ra để sửa: một dấu xanh cho một phép
// kiểm không kiểm gì.
func (c *checker) readSpec() (map[operation]string, error) {
	root2 := filepath.Join(c.root, "api", "openapi.yaml")
	content, err := os.ReadFile(root2)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", root2, err)
	}

	out := map[operation]string{}
	line := strings.Split(string(content), "\n")

	for i := 0; i < len(line); i++ {
		m := rePath.FindStringSubmatch(line[i])
		if m == nil {
			continue
		}
		fullPath := m[1]

		// Dòng ngay sau PHẢI là $ref. Đặc tả này không dùng cách khai nào
		// khác, và nếu có thì công cụ phải biết trước khi tin kết quả.
		if i+1 >= len(line) {
			return nil, fmt.Errorf("%s: đường dẫn %q ở cuối file, không có $ref",
				root2, fullPath)
		}
		r := reRef.FindStringSubmatch(line[i+1])
		if r == nil {
			return nil, fmt.Errorf(
				"%s dòng %d: đường dẫn %q không theo sau bởi $ref tới paths/ — "+
					"apicheck chỉ hiểu cách khai đó, và đoán bừa ở đây nghĩa là "+
					"bỏ sót thao tác mà vẫn báo xanh",
				root2, i+2, fullPath)
		}

		file, neo := r[1], r[2]
		methods, err := c.readBlock(filepath.Join(c.root, "api", "paths", file), neo)
		if err != nil {
			return nil, err
		}
		for _, meth := range methods {
			out[operation{Method: meth, Path: fullPath}] = file + "#/" + neo
		}
		i++
	}
	return out, nil
}

// readBlock trả danh sách method của một khối thao tác trong file paths/.
func (c *checker) readBlock(file, neo string) ([]string, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", file, err)
	}

	line := strings.Split(string(content), "\n")
	start := -1
	for i, d := range line {
		if d == neo+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("%s: không tìm thấy khối %q mà openapi.yaml trỏ tới",
			file, neo)
	}

	var out []string
	for i := start + 1; i < len(line); i++ {
		d := line[i]
		// Khối kết thúc khi gặp khóa ở cột 0.
		if d != "" && !strings.HasPrefix(d, " ") {
			break
		}
		if m := reMethod.FindStringSubmatch(d); m != nil {
			out = append(out, strings.ToUpper(m[1]))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: khối %q không khai method nào", file, neo)
	}
	return out, nil
}

// ------------------------------------------------------------------- Tuyến

var reRoute = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) (/\S*)$`)

// readRoutes tìm mọi tuyến đăng ký vào một ServeMux.
//
// Đọc bằng AST chứ không bằng grep: chuỗi `"POST /api/v1/events"` còn xuất
// hiện trong MÔ TẢ của tham số vận hành, và một phép grep sẽ đếm nó là một
// tuyến. Chỉ đối số ĐẦU TIÊN của `Handle`/`HandleFunc` mới là tuyến.
func (c *checker) readRoutes() (map[operation]string, error) {
	out := map[operation]string{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(filepath.Join(c.root, "internal"),
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Test dựng mux riêng để thử; chúng không phải bề mặt công khai.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return fmt.Errorf("phân tích %s: %w", path, err)
			}

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc" {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				raw, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if m := reRoute.FindStringSubmatch(raw); m != nil {
					rel, _ := filepath.Rel(c.root, path)
					out[operation{Method: m[1], Path: m[2]}] = rel
				}
				return true
			})
			return nil
		})
	return out, err
}

// ---------------------------------------------------------------- Đối chiếu

func (c *checker) compare() {
	// Hướng 1: tuyến sống ngoài hợp đồng. LỖI, không có ngoại lệ.
	var extra []operation
	for t := range c.route {
		if _, co := c.spec[t]; co {
			continue
		}
		if _, exempt := c.outsideContract[t.String()]; exempt {
			continue
		}
		extra = append(extra, t)
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].String() < extra[j].String() })
	for _, t := range extra {
		c.violations = append(c.violations, fmt.Sprintf(
			"%s đăng ký ở %s nhưng KHÔNG có trong đặc tả.\n"+
				"    Endpoint ngoài hợp đồng là endpoint client không gọi được "+
				"theo cách có kiểu, và không ai rà soát khi đổi.",
			t, c.route[t]))
	}

	// Hướng 2: đặc tả khai mà chưa cài. Phải có mặt trong `notImplemented`.
	var missing []operation
	for t := range c.spec {
		if _, co := c.route[t]; !co {
			missing = append(missing, t)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].String() < missing[j].String() })
	for _, t := range missing {
		if _, declared := c.notImplemented[t.String()]; !declared {
			c.violations = append(c.violations, fmt.Sprintf(
				"%s có trong đặc tả (%s) nhưng KHÔNG có route.\n"+
					"    Chưa định cài thì khai vào `notImplemented` kèm lý do; "+
					"một thao tác im lặng không ai cài là một lời hứa với client "+
					"mà không ai giữ.",
				t, c.spec[t]))
		}
	}

	// Dòng trong `outsideContract` mà không còn tuyến nào thì phải XÓA.
	//
	// Một miễn trừ cho thứ không tồn tại là một miễn trừ sẽ che nhầm thứ
	// khác trùng tên về sau.
	for ten := range c.outsideContract {
		parts := strings.SplitN(ten, " ", 2)
		if len(parts) != 2 {
			c.violations = append(c.violations, fmt.Sprintf(
				"`outsideContract` có dòng sai định dạng: %q", ten))
			continue
		}
		if _, co := c.route[operation{Method: parts[0], Path: parts[1]}]; !co {
			c.violations = append(c.violations, fmt.Sprintf(
				"%s nằm trong `outsideContract` nhưng KHÔNG còn tuyến nào — xóa dòng đó.",
				ten))
		}
	}

	// Hướng 2b: dòng trong `notImplemented` đã được cài xong thì phải XÓA đi.
	for ten := range c.notImplemented {
		parts := strings.SplitN(ten, " ", 2)
		if len(parts) != 2 {
			c.violations = append(c.violations, fmt.Sprintf(
				"`notImplemented` có dòng sai định dạng: %q — cần \"METHOD /đường/dẫn\"", ten))
			continue
		}
		t := operation{Method: parts[0], Path: parts[1]}
		if _, coRoute := c.route[t]; coRoute {
			c.violations = append(c.violations, fmt.Sprintf(
				"%s ĐÃ có route nhưng vẫn nằm trong `notImplemented` — xóa dòng đó.\n"+
					"    Một danh sách hoãn không ai dọn sẽ thành danh sách không "+
					"ai đọc.", t))
			continue
		}
		if _, hasSpec := c.spec[t]; !hasSpec {
			c.violations = append(c.violations, fmt.Sprintf(
				"%s nằm trong `notImplemented` nhưng KHÔNG còn trong đặc tả — xóa dòng đó.", t))
		}
	}
}

func (c *checker) report() {
	if c.verbose {
		fmt.Printf("apicheck: %d thao tác đặc tả · %d tuyến · %d hoãn · "+
			"%d ngoài hợp đồng\n",
			len(c.spec), len(c.route), len(c.notImplemented), len(c.outsideContract))
		fmt.Printf("apicheck: %d cặp enum đã kiểm · %d enum khai không ghép · "+
			"%d enum CHƯA GÁC (chốt %d)\n",
			len(c.enumPair), len(c.unpairedEnums), c.ungatedEnums,
			c.ungatedEnumLedger)
		fmt.Printf("apicheck: %d header đặc tả · %d header CORS cho phép · "+
			"%d ngoài đặc tả · %d không qua trình duyệt\n",
			len(c.specHeaders), len(c.headerCORS),
			len(c.headersOutsideSpec), len(c.headersNotViaBrowser))

		var allowMissing int
		for _, pair := range c.dtoPair {
			allowMissing += len(pair.AllowMissing)
		}
		fmt.Printf("apicheck: %d cặp DTO đã kiểm · %d trường cho phép thiếu · "+
			"%d schema CHƯA GÁC (chốt %d)\n",
			len(c.dtoPair), allowMissing, c.ungatedDTOs, c.ungatedDTOLedger)
	}

	if len(c.violations) == 0 {
		fmt.Printf("apicheck: OK — %d thao tác đặc tả khớp %d tuyến "+
			"(%d hoãn, đã khai) · %d header khớp danh sách CORS · "+
			"%d cặp enum khớp · %d cặp DTO khớp\n",
			len(c.spec), len(c.route), len(c.notImplemented), len(c.headerCORS),
			len(c.enumPair), len(c.dtoPair))
		return
	}

	sort.Strings(c.violations)
	fmt.Fprintf(os.Stderr, "apicheck: %d vi phạm\n\n", len(c.violations))
	for _, v := range c.violations {
		fmt.Fprintf(os.Stderr, "  %s\n\n", v)
	}
}
