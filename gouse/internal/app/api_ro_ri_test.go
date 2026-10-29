package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// internalFields là những tên trường KHÔNG bao giờ được xuất hiện trong một
// phản hồi công khai (không kèm token).
//
// Đây là danh sách ĐEN, và danh sách đen thì luôn thiếu. Nó bắt được lỗi
// phổ biến nhất — thêm trường vào DTO công khai vì "tiện" — nhưng không
// bắt được trường đổi tên. Lớp thứ hai (`TestSensitiveDataNeverLeaksPublicly`)
// bù chỗ đó bằng cách dò theo GIÁ TRỊ.
var internalFields = map[string]string{
	"legal_name":         "tên pháp nhân — hồ sơ nội bộ",
	"tax_code":           "mã số thuế — hồ sơ nội bộ",
	"tax_id":             "mã số thuế — hồ sơ nội bộ",
	"commission_rate_bp": "tỷ lệ hoa hồng — điều khoản thương mại",
	"bank_account":       "tài khoản ngân hàng",
	"account_number":     "số tài khoản ngân hàng",
	"account_holder":     "chủ tài khoản ngân hàng",
	"contact_email":      "liên hệ nội bộ",
	"contact_phone":      "liên hệ nội bộ",
	"suspension_reason":  "lý do đình chỉ — ghi chú vận hành",
	"approved_by":        "người duyệt — danh tính nhân sự nội bộ",
	"internal_notes":     "ghi chú nội bộ",
	"cost":               "giá vốn",
	"cost_price":         "giá vốn",
	"password_hash":      "băm mật khẩu",
	"user_id":            "định danh người dùng khác",
	"reserved_quantity":  "tồn kho giữ chỗ — số liệu vận hành",
}

// publicPaths là mọi đường GET không cần token.
//
// `{}` được thay bằng mã thật lúc chạy: một phản hồi 404 thì không rò rỉ
// gì cả, nên test gọi vào dữ liệu CÓ THẬT mới có giá trị.
func publicPaths(a *apiTest, maSP, maNB string) []string {
	return []string{
		"/api/v1/products",
		"/api/v1/products?limit=20",
		"/api/v1/products/" + maSP,
		"/api/v1/products/" + maSP + "/offers",
		"/api/v1/categories",
		"/api/v1/sellers?ids=" + maNB,
		"/api/v1/search?q=ao",
		"/api/v1/offers/buy-box?product_ids=" + maSP,
	}
}

// TestAPublicResponseCarriesNoInternalField — mục backlog 2.9:
// "chưa có test khẳng định response công khai KHÔNG chứa dữ liệu nội bộ".
//
// Duyệt ĐỆ QUY thân JSON, không chỉ tầng ngoài cùng: rò rỉ hay nằm trong
// object lồng (`data[].seller.tax_code`) — chỗ mà mắt người rà không tới.
func TestAPublicResponseCarriesNoInternalField(t *testing.T) {
	a := newAPITest(t)
	maSP, maNB := publicPattern(t, a)

	for _, route := range publicPaths(a, maSP, maNB) {
		t.Run(route, func(t *testing.T) {
			res := a.call(http.MethodGet, route, nil, nil)
			if res.code != http.StatusOK {
				t.Fatalf("HTTP %d — %s", res.code, res.raw)
			}

			var found any
			if err := json.Unmarshal([]byte(res.raw), &found); err != nil {
				t.Fatalf("giải mã: %v", err)
			}

			var errs []string
			approveJSON(found, "", func(key, position string) {
				if vi, commit := internalFields[strings.ToLower(key)]; commit {
					errs = append(errs, fmt.Sprintf("%s (%s)", position, vi))
				}
			})
			if len(errs) > 0 {
				sort.Strings(errs)
				t.Errorf("phản hồi công khai rò %d trường nội bộ:\n  %s",
					len(errs), strings.Join(dedupe(errs), "\n  "))
			}
		})
	}
}

// TestSensitiveDataNeverLeaksPublicly dò theo GIÁ TRỊ, không theo tên trường.
//
// Bài trên bắt được trường tên `tax_code`. Nó KHÔNG bắt được cùng dữ liệu
// đó xuất hiện dưới tên `code`, `ref`, hay lẫn trong một chuỗi mô tả. Bài
// này nạp giá trị độc nhất vào các cột nội bộ của một gian hàng ĐANG HOẠT
// ĐỘNG, rồi khẳng định chúng không xuất hiện ở BẤT KỲ đâu trong phản hồi
// công khai — bất kể nằm dưới tên gì.
func TestSensitiveDataNeverLeaksPublicly(t *testing.T) {
	a := newAPITest(t)
	_, maNB := publicPattern(t, a)

	// Giá trị mồi: độc nhất để tìm chuỗi không dính nhầm.
	moi := map[string]string{
		"legal_name": "CONG TY TNHH MOI KIEM RO RI ZZQ",
		"tax_code":   "9988776655",
		"email":      "noibo-canary@example.invalid",
		"phone":      "0900111222",
	}

	// Nạp thẳng vào cột nội bộ. Cố ý KHÔNG đi qua API: mục đích là hỏi
	// "nếu các cột này CÓ dữ liệu thì nó có thoát ra ngoài không", chứ
	// không phải kiểm đường ghi.
	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE seller SET legal_name=$2, tax_code=$3, email=$4, phone=$5
		  WHERE id=$1`,
		maNB, moi["legal_name"], moi["tax_code"],
		moi["email"], moi["phone"]); err != nil {
		t.Fatalf("nạp giá trị mồi: %v", err)
	}

	// Gọi KHÔNG kèm token: đúng tư cách người lạ.
	for _, route := range []string{
		"/api/v1/sellers?ids=" + maNB,
		"/api/v1/products",
		"/api/v1/search?q=ao",
	} {
		res := a.call(http.MethodGet, route, nil, nil)
		if res.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", route, res.code, res.raw)
		}
		for cot, value := range moi {
			if strings.Contains(res.raw, value) {
				t.Errorf("%s: cột nội bộ %s RÒ ra công khai (%q): %s",
					route, cot, value, res.raw)
			}
		}
	}

	// Chống xanh rỗng: đường /sellers phải THẬT SỰ trả về gian hàng vừa
	// nạp mồi. Nếu không, ba vòng lặp trên chỉ đang soi phản hồi rỗng.
	gateway := a.call(http.MethodGet, "/api/v1/sellers?ids="+maNB, nil, nil)
	if !strings.Contains(gateway.raw, maNB) {
		t.Fatalf("phản hồi công khai không chứa gian hàng %s — "+
			"bài test sẽ xanh mà không kiểm gì: %s", maNB, gateway.raw)
	}
}

// approveJSON đi hết cây JSON, gọi `gap` cho mỗi khóa kèm đường dẫn tới nó.
func approveJSON(v any, position string, gap func(key, position string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, con := range x {
			route := k
			if position != "" {
				route = position + "." + k
			}
			gap(k, route)
			approveJSON(con, route, gap)
		}
	case []any:
		for i, con := range x {
			approveJSON(con, fmt.Sprintf("%s[%d]", position, i), gap)
		}
	}
}

func dedupe(ds []string) []string {
	var ra []string
	for i, s := range ds {
		if i == 0 || s != ds[i-1] {
			ra = append(ra, s)
		}
	}
	return ra
}

// publicPattern lấy một sản phẩm và một nhà bán CÓ THẬT.
func publicPattern(t *testing.T, a *apiTest) (maSP, maNB string) {
	t.Helper()
	res := a.call(http.MethodGet, "/api/v1/products", nil, nil)
	ds, _ := res.body["data"].([]any)
	if len(ds) == 0 {
		t.Skip("không có sản phẩm")
	}
	sp, _ := ds[0].(map[string]any)
	maSP, _ = sp["id"].(string)

	got := a.call(http.MethodGet, "/api/v1/products/"+maSP+"/offers", nil, nil)
	offers, _ := got.body["data"].([]any)
	if len(offers) > 0 {
		o, _ := offers[0].(map[string]any)
		maNB, _ = o["seller_id"].(string)
	}
	if maNB == "" {
		t.Skip("không có offer để lấy nhà bán")
	}
	return maSP, maNB
}
