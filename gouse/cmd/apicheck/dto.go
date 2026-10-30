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
// Nên mỗi cặp phải được KIỂM bằng mắt trước khi ghi vào `checkedDTOPairs`.
//
// # Hai chiều, hai hậu quả khác nhau
//
//	đặc tả có · Go KHÔNG có   client dựng giao diện quanh một trường không
//	                          bao giờ tới. Đây là cả năm lần trên.
//	Go có · đặc tả KHÔNG khai  một trường rời máy chủ mà hợp đồng không
//	                          nhắc: không ai biết nó tồn tại để mà dùng,
//	                          và xóa nó đi là thay đổi PHÁ VỠ không ai
//	                          nhận ra

// dtoPair là một cặp đã KIỂM: schema của đặc tả ⇄ struct Go phục vụ nó.
type dtoPair struct {
	// Schema là tên schema cấp cao nhất trong `components/schemas.yaml`.
	Schema string

	// GoPkg là đường dẫn dưới `internal/`, GoType là tên struct.
	GoPkg  string
	GoType string

	// AllowMissing là các trường đặc tả khai mà Go CỐ Ý chưa trả, kèm lý
	// do đọc được.
	//
	// Khác với một cặp không ghép: ở đây phần còn lại vẫn được gác.
	AllowMissing map[string]string

	// Reason nói cặp này về cái gì, cho người đọc thông báo lỗi.
	Reason string
}

// readSchemaProperties đọc thuộc tính của các schema cấp cao nhất.
//
// Đọc theo THỤT LỀ, cùng cách các tầng khác của công cụ này làm: dự án có
// đúng bốn phụ thuộc Go trực tiếp, và thêm một thư viện YAML chỉ để đọc
// đặc tả là đổi một ràng buộc kiến trúc lấy một tiện lợi.
//
// Chỉ nhận thuộc tính ở ĐÚNG hai cấp thụt lề dưới `properties:` của
// schema — thuộc tính lồng trong một object con là hình dạng của object
// ấy, không phải của schema này.
func (c *checker) readSchemaProperties() (map[string][]string, error) {
	path := filepath.Join(c.root, "api", "components", "schemas.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("đọc schemas.yaml: %w", err)
	}

	var (
		reSchema   = regexp.MustCompile(`^([A-Z][A-Za-z0-9]*):\s*$`)
		reProperty = regexp.MustCompile(`^    ([a-z][a-z0-9_]*):`)
	)

	byPkg := map[string][]string{}
	schema := ""
	inProps := false

	for _, l := range strings.Split(string(content), "\n") {
		if m := reSchema.FindStringSubmatch(l); m != nil {
			schema, inProps = m[1], false
			byPkg[schema] = nil
			continue
		}
		if schema == "" {
			continue
		}
		if l == "  properties:" {
			inProps = true
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}

		// Một dòng ở thụt lề 2 (`type:`, `required:`, `description:`) là
		// thuộc về CHÍNH schema, nên khối `properties:` đã kết thúc.
		indent := len(l) - len(strings.TrimLeft(l, " "))
		if indent <= 2 {
			inProps = false
			continue
		}
		if !inProps {
			continue
		}

		// CHỈ nhận thụt lề ĐÚNG 4: đó là tên thuộc tính. Thụt sâu hơn là
		// mô tả của thuộc tính ấy, hoặc thuộc tính của một object LỒNG —
		// và thuộc tính lồng là hình dạng của object con, không phải của
		// schema này.
		if indent != 4 {
			continue
		}
		if m := reProperty.FindStringSubmatch(l); m != nil {
			byPkg[schema] = append(byPkg[schema], m[1])
		}
	}
	return byPkg, nil
}

// readJSONFields đọc các tên trường JSON của một struct Go.
//
// Đọc AST chứ không grep: một chuỗi `json:"x"` trong chú thích hay trong
// một struct khác cùng file không phải trường của struct này.
//
// Trường nhúng (embedded) được mở ra: `struct{ moneyJSON }` đưa các trường
// của `moneyJSON` lên cùng cấp trong JSON.
func (c *checker) readJSONFields(pkg, kind string) ([]string, error) {
	dir := filepath.Join(c.root, "internal", filepath.FromSlash(pkg))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("đọc gói %s: %w", pkg, err)
	}

	fset := token.NewFileSet()
	var byPkg []string
	found := false

	for _, e := range entries {
		entryName := e.Name()
		if e.IsDir() || !strings.HasSuffix(entryName, ".go") ||
			strings.HasSuffix(entryName, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, entryName), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("phân tích %s: %w", entryName, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != kind {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			found = true
			for _, fld := range st.Fields.List {
				entryName := jsonName(fld)
				if entryName != "" && entryName != "-" {
					byPkg = append(byPkg, entryName)
				}
			}
			return false
		})
	}

	if !found {
		return nil, fmt.Errorf("không tìm thấy struct %s.%s — đổi tên struct "+
			"thì phải sửa cả sổ `checkedDTOPairs`", pkg, kind)
	}
	sort.Strings(byPkg)
	return byPkg, nil
}

// tenJSON lấy tên JSON của một trường struct.
func jsonName(fld *ast.Field) string {
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
	rest := tag[i+len(`json:"`):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return strings.SplitN(rest[:j], ",", 2)[0]
}

// compareDTO so từng cặp đã khai.
func (c *checker) compareDTO(properties map[string][]string) {
	for _, pair := range c.dtoPair {
		sp, present := properties[pair.Schema]
		if !present {
			c.violations = append(c.violations, fmt.Sprintf(
				"DTO sổ ghép trỏ tới schema %q (%s) mà `components/schemas.yaml` "+
					"không có — đổi tên schema thì phải sửa `checkedDTOPairs`",
				pair.Schema, pair.Reason))
			continue
		}
		if len(sp) == 0 {
			c.violations = append(c.violations, fmt.Sprintf(
				"DTO schema %q không đọc được thuộc tính nào — cặp %s đang gác "+
					"một tập rỗng, tức không gác gì", pair.Schema, pair.Reason))
			continue
		}

		gv, err := c.readJSONFields(pair.GoPkg, pair.GoType)
		if err != nil {
			c.violations = append(c.violations, fmt.Sprintf("DTO %s: %v", pair.Reason, err))
			continue
		}

		goSet, specSet := stringSet(gv), stringSet(sp)

		var missing []string
		for _, t := range sp {
			if goSet[t] {
				continue
			}
			if _, allowed := pair.AllowMissing[t]; allowed {
				continue
			}
			missing = append(missing, t)
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			c.violations = append(c.violations, fmt.Sprintf(
				"DTO %s (%s ↔ %s.%s): đặc tả khai %s mà struct Go KHÔNG có "+
					"trường tương ứng. Client dựng giao diện quanh một trường "+
					"không bao giờ tới — dạng lỗi đã xảy ra NĂM lần, xem P3-80.",
				pair.Reason, pair.Schema, pair.GoPkg, pair.GoType,
				strings.Join(missing, ", ")))
		}

		var extra []string
		for _, t := range gv {
			if !specSet[t] {
				extra = append(extra, t)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			c.violations = append(c.violations, fmt.Sprintf(
				"DTO %s (%s ↔ %s.%s): Go trả %s mà đặc tả KHÔNG khai. Một "+
					"trường rời máy chủ ngoài hợp đồng: không ai biết nó tồn "+
					"tại để dùng, và xóa nó đi là thay đổi PHÁ VỠ không ai "+
					"nhận ra.",
				pair.Reason, pair.Schema, pair.GoPkg, pair.GoType,
				strings.Join(extra, ", ")))
		}

		// Một dòng cho phép thiếu trỏ tới trường KHÔNG CÒN trong đặc tả là
		// một dòng nói dối — nó im lặng ngừng gác.
		for t := range pair.AllowMissing {
			if !specSet[t] {
				c.violations = append(c.violations, fmt.Sprintf(
					"DTO %s: `AllowMissing` còn dòng cho %q mà đặc tả không "+
						"còn khai trường ấy — xóa đi", pair.Reason, t))
			}
			if goSet[t] {
				c.violations = append(c.violations, fmt.Sprintf(
					"DTO %s: `AllowMissing` khai %q là chưa trả, nhưng Go ĐÃ "+
						"trả nó — xóa dòng ấy đi", pair.Reason, t))
			}
		}
	}
}
