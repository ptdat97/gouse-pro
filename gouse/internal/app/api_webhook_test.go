package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

const carrierSecret = "bi-mat-ghn-cho-test"

// callWebhook gửi một webhook vận chuyển, ký bằng khóa cho trước.
//
// Ký trên BYTE THÔ của thân — đúng như production tính. Ký trên bản
// serialize lại sẽ khớp trong test và trượt ngoài đời.
func (a *apiTest) callWebhook(t *testing.T, provider string, than any, secret string) reply {
	t.Helper()

	raw, err := json.Marshal(than)
	if err != nil {
		t.Fatalf("mã hóa thân: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/webhooks/shipping/"+provider, bytes.NewReader(raw))
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

// buildHandedOverBatch dựng một đơn thực hiện đã có mã vận đơn.
func (a *apiTest) buildHandedOverBatch(t *testing.T, trackingNumber string) string {
	t.Helper()
	ctx := context.Background()

	checkoutID := a.buildCheckoutReadyToComplete("webhook@example.com", "0900666555")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)

	a.emitEvent(t)
	fo := a.findFulfillment(t, orderID)
	if fo.id == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	for _, steps := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, fo.sellerID, fo.id) },
	} {
		if err := steps(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}
	if err := a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
		SellerID: fo.sellerID, FulfillmentID: fo.id,
		Provider: "ghn", TrackingNumber: trackingNumber,
	}); err != nil {
		t.Fatalf("bàn giao: %v", err)
	}
	return fo.id
}

// TestAForgedWebhookIsRejected — bất biến BẢO MẬT quan trọng nhất của webhook.
//
// Không có bước xác minh chữ ký, bất kỳ ai biết địa chỉ endpoint đều gửi
// được "đã giao hàng" giả. Hệ thống sẽ chuyển đơn sang DELIVERED, bắt đầu
// đếm hạn đổi trả, rồi tới kỳ là chi tiền cho nhà bán.
func TestAForgedWebhookIsRejected(t *testing.T) {
	a := newAPITest(t)
	const trackingNumber = "GHN-GIA-MAO-001"
	foID := a.buildHandedOverBatch(t, trackingNumber)

	than := map[string]any{
		"event_id": "evt_gia_mao_1", "tracking_number": trackingNumber, "status": "DELIVERED",
	}

	items := []struct {
		ten    string
		secret string
	}{
		{"không có chữ ký", ""},
		{"chữ ký ký bằng khóa khác", "khoa-cua-ke-tan-cong"},
	}
	for _, c := range items {
		t.Run(c.ten, func(t *testing.T) {
			got := a.callWebhook(t, "ghn", than, c.secret)
			if got.code != http.StatusUnauthorized {
				t.Errorf("HTTP %d, cần 401 — %s", got.code, got.raw)
			}
		})
	}

	// Và trạng thái KHÔNG được đổi.
	if tt := a.foStatus(t, foID); tt == "DELIVERED" {
		t.Error("webhook giả mạo đã chuyển được đơn sang DELIVERED")
	}
}

// TestAnUnconfiguredProviderKeyIsRejected — mặc định phải là ĐÓNG.
func TestAnUnconfiguredProviderKeyIsRejected(t *testing.T) {
	a := newAPITest(t)
	than := map[string]any{
		"event_id": "evt_la_1", "tracking_number": "X-1", "status": "DELIVERED",
	}
	// Ký đúng bằng khóa của GHN nhưng gửi tới một hãng chưa cấu hình.
	got := a.callWebhook(t, "hang-la", than, carrierSecret)
	if got.code != http.StatusUnauthorized {
		t.Errorf("hãng chưa cấu hình khóa vẫn qua: HTTP %d — %s", got.code, got.raw)
	}
}

// TestADeliveryWebhookMovesTheStatus — đường thành công và ca gửi trùng.
//
// # Chống gửi trùng có HAI lớp, và bài này không phân biệt được
//
//	chỉ mục UNIQUE (provider, provider_event_id)  → chặn ghi bản ghi thứ hai
//	CapNhatTuHangVanChuyen trả nil khi đã ở đích  → chặn xử lý lần hai
//
// Đã kiểm bằng cách phá: vô hiệu hóa lớp thứ hai thì bài này VẪN XANH, vì
// lớp thứ nhất chặn. Khẳng định "1 bản ghi cho cùng event_id" ở cuối là
// thứ duy nhất chạm thẳng vào lớp thứ nhất.
func TestADeliveryWebhookMovesTheStatus(t *testing.T) {
	a := newAPITest(t)
	const trackingNumber = "GHN-THAT-001"
	foID := a.buildHandedOverBatch(t, trackingNumber)

	than := map[string]any{
		"event_id": "evt_that_1", "tracking_number": trackingNumber, "status": "DELIVERED",
	}

	got := a.callWebhook(t, "ghn", than, carrierSecret)
	if got.code != http.StatusOK {
		t.Fatalf("webhook hợp lệ: HTTP %d — %s", got.code, got.raw)
	}
	if tt := a.foStatus(t, foID); tt != "DELIVERED" {
		t.Errorf("sau webhook, đơn ở %q, cần DELIVERED", tt)
	}

	// GỬI TRÙNG — nhà cung cấp SẼ làm việc này.
	again := a.callWebhook(t, "ghn", than, carrierSecret)
	if again.code != http.StatusOK {
		t.Errorf("gửi trùng trả HTTP %d, cần 200 — báo lỗi khiến họ gửi lại mãi", again.code)
	}
	if processed, _ := again.body["already_processed"].(bool); !processed {
		t.Error("lần gửi trùng không được đánh dấu already_processed")
	}
	if n := a.countRows("webhook_event", "provider_event_id = $1", "evt_that_1"); n != 1 {
		t.Errorf("có %d bản ghi webhook cho cùng event_id, cần 1", n)
	}
}

// TestAnUnknownTrackingNumberReturns404AndStillLogs.
//
// Ghi nhật ký cả khi không xử lý được là yêu cầu số 4 của đặc tả: bản ghi
// ấy là bằng chứng hãng ĐÃ gửi cho một mã ta không biết — thường là dấu
// hiệu dữ liệu hai bên đã lệch nhau.
func TestAnUnknownTrackingNumberReturns404AndStillLogs(t *testing.T) {
	a := newAPITest(t)
	than := map[string]any{
		"event_id": "evt_ma_la_1", "tracking_number": "KHONG-CO-THAT", "status": "DELIVERED",
	}

	got := a.callWebhook(t, "ghn", than, carrierSecret)
	if got.code != http.StatusNotFound {
		t.Errorf("mã vận đơn lạ: HTTP %d, cần 404 — %s", got.code, got.raw)
	}
	if n := a.countRows("webhook_event", "provider_event_id = $1", "evt_ma_la_1"); n != 1 {
		t.Error("webhook không xử lý được nhưng KHÔNG được ghi nhật ký")
	}
}

func (a *apiTest) foStatus(t *testing.T, foID string) string {
	t.Helper()
	var s string
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT status FROM fulfillment_order WHERE id = $1`, foID).Scan(&s); err != nil {
		t.Fatalf("đọc trạng thái đơn thực hiện: %v", err)
	}
	return s
}
