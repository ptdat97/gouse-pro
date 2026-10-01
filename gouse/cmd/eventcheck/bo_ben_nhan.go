package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

// R4 — BỘ BÊN NHẬN: test tích hợp phải đăng ký ĐÚNG bộ bên nhận của worker.
//
// # Vì sao quy tắc này tồn tại
//
// `internal/app/api_ledger_test.go` có `dangKyBenNhan`, và chú thích của nó
// nói rõ mục đích: *"bộ bên nhận chuẩn, dùng chung cho mọi bài test cần phát
// event, để KHÔNG CÓ BÀI NÀO chạy với bộ bên nhận khác production"*.
//
// Tới 01/10/2026 nó thiếu `inventory.NewReleaseHandler` — ĐƯỜNG RA của kho —
// và không chú thích nào nói vì sao. Nên mọi bài test ở lớp ấy chạy với một
// hệ thống không có đường ra của kho, và lỗi *"đặt 5 món rồi hủy, tồn kho
// đứng nguyên 15 khả dụng / 5 cam kết. Không lỗi, không log."* không bài nào
// bắt được.
//
// # Vì sao nó không tự lộ ra
//
// Thiếu một bên nhận KHÔNG làm bài test nào đỏ — nó làm bài test không hỏi
// tới phần ấy. Đúng dạng lỗi mà R1/R2/R3 ở trên gác cho event, chỉ khác là ở
// tầng NỐI DÂY: hợp đồng ở đây không phải "ai phát, ai nghe" mà "cái gì được
// bật lên trong lúc kiểm chứng".
//
// Nó được tìm ra bằng một phép phá KHÔNG ĐỎ: bỏ `MaxEventVersion` của chính
// bên nhận ấy mà bài test vẫn xanh, vì nó chưa bao giờ chạy ở đó. Một phép
// phá không đỏ nói về PHÉP THỬ, không chỉ về mã.
//
// # Nguồn sự thật là mã, không phải danh sách
//
// Công cụ này đọc `bus.Subscribe(...)` ở CẢ HAI chỗ bằng `go/ast`. Giữ một
// danh sách riêng sẽ là bản sao thứ ba, và nó sẽ lệch.

// benNhanChiOWorker là các bên nhận CỐ Ý chỉ có ở worker, kèm lý do.
//
// Cùng khuôn với `khongAiPhat`/`khongAiNghe`: miễn trừ phải là quyết định có
// người ký, không phải một chỗ bị bỏ quên.
var benNhanChiOWorker = map[string]string{
	"supplychain.NewSignalHandler": "module `supplychain` không nằm trong " +
		"`Modules` của internal/app, nên tín hiệu nhu cầu không dựng được " +
		"ở lớp test. Khác biệt CÓ THẬT so với worker — xem P3-86",

	"payment.NewChiPhiVanChuyenHandler": "cần một nguồn GIÁ vận chuyển, nên " +
		"`api_chi_phi_van_chuyen_test.go` tự đăng ký kèm giá cố định qua " +
		"`phatEventKemChiPhi`. Đăng ký vào bộ chung sẽ phải chọn một giá " +
		"cho MỌI bài, và giá ấy là thứ bài kia đang kiểm",
}

// fileWorker và fileBoTest là hai chỗ đăng ký bên nhận.
//
// Đổi chỗ chúng thì công cụ này ĐỎ với lỗi "không đọc được file", chứ không
// im lặng bỏ gác — cùng cách `docSoHuuBang` của archcheck xử lý.
const (
	fileWorker = "cmd/worker/main.go"
	fileBoTest = "internal/app/api_ledger_test.go"

	// hamBoTest là hàm dựng bộ bên nhận chuẩn của test.
	//
	// CHỈ hàm này, không phải cả file: `phatEvent` và các bài test khác cũng
	// gọi `bus.Subscribe` cho những bên nhận riêng của chúng, và gộp vào sẽ
	// làm sổ miễn trừ vô nghĩa.
	hamBoTest = "dangKyBenNhan"
)

// doiChieuBoBenNhan thực thi R4.
func (c *checker) doiChieuBoBenNhan() error {
	cuaWorker, err := c.benNhanTrongFile(fileWorker, "")
	if err != nil {
		return err
	}
	cuaTest, err := c.benNhanTrongFile(fileBoTest, hamBoTest)
	if err != nil {
		return err
	}

	if len(cuaWorker) == 0 {
		return fmt.Errorf("không tìm thấy lời gọi bus.Subscribe nào trong %s "+
			"— đổi chỗ đăng ký bên nhận thì phải sửa cả công cụ này", fileWorker)
	}
	if len(cuaTest) == 0 {
		return fmt.Errorf("không tìm thấy lời gọi bus.Subscribe nào trong "+
			"%s hàm %s() — đổi tên hàm thì phải sửa cả công cụ này",
			fileBoTest, hamBoTest)
	}

	// Lý do khai cho một bên nhận KHÔNG tồn tại là một sổ đã mục.
	for ten := range c.chiOWorker {
		if !cuaWorker[ten] {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"[R4] sổ `benNhanChiOWorker` khai %q mà %s KHÔNG đăng ký nó\n"+
					"  → bên nhận đã đổi tên hoặc bị xóa; bỏ dòng ấy khỏi sổ",
				ten, fileWorker))
		}
	}

	var thieu []string
	for ten := range cuaWorker {
		if cuaTest[ten] {
			continue
		}
		if _, coLyDo := c.chiOWorker[ten]; coLyDo {
			continue
		}
		thieu = append(thieu, ten)
	}
	sort.Strings(thieu)

	for _, ten := range thieu {
		c.viPham = append(c.viPham, fmt.Sprintf(
			"[R4] %s đăng ký %q, %s() thì KHÔNG\n"+
				"  → mọi bài test lớp ấy chạy với một hệ thống KHÁC "+
				"production, và thiếu một bên nhận không làm bài nào đỏ\n"+
				"  → thêm vào %s(), hoặc khai lý do trong `benNhanChiOWorker`",
			fileWorker, ten, hamBoTest, hamBoTest))
	}

	// Chiều NGƯỢC lại cũng là lỗi, và nó tệ hơn: một bên nhận chỉ chạy
	// trong test là mã được kiểm chứng mà KHÔNG chạy ở production.
	var chiOTest []string
	for ten := range cuaTest {
		if !cuaWorker[ten] {
			chiOTest = append(chiOTest, ten)
		}
	}
	sort.Strings(chiOTest)

	for _, ten := range chiOTest {
		c.viPham = append(c.viPham, fmt.Sprintf(
			"[R4] %s() đăng ký %q, %s thì KHÔNG\n"+
				"  → bài test xanh cho một đường KHÔNG chạy ở production — "+
				"tệ hơn chiều ngược lại, vì nó tạo ra lòng tin sai\n"+
				"  → thêm vào %s",
			hamBoTest, ten, fileWorker, fileWorker))
	}

	c.soBenNhanWorker = len(cuaWorker)
	return nil
}

// benNhanTrongFile đọc tên các bên nhận được `bus.Subscribe(...)`.
//
// `trongHam` rỗng = cả file. Khác rỗng = chỉ thân hàm ấy.
//
// Trả về tên dạng `goi.NewXxxHandler` — tên HÀM TẠO, không phải tên kiểu:
// đó là thứ cả hai chỗ viết ra, nên so được trực tiếp mà không phải phân
// giải kiểu.
func (c *checker) benNhanTrongFile(
	duongDan, trongHam string,
) (map[string]bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, c.root+"/"+duongDan, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", duongDan, err)
	}

	out := map[string]bool{}
	gom := func(n ast.Node) bool {
		goi, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Khớp `<gì đó>.Subscribe(...)` với ĐÚNG một tham số.
		sel, ok := goi.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Subscribe" || len(goi.Args) != 1 {
			return true
		}
		if ten := tenHamTao(goi.Args[0]); ten != "" {
			out[ten] = true
		}
		return true
	}

	if trongHam == "" {
		ast.Inspect(f, gom)
		return out, nil
	}

	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != trongHam || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, gom)
		return out, nil
	}
	return out, nil
}

// tenHamTao lấy `goi.NewXxx` từ biểu thức `goi.NewXxx(...)`.
//
// Trả chuỗi rỗng cho thứ không phải lời gọi hàm tạo của một gói — ví dụ một
// biến đã dựng sẵn. Bỏ qua thay vì đoán: đoán sai ở đây tạo ra vi phạm giả,
// và một công cụ kêu oan sẽ bị tắt đi.
func tenHamTao(e ast.Expr) string {
	goi, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := goi.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	goiTen, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	if !strings.HasPrefix(sel.Sel.Name, "New") {
		return ""
	}
	return goiTen.Name + "." + sel.Sel.Name
}
