package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Tầng thứ TƯ của hợp đồng API: HÌNH DẠNG phản hồi.
//
// # Dạng lỗi này đã xảy ra NĂM lần
//
//	P3-50  shipping_groups   đặc tả có từ đầu, DTO không có trường → API
//	                         chỉ trả ba con số tổng
//	P3-64  variants          cùng vậy: nạp xong rồi vứt ở tầng DTO
//	P3-65  trường sửa được   của sản phẩm
//	P3-69  lines             đợt đối soát không nói được nó GỒM GÌ
//	P3-73  shipping_method   thiếu ở CẢ HAI phía, nên nút Đặt hàng khóa
//	                         vĩnh viễn khi đơn được miễn phí ship
//
// Cả năm đều đi qua `types:check` mà CI vẫn xanh: phép kiểm ấy so đặc tả
// với TypeScript sinh ra từ chính nó, và không biết gì về Go.
//
// Cả năm đều tìm ra BẰNG TAY, muộn. Bốn cái đầu chỉ lộ ra khi có người mở
// giao diện và thấy thiếu dữ liệu.
//
// # Vì sao ghép cặp TAY
//
// Không có gì trong mã nối một schema của đặc tả với một struct Go. Ghép
// theo tên (`Checkout` ↔ `checkoutJSON`) đúng ở phần lớn trường hợp nhưng
// không phải mọi trường hợp, và một cảnh báo giả là thứ làm người ta tắt
// hẳn phép kiểm — cùng lý do với tầng enum (P3-70).
//
// Nên mỗi cặp phải được KIỂM bằng mắt trước khi ghi vào `capDTODaKiem`.
//
// # Hai chiều, hai hậu quả khác nhau
//
//	đặc tả có · Go KHÔNG có   client dựng giao diện quanh một trường không
//	                          bao giờ tới. Đây là cả năm lần trên.
//	Go có · đặc tả KHÔNG khai  một trường rời máy chủ mà hợp đồng không
//	                          nhắc: không ai biết nó tồn tại để mà dùng,
//	                          và xóa nó đi là thay đổi PHÁ VỠ không ai
//	                          nhận ra

// capDTO là một cặp đã KIỂM: schema của đặc tả ⇄ struct Go phục vụ nó.
type capDTO struct {
	// Schema là tên schema cấp cao nhất trong `components/schemas.yaml`.
	Schema string

	// GoiGo là đường dẫn dưới `internal/`, KieuGo là tên struct.
	GoiGo  string
	KieuGo string

	// ChoPhepThieu là các trường đặc tả khai mà Go CỐ Ý chưa trả, kèm lý
	// do đọc được.
	//
	// Khác với một cặp không ghép: ở đây phần còn lại vẫn được gác.
	ChoPhepThieu map[string]string

	// LyDo nói cặp này về cái gì, cho người đọc thông báo lỗi.
	LyDo string
}

// docThuocTinhSchema đọc thuộc tính của các schema cấp cao nhất.
//
// Đọc theo THỤT LỀ, cùng cách các tầng khác của công cụ này làm: dự án có
// đúng bốn phụ thuộc Go trực tiếp, và thêm một thư viện YAML chỉ để đọc
// đặc tả là đổi một ràng buộc kiến trúc lấy một tiện lợi.
//
// Chỉ nhận thuộc tính ở ĐÚNG hai cấp thụt lề dưới `properties:` của
// schema — thuộc tính lồng trong một object con là hình dạng của object
// ấy, không phải của schema này.
func (c *checker) docThuocTinhSchema() (map[string][]string, error) {
	duong := filepath.Join(c.root, "api", "components", "schemas.yaml")
	noiDung, err := os.ReadFile(duong)
	if err != nil {
		return nil, fmt.Errorf("đọc schemas.yaml: %w", err)
	}

	var (
		reSchema    = regexp.MustCompile(`^([A-Z][A-Za-z0-9]*):\s*$`)
		reThuocTinh = regexp.MustCompile(`^    ([a-z][a-z0-9_]*):`)
	)

	ra := map[string][]string{}
	schema := ""
	trongProps := false

	for _, l := range strings.Split(string(noiDung), "\n") {
		if m := reSchema.FindStringSubmatch(l); m != nil {
			schema, trongProps = m[1], false
			ra[schema] = nil
			continue
		}
		if schema == "" {
			continue
		}
		if l == "  properties:" {
			trongProps = true
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}

		// Một dòng ở thụt lề 2 (`type:`, `required:`, `description:`) là
		// thuộc về CHÍNH schema, nên khối `properties:` đã kết thúc.
		thut := len(l) - len(strings.TrimLeft(l, " "))
		if thut <= 2 {
			trongProps = false
			continue
		}
		if !trongProps {
			continue
		}

		// CHỈ nhận thụt lề ĐÚNG 4: đó là tên thuộc tính. Thụt sâu hơn là
		// mô tả của thuộc tính ấy, hoặc thuộc tính của một object LỒNG —
		// và thuộc tính lồng là hình dạng của object con, không phải của
		// schema này.
		if thut != 4 {
			continue
		}
		if m := reThuocTinh.FindStringSubmatch(l); m != nil {
			ra[schema] = append(ra[schema], m[1])
		}
	}
	return ra, nil
}

// docTruongJSON đọc các tên trường JSON của một struct Go.
//
// Đọc AST chứ không grep: một chuỗi `json:"x"` trong chú thích hay trong
// một struct khác cùng file không phải trường của struct này.
//
// Trường nhúng (embedded) được mở ra: `struct{ moneyJSON }` đưa các trường
// của `moneyJSON` lên cùng cấp trong JSON.
func (c *checker) docTruongJSON(goi, kieu string) ([]string, error) {
	thuMuc := filepath.Join(c.root, "internal", filepath.FromSlash(goi))
	vao, err := os.ReadDir(thuMuc)
	if err != nil {
		return nil, fmt.Errorf("đọc gói %s: %w", goi, err)
	}

	fset := token.NewFileSet()
	var ra []string
	timThay := false

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
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != kieu {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			timThay = true
			for _, fld := range st.Fields.List {
				ten := tenJSON(fld)
				if ten != "" && ten != "-" {
					ra = append(ra, ten)
				}
			}
			return false
		})
	}

	if !timThay {
		return nil, fmt.Errorf("không tìm thấy struct %s.%s — đổi tên struct "+
			"thì phải sửa cả sổ `capDTODaKiem`", goi, kieu)
	}
	sort.Strings(ra)
	return ra, nil
}

// tenJSON lấy tên JSON của một trường struct.
func tenJSON(fld *ast.Field) string {
	if fld.Tag == nil {
		return ""
	}
	tag, err := strconv.Unquote(fld.Tag.Value)
	if err != nil {
		return ""
	}
	i := strings.Index(tag, `json:"`)
	if i < 0 {
		return ""
	}
	con := tag[i+len(`json:"`):]
	j := strings.Index(con, `"`)
	if j < 0 {
		return ""
	}
	return strings.SplitN(con[:j], ",", 2)[0]
}

// doiChieuDTO so từng cặp đã khai.
func (c *checker) doiChieuDTO(thuocTinh map[string][]string) {
	for _, cap := range c.capDTO {
		sp, co := thuocTinh[cap.Schema]
		if !co {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"DTO sổ ghép trỏ tới schema %q (%s) mà `components/schemas.yaml` "+
					"không có — đổi tên schema thì phải sửa `capDTODaKiem`",
				cap.Schema, cap.LyDo))
			continue
		}
		if len(sp) == 0 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"DTO schema %q không đọc được thuộc tính nào — cặp %s đang gác "+
					"một tập rỗng, tức không gác gì", cap.Schema, cap.LyDo))
			continue
		}

		gv, err := c.docTruongJSON(cap.GoiGo, cap.KieuGo)
		if err != nil {
			c.viPham = append(c.viPham, fmt.Sprintf("DTO %s: %v", cap.LyDo, err))
			continue
		}

		tapGo, tapSpec := tapChuoi(gv), tapChuoi(sp)

		var thieu []string
		for _, t := range sp {
			if tapGo[t] {
				continue
			}
			if _, choPhep := cap.ChoPhepThieu[t]; choPhep {
				continue
			}
			thieu = append(thieu, t)
		}
		sort.Strings(thieu)
		if len(thieu) > 0 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"DTO %s (%s ↔ %s.%s): đặc tả khai %s mà struct Go KHÔNG có "+
					"trường tương ứng. Client dựng giao diện quanh một trường "+
					"không bao giờ tới — dạng lỗi đã xảy ra NĂM lần, xem P3-80.",
				cap.LyDo, cap.Schema, cap.GoiGo, cap.KieuGo,
				strings.Join(thieu, ", ")))
		}

		var thua []string
		for _, t := range gv {
			if !tapSpec[t] {
				thua = append(thua, t)
			}
		}
		sort.Strings(thua)
		if len(thua) > 0 {
			c.viPham = append(c.viPham, fmt.Sprintf(
				"DTO %s (%s ↔ %s.%s): Go trả %s mà đặc tả KHÔNG khai. Một "+
					"trường rời máy chủ ngoài hợp đồng: không ai biết nó tồn "+
					"tại để dùng, và xóa nó đi là thay đổi PHÁ VỠ không ai "+
					"nhận ra.",
				cap.LyDo, cap.Schema, cap.GoiGo, cap.KieuGo,
				strings.Join(thua, ", ")))
		}

		// Một dòng cho phép thiếu trỏ tới trường KHÔNG CÒN trong đặc tả là
		// một dòng nói dối — nó im lặng ngừng gác.
		for t := range cap.ChoPhepThieu {
			if !tapSpec[t] {
				c.viPham = append(c.viPham, fmt.Sprintf(
					"DTO %s: `ChoPhepThieu` còn dòng cho %q mà đặc tả không "+
						"còn khai trường ấy — xóa đi", cap.LyDo, t))
			}
			if tapGo[t] {
				c.viPham = append(c.viPham, fmt.Sprintf(
					"DTO %s: `ChoPhepThieu` khai %q là chưa trả, nhưng Go ĐÃ "+
						"trả nó — xóa dòng ấy đi", cap.LyDo, t))
			}
		}
	}
}
