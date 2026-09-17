package app

import (
	"net/http"
	"testing"
)

// TestKhachBatTatDuocThuKhuyenMai.
//
// # Vì sao bài này tồn tại
//
// Từ 17/09 `notification.Send` từ chối mọi thư MARKETING khi chưa tra được
// đồng ý, và từ chối theo hướng ĐÓNG. Hàng rào ấy đúng — nhưng nếu không
// có đường cho khách CHO đồng ý thì tính năng marketing đầu tiên sẽ chạy
// đúng luật và gửi được KHÔNG một lá thư nào.
//
// `marketing_consent` trước đó nằm trong khối `preferences`, và cả khối bị
// hoãn vì số đo cơ thể cần mã hóa khi lưu (P3-14). Hai thứ nằm chung chỉ
// vì cùng là "tùy chọn của khách"; một bên là dữ liệu nhạy cảm, bên kia là
// hai cờ boolean.
//
// Bài này đi đúng đường khách đi: đọc hồ sơ, bật, đọc lại.
func TestKhachBatTatDuocThuKhuyenMai(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("dongymkt"))

	bear := func() map[string]string {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + tok
		return h
	}
	doc := func() map[string]any {
		t.Helper()
		res := a.call(http.MethodGet, "/api/v1/me", nil,
			map[string]string{"Authorization": "Bearer " + tok})
		if res.code != http.StatusOK {
			t.Fatalf("đọc hồ sơ: HTTP %d — %s", res.code, res.raw)
		}
		kh, _ := res.body["customer"].(map[string]any)
		dy, co := kh["marketing_consent"].(map[string]any)
		if !co {
			t.Fatalf("hồ sơ KHÔNG có marketing_consent — khách không có "+
				"cách nào biết mình đang bật hay tắt: %s", res.raw)
		}
		return dy
	}

	// Mặc định: CHƯA đồng ý. Im lặng coi như đồng ý là cách vi phạm.
	if dau := doc(); dau["email"] != false || dau["sms"] != false {
		t.Fatalf("khách mới đăng ký đã được coi là đồng ý nhận thư: %v", dau)
	}

	res := a.call(http.MethodPatch, "/api/v1/me",
		map[string]any{"marketing_consent": map[string]any{"email": true}}, bear())
	if res.code != http.StatusOK {
		t.Fatalf("bật đồng ý email: HTTP %d — %s", res.code, res.raw)
	}

	sau := doc()
	if sau["email"] != true {
		t.Errorf("bật đồng ý email xong đọc lại vẫn %v", sau["email"])
	}

	// SMS KHÔNG được bật lây: khách tick ô email không phải giấy phép
	// nhắn tin, và `customer` giữ hai loại đồng ý riêng cho đúng lý do đó.
	if sau["sms"] != false {
		t.Errorf("bật email mà SMS cũng bật theo: %v", sau)
	}

	// Rút lại được — "Quyền rút đồng ý" ở docs/09-operations/security.md.
	res = a.call(http.MethodPatch, "/api/v1/me",
		map[string]any{"marketing_consent": map[string]any{"email": false}}, bear())
	if res.code != http.StatusOK {
		t.Fatalf("rút đồng ý: HTTP %d — %s", res.code, res.raw)
	}
	if cuoi := doc(); cuoi["email"] != false {
		t.Errorf("rút đồng ý xong vẫn %v", cuoi["email"])
	}
}

// TestSuaHoSoKhongDUOCRutDongYAmTham.
//
// # Vì sao bài này tồn tại
//
// PATCH nghĩa là "sửa những gì tôi gửi". Nếu `marketing_consent` dùng
// `bool` thay vì con trỏ thì trường vắng mặt đọc ra `false`, nên một lần
// đổi số điện thoại sẽ âm thầm RÚT đồng ý nhận thư khuyến mãi.
//
// Thiệt hại không dừng ở chỗ khách mất thư: bản ghi rút ấy đi vào
// `customer_consent` kèm dấu thời gian và nguồn "settings", nên nó trông
// y như khách tự bấm. Một bằng chứng pháp lý bịa ra bởi một phép gán mặc
// định.
func TestSuaHoSoKhongDUOCRutDongYAmTham(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("khongrutngam"))

	bear := func() map[string]string {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + tok
		return h
	}

	if res := a.call(http.MethodPatch, "/api/v1/me",
		map[string]any{"marketing_consent": map[string]any{
			"email": true, "sms": true,
		}}, bear()); res.code != http.StatusOK {
		t.Fatalf("bật đồng ý: HTTP %d — %s", res.code, res.raw)
	}

	// Sửa MỘT thứ hoàn toàn khác.
	res := a.call(http.MethodPatch, "/api/v1/me",
		map[string]any{"phone": "0900123456"}, bear())
	if res.code != http.StatusOK {
		t.Fatalf("đổi số điện thoại: HTTP %d — %s", res.code, res.raw)
	}

	kh, _ := res.body["customer"].(map[string]any)
	dy, _ := kh["marketing_consent"].(map[string]any)
	if dy["email"] != true || dy["sms"] != true {
		t.Errorf("đổi số điện thoại làm rút đồng ý: %v — trường vắng mặt "+
			"trong PATCH phải là KHÔNG ĐỔI, không phải false", dy)
	}
}
