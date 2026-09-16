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
