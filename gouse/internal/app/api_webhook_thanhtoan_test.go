package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

const paymentGatewaySecret = "bi-mat-cong-thanh-toan-cho-test"

// callPaymentWebhook gửi một webhook thanh toán, ký bằng khóa cho trước.
//
// Ký trên BYTE THÔ của thân, đúng như production tính: ký trên bản
// serialize lại sẽ khớp trong test và trượt ngoài đời.
func (a *apiTest) callPaymentWebhook(
	t *testing.T, provider string, than any, secret string,
) reply {
	t.Helper()

	raw, err := json.Marshal(than)
	if err != nil {
		t.Fatalf("mã hóa thân: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/webhooks/payment/"+provider, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Signature", httpserver.SignHMAC(raw, secret))
	}

	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)

	out := reply{code: rec.Code, raw: rec.Body.String()}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	}
	return out
}

// prepaidOrder dựng một đơn đã đặt bằng phương thức TRẢ TRƯỚC, trả về mã
// đơn và tổng tiền của nó.
func (a *apiTest) prepaidOrder(t *testing.T, email, phone string) (string, int64) {
	t.Helper()

	checkoutID := a.buildCheckoutReadyToComplete(email, phone)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "CARD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)
	if orderID == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}

	sumTotal, _ := ord["total"].(map[string]any)
	amount, _ := sumTotal["amount"].(float64)
	if amount <= 0 {
		t.Fatalf("tổng đơn = %v, phải > 0: %s", amount, res.raw)
	}
	return orderID, int64(amount)
}

// TestAWebhookWithTheWrongAmountDoesNotCapture là bài test mà cả PH-36 tồn tại
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
func TestAWebhookWithTheWrongAmountDoesNotCapture(t *testing.T) {
	a := newAPITest(t)

	const phone = "0900888777"
	orderID, sumTotal := a.prepaidOrder(t, "webhook-tt@example.com", phone)

	if tt := a.orderStatus(t, orderID); tt != "PENDING_PAYMENT" {
		t.Fatalf("trạng thái ban đầu = %q, cần PENDING_PAYMENT", tt)
	}

	than := func(amount int64, eventID string) map[string]any {
		return map[string]any{
			"event_id":   eventID,
			"event_type": "payment.succeeded",
			"data": map[string]any{
				"payment_intent_id": "pi_ben_ngoai_123",
				"amount":            amount,
				"currency":          "VND",
				"metadata":          map[string]any{"order_id": orderID},
			},
		}
	}

	t.Run("số tiền LỆCH thì 422 và đơn KHÔNG đổi", func(t *testing.T) {
		// Chữ ký HỢP LỆ, số tiền SAI — đúng hình dạng của cả ba tình
		// huống ngoài đời.
		res := a.callPaymentWebhook(t, "cong-tt", than(sumTotal+1000, "evt_lech_01"), paymentGatewaySecret)

		if res.code != http.StatusUnprocessableEntity {
			t.Errorf("HTTP %d, cần 422 — %s", res.code, res.raw)
		}
		if tt := a.orderStatus(t, orderID); tt != "PENDING_PAYMENT" {
			t.Errorf("trạng thái = %q sau webhook SAI SỐ TIỀN — cần giữ "+
				"PENDING_PAYMENT. Đây là tiền ghi sai vào sổ cái bất biến", tt)
		}
	})

	t.Run("chữ ký sai thì 401", func(t *testing.T) {
		res := a.callPaymentWebhook(t, "cong-tt", than(sumTotal, "evt_kysai_01"), "khoa-gia-mao")
		if res.code != http.StatusUnauthorized {
			t.Errorf("HTTP %d, cần 401 — %s", res.code, res.raw)
		}
		if tt := a.orderStatus(t, orderID); tt != "PENDING_PAYMENT" {
			t.Errorf("trạng thái = %q sau webhook GIẢ MẠO", tt)
		}
	})

	t.Run("số tiền KHỚP thì thu tiền và đơn thành PAID", func(t *testing.T) {
		res := a.callPaymentWebhook(t, "cong-tt", than(sumTotal, "evt_dung_01"), paymentGatewaySecret)
		if res.code != http.StatusOK {
			t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
		}
		if tt := a.orderStatus(t, orderID); tt != "PAID" {
			t.Errorf("trạng thái = %q, cần PAID — tiền đã về mà đơn vẫn "+
				"treo là đúng tình trạng trước PH-36", tt)
		}
	})

	t.Run("gửi TRÙNG thì 200 và không hỏng gì", func(t *testing.T) {
		// Yêu cầu 2 của webhooks.yaml: nhà cung cấp SẼ gửi trùng, và báo
		// lỗi khiến họ gửi lại mãi cho một việc đã xong.
		res := a.callPaymentWebhook(t, "cong-tt", than(sumTotal, "evt_dung_01"), paymentGatewaySecret)
		if res.code != http.StatusOK {
			t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
		}
		if seen, _ := res.body["already_processed"].(bool); !seen {
			t.Errorf("already_processed = false, cần true — %s", res.raw)
		}
		if tt := a.orderStatus(t, orderID); tt != "PAID" {
			t.Errorf("trạng thái = %q sau bản gửi trùng, cần vẫn PAID", tt)
		}
	})
}

// TestAWebhookForACODOrderIsNotProcessed — COD không có intent, nên không có gì
// để đối chiếu.
//
// Một webhook báo về đơn COD nghĩa là: nhà cung cấp nhầm, hoặc ai đó đang
// thử đường này để xem hệ thống có nuốt không. Cả hai đều không được xử lý.
func TestAWebhookForACODOrderIsNotProcessed(t *testing.T) {
	a := newAPITest(t)

	const phone = "0900888666"
	checkoutID := a.buildCheckoutReadyToComplete("webhook-cod@example.com", phone)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)
	sumTotal, _ := ord["total"].(map[string]any)
	amount, _ := sumTotal["amount"].(float64)

	got := a.callPaymentWebhook(t, "cong-tt", map[string]any{
		"event_id":   "evt_cod_01",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"amount":   int64(amount),
			"currency": "VND",
			"metadata": map[string]any{"order_id": orderID},
		},
	}, paymentGatewaySecret)

	if got.code != http.StatusNotFound {
		t.Errorf("HTTP %d, cần 404 — đơn COD không có ý định thanh toán "+
			"nào để đối chiếu: %s", got.code, got.raw)
	}
	if tt := a.orderStatus(t, orderID); tt != "PENDING_PAYMENT" {
		t.Errorf("trạng thái = %q — đơn COD bị webhook đánh dấu đã trả tiền", tt)
	}
}
