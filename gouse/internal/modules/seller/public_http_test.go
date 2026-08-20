package seller_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// publicMux dựng mux CHỈ có route công khai — không Auth, không RequireRole.
//
// Đúng như cmd/api nối nó: bất kỳ ai cũng gọi được.
func publicMux(t *testing.T) (*http.ServeMux, *seller.Module) {
	t.Helper()
	m, _ := newModule(t)
	mux := http.NewServeMux()
	m.RegisterPublicRoutes(mux, slog.Default())
	return mux, m
}

func getJSON(t *testing.T, mux *http.ServeMux, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("đọc JSON: %v — thân: %s", err, rec.Body.String())
		}
	}
	return rec.Code, body
}

// taoNhaBan tạo một nhà bán ngoài với đầy đủ dữ liệu NHẠY CẢM.
//
// Điền hết các trường nội bộ là có chủ ý: test dưới đây kiểm tra chúng
// KHÔNG lọt ra endpoint công khai, và trường rỗng thì không kiểm được gì.
func taoNhaBan(t *testing.T, m *seller.Module, ten string) string {
	t.Helper()
	v, err := m.ApplyAsSeller(context.Background(), seller.ApplicationRequest{
		Name:             ten,
		Slug:             strings.ToLower(strings.ReplaceAll(ten, " ", "-")),
		SellerType:       "BUSINESS",
		LegalName:        "Công ty TNHH " + ten,
		TaxCode:          "0123456789",
		Email:            "lienhe@" + strings.ToLower(ten) + ".example.com",
		Phone:            "+84901234567",
		CommissionRateBP: 1500,
	})
	if err != nil {
		t.Fatalf("ApplyAsSeller: %v", err)
	}
	return v.ID
}

// TestHoSoCongKhaiKhongLoDuLieuNoiBo là bài test QUAN TRỌNG NHẤT của
// endpoint này.
//
// Endpoint không có Auth: bất kỳ ai trên internet gọi được. Tên pháp lý,
// mã số thuế, email, số điện thoại và TỶ LỆ HOA HỒNG đều nằm trong cùng
// một aggregate với tên hiển thị — chỉ cần một dòng `json:` thừa là chúng
// ra ngoài, và không có gì báo động.
//
// Tỷ lệ hoa hồng đặc biệt nhạy: nó là điều khoản thương mại riêng giữa nền
// tảng và từng nhà bán. Lộ ra nghĩa là mọi nhà bán biết đối thủ đang trả
// bao nhiêu.
func TestHoSoCongKhaiKhongLoDuLieuNoiBo(t *testing.T) {
	mux, m := publicMux(t)
	id := taoNhaBan(t, m, "Nha Ban Thu")

	code, body := getJSON(t, mux, "/api/v1/sellers?ids="+id)
	if code != http.StatusOK {
		t.Fatalf("mã HTTP = %d, cần 200", code)
	}

	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("số bản ghi = %d, cần 1", len(data))
	}
	row, _ := data[0].(map[string]any)

	// Chỉ đúng ba trường, không hơn. Kiểm tra danh sách TRẮNG chứ không
	// phải danh sách đen: thêm trường mới vào aggregate mà quên cập nhật
	// danh sách đen thì test vẫn xanh, và dữ liệu vẫn lọt.
	choPhep := map[string]bool{"id": true, "name": true, "is_official": true}
	for k := range row {
		if !choPhep[k] {
			t.Errorf("trường %q lọt ra endpoint công khai", k)
		}
	}

	// Và kiểm tra thẳng vào thân response: giá trị nhạy cảm không được
	// xuất hiện ở BẤT KỲ đâu, kể cả trong một trường lồng nhau nào đó.
	raw, _ := json.Marshal(body)
	for _, bimat := range []string{
		"Công ty TNHH", "0123456789", "example.com", "+84901234567", "1500",
	} {
		if strings.Contains(string(raw), bimat) {
			t.Errorf("dữ liệu nội bộ %q xuất hiện trong response công khai: %s",
				bimat, raw)
		}
	}
}

// TestGiuDungThuTuMaDuocHoi: bên gọi ghép kết quả vào một danh sách đã sắp
// xếp sẵn, nên thứ tự phải theo thứ tự HỎI, không theo thứ tự map.
//
// Sáu nhà bán chứ không phải hai: thứ tự duyệt map trong Go là ngẫu nhiên,
// nên với hai phần tử một cài đặt sai vẫn đúng 50% số lần chạy. Với sáu là
// 1/720.
func TestGiuDungThuTuMaDuocHoi(t *testing.T) {
	mux, m := publicMux(t)

	var ids []string
	for _, ten := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot"} {
		ids = append(ids, taoNhaBan(t, m, ten))
	}

	_, body := getJSON(t, mux, "/api/v1/sellers?ids="+strings.Join(ids, ","))
	data, _ := body["data"].([]any)
	if len(data) != len(ids) {
		t.Fatalf("số bản ghi = %d, cần %d", len(data), len(ids))
	}
	for i, want := range ids {
		row, _ := data[i].(map[string]any)
		if got := row["id"]; got != want {
			t.Fatalf("vị trí %d: %v, cần %s", i, got, want)
		}
	}
}

// TestMaHongKhongLamHongCaLoiGoi: trang đang hiển thị một danh sách, và
// một mã hỏng không đáng để cả danh sách trống.
func TestMaHongKhongLamHongCaLoiGoi(t *testing.T) {
	mux, m := publicMux(t)
	that := taoNhaBan(t, m, "Co That")
	khongTonTai := ids.MustNew(ids.PrefixSeller).String()

	code, body := getJSON(t, mux,
		"/api/v1/sellers?ids=khong-phai-ma,"+khongTonTai+","+that)
	if code != http.StatusOK {
		t.Fatalf("mã HTTP = %d, cần 200", code)
	}

	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("số bản ghi = %d, cần 1 (chỉ nhà bán có thật)", len(data))
	}
	row, _ := data[0].(map[string]any)
	if row["id"] != that {
		t.Errorf("trả về %v, cần %s", row["id"], that)
	}
}

// TestChanLoiGoiKeoCaBang: endpoint công khai không giới hạn là công cụ
// trích xuất dữ liệu miễn phí.
func TestChanLoiGoiKeoCaBang(t *testing.T) {
	mux, _ := publicMux(t)

	var many []string
	for i := 0; i < 51; i++ {
		many = append(many, ids.MustNew(ids.PrefixSeller).String())
	}

	code, _ := getJSON(t, mux, "/api/v1/sellers?ids="+strings.Join(many, ","))
	if code != http.StatusBadRequest {
		t.Fatalf("mã HTTP = %d, cần 400 khi hỏi quá 50 mã", code)
	}
}

// TestThieuThamSoIdsBiChan: thiếu `ids` mà trả cả bảng là hỏng theo kiểu
// nguy hiểm nhất — im lặng và tiện lợi.
func TestThieuThamSoIdsBiChan(t *testing.T) {
	mux, _ := publicMux(t)
	code, _ := getJSON(t, mux, "/api/v1/sellers")
	if code != http.StatusBadRequest {
		t.Fatalf("mã HTTP = %d, cần 400", code)
	}
}
