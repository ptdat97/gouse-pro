package app

import (
	"net/http"
	"testing"
)

// Đơn ĐẠT NGƯỠNG MIỄN PHÍ SHIP vẫn phải đặt được.
//
// # Lỗi mà bài này canh
//
// `checkoutJSON` không có trường `shipping_method`, nên trang thanh toán
// không có cách nào biết khách đã chọn cách giao hay chưa. Nó suy ra từ
// `shipping_fee > 0`.
//
// Cách suy ấy sai ở đúng chỗ đắt nhất. Ngưỡng miễn phí ship là 499.000đ;
// đơn vượt ngưỡng có phí bằng 0, nên giao diện đọc thành "chưa chọn cách
// giao" rồi KHÓA nút Đặt hàng vĩnh viễn — kèm dòng gợi ý "Chọn hình thức
// giao hàng để đặt đơn" trong khi khách vừa chọn xong.
//
// Khách mua càng nhiều càng không đặt được hàng. Xem P3-73.
//
// # Vì sao ở tầng HTTP chứ không ở miền
//
// Miền luôn giữ đúng `shippingMethod` — lỗi chưa bao giờ nằm ở đó. Chỗ
// hỏng là tầng DTO vứt trường ấy đi, và chỉ một bài đi qua JSON mới thấy.
// Cùng dạng với `shipping_groups` (P3-50), `variants` (P3-64), trường sửa
// được của sản phẩm (P3-65) và dòng đối soát (P3-69).
//
// # Vì sao KHÔNG đo bằng con số ngưỡng
//
// Ngưỡng là cấu hình nghiệp vụ, đổi được lúc chạy. Bài này dựng một giỏ
// chắc chắn vượt ngưỡng rồi khẳng định BẤT BIẾN: phí bằng 0 thì
// `shipping_method` vẫn phải có mặt. Nếu ai đó nâng ngưỡng lên mười triệu,
// bài này không im lặng xanh — nó sẽ báo rằng giỏ chưa đạt miễn phí ship.
func TestMienPhiShipVanBietDaChonCachGiao(t *testing.T) {
	a := newAPITest(t)

	offerA, offerB := a.haiOfferKhacNhaBan(t)
	if offerA == "" || offerB == "" {
		t.Skip("không có đủ hai nhà bán bán được hàng")
	}

	// Hai nhà bán, mỗi bên hai món: cốt để tiền hàng vượt ngưỡng miễn phí.
	for _, off := range []string{offerA, offerB} {
		if res := a.call(http.MethodPost, "/api/v1/cart/items",
			map[string]any{"offer_id": off, "quantity": 2},
			khoaIdem()); res.code != http.StatusOK {
			t.Fatalf("thêm %s vào giỏ: HTTP %d — %s", off, res.code, res.raw)
		}
	}

	res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("đọc giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": emailMoi("mienphiship"),
		"guest_phone": "0900333222",
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	// Chưa chọn thì trường phải VẮNG MẶT — `omitempty`, không phải chuỗi
	// rỗng. Có mặt với giá trị rỗng sẽ làm `Boolean(...)` ở giao diện vẫn
	// ra false, nhưng nó là một lời khai mập mờ trong hợp đồng.
	if v, co := res.body["shipping_method"]; co {
		t.Errorf("phiên chưa chọn cách giao mà đã khai shipping_method=%v", v)
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Miễn Phí", "phone": "0900333222",
			"street_address": "2 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())

	res = a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("chọn cách giao: HTTP %d — %s", res.code, res.raw)
	}

	phi := soTien(t, res.body, "shipping_fee")
	tienHang := soTien(t, res.body, "subtotal")

	if phi != 0 {
		t.Skipf("giỏ %d đ chưa đạt ngưỡng miễn phí ship (phí %d đ) — "+
			"bài này chỉ có nghĩa khi phí bằng 0", tienHang, phi)
	}

	// ĐÂY LÀ BẤT BIẾN.
	if got, _ := res.body["shipping_method"].(string); got != "STANDARD" {
		t.Fatalf("phí ship bằng 0 mà shipping_method = %q — giao diện sẽ "+
			"đọc thành \"chưa chọn cách giao\" và khóa nút Đặt hàng: %s",
			got, res.raw)
	}

	// Và đọc lại phiên cũng phải thấy, không chỉ ở phản hồi của lệnh ghi.
	res = a.call(http.MethodGet, "/api/v1/checkout/"+maPhien, nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("đọc lại phiên: HTTP %d — %s", res.code, res.raw)
	}
	if got, _ := res.body["shipping_method"].(string); got != "STANDARD" {
		t.Fatalf("đọc lại phiên mất shipping_method (%q) — khách tải lại "+
			"trang thanh toán là mất luôn nút Đặt hàng: %s", got, res.raw)
	}
}

// soTien đọc `{amount, currency}` ở một khóa của phản hồi.
func soTien(t *testing.T, body map[string]any, khoa string) int64 {
	t.Helper()
	m, ok := body[khoa].(map[string]any)
	if !ok {
		t.Fatalf("phản hồi thiếu %s", khoa)
	}
	f, ok := m["amount"].(float64)
	if !ok {
		t.Fatalf("%s.amount không phải số: %v", khoa, m["amount"])
	}
	return int64(f)
}
