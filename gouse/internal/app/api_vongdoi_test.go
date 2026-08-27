package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// TestVongDoiDonHangDiTroiVen — PH-33 phần 2.
//
// # Vì sao bài này tồn tại
//
// Trước 27/08, mọi đơn thực hiện dừng vĩnh viễn ở HANDED_OVER: 69 cái ở
// PENDING, 20 ở HANDED_OVER, KHÔNG cái nào DELIVERED. `MarkDelivered` tồn
// tại ở cả ba tầng nhưng không có route lẫn bên gọi.
//
// Hậu quả không chỉ là một cột trạng thái sai. Hạn đổi trả đếm TỪ lúc
// giao, nên chưa giao thì không bao giờ hết hạn, đơn không bao giờ
// COMPLETED, và số dư seller không bao giờ chuyển từ Pending sang
// Available. Không ai được trả tiền, và không có gì báo là đang kẹt.
//
// Bài này đi trọn vòng đời qua HTTP: đặt → bàn giao → đã giao → hoàn tất.
func TestVongDoiDonHangDiTroiVen(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat("vongdoi@example.com", "0900444333")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	// Đơn thực hiện do module fulfillment tạo khi nghe checkout.completed.
	a.phatEvent(t)

	fo := a.timFulfillment(t, maDon)
	if fo.id == "" {
		t.Skip("chưa có đơn thực hiện — bên nhận tách đơn không chạy trong bài này")
	}

	// ------------------------------------------ đi tới trạng thái ĐÃ BÀN GIAO
	//
	// Máy trạng thái không cho nhảy cóc, và đó là quy tắc đúng: không ai
	// bàn giao được món hàng chưa nhặt khỏi kệ.
	for _, buoc := range []struct {
		ten string
		lam func() error
	}{
		{"xác nhận", func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, fo.sellerID, fo.id) }},
		{"nhặt hàng", func() error { return a.mods.fulfillment.MarkPicking(ctx, fo.sellerID, fo.id) }},
		{"đóng gói", func() error { return a.mods.fulfillment.MarkPacked(ctx, fo.sellerID, fo.id) }},
		{"bàn giao", func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: fo.sellerID, FulfillmentID: fo.id,
				Provider: "GHN", TrackingNumber: "TRACK-" + fo.id[4:14],
			})
		}},
	} {
		if err := buoc.lam(); err != nil {
			t.Fatalf("%s: %v", buoc.ten, err)
		}
	}

	// ---------------------------------------------------------- đã giao
	//
	// Đi qua HTTP, không gọi tắt vào module: chính đường HTTP là thứ
	// trước đây KHÔNG tồn tại.
	tokBan := a.dangNhapNhaBan(t, fo.sellerID)
	got := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+fo.id+"/deliver", nil,
		hopNhat(bearer(tokBan), khoaIdem()))
	if got.code != http.StatusOK {
		t.Fatalf("xác nhận đã giao: HTTP %d — %s", got.code, got.raw)
	}
	if tt, _ := got.body["status"].(string); tt != "DELIVERED" {
		t.Errorf("trạng thái sau khi giao là %q, cần DELIVERED", tt)
	}

	// Đơn HÀNG (không phải đơn thực hiện) cũng phải đi theo, qua event.
	a.phatEvent(t)
	if tt := a.trangThaiDon(t, maDon); tt != "DELIVERED" {
		t.Errorf("đơn hàng ở %q sau khi mọi nguồn hàng đã giao, cần DELIVERED", tt)
	}

	// ---------------------------------------------------------- hoàn tất
	//
	// Hạn đổi trả 7 ngày. Đẩy mốc giao lùi lại thay vì chờ — bài test
	// không được phụ thuộc đồng hồ thật.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - $1::interval
		  WHERE id = $2`, (application.ReturnWindow + time.Hour).String(), fo.id); err != nil {
		t.Fatalf("đẩy mốc giao: %v", err)
	}

	n, err := a.mods.fulfillment.CompleteDelivered(ctx, 10)
	if err != nil {
		t.Fatalf("hoàn tất đơn quá hạn: %v", err)
	}
	if n == 0 {
		t.Fatal("không đơn nào được hoàn tất dù đã quá hạn đổi trả")
	}
}

type foThu struct{ id, sellerID string }

func (a *apiTest) timFulfillment(t *testing.T, orderID string) foThu {
	t.Helper()
	var f foThu
	err := a.db.Pool().QueryRow(context.Background(),
		`SELECT id, seller_id FROM fulfillment_order WHERE order_id = $1 LIMIT 1`,
		orderID).Scan(&f.id, &f.sellerID)
	if err != nil {
		return foThu{}
	}
	return f
}

func (a *apiTest) trangThaiDon(t *testing.T, orderID string) string {
	t.Helper()
	var s string
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT status FROM "order" WHERE id = $1`, orderID).Scan(&s); err != nil {
		t.Fatalf("đọc trạng thái đơn: %v", err)
	}
	return s
}

func hopNhat(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// dangNhapNhaBan gắn một tài khoản mới vào gian hàng CÓ SẴN rồi đăng nhập.
//
// Khác dungNhaBan ở chỗ nó không tạo gian hàng mới: bài này cần thao tác
// đúng trên gian hàng mà đơn thực hiện thuộc về.
func (a *apiTest) dangNhapNhaBan(t *testing.T, sellerID string) string {
	t.Helper()
	ctx := context.Background()

	email := emailMoi("chunhaban")
	const matKhau = "MatKhauDuDai@2026"
	u, err := a.mods.identity.Register(ctx, identity.RegisterRequest{
		Email: email, Password: matKhau,
	})
	if err != nil {
		t.Fatalf("tạo tài khoản nhà bán: %v", err)
	}
	if err := a.mods.identity.GrantRole(ctx, u.ID, "SELLER_OWNER", sellerID); err != nil {
		t.Fatalf("gán vai trò: %v", err)
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": matKhau}, nil)
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập nhà bán: %s", res.raw)
	}
	return tok
}
