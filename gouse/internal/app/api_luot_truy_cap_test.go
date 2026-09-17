package app

import (
	"context"
	"net/http"
	"testing"
)

// TestMaLuotTruyCapDiTuHeaderToiPheuChuyenDoi.
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
func TestMaLuotTruyCapDiTuHeaderToiPheuChuyenDoi(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const maLuot = "vis_luot_thu_nghiem_01"
	kem := func() map[string]string {
		h := khoaIdem()
		h["X-Visit-Id"] = maLuot
		return h
	}

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, kem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": emailMoi("luottruycap"),
		"guest_phone": "0900444333",
	}, kem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900444333",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, kem())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, kem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}

	a.phatEvent(t)

	dem := func(ten string) int {
		t.Helper()
		var n int
		if err := a.db.Pool().QueryRow(ctx,
			`SELECT count(*) FROM event_log
			  WHERE event_name = $1 AND session_id = $2`,
			ten, maLuot).Scan(&n); err != nil {
			t.Fatalf("đếm %s: %v", ten, err)
		}
		return n
	}

	if n := dem("order.placed"); n == 0 {
		var thay string
		a.db.Pool().QueryRow(ctx,
			`SELECT coalesce(max(session_id), '(không có sự kiện nào)')
			   FROM event_log WHERE event_name = 'order.placed'`).Scan(&thay)
		t.Errorf("không sự kiện order.placed nào mang mã lượt truy cập %q "+
			"— đang mang %q. Tử số của conversion_rate nằm ở không gian mã "+
			"khác mẫu số, nên tỷ lệ bằng 0 bất kể bán được bao nhiêu.",
			maLuot, thay)
	}
	if n := dem("add_to_cart"); n == 0 {
		t.Errorf("sự kiện add_to_cart không mang mã lượt truy cập — " +
			"bước giữa của phễu đứt khỏi hai bước kia")
	}
}

// TestKhongCoHeaderThiKhongBIACHIRA.
//
// Nửa còn lại của quyết định: client cũ không gửi header là chuyện BÌNH
// THƯỜNG, và máy chủ KHÔNG được bịa một mã thay.
//
// Một mã bịa ra ở máy chủ sẽ khác nhau ở mỗi request, nên mỗi lượt gọi
// thành một "lượt truy cập" riêng — và nó sẽ là một lượt truy cập chưa bao
// giờ xem sản phẩm nào. Mẫu số không đổi, tử số phình lên, tỷ lệ chuyển
// đổi vọt lên một con số vô nghĩa. Thà để TRỐNG (ADR-0020 điều 1).
func TestKhongCoHeaderThiKhongBIACHIRA(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon := a.datDonCOD(t, "khongheader")
	if maDon == "" {
		t.Fatal("không đặt được đơn")
	}
	a.phatEvent(t)

	// Hỏi về ĐÚNG đơn này, không hỏi cả bảng: database test dùng chung
	// nên bài khác cũng ghi vào đây, và một phép đếm rộng sẽ đỏ vì dữ
	// liệu của người khác.
	var maPhien string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT coalesce(max(session_id), '') FROM event_log
		  WHERE event_name = 'order.placed' AND subject_id = $1`,
		maDon).Scan(&maPhien); err != nil {
		t.Fatalf("đọc sự kiện của đơn: %v", err)
	}
	if maPhien != "" {
		t.Errorf("sự kiện của đơn %s mang session_id %q dù request không "+
			"gửi X-Visit-Id — máy chủ đang bịa mã thay client",
			maDon, maPhien)
	}
}
