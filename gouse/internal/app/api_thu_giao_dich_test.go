package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestKhachDangKyNhanDuocThuXacNhanDon.
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
// ngày — chú thích ở `dangKyBenNhan` tự ghi nhận sự thiếu đó. Đường thư
// chỉ chạy ở worker, nơi không test tích hợp nào đi qua.
func TestKhachDangKyNhanDuocThuXacNhanDon(t *testing.T) {
	a := newAPITest(t)
	if a.mods.notification == nil {
		t.Skip("module notification chưa được nối")
	}

	email := emailMoi("thudangky")
	tok := a.dangKyVaDangNhap(email)
	bear := func() map[string]string {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + tok
		return h
	}

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, bear())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout",
		map[string]any{"cart_id": maGio}, bear())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900555666",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, bear())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, bear())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}

	// Thư đi qua event, và event đi qua outbox.
	a.phatEvent(t)

	var status, lyDoBoQua, nguoiNhan string
	err := a.db.Pool().QueryRow(context.Background(), `
		SELECT status, skip_reason, recipient
		FROM notification_log
		WHERE template = 'order_confirmed'
		ORDER BY id DESC LIMIT 1`).Scan(&status, &lyDoBoQua, &nguoiNhan)
	if err != nil {
		t.Fatalf("đọc nhật ký gửi thư: %v — không có bản ghi nào nghĩa là "+
			"bên nhận notification không chạy", err)
	}

	if status != "SENT" {
		t.Fatalf("thư xác nhận đơn của khách ĐÃ ĐĂNG KÝ ở trạng thái %q "+
			"(lý do: %q). Khách vãng lai thì nhận được — ngược đúng chiều "+
			"đáng lẽ phải có.", status, lyDoBoQua)
	}
	// So không phân biệt hoa thường: hồ sơ khách lưu email đã chuẩn hóa.
	if !strings.EqualFold(nguoiNhan, email) {
		t.Errorf("thư gửi tới %q, cần %q — địa chỉ phải là email của hồ sơ "+
			"khách", nguoiNhan, email)
	}
}

// TestKhachVangLaiVanNhanDuocThu giữ nửa còn lại của quyết định trên.
//
// Bản sửa ƯU TIÊN email khách gõ ở ô thanh toán, chỉ tra hồ sơ khi ô đó
// trống. Đảo thứ tự ấy sẽ gửi nhầm địa chỉ với đơn đặt hộ: người có tài
// khoản mua giùm người khác và điền email của người nhận.
func TestKhachVangLaiVanNhanDuocThu(t *testing.T) {
	a := newAPITest(t)
	if a.mods.notification == nil {
		t.Skip("module notification chưa được nối")
	}

	if maDon := a.datDonCOD(t, "thuvanglai"); maDon == "" {
		t.Fatal("không đặt được đơn")
	}
	a.phatEvent(t)

	var status, nguoiNhan string
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT status, recipient FROM notification_log
		WHERE template = 'order_confirmed'
		ORDER BY id DESC LIMIT 1`).Scan(&status, &nguoiNhan); err != nil {
		t.Fatalf("đọc nhật ký gửi thư: %v", err)
	}

	if status != "SENT" {
		t.Fatalf("thư xác nhận đơn của khách vãng lai ở trạng thái %q", status)
	}
	if nguoiNhan == "" {
		t.Error("thư gửi tới địa chỉ rỗng")
	}
}
