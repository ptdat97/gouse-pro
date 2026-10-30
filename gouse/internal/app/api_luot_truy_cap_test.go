package app

import (
	"context"
	"net/http"
	"testing"
)

// TestTheVisitIDTravelsFromTheHeaderToTheConversionRecord.
//
// # Vì sao bài này tồn tại
//
// `conversion_rate` bằng 0 vĩnh viễn vì tử số và mẫu số đếm hai không gian
// mã khác nhau: mẫu số đếm mã LƯỢT TRUY CẬP (sự kiện xem hàng, do trình
// duyệt sinh), tử số đếm mã PHIÊN THANH TOÁN. Hai tập không bao giờ giao
// nhau (ADR-0020).
//
// Bài này đi trọn đường mà mã lượt truy cập phải đi:
//
//	header X-Visit-Id  →  giỏ hàng  →  phiên thanh toán
//	                   →  domain event  →  event_log.session_id
//
// Nó kiểm ĐÚNG chỗ đứt: một cột trong database, sau một request HTTP thật.
// Kiểm ở tầng module sẽ xanh mà không chứng minh được gì — chính vì bên
// nhận và bên phát đều đúng khi xét riêng mà lỗi sống sót nhiều tháng.
func TestTheVisitIDTravelsFromTheHeaderToTheConversionRecord(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const visitID = "vis_luot_thu_nghiem_01"
	extraHeaders := func() map[string]string {
		h := idemKey()
		h["X-Visit-Id"] = visitID
		return h
	}

	offerID := a.findSellableOffer()
	if offerID == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": offerID, "quantity": 1}, extraHeaders())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": newEmail("luottruycap"),
		"guest_phone": "0900444333",
	}, extraHeaders())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900444333",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, extraHeaders())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, extraHeaders())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}

	a.emitEvent(t)

	count := func(ten string) int {
		t.Helper()
		var n int
		if err := a.db.Pool().QueryRow(ctx,
			`SELECT count(*) FROM event_log
			  WHERE event_name = $1 AND session_id = $2`,
			ten, visitID).Scan(&n); err != nil {
			t.Fatalf("đếm %s: %v", ten, err)
		}
		return n
	}

	if n := count("order.placed"); n == 0 {
		var found string
		a.db.Pool().QueryRow(ctx,
			`SELECT coalesce(max(session_id), '(không có sự kiện nào)')
			   FROM event_log WHERE event_name = 'order.placed'`).Scan(&found)
		t.Errorf("không sự kiện order.placed nào mang mã lượt truy cập %q "+
			"— đang mang %q. Tử số của conversion_rate nằm ở không gian mã "+
			"khác mẫu số, nên tỷ lệ bằng 0 bất kể bán được bao nhiêu.",
			visitID, found)
	}
	if n := count("add_to_cart"); n == 0 {
		t.Errorf("sự kiện add_to_cart không mang mã lượt truy cập — " +
			"bước giữa của phễu đứt khỏi hai bước kia")
	}
}

// TestWithoutTheHeaderNothingIsDisclosed.
//
// Nửa còn lại của quyết định: client cũ không gửi header là chuyện BÌNH
// THƯỜNG, và máy chủ KHÔNG được bịa một mã thay.
//
// Một mã bịa ra ở máy chủ sẽ khác nhau ở mỗi request, nên mỗi lượt gọi
// thành một "lượt truy cập" riêng — và nó sẽ là một lượt truy cập chưa bao
// giờ xem sản phẩm nào. Mẫu số không đổi, tử số phình lên, tỷ lệ chuyển
// đổi vọt lên một con số vô nghĩa. Thà để TRỐNG (ADR-0020 điều 1).
func TestWithoutTheHeaderNothingIsDisclosed(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID := a.placeCODOrder(t, "khongheader")
	if orderID == "" {
		t.Fatal("không đặt được đơn")
	}
	a.emitEvent(t)

	// Hỏi về ĐÚNG đơn này, không hỏi cả bảng: database test dùng chung
	// nên bài khác cũng ghi vào đây, và một phép đếm rộng sẽ đỏ vì dữ
	// liệu của người khác.
	var checkoutID string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT coalesce(max(session_id), '') FROM event_log
		  WHERE event_name = 'order.placed' AND subject_id = $1`,
		orderID).Scan(&checkoutID); err != nil {
		t.Fatalf("đọc sự kiện của đơn: %v", err)
	}
	if checkoutID != "" {
		t.Errorf("sự kiện của đơn %s mang session_id %q dù request không "+
			"gửi X-Visit-Id — máy chủ đang bịa mã thay client",
			orderID, checkoutID)
	}
}
