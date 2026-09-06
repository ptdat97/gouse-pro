package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

const biMatCongTT = "bi-mat-cong-thanh-toan-cho-test"

// goiWebhookThanhToan gửi một webhook thanh toán, ký bằng khóa cho trước.
//
// Ký trên BYTE THÔ của thân, đúng như production tính: ký trên bản
// serialize lại sẽ khớp trong test và trượt ngoài đời.
func (a *apiTest) goiWebhookThanhToan(
	t *testing.T, nhaCungCap string, than any, biMat string,
) reply {
	t.Helper()

	raw, err := json.Marshal(than)
	if err != nil {
		t.Fatalf("mã hóa thân: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/webhooks/payment/"+nhaCungCap, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if biMat != "" {
		req.Header.Set("X-Signature", httpserver.KyHMAC(raw, biMat))
	}

	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)

	out := reply{code: rec.Code, raw: rec.Body.String()}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	}
	return out
}

// donTraTruoc dựng một đơn đã đặt bằng phương thức TRẢ TRƯỚC, trả về mã
// đơn và tổng tiền của nó.
func (a *apiTest) donTraTruoc(t *testing.T, email, dienThoai string) (string, int64) {
	t.Helper()

	maPhien := a.dungPhienSanHoanTat(email, dienThoai)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "CARD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)
	if maDon == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}

	tong, _ := don["total"].(map[string]any)
	soTien, _ := tong["amount"].(float64)
	if soTien <= 0 {
		t.Fatalf("tổng đơn = %v, phải > 0: %s", soTien, res.raw)
	}
	return maDon, int64(soTien)
}

// TestWebhookBaoSaiSoTienThiKHONGThuTien là bài test mà cả PH-36 tồn tại
// vì nó.
//
// # Vì sao chữ ký HMAC là KHÔNG đủ
//
// Webhook trong bài này có chữ ký HỢP LỆ — nó được ký bằng đúng khóa bí
// mật của nhà cung cấp. Thứ sai là CON SỐ bên trong.
//
// Chữ ký chứng minh thông điệp đến TỪ nhà cung cấp; nó không chứng minh
// nội dung khớp với thứ khách đã trả. Ba đường dẫn tới đúng tình huống
// này ngoài đời: lỗi tích hợp phía họ, một khóa bị lộ, và môi trường test
// gọi nhầm vào production.
//
// Cả ba đều kết thúc bằng tiền ghi sai vào một cuốn sổ BẤT BIẾN — ghi rồi
// thì phải ghi bút toán đảo, không xóa được (ADR-0008). Đó là lý do
// endpoint này không được mở trước khi có `payment_intent` để đối chiếu.
func TestWebhookBaoSaiSoTienThiKHONGThuTien(t *testing.T) {
	a := newAPITest(t)

	const dienThoai = "0900888777"
	maDon, tong := a.donTraTruoc(t, "webhook-tt@example.com", dienThoai)

	if tt := a.trangThaiDon(t, maDon); tt != "PENDING_PAYMENT" {
		t.Fatalf("trạng thái ban đầu = %q, cần PENDING_PAYMENT", tt)
	}

	than := func(soTien int64, maSuKien string) map[string]any {
		return map[string]any{
			"event_id":   maSuKien,
			"event_type": "payment.succeeded",
			"data": map[string]any{
				"payment_intent_id": "pi_ben_ngoai_123",
				"amount":            soTien,
				"currency":          "VND",
				"metadata":          map[string]any{"order_id": maDon},
			},
		}
	}

	t.Run("số tiền LỆCH thì 422 và đơn KHÔNG đổi", func(t *testing.T) {
		// Chữ ký HỢP LỆ, số tiền SAI — đúng hình dạng của cả ba tình
		// huống ngoài đời.
		res := a.goiWebhookThanhToan(t, "cong-tt", than(tong+1000, "evt_lech_01"), biMatCongTT)

		if res.code != http.StatusUnprocessableEntity {
			t.Errorf("HTTP %d, cần 422 — %s", res.code, res.raw)
		}
		if tt := a.trangThaiDon(t, maDon); tt != "PENDING_PAYMENT" {
			t.Errorf("trạng thái = %q sau webhook SAI SỐ TIỀN — cần giữ "+
				"PENDING_PAYMENT. Đây là tiền ghi sai vào sổ cái bất biến", tt)
		}
	})

	t.Run("chữ ký sai thì 401", func(t *testing.T) {
		res := a.goiWebhookThanhToan(t, "cong-tt", than(tong, "evt_kysai_01"), "khoa-gia-mao")
		if res.code != http.StatusUnauthorized {
			t.Errorf("HTTP %d, cần 401 — %s", res.code, res.raw)
		}
		if tt := a.trangThaiDon(t, maDon); tt != "PENDING_PAYMENT" {
			t.Errorf("trạng thái = %q sau webhook GIẢ MẠO", tt)
		}
	})

	t.Run("số tiền KHỚP thì thu tiền và đơn thành PAID", func(t *testing.T) {
		res := a.goiWebhookThanhToan(t, "cong-tt", than(tong, "evt_dung_01"), biMatCongTT)
		if res.code != http.StatusOK {
			t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
		}
		if tt := a.trangThaiDon(t, maDon); tt != "PAID" {
			t.Errorf("trạng thái = %q, cần PAID — tiền đã về mà đơn vẫn "+
				"treo là đúng tình trạng trước PH-36", tt)
		}
	})

	t.Run("gửi TRÙNG thì 200 và không hỏng gì", func(t *testing.T) {
		// Yêu cầu 2 của webhooks.yaml: nhà cung cấp SẼ gửi trùng, và báo
		// lỗi khiến họ gửi lại mãi cho một việc đã xong.
		res := a.goiWebhookThanhToan(t, "cong-tt", than(tong, "evt_dung_01"), biMatCongTT)
		if res.code != http.StatusOK {
			t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
		}
		if da, _ := res.body["already_processed"].(bool); !da {
			t.Errorf("already_processed = false, cần true — %s", res.raw)
		}
		if tt := a.trangThaiDon(t, maDon); tt != "PAID" {
			t.Errorf("trạng thái = %q sau bản gửi trùng, cần vẫn PAID", tt)
		}
	})
}

// TestWebhookChoDonCODThiKhongXuLy — COD không có intent, nên không có gì
// để đối chiếu.
//
// Một webhook báo về đơn COD nghĩa là: nhà cung cấp nhầm, hoặc ai đó đang
// thử đường này để xem hệ thống có nuốt không. Cả hai đều không được xử lý.
func TestWebhookChoDonCODThiKhongXuLy(t *testing.T) {
	a := newAPITest(t)

	const dienThoai = "0900888666"
	maPhien := a.dungPhienSanHoanTat("webhook-cod@example.com", dienThoai)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)
	tong, _ := don["total"].(map[string]any)
	soTien, _ := tong["amount"].(float64)

	got := a.goiWebhookThanhToan(t, "cong-tt", map[string]any{
		"event_id":   "evt_cod_01",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"amount":   int64(soTien),
			"currency": "VND",
			"metadata": map[string]any{"order_id": maDon},
		},
	}, biMatCongTT)

	if got.code != http.StatusNotFound {
		t.Errorf("HTTP %d, cần 404 — đơn COD không có ý định thanh toán "+
			"nào để đối chiếu: %s", got.code, got.raw)
	}
	if tt := a.trangThaiDon(t, maDon); tt != "PENDING_PAYMENT" {
		t.Errorf("trạng thái = %q — đơn COD bị webhook đánh dấu đã trả tiền", tt)
	}
}
