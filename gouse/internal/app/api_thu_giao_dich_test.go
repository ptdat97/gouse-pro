package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestARegisteredCustomerGetsTheOrderConfirmationEmail.
//
// # Vì sao bài này tồn tại
//
// Chạy chuỗi mua hàng thật ở lớp này ngày 17/09 và đọc `notification_log`:
//
//	khách VÃNG LAI    → order_confirmed | SENT
//	khách ĐÃ ĐĂNG KÝ  → order_confirmed | SKIPPED | "thiếu địa chỉ người nhận"
//
// Ngược đúng chiều đáng lẽ phải có: người có tài khoản là người nền tảng
// biết rõ email nhất, và là người nền tảng muốn giữ chân nhất.
//
// Chú thích ở chính chỗ gửi gọi đây là "email QUAN TRỌNG NHẤT của hệ
// thống: bằng chứng đầu tiên khách có rằng tiền của họ đã đổi lấy một cam
// kết". Với khách đã đăng ký, bằng chứng ấy chưa bao giờ được gửi.
//
// Gốc: `Recipient` lấy từ `guest_email` — ô mà chỉ khách vãng lai gõ. Một
// chú thích cạnh đó đã ghi rõ giả định đã cũ: "hiện chưa có module
// customer nên cũng dùng trường này". Module đó tồn tại từ lâu.
//
// # Vì sao không bài test cũ nào bắt được
//
// Bộ bên nhận của lớp test này KHÔNG đăng ký `notification` cho tới cùng
// ngày — chú thích ở `registerReceivers` tự ghi nhận sự thiếu đó. Đường thư
// chỉ chạy ở worker, nơi không test tích hợp nào đi qua.
func TestARegisteredCustomerGetsTheOrderConfirmationEmail(t *testing.T) {
	a := newAPITest(t)
	if a.mods.notification == nil {
		t.Skip("module notification chưa được nối")
	}

	email := newEmail("thudangky")
	tok := a.registerAndLogIn(email)
	bear := func() map[string]string {
		h := idemKey()
		h["Authorization"] = "Bearer " + tok
		return h
	}

	maOffer := a.findSellableOffer()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, bear())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout",
		map[string]any{"cart_id": cartID}, bear())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900555666",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, bear())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, bear())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}

	// Thư đi qua event, và event đi qua outbox.
	a.emitEvent(t)

	var status, skipReason, recipient string
	err := a.db.Pool().QueryRow(context.Background(), `
		SELECT status, skip_reason, recipient
		FROM notification_log
		WHERE template = 'order_confirmed'
		ORDER BY id DESC LIMIT 1`).Scan(&status, &skipReason, &recipient)
	if err != nil {
		t.Fatalf("đọc nhật ký gửi thư: %v — không có bản ghi nào nghĩa là "+
			"bên nhận notification không chạy", err)
	}

	if status != "SENT" {
		t.Fatalf("thư xác nhận đơn của khách ĐÃ ĐĂNG KÝ ở trạng thái %q "+
			"(lý do: %q). Khách vãng lai thì nhận được — ngược đúng chiều "+
			"đáng lẽ phải có.", status, skipReason)
	}
	// So không phân biệt hoa thường: hồ sơ khách lưu email đã chuẩn hóa.
	if !strings.EqualFold(recipient, email) {
		t.Errorf("thư gửi tới %q, cần %q — địa chỉ phải là email của hồ sơ "+
			"khách", recipient, email)
	}
}

// TestAGuestStillGetsTheEmail giữ nửa còn lại của quyết định trên.
//
// Bản sửa ƯU TIÊN email khách gõ ở ô thanh toán, chỉ tra hồ sơ khi ô đó
// trống. Đảo thứ tự ấy sẽ gửi nhầm địa chỉ với đơn đặt hộ: người có tài
// khoản mua giùm người khác và điền email của người nhận.
func TestAGuestStillGetsTheEmail(t *testing.T) {
	a := newAPITest(t)
	if a.mods.notification == nil {
		t.Skip("module notification chưa được nối")
	}

	if orderID := a.placeCODOrder(t, "thuvanglai"); orderID == "" {
		t.Fatal("không đặt được đơn")
	}
	a.emitEvent(t)

	var status, recipient string
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT status, recipient FROM notification_log
		WHERE template = 'order_confirmed'
		ORDER BY id DESC LIMIT 1`).Scan(&status, &recipient); err != nil {
		t.Fatalf("đọc nhật ký gửi thư: %v", err)
	}

	if status != "SENT" {
		t.Fatalf("thư xác nhận đơn của khách vãng lai ở trạng thái %q", status)
	}
	if recipient == "" {
		t.Error("thư gửi tới địa chỉ rỗng")
	}
}
