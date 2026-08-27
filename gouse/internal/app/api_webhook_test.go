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

const biMatGHN = "bi-mat-ghn-cho-test"

// goiWebhook gửi một webhook vận chuyển, ký bằng khóa cho trước.
//
// Ký trên BYTE THÔ của thân — đúng như production tính. Ký trên bản
// serialize lại sẽ khớp trong test và trượt ngoài đời.
func (a *apiTest) goiWebhook(t *testing.T, nhaCungCap string, than any, biMat string) reply {
	t.Helper()

	raw, err := json.Marshal(than)
	if err != nil {
		t.Fatalf("mã hóa thân: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/webhooks/shipping/"+nhaCungCap, bytes.NewReader(raw))
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

// dungLoHangDaBanGiao dựng một đơn thực hiện đã có mã vận đơn.
func (a *apiTest) dungLoHangDaBanGiao(t *testing.T, maVanDon string) string {
	t.Helper()
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat("webhook@example.com", "0900666555")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	a.phatEvent(t)
	fo := a.timFulfillment(t, maDon)
	if fo.id == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	for _, buoc := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, fo.sellerID, fo.id) },
	} {
		if err := buoc(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}
	if err := a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
		SellerID: fo.sellerID, FulfillmentID: fo.id,
		Provider: "ghn", TrackingNumber: maVanDon,
	}); err != nil {
		t.Fatalf("bàn giao: %v", err)
	}
	return fo.id
}

// TestWebhookGiaMaoBiTuChoi — bất biến BẢO MẬT quan trọng nhất của webhook.
//
// Không có bước xác minh chữ ký, bất kỳ ai biết địa chỉ endpoint đều gửi
// được "đã giao hàng" giả. Hệ thống sẽ chuyển đơn sang DELIVERED, bắt đầu
// đếm hạn đổi trả, rồi tới kỳ là chi tiền cho nhà bán.
func TestWebhookGiaMaoBiTuChoi(t *testing.T) {
	a := newAPITest(t)
	const maVanDon = "GHN-GIA-MAO-001"
	foID := a.dungLoHangDaBanGiao(t, maVanDon)

	than := map[string]any{
		"event_id": "evt_gia_mao_1", "tracking_number": maVanDon, "status": "DELIVERED",
	}

	cac := []struct {
		ten   string
		biMat string
	}{
		{"không có chữ ký", ""},
		{"chữ ký ký bằng khóa khác", "khoa-cua-ke-tan-cong"},
	}
	for _, c := range cac {
		t.Run(c.ten, func(t *testing.T) {
			got := a.goiWebhook(t, "ghn", than, c.biMat)
			if got.code != http.StatusUnauthorized {
				t.Errorf("HTTP %d, cần 401 — %s", got.code, got.raw)
			}
		})
	}

	// Và trạng thái KHÔNG được đổi.
	if tt := a.trangThaiFO(t, foID); tt == "DELIVERED" {
		t.Error("webhook giả mạo đã chuyển được đơn sang DELIVERED")
	}
}

// TestNhaCungCapChuaCauHinhKhoaThiTuChoi — mặc định phải là ĐÓNG.
func TestNhaCungCapChuaCauHinhKhoaThiTuChoi(t *testing.T) {
	a := newAPITest(t)
	than := map[string]any{
		"event_id": "evt_la_1", "tracking_number": "X-1", "status": "DELIVERED",
	}
	// Ký đúng bằng khóa của GHN nhưng gửi tới một hãng chưa cấu hình.
	got := a.goiWebhook(t, "hang-la", than, biMatGHN)
	if got.code != http.StatusUnauthorized {
		t.Errorf("hãng chưa cấu hình khóa vẫn qua: HTTP %d — %s", got.code, got.raw)
	}
}

// TestWebhookGiaoHangChuyenTrangThai — đường thành công và ca gửi trùng.
//
// # Chống gửi trùng có HAI lớp, và bài này không phân biệt được
//
//	chỉ mục UNIQUE (provider, provider_event_id)  → chặn ghi bản ghi thứ hai
//	CapNhatTuHangVanChuyen trả nil khi đã ở đích  → chặn xử lý lần hai
//
// Đã kiểm bằng cách phá: vô hiệu hóa lớp thứ hai thì bài này VẪN XANH, vì
// lớp thứ nhất chặn. Khẳng định "1 bản ghi cho cùng event_id" ở cuối là
// thứ duy nhất chạm thẳng vào lớp thứ nhất.
func TestWebhookGiaoHangChuyenTrangThai(t *testing.T) {
	a := newAPITest(t)
	const maVanDon = "GHN-THAT-001"
	foID := a.dungLoHangDaBanGiao(t, maVanDon)

	than := map[string]any{
		"event_id": "evt_that_1", "tracking_number": maVanDon, "status": "DELIVERED",
	}

	got := a.goiWebhook(t, "ghn", than, biMatGHN)
	if got.code != http.StatusOK {
		t.Fatalf("webhook hợp lệ: HTTP %d — %s", got.code, got.raw)
	}
	if tt := a.trangThaiFO(t, foID); tt != "DELIVERED" {
		t.Errorf("sau webhook, đơn ở %q, cần DELIVERED", tt)
	}

	// GỬI TRÙNG — nhà cung cấp SẼ làm việc này.
	lai := a.goiWebhook(t, "ghn", than, biMatGHN)
	if lai.code != http.StatusOK {
		t.Errorf("gửi trùng trả HTTP %d, cần 200 — báo lỗi khiến họ gửi lại mãi", lai.code)
	}
	if daXuLy, _ := lai.body["already_processed"].(bool); !daXuLy {
		t.Error("lần gửi trùng không được đánh dấu already_processed")
	}
	if n := a.demDong("webhook_event", "provider_event_id = $1", "evt_that_1"); n != 1 {
		t.Errorf("có %d bản ghi webhook cho cùng event_id, cần 1", n)
	}
}

// TestMaVanDonLaTraVe404VaVanGhiNhatKy.
//
// Ghi nhật ký cả khi không xử lý được là yêu cầu số 4 của đặc tả: bản ghi
// ấy là bằng chứng hãng ĐÃ gửi cho một mã ta không biết — thường là dấu
// hiệu dữ liệu hai bên đã lệch nhau.
func TestMaVanDonLaTraVe404VaVanGhiNhatKy(t *testing.T) {
	a := newAPITest(t)
	than := map[string]any{
		"event_id": "evt_ma_la_1", "tracking_number": "KHONG-CO-THAT", "status": "DELIVERED",
	}

	got := a.goiWebhook(t, "ghn", than, biMatGHN)
	if got.code != http.StatusNotFound {
		t.Errorf("mã vận đơn lạ: HTTP %d, cần 404 — %s", got.code, got.raw)
	}
	if n := a.demDong("webhook_event", "provider_event_id = $1", "evt_ma_la_1"); n != 1 {
		t.Error("webhook không xử lý được nhưng KHÔNG được ghi nhật ký")
	}
}

func (a *apiTest) trangThaiFO(t *testing.T, foID string) string {
	t.Helper()
	var s string
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT status FROM fulfillment_order WHERE id = $1`, foID).Scan(&s); err != nil {
		t.Fatalf("đọc trạng thái đơn thực hiện: %v", err)
	}
	return s
}
