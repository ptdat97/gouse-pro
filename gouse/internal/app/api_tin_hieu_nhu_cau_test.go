package app

import (
	"context"
	"net/http"
	"testing"
)

// Bánh đà dữ liệu: hành vi khách phải chảy ngược vào lập kế hoạch sản xuất.
//
// # P3-90 — "đã ghi" không phải "đã gác"
//
// `supplychain.NewSignalHandler` được worker đăng ký từ lâu, nhưng
// `supplychain` KHÔNG nằm trong `Modules` của `internal/app`, nên bộ bên nhận
// của test tích hợp thiếu nó.
//
// Khác biệt ấy đã được GHI trong chú thích của `dangKyBenNhan` suốt nhiều
// tháng — *"KHÔNG đủ bộ: supplychain vẫn không nằm trong Modules"*. Nó vẫn là
// một khoảng trống: không bài nào ở lớp này chứng minh được tín hiệu nhu cầu
// có được ghi, nên một lần nối dây hỏng sẽ im lặng y như
// `inventory.NewReleaseHandler` (P3-86).
//
// Một dòng chú thích mô tả hiện trạng còn làm hiện trạng ấy sống lâu hơn: nó
// biến một việc phải làm thành một đặc điểm đã được giải thích.
//
// # Vì sao dữ liệu này không tạo ngược được
//
// `demand_signal` ghi thứ KHÔNG xuất hiện trong dữ liệu bán hàng: khách tìm
// mà không ra, hàng hết, khách lưu để mua sau. Không ghi hôm nay thì Phase 3
// khởi động với lịch sử trống — cùng lý do `search.no_result` được ghi từ MVP
// dù chưa ai dùng.
func TestThemGioThiGhiTinHieuNhuCau(t *testing.T) {
	a := newAPITest(t)

	if a.mods.supplychain == nil {
		t.Skip("module supplychain cần kho lưu trữ postgres")
	}

	offerID := a.timOfferBanDuoc()
	if offerID == "" {
		t.Skip("không có offer nào bán được")
	}

	truoc := a.demDong("demand_signal", "signal_type = 'ADD_TO_CART'")

	res := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
		"offer_id": offerID, "quantity": 2,
	}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ: HTTP %d — %s", res.code, res.raw)
	}
	a.phatEvent(t)

	sau := a.demDong("demand_signal", "signal_type = 'ADD_TO_CART'")
	if sau != truoc+1 {
		t.Fatalf("tín hiệu ADD_TO_CART: %d → %d, cần tăng ĐÚNG 1 — "+
			"bánh đà dữ liệu hành vi → sản xuất chưa nối ở lớp này", truoc, sau)
	}
}

// Đặt đơn ghi tín hiệu `ORDER`, và nó mang SỐ LƯỢNG.
//
// # Vì sao đo số lượng, không chỉ đếm dòng
//
// Lập kế hoạch sản xuất cần biết BAO NHIÊU, không chỉ "có nhu cầu". Một bản
// cài ghi đủ dòng mà bỏ số lượng sẽ xanh ở phép đếm, và Phase 3 khởi động
// với một bảng đầy dòng vô dụng.
func TestDatDonThiGhiTinHieuORDER(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.supplychain == nil {
		t.Skip("module supplychain cần kho lưu trữ postgres")
	}

	offerID := a.timOfferBanDuoc()
	if offerID == "" {
		t.Skip("không có offer nào bán được")
	}

	const soLuong = 2
	res := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
		"offer_id": offerID, "quantity": soLuong,
	}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	maDon := a.datDonTuGio(t, maGio, emailMoi("tinhieu"), "0900333222")
	a.phatEvent(t)

	// Tín hiệu ORDER của ĐÚNG đơn này, không phải của cả bảng: gói test
	// dùng chung một database, nên đếm cả bảng sẽ trộn đơn của bài khác.
	//
	// `(source_type, source_id)` là cặp truy vết của `demand_signal` — bảng
	// này KHÔNG có `reference_id`, vì tham chiếu của nó vượt module nên
	// không có khóa ngoại nào để đặt tên theo.
	var soDong, tongSoLuong int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*), COALESCE(SUM(quantity), 0)
		  FROM demand_signal
		 WHERE signal_type = 'ORDER'
		   AND source_type = 'order' AND source_id = $1`,
		maDon).Scan(&soDong, &tongSoLuong); err != nil {
		t.Fatalf("đọc tín hiệu nhu cầu: %v", err)
	}

	if soDong == 0 {
		t.Fatalf("đơn %s không sinh tín hiệu ORDER nào — đường "+
			"checkout.completed → supplychain chưa nối", maDon)
	}
	if tongSoLuong != soLuong {
		t.Errorf("tổng số lượng trong tín hiệu = %d, cần %d — lập kế hoạch "+
			"sản xuất cần biết BAO NHIÊU, không chỉ \"có nhu cầu\"",
			tongSoLuong, soLuong)
	}
}
