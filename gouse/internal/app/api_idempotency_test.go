package app

import (
	"net/http"
	"testing"
)

// TestCungKhoaIdempotencyChayCungLucChiTaoMotDonHang.
//
// # Vì sao phải chạy SONG SONG chứ không gọi hai lần liên tiếp
//
// Gọi tuần tự chỉ chứng minh đường "đọc thấy bản ghi cũ rồi trả về": lần
// hai nhìn thấy kết quả lần một đã ghi xong. Nó bỏ sót đúng cái ca gây mất
// tiền — hai request cùng lúc, CẢ HAI cùng đọc "chưa có đơn nào", cả hai
// cùng đi tiếp, và ta có hai đơn hàng cho một lần bấm nút.
//
// Cửa sổ ấy chỉ mở ra khi hai giao dịch chồng lên nhau. Nút thắt duy nhất
// đóng được nó là ràng buộc UNIQUE trên `order.idempotency_key`, vì chỉ ở
// đó việc kiểm tra và việc ghi mới nằm trong CÙNG một giao dịch — đúng như
// internal/platform/httpserver/idempotency.go giải thích vì sao middleware
// cố tình KHÔNG tự đệm response.
//
// Bài này bắn nhiều lượt hoàn tất cùng một phiên với CÙNG một khóa, rồi
// đếm ở tầng DATABASE. Đếm qua response là không đủ: hai đơn được tạo
// nhưng response thứ hai lỗi vì lý do khác vẫn sẽ lọt.
func TestCungKhoaIdempotencyChayCungLucChiTaoMotDonHang(t *testing.T) {
	a := newAPITest(t)

	maPhien := a.dungPhienSanHoanTat("idem@example.com", "0900111222")

	truocDon := a.demDong(`"order"`, "")
	truocSo := a.demDong("ledger_entry", "")

	const soLuot = 8
	khoa := khoaIdem()
	ketQua := a.goiSongSong(soLuot, http.MethodPost,
		"/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoa)

	sauDon := a.demDong(`"order"`, "")
	sauSo := a.demDong("ledger_entry", "")

	if got := sauDon - truocDon; got != 1 {
		t.Errorf("%d lượt cùng khóa tạo ra %d đơn hàng, cần đúng 1",
			soLuot, got)
	}

	// Sổ cái là chỗ TIỀN nằm. Một đơn hàng nhưng hai bút toán vẫn là
	// ghi nhận doanh thu gấp đôi.
	if got := sauSo - truocSo; got > 1 {
		t.Errorf("%d lượt cùng khóa tạo ra %d bút toán sổ cái, cần tối đa 1",
			soLuot, got)
	}

	// Mọi lượt THÀNH CÔNG phải chỉ về CÙNG một đơn: nếu hai lượt cùng
	// trả 200 với hai mã đơn khác nhau thì khách nhận hai xác nhận.
	maDon := map[string]bool{}
	soThanhCong := 0
	for _, r := range ketQua {
		if r.code != http.StatusOK && r.code != http.StatusCreated {
			continue
		}
		soThanhCong++
		don, _ := r.body["order"].(map[string]any)
		if id, _ := don["id"].(string); id != "" {
			maDon[id] = true
		}
	}
	if len(maDon) > 1 {
		t.Errorf("các lượt thành công trả về %d mã đơn khác nhau: %v",
			len(maDon), maDon)
	}
	if soThanhCong == 0 {
		t.Errorf("không lượt nào thành công — kiểm lại bài test, "+
			"không lượt nào chạm tới đường ghi: %s", ketQua[0].raw)
	}

	t.Logf("%d lượt: %d thành công, %d mã đơn, %d đơn mới",
		soLuot, soThanhCong, len(maDon), sauDon-truocDon)
}

// TestThemGioSongSongKhongMatCapNhat.
//
// # Bất biến này đo GÌ, và KHÔNG đo gì
//
// Giỏ hàng KHÔNG có ràng buộc UNIQUE nào trên khóa idempotency — khác đơn
// hàng và sổ cái. Nên đây KHÔNG phải bài kiểm idempotency; nó kiểm thứ
// khác và quan trọng hơn với giỏ: MẤT CẬP NHẬT.
//
// Thêm vào giỏ là đọc-sửa-ghi. Hai lượt cùng đọc "đang có 1", cùng tính
// "thành 2", cùng ghi 2 — khách bấm hai lần, giỏ chỉ tăng một. Cửa sổ ấy
// chỉ mở khi hai giao dịch chồng lên nhau, nên phải bắn song song.
//
// Khẳng định là ĐẲNG THỨC: mỗi lượt thành công phải cộng đúng 1 món. Nhỏ
// hơn là mất cập nhật, lớn hơn là cộng thừa.
func TestThemGioSongSongKhongMatCapNhat(t *testing.T) {
	a := newAPITest(t)

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	// Một lượt TUẦN TỰ trước, để máy chủ cấp cookie giỏ khách vãng lai.
	// Thiếu bước này, mỗi goroutine tự mở một giỏ riêng và bài test đo
	// nhầm: sáu giỏ mỗi giỏ một món, không phải một giỏ sáu món.
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm lượt đầu: HTTP %d — %s", res.code, res.raw)
	}
	banDau := tongSoMon(t, a)

	// Mỗi lượt một khóa RIÊNG: đây là sáu lần bấm thật, không phải một
	// lần bấm bị gửi lặp. Cùng khóa sẽ biến bài này thành bài idempotency.
	const soLuot = 12
	ketQua := make([]reply, 0, soLuot)
	kq := a.goiSongSongKhoaRieng(soLuot, http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1})
	ketQua = append(ketQua, kq...)

	soThanhCong := 0
	for _, r := range ketQua {
		if r.code == http.StatusOK {
			soThanhCong++
		}
	}
	if soThanhCong == 0 {
		t.Fatalf("không lượt nào thành công: %s", ketQua[0].raw)
	}

	sau := tongSoMon(t, a)
	can := banDau + float64(soThanhCong)

	switch {
	case sau < can:
		t.Errorf("MẤT CẬP NHẬT: %d lượt thành công nhưng giỏ chỉ có %v món, "+
			"cần %v — có lượt ghi đè lên lượt khác", soThanhCong, sau, can)
	case sau > can:
		t.Errorf("CỘNG THỪA: %d lượt thành công nhưng giỏ có %v món, cần %v",
			soThanhCong, sau, can)
	}
	t.Logf("%d lượt, %d thành công, giỏ %v → %v món",
		soLuot, soThanhCong, banDau, sau)
}

// tongSoMon đọc lại giỏ qua HTTP và cộng số lượng mọi dòng.
func tongSoMon(t *testing.T, a *apiTest) float64 {
	t.Helper()
	res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	gio, _ := res.body["cart"].(map[string]any)

	// Món hàng nằm trong `groups[].items[]`, gom theo nhà bán — KHÔNG
	// phải `cart.items`. Đọc nhầm chỗ cho tổng bằng 0 và bài test sẽ báo
	// "mất cập nhật" ở mọi lần chạy.
	nhom, _ := gio["groups"].([]any)

	tong := 0.0
	for _, g := range nhom {
		gm, _ := g.(map[string]any)
		dong, _ := gm["items"].([]any)
		for _, it := range dong {
			m, _ := it.(map[string]any)
			q, _ := m["quantity"].(float64)
			tong += q
		}
	}
	return tong
}

// TestMotPhienThanhToanChiSinhMotDonHang.
//
// # Bất biến
//
// Một phiên thanh toán sinh ra TỐI ĐA MỘT đơn hàng. Không phải "một khóa
// idempotency một đơn" — mà MỘT PHIÊN, một đơn.
//
// Khác biệt ấy là toàn bộ vấn đề. Khóa idempotency bảo vệ trước việc CÙNG
// một lần bấm bị gửi lặp. Nó không bảo vệ được gì trước HAI lần bấm thật:
// khách mở hai tab, mỗi tab sinh khóa riêng, cả hai cùng hoàn tất một
// phiên. Đó là hai ý định khác nhau theo nghĩa của khóa, nhưng vẫn chỉ
// được phép thành một đơn — vì chỉ có một giỏ hàng và một lần giữ hàng.
//
// Ba lớp phòng vệ hiện có đều KHÔNG bắt được ca này khi chạy song song:
// lớp 1 đọc trạng thái phiên ngoài mọi giao dịch (đọc xong mới kiểm, ai
// cũng thấy "chưa hoàn tất"), lớp 2 chỉ idempotent theo CÙNG khóa, và
// lớp 3 ghi trạng thái COMPLETED sau khi đơn đã được tạo — quá muộn.
func TestMotPhienThanhToanChiSinhMotDonHang(t *testing.T) {
	a := newAPITest(t)

	maPhien := a.dungPhienSanHoanTat("motphien@example.com", "0900333444")
	truoc := a.demDong(`"order"`, "")

	// Mỗi lượt một khóa RIÊNG: mô phỏng nhiều tab, không phải một lần
	// bấm bị gửi lặp.
	const soLuot = 8
	ketQua := a.goiSongSongKhoaRieng(soLuot, http.MethodPost,
		"/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"})

	if got := a.demDong(`"order"`, "") - truoc; got != 1 {
		t.Errorf("một phiên sinh ra %d đơn hàng, cần đúng 1", got)
	}

	maDon := map[string]bool{}
	for _, r := range ketQua {
		if r.code != http.StatusOK && r.code != http.StatusCreated {
			continue
		}
		don, _ := r.body["order"].(map[string]any)
		if id, _ := don["id"].(string); id != "" {
			maDon[id] = true
		}
	}
	if len(maDon) > 1 {
		t.Errorf("các lượt thành công trả về %d mã đơn khác nhau — "+
			"khách nhận nhiều xác nhận cho một lần mua: %v", len(maDon), maDon)
	}
	t.Logf("%d lượt → %d mã đơn", soLuot, len(maDon))
}
