package app

import (
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Sửa sản phẩm nháp, đi qua HTTP thật.
//
// # Kịch bản bài này dựng lại
//
// Trước 19/09/2026 KHÔNG có đường sửa sản phẩm. Nên một sản phẩm tạo
// thiếu ảnh là một bản ghi chết: gửi duyệt thì "Sản phẩm chưa có ảnh nào",
// sửa thì không có cửa. Bài này đi đúng con đường ấy — và lần này ra được.
func TestSanPhamTaoThieuAnhGioCuuDuoc(t *testing.T) {
	a := newAPITest(t)
	nb := dungNhaBan(t, a, "suasp"+ids.MustNew(ids.PrefixRequest).String()[24:])
	duy := ids.MustNew(ids.PrefixRequest).String()[20:]

	goi := func(method, duong string, than map[string]any) reply {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + nb.token
		return a.call(method, duong, than, h)
	}

	res := goi(http.MethodPost, "/api/v1/seller/products", map[string]any{
		"brand_id":      a.thuongHieuMo(t),
		"category_id":   a.mauDanhMuc(t),
		"name":          "Nháp thiếu ảnh " + duy,
		"slug":          "nhap-thieu-anh-" + duy,
		"product_type":  "BAG",
		"gender_target": "UNISEX",
		"description":   "Túi vải canvas",
	})
	if res.code != http.StatusCreated {
		t.Fatalf("tạo: %d — %s", res.code, res.raw)
	}
	pid, _ := res.body["id"].(string)
	duong := "/api/v1/seller/products/" + pid

	// BAG không cần bảng size, nên thiếu duy nhất ảnh + chất liệu + biến thể.
	if res := goi(http.MethodPost, duong+"/variants", map[string]any{
		"attributes": map[string]string{"color": "Be"},
		"skus":       []map[string]any{{"sku_code": "TUI-BE-" + duy}},
	}); res.code != http.StatusOK {
		t.Fatalf("thêm biến thể: %d — %s", res.code, res.raw)
	}

	// Trước khi có PATCH, đây là NGÕ CỤT.
	if res := goi(http.MethodPost, duong+"/submit", nil); res.code != http.StatusBadRequest {
		t.Fatalf("gửi duyệt khi thiếu ảnh phải bị từ chối, nhận %d — %s", res.code, res.raw)
	}

	t.Run("PATCH bổ sung đúng thứ còn thiếu, giữ nguyên phần còn lại", func(t *testing.T) {
		res := goi(http.MethodPatch, duong, map[string]any{
			"images":               []string{"https://cdn.example.com/tui-1.jpg"},
			"material_composition": "100% cotton canvas",
		})
		if res.code != http.StatusOK {
			t.Fatalf("PATCH: %d — %s", res.code, res.raw)
		}
		// Tên KHÔNG được gửi, nên phải còn nguyên.
		if got, _ := res.body["name"].(string); got != "Nháp thiếu ảnh "+duy {
			t.Errorf("tên bị đổi thành %q dù PATCH không gửi tên", got)
		}
	})

	t.Run("giờ gửi duyệt được", func(t *testing.T) {
		res := goi(http.MethodPost, duong+"/submit", nil)
		if res.code != http.StatusOK {
			t.Fatalf("gửi duyệt sau khi sửa: %d — %s", res.code, res.raw)
		}
		if got, _ := res.body["status"].(string); got != "PENDING_REVIEW" {
			t.Errorf("mong PENDING_REVIEW, nhận %q", got)
		}
	})

	t.Run("đang CHỜ DUYỆT thì không sửa được nữa", func(t *testing.T) {
		res := goi(http.MethodPatch, duong, map[string]any{"name": "Đổi sau khi gửi"})
		if res.code != http.StatusConflict {
			t.Fatalf("sửa được sản phẩm đang chờ duyệt: %d — %s", res.code, res.raw)
		}
	})
}

// Ba cửa PATCH KHÔNG được mở.
func TestSuaSanPhamCacCuaPhaiDong(t *testing.T) {
	a := newAPITest(t)
	A := dungNhaBan(t, a, "suaa"+ids.MustNew(ids.PrefixRequest).String()[24:])
	B := dungNhaBan(t, a, "suab"+ids.MustNew(ids.PrefixRequest).String()[24:])
	duy := ids.MustNew(ids.PrefixRequest).String()[20:]

	goi := func(tok, method, duong string, than map[string]any) reply {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + tok
		return a.call(method, duong, than, h)
	}

	res := goi(A.token, http.MethodPost, "/api/v1/seller/products", map[string]any{
		"brand_id":      a.thuongHieuMo(t),
		"category_id":   a.mauDanhMuc(t),
		"name":          "Của A " + duy,
		"slug":          "cua-a-" + duy,
		"product_type":  "TOP",
		"gender_target": "WOMEN",
	})
	if res.code != http.StatusCreated {
		t.Fatalf("tạo: %d — %s", res.code, res.raw)
	}
	duong := "/api/v1/seller/products/" + res.body["id"].(string)

	t.Run("gian hàng KHÁC không sửa được — và không biết sản phẩm tồn tại", func(t *testing.T) {
		res := goi(B.token, http.MethodPatch, duong, map[string]any{"name": "B chiếm"})
		// 404 chứ không phải 403: 403 xác nhận sản phẩm tồn tại, đủ để dò
		// mã sản phẩm chưa phát hành của đối thủ.
		if res.code != http.StatusNotFound {
			t.Fatalf("mong 404, nhận %d — %s", res.code, res.raw)
		}
	})

	t.Run("đổi thương hiệu bị từ chối KÈM LÝ DO", func(t *testing.T) {
		res := goi(A.token, http.MethodPatch, duong, map[string]any{
			"brand_id": a.thuongHieuMo(t),
		})
		if res.code != http.StatusBadRequest {
			t.Fatalf("mong 400, nhận %d — %s", res.code, res.raw)
		}
		// Lý do phải nói ĐƯỢC vì sao. "Dữ liệu không hợp lệ" chung chung là
		// thứ `DisallowUnknownFields` tự trả nếu trường không có trong thân.
		if msg := loiMessage(res); !containsAll(msg, "thương hiệu", "tạo sản phẩm mới") {
			t.Errorf("400 không nói vì sao và làm gì: %q", msg)
		}
	})

	t.Run("danh mục KHÔNG tồn tại bị từ chối", func(t *testing.T) {
		res := goi(A.token, http.MethodPatch, duong, map[string]any{
			"category_id": ids.MustNew(ids.PrefixCategory).String(),
		})
		if res.code != http.StatusBadRequest {
			t.Fatalf("ghi được một mã danh mục rác: %d — %s", res.code, res.raw)
		}
	})
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}
