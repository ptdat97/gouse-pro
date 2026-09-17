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

// Tầng thứ ba của hợp đồng API: HEADER.
//
// # Vì sao tầng này cần phép kiểm riêng
//
// P3-57 đã ghi ba tầng lệch của hợp đồng API và nói tầng thứ ba — HÌNH
// DẠNG — chưa có phép kiểm nào. Header là phần rẻ nhất của tầng ấy, và là
// phần hỏng theo kiểu tệ nhất.
//
// Ngày 17/09/2026, `X-Visit-Id` được thêm vào `api-client` (ADR-0020) và
// KHÔNG được thêm vào danh sách CORS của máy chủ. Hậu quả không phải một
// tính năng hỏng mà là **cả cửa hàng ngừng tải được dữ liệu trên mọi trình
// duyệt thật**: preflight bị từ chối nên request thật không bao giờ rời
// máy khách.
//
// Thứ làm nó sống sót qua mọi phép kiểm hiện có:
//
//	go test          xanh — httptest gọi handler trực tiếp, không preflight
//	Playwright e2e   xanh — chạy cùng origin qua proxy của Next.js
//	apicheck         xanh — nó so ĐƯỜNG DẪN và METHOD, không so header
//	types:check      xanh — đặc tả không khai header này nên không có gì lệch
//	log máy chủ      SẠCH — request bị chặn trước khi tới máy chủ
//
// Năm dấu xanh cho một cửa hàng không dùng được. Phép kiểm dưới đây là cái
// thứ sáu, và nó nhìn đúng chỗ.
//
// # Hai hướng, và chúng cũng KHÔNG đối xứng
//
//	đặc tả khai, CORS không cho   LỖI. Client làm đúng đặc tả vẫn bị chặn.
//	CORS cho, đặc tả không khai   phải KHAI vào `headerNgoaiDacTa` kèm lý do.

// headerKhongQuaTrinhDuyet là những header đặc tả CÓ khai mà CORS KHÔNG
// được cho phép, kèm lý do.
//
// Đây không phải miễn trừ cho tiện. Cho một header server-to-server vào
// danh sách CORS là MỞ RỘNG bề mặt tấn công: nó nói với mọi trang web rằng
// header ấy gửi kèm được từ trình duyệt.
var headerKhongQuaTrinhDuyet = map[string]string{
	"X-Signature": "webhook từ nhà cung cấp thanh toán/vận chuyển — " +
		"server-to-server, KHÔNG bao giờ từ trình duyệt. Cho vào CORS là " +
		"mời trình duyệt thử ký giả.",
}

// headerNgoaiDacTa là những header được CORS cho phép mà đặc tả OpenAPI
// KHÔNG khai thành parameter, kèm lý do vì sao hợp lệ.
//
// Chúng hợp lệ vì đặc tả khai chúng theo cách khác — qua `securitySchemes`,
// qua `requestBody`, hoặc theo chuẩn HTTP — chứ không phải vì chúng được
// miễn kiểm.
var headerNgoaiDacTa = map[string]string{
	"Authorization": "khai qua securitySchemes.bearerAuth, không phải parameter",
	"Content-Type":  "chuẩn HTTP; đặc tả khai qua requestBody.content",
}

// docHeaderDacTa đọc mọi header khai `in: header` trong thư mục api/.
//
// # Vì sao quét THƯ MỤC chứ không đi theo $ref
//
// Header nằm rải ở ba chỗ với ba cách khai khác nhau: parameter dùng chung
// trong `components/common.yaml`, parameter của riêng một thao tác trong
// `paths/*.yaml`, và `securitySchemes` trong `openapi.yaml`. Đi theo $ref
// sẽ bỏ sót chỗ thứ ba, mà chỗ thứ ba chính là nơi `X-Signature` sống.
//
// # Thứ tự hai khóa KHÔNG cố định
//
// Đặc tả này viết cả `name` trước `in` lẫn `in` trước `name`. Bộ đọc chấp
// nhận cả hai, nhưng KHÔNG chấp nhận một khối `in: header` không tìm được
// tên — gặp thế thì báo lỗi, vì một header bị bỏ sót âm thầm là đúng loại
// lỗi công cụ này sinh ra để chặn.
func (c *checker) docHeaderDacTa() (map[string]string, error) {
	goc := filepath.Join(c.root, "api")
	out := map[string]string{}

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
		dong := strings.Split(string(noiDung), "\n")

		for i, l := range dong {
			// Cả hai cách viết: `in: header` và `- in: header`. YAML cho
			// phép đặt `in` trước `name` trong một phần tử danh sách, và
			// một bộ đọc chỉ nhận dạng thứ nhất sẽ ÂM THẦM bỏ qua dạng
			// thứ hai — tức đúng kiểu dễ dãi công cụ này sinh ra để chặn.
			t := strings.TrimSpace(l)
			if t != "in: header" && t != "- in: header" {
				continue
			}
			ten := timTenHeader(dong, i)
			if ten == "" {
				return fmt.Errorf("%s dòng %d: khai `in: header` mà không "+
					"tìm được `name:` đi kèm — bộ đọc không đoán, xem "+
					"cmd/apicheck/header.go", duong, i+1)
			}
			rel, _ := filepath.Rel(c.root, duong)
			out[ten] = rel
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// KHÔNG báo lỗi khi rỗng: một đặc tả không khai header nào là hợp lệ.
	// Việc canh "-root trỏ sai chỗ" đã do `run` làm, dựa trên số thao tác
	// và số tuyến — hai con số không bao giờ được phép bằng 0.
	return out, nil
}

// reTenHeader khớp dòng khai tên, có hoặc không có gạch đầu dòng.
var reTenHeader = regexp.MustCompile(`^\s*-?\s*name:\s*([A-Za-z][A-Za-z0-9-]*)\s*$`)

// timTenHeader tìm `name:` quanh dòng `in: header`.
//
// Cửa sổ ±3 dòng: đủ cho `description` hoặc `required` xen giữa, đủ hẹp để
// không vớ phải tên của parameter kế bên.
func timTenHeader(dong []string, i int) string {
	for d := 1; d <= 3; d++ {
		for _, j := range []int{i - d, i + d} {
			if j < 0 || j >= len(dong) {
				continue
			}
			// Dừng ở ranh giới khối: một dòng `- name:` mới bắt đầu
			// parameter khác, nên chỉ nhận nó nếu nó là dòng gần nhất.
			if m := reTenHeader.FindStringSubmatch(dong[j]); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

// docHeaderCORS đọc danh sách `HeaderChoPhep` bằng AST.
//
// Đọc AST chứ không grep chuỗi vì cùng lý do như phần đọc tuyến: một tên
// header xuất hiện trong bình luận hay trong thông điệp lỗi không phải là
// một header được cho phép.
func (c *checker) docHeaderCORS() (map[string]bool, error) {
	duong := filepath.Join(c.root, "internal", "platform", "httpserver", "cors.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, duong, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", duong, err)
	}

	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "HeaderChoPhep" {
			return true
		}
		if len(vs.Values) != 1 {
			return true
		}
		lit, ok := vs.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, e := range lit.Elts {
			bl, ok := e.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				continue
			}
			s, err := strconv.Unquote(bl.Value)
			if err == nil {
				out[s] = true
			}
		}
		return false
	})

	if len(out) == 0 {
		return nil, fmt.Errorf("%s: không tìm thấy `var HeaderChoPhep = []string{...}` "+
			"— công cụ này đọc đúng biến đó, đổi tên thì phải sửa cả đây", duong)
	}
	return out, nil
}

// doiChieuHeader so hai danh sách và ghi vi phạm.
func (c *checker) doiChieuHeader(dacTa map[string]string, cors map[string]bool) {
	// So KHÔNG phân biệt hoa thường: chuẩn HTTP nói tên header không phân
	// biệt hoa thường, và trình duyệt gửi `x-visit-id` viết thường. Một
	// phép so phân biệt hoa thường ở đây sẽ báo lỗi giả cho `X-Request-ID`
	// và `X-Request-Id` — hai cách viết của cùng một header.
	corsThuong := map[string]string{}
	for h := range cors {
		corsThuong[strings.ToLower(h)] = h
	}

	var thieu []string
	for h := range dacTa {
		if _, ok := corsThuong[strings.ToLower(h)]; ok {
			continue
		}
		if _, co := c.headerKhongQuaTrinhDuyet[h]; co {
			continue
		}
		thieu = append(thieu, h)
	}
	sort.Strings(thieu)
	for _, h := range thieu {
		c.viPham = append(c.viPham, fmt.Sprintf(
			"HEADER %s khai trong đặc tả (%s) mà CORS KHÔNG cho phép — "+
				"trình duyệt sẽ chặn ở preflight và log máy chủ sẽ SẠCH. "+
				"Thêm vào `HeaderChoPhep` trong internal/platform/httpserver/cors.go",
			h, dacTa[h]))
	}

	dacTaThuong := map[string]bool{}
	for h := range dacTa {
		dacTaThuong[strings.ToLower(h)] = true
	}

	var thua []string
	for h := range cors {
		if dacTaThuong[strings.ToLower(h)] {
			continue
		}
		if _, co := c.headerNgoaiDacTa[h]; co {
			continue
		}
		thua = append(thua, h)
	}
	sort.Strings(thua)
	for _, h := range thua {
		c.viPham = append(c.viPham, fmt.Sprintf(
			"HEADER %s được CORS cho phép mà đặc tả KHÔNG khai ở đâu — "+
				"hoặc khai nó vào api/ (`in: header`), hoặc khai lý do vào "+
				"`headerNgoaiDacTa` trong cmd/apicheck/header.go", h))
	}

	// Miễn trừ cho thứ không còn tồn tại cũng là một dòng nói dối, và một
	// sổ đầy dòng chết là sổ không ai đọc.
	var mienTruThua []string
	for h := range c.headerNgoaiDacTa {
		if _, ok := corsThuong[strings.ToLower(h)]; !ok {
			mienTruThua = append(mienTruThua, fmt.Sprintf(
				"HEADER %s có dòng miễn trừ trong `headerNgoaiDacTa` mà "+
					"CORS không còn cho phép — xóa dòng ấy đi", h))
		}
	}
	for h := range c.headerKhongQuaTrinhDuyet {
		if !dacTaThuong[strings.ToLower(h)] {
			mienTruThua = append(mienTruThua, fmt.Sprintf(
				"HEADER %s có dòng miễn trừ trong `headerKhongQuaTrinhDuyet` "+
					"mà đặc tả không còn khai — xóa dòng ấy đi", h))
		}
	}
	sort.Strings(mienTruThua)
	c.viPham = append(c.viPham, mienTruThua...)
}
