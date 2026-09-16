package app

import (
	"net/http"
	"testing"
)

// TestTokenHetHanGiuaCheckoutTraVe401.
//
// # Vì sao bài này tồn tại
//
// Chạy hệ thống thật trên Docker (16/09): access token hết hạn giữa lúc
// mua hàng, rồi `POST /api/v1/checkout` trả **403 "Giỏ hàng này không
// thuộc về bạn"**.
//
// Mã 403 đó là ngõ cụt. Hợp đồng với giao diện nói rõ: *401 → thử refresh
// một lần rồi retry; 403 → KHÔNG retry* (admin-ui-plan.md mục 291). Nên
// khách kẹt lại giữa bước thanh toán dù refresh token của họ còn hạn và
// chỉ cần một lượt làm mới là đi tiếp được.
//
// Chuỗi dẫn tới đó đều ĐÚNG ở từng mắt: OptionalAuth cố ý bỏ qua token
// hỏng để khách vãng lai mua được; ResolveShopper vì thế rơi về phiên
// cookie; giỏ của phiên cookie khác giỏ của tài khoản; handler thấy hai
// mã không khớp và từ chối. Chỗ sai là câu trả lời cuối: người vừa hết
// hạn token không phải người lạ.
//
// Bài này dùng token RÁC thay cho token hết hạn — cùng một nhánh mã, vì
// OptionalAuth xử lý mọi lỗi xác minh như nhau.
func TestTokenHetHanGiuaCheckoutTraVe401(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("tokenhethan"))

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	// Giỏ tạo khi ĐANG đăng nhập → gắn với hồ sơ khách.
	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, h)
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)
	if maGio == "" {
		t.Fatalf("không lấy được mã giỏ: %s", res.raw)
	}

	// Sang THIẾT BỊ khác: cookie phiên vãng lai không đi theo, nên danh
	// tính duy nhất còn lại là token — và token vừa chết.
	//
	// Cùng một trình duyệt thì cookie `shopper_session` vẫn trỏ đúng giỏ
	// kể cả khi token hỏng, nên lỗi không lộ ra. Nó lộ ra ở máy thứ hai,
	// hoặc sau khi khách xóa cookie: giỏ đến từ HỒ SƠ khách, còn phiên
	// vãng lai mới thì chưa có giỏ nào.
	delete(a.cookies, "shopper_session")

	// Token chết giữa chừng.
	h = khoaIdem()
	h["Authorization"] = "Bearer khong.phai.token"
	res = a.call(http.MethodPost, "/api/v1/checkout",
		map[string]any{"cart_id": maGio}, h)

	if res.code == http.StatusForbidden {
		t.Fatalf("token hết hạn bị trả 403 — client sẽ KHÔNG làm mới token "+
			"và khách kẹt giữa bước thanh toán: %s", res.raw)
	}
	if res.code != http.StatusUnauthorized {
		t.Fatalf("mong 401 để client làm mới token rồi gọi lại, nhận HTTP %d — %s",
			res.code, res.raw)
	}
}

// TestKhachLaKhongDuocMuonGioNguoiKhac giữ nửa còn lại của quyết định trên.
//
// Nới 403 thành 401 cho token hỏng KHÔNG được nới luôn cho người gọi
// không mang token nào. Với họ 403 vẫn đúng: không có gì để làm mới, và
// một mã giỏ đoán trúng không được đổi thành đường thử lại vô hạn.
func TestKhachLaKhongDuocMuonGioNguoiKhac(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("gionguoikhac"))

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, h)
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	// KHÔNG header Authorization và KHÔNG cookie phiên của chủ giỏ: đây
	// là người lạ cầm được mã giỏ, không phải chủ giỏ mất token.
	delete(a.cookies, "shopper_session")
	res = a.call(http.MethodPost, "/api/v1/checkout",
		map[string]any{"cart_id": maGio}, khoaIdem())
	if res.code != http.StatusForbidden {
		t.Fatalf("khách ẩn danh dùng mã giỏ của người khác phải nhận 403, "+
			"nhận HTTP %d — %s", res.code, res.raw)
	}
}

// TestTokenHetHanKhongBienChuDonThanhNguoiLa.
//
// Cùng một quyết định với bài trên, ở ba đường KHÁC: đọc đơn, đọc kiện
// hàng, xin trả hàng. Ba đường này trả **404** cho người không phải chủ —
// và câu 404 đó đúng với người lạ, vì mã đơn tăng dần nên hai câu trả lời
// khác nhau sẽ đếm được số đơn nền tảng bán mỗi tháng.
//
// Nhưng với chủ đơn vừa hết hạn token, 404 nói "đơn của bạn không tồn
// tại" và client không thử lại sau 404. Thấy trên Docker (16/09): token
// hết hạn 48 giây trước, `POST /orders/{id}/returns` trả 404.
func TestTokenHetHanKhongBienChuDonThanhNguoiLa(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("chudonhethan"))

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, h)
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	h = khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res = a.call(http.MethodPost, "/api/v1/checkout",
		map[string]any{"cart_id": maGio}, h)
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	h = khoaIdem()
	h["Authorization"] = "Bearer " + tok
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900555666",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, h)

	h = khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res = a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, h)
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	// Token chết. Cookie phiên vãng lai không cứu được ở đây: quyền xem
	// đơn tra theo customer_id, không theo phiên.
	hong := map[string]string{"Authorization": "Bearer khong.phai.token"}

	duong := []struct {
		ten    string
		method string
		path   string
		than   any
	}{
		{"đọc đơn", http.MethodGet, "/api/v1/orders/" + maDon, nil},
		{"đọc kiện", http.MethodGet, "/api/v1/orders/" + maDon + "/shipments", nil},
		{"xin trả", http.MethodPost, "/api/v1/orders/" + maDon + "/returns",
			map[string]any{"lines": []any{}}},
	}
	for _, d := range duong {
		hd := hong
		if d.than != nil {
			hd = hopNhat(khoaIdem(), hong)
		}
		got := a.call(d.method, d.path, d.than, hd)
		if got.code == http.StatusNotFound {
			t.Errorf("%s: token hết hạn bị trả 404 — client không thử lại "+
				"và chủ đơn tưởng đơn của mình biến mất: %s", d.ten, got.raw)
			continue
		}
		if got.code != http.StatusUnauthorized {
			t.Errorf("%s: mong 401 để client làm mới token, nhận HTTP %d — %s",
				d.ten, got.code, got.raw)
		}
	}
}
