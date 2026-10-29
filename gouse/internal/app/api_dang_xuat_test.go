package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// TestLogoutRevokesTheSession.
//
// # Vì sao bài này tồn tại
//
// Phép quét "tuyến nào không bài test nào gọi tới" ra đúng HAI tuyến trên
// tổng 86, và đây là một trong hai.
//
// Đăng xuất không thu hồi được phiên là lỗ hổng NHÌN TỪ NGOÀI KHÔNG THẤY:
// giao diện báo đã đăng xuất, cookie biến mất khỏi trình duyệt, nhưng ai
// giữ được token cũ — người mượn máy, một bản sao trong log, một tiện ích
// trình duyệt — vẫn đăng nhập lại được.
//
// Nên bài này KHÔNG chỉ kiểm cookie đã xóa. Nó giữ lại token và thử dùng
// sau khi đăng xuất, đúng cách kẻ giữ token sẽ làm.
func TestLogoutRevokesTheSession(t *testing.T) {
	a := newAPITest(t)
	a.registerAndLogIn(newEmail("dangxuat"))

	token := a.cookies["refresh_token"]
	if token == "" {
		t.Fatal("đăng nhập không cấp refresh token — bài kiểm không có gì để thu hồi")
	}

	// Trước khi đăng xuất, token dùng được.
	if res := a.call(http.MethodPost, "/api/v1/auth/refresh", nil, nil); res.code != http.StatusOK {
		t.Fatalf("làm mới phiên trước khi đăng xuất: HTTP %d — %s", res.code, res.raw)
	}

	// Token có thể vừa được XOAY khi làm mới — lấy bản hiện hành.
	token = a.cookies["refresh_token"]

	res := a.call(http.MethodPost, "/api/v1/auth/logout", nil, idemKey())
	if res.code != http.StatusNoContent && res.code != http.StatusOK {
		t.Fatalf("đăng xuất: HTTP %d — %s", res.code, res.raw)
	}

	// 1. Cookie phải bị xóa khỏi trình duyệt.
	if a.cookies["refresh_token"] != "" {
		t.Error("cookie refresh_token vẫn còn sau khi đăng xuất")
	}

	// 2. VÀ token cũ phải CHẾT ở phía máy chủ.
	//
	// Đây là nửa quan trọng hơn: xóa cookie chỉ dọn trình duyệt của người
	// đang ngồi đó. Ai giữ bản sao token vẫn dùng được nếu máy chủ không
	// thu hồi.
	a.cookies["refresh_token"] = token
	res = a.call(http.MethodPost, "/api/v1/auth/refresh", nil, nil)
	if res.code == http.StatusOK {
		t.Error("token CŨ vẫn làm mới được phiên sau khi đăng xuất — " +
			"đăng xuất chỉ dọn trình duyệt chứ không thu hồi ở máy chủ")
	}
}

// TestLoggingOutOneDeviceDoesNotEvictTheOthers.
//
// Đăng xuất trên điện thoại KHÔNG được đá người dùng ra khỏi máy tính.
//
// Sai theo hướng này khó thấy hơn hướng kia: người dùng chỉ thấy mình bị
// đăng xuất "vô cớ" và không nối được với hành động vừa làm trên máy khác.
func TestLoggingOutOneDeviceDoesNotEvictTheOthers(t *testing.T) {
	a := newAPITest(t)
	email := newEmail("haithietbi")
	const password = "MatKhauDuDai@2026"

	res := a.call(http.MethodPost, "/api/v1/auth/register",
		map[string]any{"email": email, "password": password},
		map[string]string{"Idempotency-Key": ids.MustNew(ids.PrefixRequest).String()})
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("đăng ký: HTTP %d — %s", res.code, res.raw)
	}

	logIn := func() string {
		t.Helper()
		r := a.call(http.MethodPost, "/api/v1/auth/login",
			map[string]any{"email": email, "password": password}, nil)
		if r.code != http.StatusOK {
			t.Fatalf("đăng nhập: HTTP %d — %s", r.code, r.raw)
		}
		tok := a.cookies["refresh_token"]
		if tok == "" {
			t.Fatal("đăng nhập không cấp refresh token")
		}
		return tok
	}

	phone := logIn()
	device := logIn()
	if phone == device {
		t.Fatal("hai lần đăng nhập cấp CÙNG một token — hai thiết bị đang " +
			"dùng chung một phiên, và đăng xuất ở đâu cũng đá cả hai")
	}

	// Đăng xuất trên ĐIỆN THOẠI.
	a.cookies["refresh_token"] = phone
	if res := a.call(http.MethodPost, "/api/v1/auth/logout", nil,
		idemKey()); res.code != http.StatusNoContent && res.code != http.StatusOK {
		t.Fatalf("đăng xuất: HTTP %d — %s", res.code, res.raw)
	}

	// Phiên MÁY TÍNH phải còn sống.
	a.cookies["refresh_token"] = device
	if res := a.call(http.MethodPost, "/api/v1/auth/refresh", nil, nil); res.code != http.StatusOK {
		t.Errorf("phiên thiết bị KHÁC chết theo: HTTP %d — %s — đăng xuất "+
			"trên điện thoại đã đá người dùng khỏi máy tính", res.code, res.raw)
	}
}

// TestASessionErrorSaysNothingAboutThePassword.
//
// # Vì sao bài này tồn tại
//
// Chạy hệ thống thật trên Docker (16/09): xoay refresh token rồi gửi lại
// token CŨ trả 401 kèm "Email hoặc mật khẩu không đúng". Request đó không
// mang email lẫn mật khẩu.
//
// Thiệt hại không nằm ở mã trạng thái — 401 vẫn đúng. Nó nằm ở chỗ người
// đọc câu đó đi sai đường: khách sẽ đặt lại mật khẩu đang dùng tốt, người
// trực hỗ trợ đọc log sẽ soi bảng mật khẩu thay vì bảng phiên.
//
// Bài này khóa NỘI DUNG câu trả lời, không chỉ mã lỗi.
func TestASessionErrorSaysNothingAboutThePassword(t *testing.T) {
	a := newAPITest(t)
	a.registerAndLogIn(newEmail("loiphien"))

	cu := a.cookies["refresh_token"]
	if cu == "" {
		t.Fatal("đăng nhập không cấp refresh token")
	}

	// Xoay một lần: token `cu` từ đây là token đã dùng.
	if res := a.call(http.MethodPost, "/api/v1/auth/refresh", nil, nil); res.code != http.StatusOK {
		t.Fatalf("làm mới phiên: HTTP %d — %s", res.code, res.raw)
	}

	a.cookies["refresh_token"] = cu
	res := a.call(http.MethodPost, "/api/v1/auth/refresh", nil, nil)
	if res.code != http.StatusUnauthorized {
		t.Fatalf("gửi lại token đã xoay phải bị từ chối: HTTP %d — %s", res.code, res.raw)
	}
	if strings.Contains(res.raw, "mật khẩu") {
		t.Errorf("lỗi phiên nói về mật khẩu trong khi request không có mật khẩu: %s", res.raw)
	}
	if !strings.Contains(res.raw, "Phiên") {
		t.Errorf("lỗi phiên không nhắc tới phiên — người đọc không biết phải làm gì: %s", res.raw)
	}
}
