package app

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

func (a *apiTest) xemHieuSuat(t *testing.T, tok, ky string) reply {
	t.Helper()
	duong := "/api/v1/seller/performance"
	if ky != "" {
		// Mã hoá: giá trị thử nghiệm có khoảng trắng và dấu chấm phẩy, và
		// httptest.NewRequest PANIC với URL không hợp lệ — bản đầu của bài
		// này chết ở đó chứ không phải ở endpoint.
		duong += "?period=" + url.QueryEscape(ky)
	}
	return a.call(http.MethodGet, duong, nil,
		map[string]string{"Authorization": "Bearer " + tok})
}

// mauNhaBanCoDon lấy một gian hàng có sẵn đơn thực hiện.
func (a *apiTest) mauNhaBanCoDon(t *testing.T) string {
	t.Helper()
	var id string
	err := a.db.Pool().QueryRow(context.Background(),
		`SELECT seller_id FROM fulfillment_order LIMIT 1`).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

// bảoĐảmCóĐơnThựcHiện dựng một đơn thực hiện nếu chưa có.
//
// Database test là khuôn SẠCH, nên bài nào chỉ `t.Skip` khi không có dữ
// liệu là bài không chạy trong mọi lần chạy bình thường — xanh mà rỗng.
func (a *apiTest) baoDamCoDonThucHien(t *testing.T) string {
	t.Helper()
	if id := a.mauNhaBanCoDon(t); id != "" {
		return id
	}

	maPhien := a.dungPhienSanHoanTat(emailMoi("hsdon"), "0900888222")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	a.phatEvent(t)

	id := a.mauNhaBanCoDon(t)
	if id == "" {
		t.Fatalf("không dựng được đơn thực hiện")
	}
	return id
}

// TestHieuSuatTraChiSoVaThuocDoDangDung.
//
// Đặc tả: "chỉ số, ngưỡng, và tác động đều công khai và tường minh", vì
// mô hình chấm điểm hộp đen tạo tranh chấp không giải quyết được.
//
// Một con số không kèm thước đo và cỡ mẫu thì vẫn là hộp đen: "tỷ lệ hủy
// 5%" — của bao nhiêu đơn, và đo đúng hạn theo mốc nào?
func TestHieuSuatTraChiSoVaThuocDoDangDung(t *testing.T) {
	a := newAPITest(t)

	sellerID := a.baoDamCoDonThucHien(t)

	tok := a.taoTokenNhaBan(t, sellerID)
	res := a.xemHieuSuat(t, tok, "")
	if res.code != http.StatusOK {
		t.Fatalf("xem hiệu suất: HTTP %d — %s", res.code, res.raw)
	}

	if got, _ := res.body["period"].(string); got != string(domain.KyMacDinh) {
		t.Errorf("period = %q, cần %q", got, domain.KyMacDinh)
	}

	// Thước đo phải hiện ra: không có nó thì "đúng hạn" là một khẳng định
	// không kiểm chứng được.
	sla, co := res.body["shipping_sla_hours"].(float64)
	if !co || sla != domain.SLAGiaoHang.Hours() {
		t.Errorf("shipping_sla_hours = %v (có=%v), cần %v — thước đo phải "+
			"công khai, nếu không thì chấm điểm là hộp đen",
			sla, co, domain.SLAGiaoHang.Hours())
	}

	// Cỡ mẫu phải hiện ra.
	if _, co := res.body["sample_size"].(float64); !co {
		t.Errorf("thiếu sample_size — không có nó thì tỷ lệ không kiểm "+
			"chứng được: %s", res.raw)
	}
}

// TestHieuSuatNoiRoChiSoCHUAdo là điều kiện để endpoint này trung thực.
//
// Đặc tả khai năm chỉ số; hệ thống chỉ đo được hai. Trả hai rồi im lặng về
// ba chỉ số còn lại tạo ra đúng thứ hộp đen mà đặc tả cấm — chỉ khác là ở
// phía người viết API. Nhà bán không có cách nào biết mình đang được chấm
// bằng những gì.
func TestHieuSuatNoiRoChiSoChuaDo(t *testing.T) {
	a := newAPITest(t)
	sellerID := a.baoDamCoDonThucHien(t)

	res := a.xemHieuSuat(t, a.taoTokenNhaBan(t, sellerID), "")
	if res.code != http.StatusOK {
		t.Fatalf("HTTP %d — %s", res.code, res.raw)
	}

	ds, _ := res.body["not_measured"].([]any)
	if len(ds) == 0 {
		t.Fatalf("không khai chỉ số nào là chưa đo, trong khi đặc tả có "+
			"năm chỉ số mà hệ thống chỉ đo được hai: %s", res.raw)
	}

	// `buy_box_win_rate` là trường hợp quan trọng nhất: đặc tả đặt nó ở
	// mục "tác động", nên một con số bịa ở đó sẽ dẫn tới quyết định kinh
	// doanh sai.
	var coBuyBox bool
	for _, x := range ds {
		m, _ := x.(map[string]any)
		if ten, _ := m["name"].(string); ten == "buy_box_win_rate" {
			coBuyBox = true
			if lyDo, _ := m["reason"].(string); len(lyDo) < 20 {
				t.Errorf("buy_box_win_rate khai chưa đo nhưng lý do trống rỗng")
			}
		}
	}
	if !coBuyBox {
		t.Errorf("không khai buy_box_win_rate là chưa đo — nó phải nằm ở "+
			"đây hoặc phải có số thật, không được biến mất im lặng: %s",
			res.raw)
	}

	// Và KHÔNG được có con số buy box giả ở mục tác động.
	imp, _ := res.body["impact"].(map[string]any)
	if _, co := imp["buy_box_win_rate"]; co {
		t.Errorf("trả buy_box_win_rate trong impact — buy box không được "+
			"lưu lại nên con số đó không thể có thật: %s", res.raw)
	}
}

// TestHieuSuatChanKyLa: tập kỳ là tập ĐÓNG.
//
// Cho phép kỳ tùy ý nghĩa là mở cửa cho một truy vấn quét toàn bộ bảng.
func TestHieuSuatChanKyLa(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTokenNhaBan(t, a.baoDamCoDonThucHien(t))

	for _, ky := range []string{"ALL_TIME", "LAST_1000_DAYS", "'; DROP TABLE"} {
		if got := a.xemHieuSuat(t, tok, ky); got.code != http.StatusBadRequest {
			t.Errorf("kỳ %q được chấp nhận: HTTP %d — %s",
				ky, got.code, got.raw)
		}
	}
	for _, ky := range []string{"LAST_7_DAYS", "LAST_30_DAYS", "LAST_90_DAYS"} {
		if got := a.xemHieuSuat(t, tok, ky); got.code != http.StatusOK {
			t.Errorf("kỳ hợp lệ %q bị từ chối: HTTP %d — %s",
				ky, got.code, got.raw)
		}
	}
}

// TestHieuSuatKhongThayCuaNhaBanKhac — cách ly dữ liệu.
//
// Hiệu suất là thông tin cạnh tranh: tỷ lệ hủy và tốc độ giao của đối thủ
// nói cho biết họ đang yếu ở đâu, và ở đâu thì chen vào được.
//
// # Vì sao so với một gian hàng KHÔNG có đơn
//
// Bản đầu tìm hai gian hàng đều có đơn rồi so cỡ mẫu. Trên khuôn test sạch
// chỉ có một gian hàng như thế, nên bài `t.Skip` trong mọi lần chạy bình
// thường — xanh mà không kiểm gì.
//
// So với một gian hàng KHÔNG có đơn chặt hơn và luôn chạy được: nếu truy
// vấn quên lọc `seller_id`, gian hàng rỗng sẽ thấy đúng số đơn của gian
// hàng kia, và chênh lệch lộ ra ngay.
func TestHieuSuatKhongThayCuaNhaBanKhac(t *testing.T) {
	a := newAPITest(t)

	coDon := a.baoDamCoDonThucHien(t)

	r1 := a.xemHieuSuat(t, a.taoTokenNhaBan(t, coDon), "LAST_90_DAYS")
	if r1.code != http.StatusOK {
		t.Fatalf("gian hàng có đơn: HTTP %d — %s", r1.code, r1.raw)
	}
	n1, _ := r1.body["sample_size"].(float64)
	if n1 <= 0 {
		t.Fatalf("gian hàng có đơn lại báo cỡ mẫu %v — bài test không dựng "+
			"được tình huống để so", n1)
	}

	// Gian hàng KHÔNG có đơn nào.
	trong := ids.MustNew(ids.PrefixSeller).String()
	r2 := a.xemHieuSuat(t, a.taoTokenNhaBan(t, trong), "LAST_90_DAYS")
	if r2.code != http.StatusOK {
		t.Fatalf("gian hàng rỗng: HTTP %d — %s", r2.code, r2.raw)
	}
	n2, _ := r2.body["sample_size"].(float64)

	if n2 != 0 {
		t.Errorf("gian hàng KHÔNG có đơn nào lại thấy cỡ mẫu %v (gian hàng "+
			"kia có %v) — truy vấn không lọc theo seller_id, và mỗi nhà bán "+
			"đang đọc được số liệu của toàn sàn", n2, n1)
	}
}
