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
//	đặc tả có, tuyến không   phải KHAI vào `chuaCai` kèm lý do. Phần lớn
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
	)
	flag.Parse()

	c := &checker{
		root: *root, verbose: *verbose,
		chuaCai: chuaCai, ngoaiHopDong: ngoaiHopDong,
	}
	if err := c.run(); err != nil {
		fmt.Fprintf(os.Stderr, "apicheck: %v\n", err)
		os.Exit(2)
	}

	c.report()
	if len(c.viPham) > 0 {
		os.Exit(1)
	}
}

// thaoTac là một cặp (method, đường dẫn).
type thaoTac struct {
	Method string
	Path   string
}

func (t thaoTac) String() string { return t.Method + " " + t.Path }

type checker struct {
	root    string
	verbose bool

	// Hai sổ khai báo, truyền vào chứ không đọc biến gói.
	//
	// Nhờ vậy bài test dựng được một dự án giả với sổ RỖNG: nếu công cụ
	// đọc thẳng sổ thật thì mọi bài test đều kèm theo mười chín vi phạm
	// của dự án thật, và không bài nào đo được thứ nó muốn đo.
	chuaCai      map[string]string
	ngoaiHopDong map[string]string

	dacTa map[thaoTac]string // thao tác -> file đặc tả khai nó
	tuyen map[thaoTac]string // tuyến   -> file Go đăng ký nó

	viPham []string
}

func (c *checker) run() error {
	var err error
	if c.dacTa, err = c.docDacTa(); err != nil {
		return err
	}
	if c.tuyen, err = c.docTuyen(); err != nil {
		return err
	}
	if len(c.dacTa) == 0 || len(c.tuyen) == 0 {
		return fmt.Errorf("đọc được %d thao tác và %d tuyến — quá ít để "+
			"kết luận gì; kiểm lại -root", len(c.dacTa), len(c.tuyen))
	}

	c.doiChieu()
	return nil
}

// ------------------------------------------------------------------ Đặc tả

var (
	// Dòng khai một đường dẫn trong openapi.yaml: hai dấu cách rồi "/...:".
	reDuongDan = regexp.MustCompile(`^  (/\S*):\s*$`)

	// Dòng $ref trỏ tới khối trong file con.
	reRef = regexp.MustCompile(`^\s+\$ref:\s*'\./paths/([^#']+)#/([^']+)'\s*$`)

	// Khóa method bên trong một khối thao tác.
	reMethod = regexp.MustCompile(`^  (get|post|put|patch|delete):\s*$`)
)

// docDacTa đọc mọi thao tác khai trong api/openapi.yaml.
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
func (c *checker) docDacTa() (map[thaoTac]string, error) {
	goc := filepath.Join(c.root, "api", "openapi.yaml")
	noiDung, err := os.ReadFile(goc)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", goc, err)
	}

	out := map[thaoTac]string{}
	dong := strings.Split(string(noiDung), "\n")

	for i := 0; i < len(dong); i++ {
		m := reDuongDan.FindStringSubmatch(dong[i])
		if m == nil {
			continue
		}
		duongDan := m[1]

		// Dòng ngay sau PHẢI là $ref. Đặc tả này không dùng cách khai nào
		// khác, và nếu có thì công cụ phải biết trước khi tin kết quả.
		if i+1 >= len(dong) {
			return nil, fmt.Errorf("%s: đường dẫn %q ở cuối file, không có $ref",
				goc, duongDan)
		}
		r := reRef.FindStringSubmatch(dong[i+1])
		if r == nil {
			return nil, fmt.Errorf(
				"%s dòng %d: đường dẫn %q không theo sau bởi $ref tới paths/ — "+
					"apicheck chỉ hiểu cách khai đó, và đoán bừa ở đây nghĩa là "+
					"bỏ sót thao tác mà vẫn báo xanh",
				goc, i+2, duongDan)
		}

		tep, neo := r[1], r[2]
		methods, err := c.docKhoi(filepath.Join(c.root, "api", "paths", tep), neo)
		if err != nil {
			return nil, err
		}
		for _, meth := range methods {
			out[thaoTac{Method: meth, Path: duongDan}] = tep + "#/" + neo
		}
		i++
	}
	return out, nil
}

// docKhoi trả danh sách method của một khối thao tác trong file paths/.
func (c *checker) docKhoi(tep, neo string) ([]string, error) {
	noiDung, err := os.ReadFile(tep)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", tep, err)
	}

	dong := strings.Split(string(noiDung), "\n")
	batDau := -1
	for i, d := range dong {
		if d == neo+":" {
			batDau = i
			break
		}
	}
	if batDau < 0 {
		return nil, fmt.Errorf("%s: không tìm thấy khối %q mà openapi.yaml trỏ tới",
			tep, neo)
	}

	var out []string
	for i := batDau + 1; i < len(dong); i++ {
		d := dong[i]
		// Khối kết thúc khi gặp khóa ở cột 0.
		if d != "" && !strings.HasPrefix(d, " ") {
			break
		}
		if m := reMethod.FindStringSubmatch(d); m != nil {
			out = append(out, strings.ToUpper(m[1]))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: khối %q không khai method nào", tep, neo)
	}
	return out, nil
}

// ------------------------------------------------------------------- Tuyến

var reTuyen = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) (/\S*)$`)

// docTuyen tìm mọi tuyến đăng ký vào một ServeMux.
//
// Đọc bằng AST chứ không bằng grep: chuỗi `"POST /api/v1/events"` còn xuất
// hiện trong MÔ TẢ của tham số vận hành, và một phép grep sẽ đếm nó là một
// tuyến. Chỉ đối số ĐẦU TIÊN của `Handle`/`HandleFunc` mới là tuyến.
func (c *checker) docTuyen() (map[thaoTac]string, error) {
	out := map[thaoTac]string{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(filepath.Join(c.root, "internal"),
		func(duong string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(duong, ".go") {
				return nil
			}
			// Test dựng mux riêng để thử; chúng không phải bề mặt công khai.
			if strings.HasSuffix(duong, "_test.go") {
				return nil
			}

			tep, err := parser.ParseFile(fset, duong, nil, 0)
			if err != nil {
				return fmt.Errorf("phân tích %s: %w", duong, err)
			}

			ast.Inspect(tep, func(n ast.Node) bool {
				goi, ok := n.(*ast.CallExpr)
				if !ok || len(goi.Args) == 0 {
					return true
				}
				sel, ok := goi.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc" {
					return true
				}
				lit, ok := goi.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				gia, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if m := reTuyen.FindStringSubmatch(gia); m != nil {
					rel, _ := filepath.Rel(c.root, duong)
					out[thaoTac{Method: m[1], Path: m[2]}] = rel
				}
				return true
			})
			return nil
		})
	return out, err
}

// ---------------------------------------------------------------- Đối chiếu

func (c *checker) doiChieu() {
	// Hướng 1: tuyến sống ngoài hợp đồng. LỖI, không có ngoại lệ.
	var thua []thaoTac
	for t := range c.tuyen {
		if _, co := c.dacTa[t]; co {
			continue
		}
		if _, mien := c.ngoaiHopDong[t.String()]; mien {
			continue
		}
		thua = append(thua, t)
	}
	sort.Slice(thua, func(i, j int) bool { return thua[i].String() < thua[j].String() })
	for _, t := range thua {
		c.viPham = append(c.viPham, fmt.Sprintf(
			"%s đăng ký ở %s nhưng KHÔNG có trong đặc tả.\n"+
				"    Endpoint ngoài hợp đồng là endpoint client không gọi được "+
				"theo cách có kiểu, và không ai rà soát khi đổi.",
			t, c.tuyen[t]))
	}

	// Hướng 2: đặc tả khai mà chưa cài. Phải có mặt trong `chuaCai`.
	var thieu []thaoTac
	for t := range c.dacTa {
		if _, co := c.tuyen[t]; !co {
			thieu = append(thieu, t)
		}
	}
	sort.Slice(thieu, func(i, j int) bool { return thieu[i].String() < thieu[j].String() })
	for _, t := range thieu {
		if _, khai := c.chuaCai[t.String()]; !khai {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"%s có trong đặc tả (%s) nhưng KHÔNG có route.\n"+
					"    Chưa định cài thì khai vào `chuaCai` kèm lý do; "+
					"một thao tác im lặng không ai cài là một lời hứa với client "+
					"mà không ai giữ.",
				t, c.dacTa[t]))
		}
	}

	// Dòng trong `ngoaiHopDong` mà không còn tuyến nào thì phải XÓA.
	//
	// Một miễn trừ cho thứ không tồn tại là một miễn trừ sẽ che nhầm thứ
	// khác trùng tên về sau.
	for ten := range c.ngoaiHopDong {
		phan := strings.SplitN(ten, " ", 2)
		if len(phan) != 2 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"`ngoaiHopDong` có dòng sai định dạng: %q", ten))
			continue
		}
		if _, co := c.tuyen[thaoTac{Method: phan[0], Path: phan[1]}]; !co {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"%s nằm trong `ngoaiHopDong` nhưng KHÔNG còn tuyến nào — xóa dòng đó.",
				ten))
		}
	}

	// Hướng 2b: dòng trong `chuaCai` đã được cài xong thì phải XÓA đi.
	for ten := range c.chuaCai {
		phan := strings.SplitN(ten, " ", 2)
		if len(phan) != 2 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"`chuaCai` có dòng sai định dạng: %q — cần \"METHOD /đường/dẫn\"", ten))
			continue
		}
		t := thaoTac{Method: phan[0], Path: phan[1]}
		if _, coRoute := c.tuyen[t]; coRoute {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"%s ĐÃ có route nhưng vẫn nằm trong `chuaCai` — xóa dòng đó.\n"+
					"    Một danh sách hoãn không ai dọn sẽ thành danh sách không "+
					"ai đọc.", t))
			continue
		}
		if _, coDacTa := c.dacTa[t]; !coDacTa {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"%s nằm trong `chuaCai` nhưng KHÔNG còn trong đặc tả — xóa dòng đó.", t))
		}
	}
}

func (c *checker) report() {
	if c.verbose {
		fmt.Printf("apicheck: %d thao tác đặc tả · %d tuyến · %d hoãn · "+
			"%d ngoài hợp đồng\n",
			len(c.dacTa), len(c.tuyen), len(c.chuaCai), len(c.ngoaiHopDong))
	}

	if len(c.viPham) == 0 {
		fmt.Printf("apicheck: OK — %d thao tác đặc tả khớp %d tuyến "+
			"(%d hoãn, đã khai)\n", len(c.dacTa), len(c.tuyen), len(c.chuaCai))
		return
	}

	sort.Strings(c.viPham)
	fmt.Fprintf(os.Stderr, "apicheck: %d vi phạm\n\n", len(c.viPham))
	for _, v := range c.viPham {
		fmt.Fprintf(os.Stderr, "  %s\n\n", v)
	}
}
