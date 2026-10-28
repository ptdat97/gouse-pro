package http

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// MỌI lỗi miền phải có đường ra KHÁC 500.
//
// # Chuyện đã xảy ra
//
// Ngày 17/09/2026, tạo sản phẩm mà thiếu `category_id` trả về:
//
//	500 INTERNAL_ERROR — "Đã có lỗi xảy ra, vui lòng thử lại"
//
// Lời khuyên ấy không bao giờ đúng: thử lại y hệt sẽ hỏng y hệt. Nguyên
// nhân là `translateWriteError` thiếu một nhánh, nên lỗi rơi xuống nhánh mặc định.
//
// Rà cả bảng thì không phải một mà CHÍN trong hai mươi lỗi miền thiếu
// nhánh — chín cách một nhà bán nhập sai và được báo rằng máy chủ hỏng.
// Trong đó có `ErrSlugTaken` và `ErrSKUCodeTaken`, hai lỗi hoàn toàn bình
// thường mà người dùng gặp hằng ngày.
//
// # Vì sao đọc AST thay vì viết một danh sách
//
// Một danh sách tên lỗi chép tay ở đây sẽ lệch đúng theo cách `translateWriteError`
// đã lệch — đó là chính xác dạng lỗi bài test này sinh ra để chặn. Nên nó
// ĐỌC thư mục domain và tự tìm mọi `Err…`.
//
// Cùng kỹ thuật `cmd/archcheck` và `cmd/apicheck` dùng, ở quy mô một module.
func TestEveryDomainErrorHasANon500Path(t *testing.T) {
	wantErr := readDomainErrors(t, "../../domain")
	if len(wantErr) < 10 {
		t.Fatalf("chỉ đọc được %d lỗi miền — quá ít để kết luận, "+
			"kiểm lại đường dẫn", len(wantErr))
	}

	fnSrc := readTranslateWriteErrorBody(t, "seller.go")

	var missingErrs []string
	for _, name := range wantErr {
		if _, skip := errorsWithoutBranch[name]; skip {
			continue
		}
		if !strings.Contains(fnSrc, "domain."+name) {
			missingErrs = append(missingErrs, name)
		}
	}
	sort.Strings(missingErrs)

	for _, name := range missingErrs {
		t.Errorf("domain.%s không có nhánh trong `translateWriteError` — nó sẽ ra "+
			"500 \"vui lòng thử lại\", một lời khuyên không bao giờ đúng. "+
			"Thêm nhánh, hoặc khai lý do vào `errorsWithoutBranch`.", name)
	}
}

// errorsWithoutBranch là những lỗi miền CỐ Ý không có nhánh, kèm lý do.
//
// Sổ này phải rỗng hoặc gần rỗng. Mỗi dòng ở đây là một lời hứa rằng lỗi
// ấy không bao giờ đi qua tầng HTTP của nhà bán.
var errorsWithoutBranch = map[string]string{
	// Hai lỗi LẬP TRÌNH, không phải lỗi người dùng: chỉ xảy ra khi mã gọi
	// truyền con trỏ nil. Không đầu vào HTTP nào sinh ra được chúng — tầng
	// HTTP luôn dựng biến thể và SKU từ thân request trước khi gọi miền.
	// 500 là ĐÚNG ở đây: nếu một ngày chúng lọt ra thì đó là lỗi của ta.
	"ErrNilVariant": "lỗi lập trình — biến thể nil, không đến từ đầu vào",
	"ErrNilSKU":     "lỗi lập trình — SKU nil, không đến từ đầu vào",
}

// Sổ miễn trừ CHẾT cũng phải bị bắt: một dòng cho một lỗi không còn tồn
// tại làm người đọc tin rằng có người đã cân nhắc nó.
func TestNoDeadExemptionRowsLeft(t *testing.T) {
	present := map[string]bool{}
	for _, name := range readDomainErrors(t, "../../domain") {
		present[name] = true
	}
	for name := range errorsWithoutBranch {
		if !present[name] {
			t.Errorf("`errorsWithoutBranch` còn dòng cho %q mà miền "+
				"không còn lỗi ấy — xóa đi", name)
		}
	}
}

// readDomainErrors tìm mọi biến `Err…` khai ở tầng domain.
//
// Bỏ file test: lỗi dựng riêng cho một bài test không đi qua HTTP bao giờ.
func readDomainErrors(t *testing.T, dir string) []string {
	t.Helper()

	input, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("đọc %s: %v", dir, err)
	}

	var names []string
	fset := token.NewFileSet()
	for _, e := range input {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("phân tích %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for _, nm := range vs.Names {
				if strings.HasPrefix(nm.Name, "Err") && nm.IsExported() {
					names = append(names, nm.Name)
				}
			}
			return true
		})
	}
	sort.Strings(names)
	return names
}

// readTranslateWriteErrorBody trả về mã nguồn của riêng hàm `translateWriteError`.
//
// Cắt đúng thân hàm chứ không đọc cả file: tên một lỗi xuất hiện ở chỗ
// khác — một bình luận, một hàm khác — KHÔNG có nghĩa là nó được dịch.
func readTranslateWriteErrorBody(t *testing.T, fileName string) string {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fileName, nil, 0)
	if err != nil {
		t.Fatalf("phân tích %s: %v", fileName, err)
	}
	src, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("đọc %s: %v", fileName, err)
	}

	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "translateWriteError" || fn.Body == nil {
			continue
		}
		return string(src[fn.Body.Pos()-1 : fn.Body.End()])
	}
	t.Fatalf("%s không có hàm `translateWriteError` — đổi tên thì phải sửa cả bài "+
		"test này", fileName)
	return ""
}

// Domain KHÔNG được tạo lỗi TẠI CHỖ trong thân hàm.
//
// # Điểm mù của bài test ở trên
//
// `TestEveryDomainErrorHasANon500Path` quét các biến `Err…` CÓ TÊN. Một lỗi
// viết thẳng `return errors.New("...")` trong thân hàm không có tên, nên
// nó không thấy — và tầng HTTP cũng không `errors.Is` được, nên lỗi ấy rơi
// xuống 500.
//
// Ngày 19/09/2026 điểm mù ấy chứa TÁM chỗ, trong đó ít nhất năm đi tới
// được HTTP. Gửi `product_type: "XYZ"` trả 500 "vui lòng thử lại" trong
// lúc bài test ở trên báo xanh. Một hàng rào có điểm mù đúng hình dạng lỗi
// nó canh là một hàng rào tạo cảm giác an toàn giả.
//
// # Luật
//
// `errors.New` chỉ được xuất hiện ở khai báo cấp gói (`var ErrX = …`).
// Trong thân hàm thì dùng lỗi có tên, hoặc `fmt.Errorf("%w: …", ErrX, …)`
// để kèm chi tiết mà `errors.Is` vẫn nhận ra.
//
// `fmt.Errorf` KHÔNG có `%w` cũng bị cấm, cùng lý do: nó tạo lỗi mới không
// ai nhận ra được.
func TestDomainCreatesNoAdHocErrors(t *testing.T) {
	dir := "../../domain"
	input, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("đọc %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	var violations []string
	for _, e := range input {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("phân tích %s: %v", name, err)
		}

		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkgIdent, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				vt := fset.Position(call.Pos())
				switch {
				case pkgIdent.Name == "errors" && sel.Sel.Name == "New":
					violations = append(violations, fmt.Sprintf(
						"%s:%d trong %s: `errors.New` tại chỗ", name, vt.Line, fn.Name.Name))
				case pkgIdent.Name == "fmt" && sel.Sel.Name == "Errorf" && !wrapsError(call):
					violations = append(violations, fmt.Sprintf(
						"%s:%d trong %s: `fmt.Errorf` không có %%w", name, vt.Line, fn.Name.Name))
				}
				return true
			})
		}
	}

	for _, v := range violations {
		t.Errorf("%s — tầng HTTP không nhận ra được lỗi này, nên nó sẽ ra "+
			"500. Khai một `var ErrX = errors.New(...)` cấp gói rồi trả "+
			"ErrX, hoặc bọc bằng `fmt.Errorf(\"%%w: ...\", ErrX, ...)`.", v)
	}
}

// wrapsError cho biết chuỗi định dạng của `fmt.Errorf` có `%w` không.
func wrapsError(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		// Chuỗi định dạng không phải hằng: không kết luận được, nên KHÔNG
		// báo — cảnh báo giả làm người ta tắt phép kiểm.
		return true
	}
	return strings.Contains(lit.Value, "%w")
}
